package api

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sublink/database"
	"sublink/models"
	"sublink/node"
	"sublink/node/protocol"
	"sublink/services"
	"sublink/services/unlock"
	"sublink/utils"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// Node-add contracts are additive and intentionally independent from the legacy
// {code,msg} response envelope. Keep these values stable for API consumers.
type NodeAddStatus string

const (
	NodeAddStatusSuccess NodeAddStatus = "success"
	NodeAddStatusPartial NodeAddStatus = "partial"
	NodeAddStatusSkipped NodeAddStatus = "skipped"
	NodeAddStatusInvalid NodeAddStatus = "invalid"
	NodeAddStatusFailed  NodeAddStatus = "failed"
)

type NodeAddItemStatus string

const (
	NodeAddItemAdded   NodeAddItemStatus = "added"
	NodeAddItemSkipped NodeAddItemStatus = "skipped"
	NodeAddItemInvalid NodeAddItemStatus = "invalid"
	NodeAddItemFailed  NodeAddItemStatus = "failed"
)

type NodeAddReason string

const (
	NodeAddReasonInvalidInput        NodeAddReason = "invalid_input"
	NodeAddReasonUnsupportedProtocol NodeAddReason = "unsupported_protocol"
	NodeAddReasonNetworkError        NodeAddReason = "network_error"
	NodeAddReasonTLSError            NodeAddReason = "tls_error"
	NodeAddReasonParseError          NodeAddReason = "parse_error"
	NodeAddReasonDuplicate           NodeAddReason = "duplicate"
	NodeAddReasonPersistenceError    NodeAddReason = "persistence_error"
)

type NodeAddDiagnostic struct {
	Code      NodeAddReason `json:"code"`
	Message   string        `json:"message,omitempty"`
	Retryable bool          `json:"retryable,omitempty"`
}

type NodeAddItem struct {
	Status     NodeAddItemStatus  `json:"status"`
	Name       string             `json:"name,omitempty"`
	Protocol   string             `json:"protocol,omitempty"`
	NodeID     int                `json:"nodeId,omitempty"`
	Reason     NodeAddReason      `json:"reason,omitempty"`
	Diagnostic *NodeAddDiagnostic `json:"diagnostic,omitempty"`
}

type NodeAddResult struct {
	Mode        string              `json:"mode,omitempty"`
	Status      NodeAddStatus       `json:"status"`
	Added       int                 `json:"added"`
	Skipped     int                 `json:"skipped"`
	Invalid     int                 `json:"invalid"`
	Failed      int                 `json:"failed"`
	Warnings    int                 `json:"warnings,omitempty"`
	Items       []NodeAddItem       `json:"items,omitempty"`
	Diagnostics []NodeAddDiagnostic `json:"diagnostics,omitempty"`
	Retryable   bool                `json:"retryable,omitempty"`
}

const (
	maxNodeAddText       = 256
	maxNodeAddDiagnostic = 512
)

func splitNodeAddLinks(link string) []string {
	trimmed := strings.TrimSpace(link)
	if trimmed == "" {
		return nil
	}
	if protocol.IsWireGuardConfig(trimmed) || strings.Contains(trimmed, "proxies:") {
		return []string{trimmed}
	}
	links := strings.FieldsFunc(trimmed, func(r rune) bool { return r == '\r' || r == '\n' || r == ',' })
	result := make([]string, 0, len(links))
	for _, item := range links {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func safeNodeAddTaskItem(link string, index int) string {
	if parsed, err := url.Parse(strings.TrimSpace(link)); err == nil && parsed.Scheme != "" {
		return SafeNodeAddURL(link)
	}
	return fmt.Sprintf("节点 %d", index+1)
}

func mergeNodeAddResult(target *NodeAddResult, item NodeAddResult) {
	target.Added += item.Added
	target.Skipped += item.Skipped
	target.Invalid += item.Invalid
	target.Failed += item.Failed
	target.Warnings += item.Warnings
	target.Items = append(target.Items, item.Items...)
	target.Diagnostics = append(target.Diagnostics, item.Diagnostics...)
	target.Retryable = target.Retryable || item.Retryable
}

func runNodeAddTask(taskID string, ctx context.Context, links []string, input nodeAddInput) {
	tm := services.GetTaskManager()
	result := NodeAddResult{Mode: "async"}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tm.FailTask(taskID, "节点添加任务异常终止")
		}
	}()
	for index, link := range links {
		if ctx.Err() != nil || tm.IsTaskCancelled(taskID) {
			return
		}
		input.Link = link
		mergeNodeAddResult(&result, processNodeAdd(ctx, input))
		classifyNodeAddResult(&result)
		_ = tm.UpdateProgress(taskID, index+1, safeNodeAddTaskItem(link, index), result)
	}
	if ctx.Err() != nil || tm.IsTaskCancelled(taskID) {
		return
	}
	classifyNodeAddResult(&result)
	_ = tm.CompleteTask(taskID, "节点添加任务完成", result)
}

// TruncateNodeAddText bounds untrusted text while preserving a clear marker.
func TruncateNodeAddText(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= limit {
		return value
	}
	if limit <= 3 {
		return string([]rune(value)[:limit])
	}
	return string([]rune(value)[:limit-3]) + "..."
}

// SafeNodeAddURL returns only scheme and authority. It never returns userinfo,
// path, query, or fragment, which may contain credentials or subscription data.
func SafeNodeAddURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return TruncateNodeAddText("[redacted]", maxNodeAddText)
	}
	scheme := strings.ToLower(u.Scheme)
	if u.Hostname() == "" {
		return scheme + "://[redacted]"
	}
	host := u.Hostname()
	if u.Port() != "" {
		host += ":" + u.Port()
	}
	return TruncateNodeAddText(scheme+"://"+host, maxNodeAddText)
}

// SafeNodeAddError formats errors without exposing credential-bearing URLs.
func SafeNodeAddError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	// Replace URL-like tokens first, including URLs embedded in parser errors.
	re := regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s\]\)>,;]+`)
	message = re.ReplaceAllStringFunc(message, SafeNodeAddURL)
	credentialPattern := regexp.MustCompile(`(?i)(password|token|passwd|secret|credential)\s*[:=]\s*[^\s,;]+`)
	message = credentialPattern.ReplaceAllString(message, "[redacted credential]")
	return TruncateNodeAddText(message, maxNodeAddDiagnostic)
}

func normalizeResidentialType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "residential", "datacenter", "untested":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeQualityStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case models.QualityStatusUntested, models.QualityStatusSuccess, models.QualityStatusPartial, models.QualityStatusFailed, models.QualityStatusDisabled:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeIPType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "native", "broadcast", "untested":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeUnlockStatus(value string) string {
	return unlock.NormalizeUnlockStatus(value)
}

var disallowedSubscriptionPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func isDisallowedSubscriptionIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() {
		return true
	}
	for _, prefix := range disallowedSubscriptionPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func validateSubscriptionDestination(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return errors.New("invalid subscription URL")
	}
	addresses, err := net.DefaultResolver.LookupIP(ctx, "ip", parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		return errors.New("subscription host resolution failed")
	}
	for _, ip := range addresses {
		if isDisallowedSubscriptionIP(ip) {
			return errors.New("subscription destination is not allowed")
		}
	}
	return nil
}

func fetchNodeSubscriptionPayload(ctx context.Context, rawURL string, skipTLSVerify bool) ([]byte, error) {
	if err := validateSubscriptionDestination(ctx, rawURL); err != nil {
		return nil, err
	}
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("unsupported default HTTP transport")
	}
	transport := baseTransport.Clone()
	baseDialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(dialCtx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIP(dialCtx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, errors.New("subscription host resolution failed")
		}
		for _, ip := range ips {
			if isDisallowedSubscriptionIP(ip) {
				return nil, errors.New("subscription destination is not allowed")
			}
		}
		return baseDialer.DialContext(dialCtx, network, net.JoinHostPort(ips[0].String(), port))
	}
	if skipTLSVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicitly requested per-node setting
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many subscription redirects")
			}
			return validateSubscriptionDestination(req.Context(), req.URL.String())
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("subscription request failed")
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

func classifyNodeAddResult(result *NodeAddResult) {
	switch {
	case result.Added > 0 && (result.Invalid > 0 || result.Failed > 0 || result.Skipped > 0 || result.Warnings > 0):
		result.Status = NodeAddStatusPartial
	case result.Added > 0:
		result.Status = NodeAddStatusSuccess
	case result.Skipped > 0 && result.Invalid == 0 && result.Failed == 0:
		result.Status = NodeAddStatusSkipped
	case result.Failed > 0:
		result.Status = NodeAddStatusFailed
	default:
		result.Status = NodeAddStatusInvalid
	}
}

func failNodeNameConflict(c *gin.Context, err error) bool {
	if errors.Is(err, models.ErrNodeVariantExists) {
		utils.FailWithMsg(c, "相同出站节点和前置代理已存在")
		return true
	}
	if errors.Is(err, models.ErrNodeNameExists) {
		utils.FailWithMsg(c, "节点备注名称已存在，请换一个备注")
		return true
	}
	return false
}

func parseUnlockRulesFromQuery(c *gin.Context) []models.UnlockFilterRule {
	rawRules := strings.TrimSpace(c.Query("unlockRules"))
	rules := models.ParseUnlockFilterRules(rawRules)
	if len(rules) > 0 {
		return rules
	}

	providers := c.QueryArray("unlockRules[][provider]")
	statuses := c.QueryArray("unlockRules[][status]")
	keywords := c.QueryArray("unlockRules[][keyword]")
	maxLen := len(providers)
	if len(statuses) > maxLen {
		maxLen = len(statuses)
	}
	if len(keywords) > maxLen {
		maxLen = len(keywords)
	}
	if maxLen > 0 {
		ruleList := make([]models.UnlockFilterRule, 0, maxLen)
		for i := 0; i < maxLen; i++ {
			rule := models.UnlockFilterRule{}
			if i < len(providers) {
				rule.Provider = models.NormalizeUnlockProvider(providers[i])
			}
			if i < len(statuses) {
				rule.Status = normalizeUnlockStatus(statuses[i])
			}
			if i < len(keywords) {
				rule.Keyword = strings.TrimSpace(keywords[i])
			}
			ruleList = append(ruleList, rule)
		}
		return models.NormalizeUnlockFilterRules(ruleList)
	}

	legacyProvider := models.NormalizeUnlockProvider(c.Query("unlockProvider"))
	legacyStatus := normalizeUnlockStatus(c.Query("unlockStatus"))
	legacyKeyword := strings.TrimSpace(c.Query("unlockKeyword"))
	if legacyProvider != "" || legacyStatus != "" || legacyKeyword != "" {
		return []models.UnlockFilterRule{{Provider: legacyProvider, Status: legacyStatus, Keyword: legacyKeyword}}
	}
	return nil
}

func parseExcludeIDs(c *gin.Context) []int {
	return parseIntList(c, []string{"excludeIds[]", "excludeIds"})
}

func parseSelectedIDs(c *gin.Context) []int {
	return parseIntList(c, []string{"ids[]", "ids", "id"})
}

func parseIntList(c *gin.Context, keys []string) []int {
	values := make([]string, 0)
	for _, key := range keys {
		values = append(values, c.QueryArray(key)...)
	}
	ids := make([]int, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		for _, part := range strings.Split(trimmed, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if id, err := strconv.Atoi(part); err == nil && id > 0 {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func buildNodeFilterFromQuery(c *gin.Context) models.NodeFilter {
	filter := models.NodeFilter{
		Search:      c.Query("search"),
		Group:       c.Query("group"),
		Source:      c.Query("source"),
		Protocol:    c.Query("protocol"),
		SpeedStatus: c.Query("speedStatus"),
		DelayStatus: c.Query("delayStatus"),
		SortBy:      c.Query("sortBy"),
		SortOrder:   c.Query("sortOrder"),
	}

	if maxDelayStr := c.Query("maxDelay"); maxDelayStr != "" {
		if maxDelay, err := strconv.Atoi(maxDelayStr); err == nil && maxDelay > 0 {
			filter.MaxDelay = maxDelay
		}
	}

	if minSpeedStr := c.Query("minSpeed"); minSpeedStr != "" {
		if minSpeed, err := strconv.ParseFloat(minSpeedStr, 64); err == nil && minSpeed > 0 {
			filter.MinSpeed = minSpeed
		}
	}

	if maxFraudScoreStr := c.Query("maxFraudScore"); maxFraudScoreStr != "" {
		if maxFraudScore, err := strconv.Atoi(maxFraudScoreStr); err == nil && maxFraudScore > 0 {
			filter.MaxFraudScore = maxFraudScore
		}
	}

	filter.Countries = c.QueryArray("countries[]")
	filter.Tags = c.QueryArray("tags[]")
	filter.ResidentialType = normalizeResidentialType(c.Query("residentialType"))
	filter.IPType = normalizeIPType(c.Query("ipType"))
	filter.QualityStatus = normalizeQualityStatus(c.Query("qualityStatus"))
	filter.UnlockRuleMode = models.NormalizeUnlockRuleMode(c.Query("unlockRuleMode"))
	filter.UnlockRules = parseUnlockRulesFromQuery(c)
	filter.ExcludeIDs = parseExcludeIDs(c)
	if len(filter.UnlockRules) == 0 {
		filter.UnlockProvider = models.NormalizeUnlockProvider(c.Query("unlockProvider"))
		filter.UnlockStatus = normalizeUnlockStatus(c.Query("unlockStatus"))
		filter.UnlockKeyword = strings.TrimSpace(c.Query("unlockKeyword"))
	}
	if filter.ResidentialType == "" && c.Query("onlyResidential") == "true" {
		filter.ResidentialType = "residential"
	}
	if filter.IPType == "" && c.Query("onlyNative") == "true" {
		filter.IPType = "native"
	}

	// 验证排序字段
	validSortFields := map[string]bool{
		"delay":    true,
		"speed":    true,
		"name":     true,
		"protocol": true,
		"group":    true,
		"source":   true,
		"country":  true,
	}
	if filter.SortBy != "" && !validSortFields[filter.SortBy] {
		filter.SortBy = ""
	}

	if filter.SortOrder != "" && filter.SortOrder != "asc" && filter.SortOrder != "desc" {
		filter.SortOrder = "asc"
	}

	return filter
}

func parsePagination(c *gin.Context) (int, int) {
	page := 0
	pageSize := 0
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if pageSizeStr := c.Query("pageSize"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}
	return page, pageSize
}

func NodeUpdadte(c *gin.Context) {
	var Node models.Node
	name := c.PostForm("name")
	oldname := c.PostForm("oldname")
	oldlink := c.PostForm("oldlink")
	link := c.PostForm("link")
	nameMode, hasNameMode := c.GetPostForm("nameMode")
	if !hasNameMode {
		nameMode, hasNameMode = c.GetPostForm("NameMode")
	}
	dialerProxyName := c.PostForm("dialerProxyName")
	remark := strings.TrimSpace(c.PostForm("remark"))
	skipTLSVerify := c.PostForm("skipTLSVerify") == "true"
	oldDialerProxyName, hasOldDialerProxyName := c.GetPostForm("oldDialerProxyName")
	if !hasOldDialerProxyName {
		oldDialerProxyName = ""
	}
	group := c.PostForm("group")
	if link == "" {
		utils.FailWithMsg(c, "节点链接不能为空")
		return
	}
	userProvidedName := strings.TrimSpace(name) != ""
	// 查找旧节点；相同出站链接可能因前置不同而有多个记录。
	Node.Name = oldname
	Node.Link = oldlink
	var err error
	if hasOldDialerProxyName {
		err = Node.FindByLinkAndDialer(oldlink, oldDialerProxyName)
	} else {
		// 兼容旧客户端：用旧链接和名称定位，避免同链接不同前置时误选兄弟节点。
		err = Node.FindByLinkAndName(oldlink, oldname)
	}
	if err != nil {
		utils.FailWithMsg(c, err.Error())
		return
	}
	storedLink := Node.Link
	oldContentHash := Node.ContentHash
	storedDialerProxyName := Node.DialerProxyName
	if hasNameMode {
		Node.NameMode = models.NormalizeNodeNameMode(nameMode)
	} else {
		Node.NameMode = models.NormalizeNodeNameMode(Node.NameMode)
	}

	//更新构造节点元数据
	// 检测是否为 WireGuard 配置文件格式，如果是则转换为 URL 格式
	if protocol.IsWireGuardConfig(link) {
		wg, err := protocol.ParseWireGuardConfig(link)
		if err != nil {
			utils.FailWithMsg(c, "WireGuard 配置文件解析失败: "+err.Error())
			return
		}
		// 转换为 URL 格式
		link = protocol.EncodeWireGuardURL(wg)
	}
	identity, err := protocol.ExtractLinkIdentity(link)
	if err != nil {
		utils.Error("解析节点链接失败: %v", err)
		return
	}
	Node.Name = name
	Node.Remark = remark
	if strings.TrimSpace(Node.Name) == "" {
		Node.Name = identity.Name
	}
	if userProvidedName {
		if err := models.EnsureNodeNameAvailable(Node.Name, Node.ID); err != nil {
			if !failNodeNameConflict(c, err) {
				utils.FailWithMsg(c, "检查节点备注名称失败")
			}
			return
		}
	} else {
		Node.Name = models.GenerateUniqueNodeNameWithSource(Node.Name, Node.Source, Node.ID, nil)
	}
	Node.LinkName = identity.Name
	Node.LinkAddress = identity.Address
	Node.LinkHost = identity.Host
	Node.LinkPort = identity.Port

	Node.Link = link
	if link != storedLink {
		// 链接发生变化后，旧链接对应的未建模 Clash 字段不再可靠，必须丢弃以避免错配。
		Node.ClashExtra = ""
	}
	Node.DialerProxyName = strings.TrimSpace(dialerProxyName)
	Node.Group = group
	if _, ok := c.GetPostForm("skipTLSVerify"); ok {
		Node.SkipTLSVerify = skipTLSVerify
	}
	Node.Protocol = protocol.GetProtocolFromLink(link)

	// 重新计算 ContentHash
	proxy, proxyErr := protocol.LinkToProxy(protocol.Urls{Url: link}, protocol.OutputConfig{})
	if proxyErr == nil {
		contentHash := protocol.GenerateProxyContentHash(proxy)
		if contentHash != "" {
			Node.ContentHash = contentHash
			// 出站内容或前置代理变化时，需要重新检查复合唯一性。
			if contentHash != oldContentHash || strings.TrimSpace(Node.DialerProxyName) != strings.TrimSpace(storedDialerProxyName) {
				// 读取全局配置：是否启用跨机场去重（默认启用）
				crossAirportDedupVal, _ := models.GetSetting("cross_airport_dedup_enabled")
				enableCrossDedup := crossAirportDedupVal != "false"

				// 检查是否与其他节点重复（排除自身）
				var dupNode *models.Node
				var exists bool
				if enableCrossDedup {
					dupNode, exists = models.GetOtherNodeByContentHashAndDialer(contentHash, Node.DialerProxyName, Node.ID)
				} else {
					dupNode, exists = models.GetOtherNodeByContentHashSourceAndDialer(contentHash, Node.SourceID, Node.DialerProxyName, Node.ID)
				}
				if exists && dupNode != nil {
					// 构建详细的重复信息
					source := dupNode.Source
					if source == "" || source == "manual" {
						source = "手动添加"
					}
					group := dupNode.Group
					if group == "" {
						group = "未分组"
					}
					utils.FailWithMsg(c, "节点内容已存在，与以下节点重复：[来源: "+source+"] [分组: "+group+"] [名称: "+dupNode.Name+"]")
					return
				}
			}
		}
	}

	err = Node.Update()
	if err != nil {
		if failNodeNameConflict(c, err) {
			return
		}
		utils.FailWithMsg(c, "更新失败")
		return
	}

	// 处理标签
	tags := c.PostForm("tags")
	if tags != "" {
		tagNames := strings.Split(tags, ",")
		// 过滤空字符串
		var validTagNames []string
		for _, t := range tagNames {
			t = strings.TrimSpace(t)
			if t != "" {
				validTagNames = append(validTagNames, t)
			}
		}
		_ = Node.SetTagNames(validTagNames)
	} else {
		// 如果 tags 参数为空，清除标签
		_ = Node.SetTagNames([]string{})
	}

	utils.OkWithMsg(c, "更新成功")
}

// 获取节点列表
func NodeGet(c *gin.Context) {
	var Node models.Node
	filter := buildNodeFilterFromQuery(c)
	page, pageSize := parsePagination(c)

	// 如果提供了分页参数，返回分页响应
	if page > 0 && pageSize > 0 {
		nodes, total, err := Node.ListWithFiltersPaginated(filter, page, pageSize)
		if err != nil {
			utils.FailWithMsg(c, "node list error")
			return
		}
		totalPages := 0
		if pageSize > 0 {
			totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
		}
		utils.OkDetailed(c, "node get", gin.H{
			"items":      nodes,
			"total":      total,
			"page":       page,
			"pageSize":   pageSize,
			"totalPages": totalPages,
		})
		return
	}

	// 不带分页参数，返回全部（向后兼容）
	nodes, err := Node.ListWithFilters(filter)
	if err != nil {
		utils.FailWithMsg(c, "node list error")
		return
	}
	utils.OkDetailed(c, "node get", nodes)
}

func NodeSelector(c *gin.Context) {
	var node models.Node
	filter := buildNodeFilterFromQuery(c)
	page, pageSize := parsePagination(c)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}

	nodes, total, err := node.ListWithFiltersPaginated(filter, page, pageSize)
	if err != nil {
		utils.FailWithMsg(c, "node selector list error")
		return
	}
	totalPages := 0
	if pageSize > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	utils.OkDetailed(c, "node selector get", gin.H{
		"items":      models.ToNodeSelectorItems(nodes),
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
		"totalPages": totalPages,
	})
}

func NodeSelectorByIDs(c *gin.Context) {
	ids := parseSelectedIDs(c)
	if len(ids) == 0 {
		utils.OkDetailed(c, "node selector get by ids", []models.NodeSelectorItem{})
		return
	}
	nodes, err := models.GetNodesByIDs(ids)
	if err != nil {
		utils.FailWithMsg(c, "node selector get by ids error")
		return
	}
	items := models.ToNodeSelectorItems(nodes)
	sorted := make([]models.NodeSelectorItem, 0, len(items))
	indexMap := make(map[int]models.NodeSelectorItem, len(items))
	for _, item := range items {
		indexMap[item.ID] = item
	}
	for _, id := range ids {
		if item, ok := indexMap[id]; ok {
			sorted = append(sorted, item)
		}
	}
	utils.OkDetailed(c, "node selector get by ids", sorted)
}

// NodeGetIDs 获取符合过滤条件的所有节点ID（用于全选操作）
func NodeGetIDs(c *gin.Context) {
	var Node models.Node
	filter := buildNodeFilterFromQuery(c)

	ids, err := Node.GetFilteredNodeIDs(filter)
	if err != nil {
		utils.FailWithMsg(c, "get node ids error")
		return
	}
	utils.OkDetailed(c, "node ids get", ids)
}

// NodeAddAsync creates a persisted background task and returns immediately.
func NodeAddAsync(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(8 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		utils.FailWithMsg(c, "解析节点添加请求失败")
		return
	}
	link := strings.TrimSpace(c.PostForm("link"))
	links := splitNodeAddLinks(link)
	if len(links) == 0 {
		utils.FailWithMsg(c, "link  不能为空")
		return
	}
	input := nodeAddInput{
		Link:            links[0],
		Name:            c.PostForm("name"),
		NameMode:        c.PostForm("nameMode"),
		DialerProxyName: strings.TrimSpace(c.PostForm("dialerProxyName")),
		Remark:          strings.TrimSpace(c.PostForm("remark")),
		Group:           c.PostForm("group"),
		SkipTLSVerify:   c.PostForm("skipTLSVerify") == "true",
		Tags:            c.PostForm("tags"),
	}
	task, ctx, err := services.GetTaskManager().CreateTask(models.TaskTypeNodeAdd, "添加节点", models.TaskTriggerManual, len(links))
	if err != nil {
		utils.FailWithMsg(c, "创建节点添加任务失败")
		return
	}
	utils.OkDetailed(c, "节点添加任务已提交", gin.H{
		"taskId": task.ID,
		"task":   task,
		"total":  len(links),
	})
	go func() {
		// Let the HTTP response reach the client before fast tasks emit terminal SSE events.
		time.Sleep(50 * time.Millisecond)
		runNodeAddTask(task.ID, ctx, links, input)
	}()
}

// 添加节点
func NodeAdd(c *gin.Context) {
	var Node models.Node
	link := c.PostForm("link")
	name := c.PostForm("name")
	nameModeInput := c.PostForm("nameMode")
	if nameModeInput == "" {
		nameModeInput = c.PostForm("NameMode")
	}
	nameMode := models.NormalizeNodeNameMode(nameModeInput)
	dialerProxyName := strings.TrimSpace(c.PostForm("dialerProxyName"))
	remark := strings.TrimSpace(c.PostForm("remark"))
	group := c.PostForm("group")
	skipTLSVerify := c.PostForm("skipTLSVerify") == "true"
	if link == "" {
		utils.FailWithMsg(c, "link  不能为空")
		return
	}
	userProvidedName := strings.TrimSpace(name) != ""

	// Remote subscription URLs are processed independently and return structured outcomes.
	if parsedURL, parseErr := url.Parse(link); parseErr == nil && (strings.EqualFold(parsedURL.Scheme, "http") || strings.EqualFold(parsedURL.Scheme, "https")) && !protocol.IsHTTPLink(link) {
		result := NodeAddResult{Mode: "subscription"}
		payload, fetchErr := fetchNodeSubscriptionPayload(c.Request.Context(), link, skipTLSVerify)
		if fetchErr != nil {
			result.Failed = 1
			result.Diagnostics = []NodeAddDiagnostic{{Code: NodeAddReasonNetworkError, Message: SafeNodeAddError(fetchErr), Retryable: true}}
			classifyNodeAddResult(&result)
			utils.FailWithData(c, "远程订阅获取失败", result)
			return
		}
		parsed, _ := node.ParseSubscriptionPayloadWithDiagnostics(payload)
		for _, diagnostic := range parsed.Diagnostics {
			result.Diagnostics = append(result.Diagnostics, NodeAddDiagnostic{Code: NodeAddReasonParseError, Message: TruncateNodeAddText(diagnostic.Message, maxNodeAddDiagnostic)})
		}
		for _, candidate := range parsed.URLs {
			identity, identityErr := protocol.ExtractLinkIdentity(candidate.Url)
			item := NodeAddItem{Name: identity.Name, Protocol: protocol.GetProtocolFromLink(candidate.Url)}
			if identityErr != nil || item.Protocol == "" {
				item.Status, item.Reason = NodeAddItemInvalid, NodeAddReasonInvalidInput
				result.Invalid++
				result.Items = append(result.Items, item)
				continue
			}
			n := models.Node{Name: identity.Name, Remark: remark, NameMode: models.NodeNameModeLink, Link: candidate.Url, LinkName: identity.Name, LinkAddress: identity.Address, LinkHost: identity.Host, LinkPort: identity.Port, Protocol: item.Protocol, DialerProxyName: dialerProxyName, Group: group, SkipTLSVerify: skipTLSVerify, ClashExtra: candidate.ClashExtra, ValidationStatus: models.ValidationStatusSuccess, ValidationReason: "validated", ValidationCheckAt: time.Now().UTC().Format(time.RFC3339)}
			if err := n.Add(); err != nil {
				item.Status, item.Reason = NodeAddItemSkipped, NodeAddReasonDuplicate
				result.Skipped++
			} else {
				item.Status, item.NodeID = NodeAddItemAdded, n.ID
				result.Added++
			}
			result.Items = append(result.Items, item)
		}
		classifyNodeAddResult(&result)
		if result.Added > 0 {
			utils.OkDetailed(c, "订阅导入完成", result)
		} else {
			utils.FailWithData(c, "订阅未导入节点", result)
		}
		return
	}

	// 读取全局配置：是否启用跨机场去重（默认启用）
	crossAirportDedupVal, _ := models.GetSetting("cross_airport_dedup_enabled")
	enableCrossDedup := crossAirportDedupVal != "false"
	// 检测是否为 WireGuard 配置文件格式，如果是则转换为 URL 格式
	if protocol.IsWireGuardConfig(link) {
		wg, err := protocol.ParseWireGuardConfig(link)
		if err != nil {
			utils.FailWithMsg(c, "WireGuard 配置文件解析失败: "+err.Error())
			return
		}
		// 转换为 URL 格式
		link = protocol.EncodeWireGuardURL(wg)
	}

	// 检测是否为 Clash YAML 配置格式
	if strings.Contains(link, "proxies:") {
		var clashConfig node.ClashConfig
		if err := yaml.Unmarshal([]byte(link), &clashConfig); err == nil && len(clashConfig.Proxies) > 0 {
			// 成功解析为 Clash YAML 格式，处理每个代理节点
			var addedCount, failedCount int
			for _, proxy := range clashConfig.Proxies {
				proxyLink := node.GenerateProxyLink(proxy)
				if proxyLink == "" {
					failedCount++
					continue
				}
				clashExtra, extraErr := protocol.EncodeClashExtra(proxy)
				if extraErr != nil {
					utils.Warn("节点【%s】未知 Clash 属性序列化失败，跳过: %v", proxy.Name, extraErr)
					failedCount++
					continue
				}
				// 创建节点并添加；Clash YAML 中的备注由系统生成，重复时自动追加编号。
				var n models.Node
				n.Name = models.GenerateUniqueNodeName(proxy.Name, 0, nil)
				n.Remark = remark
				n.NameMode = models.NodeNameModeLink
				n.Link = proxyLink
				n.ClashExtra = clashExtra
				n.LinkName = proxy.Name
				n.LinkHost = proxy.Server
				n.LinkPort = strconv.Itoa(proxy.Port.Int())
				n.LinkAddress = proxy.Server + ":" + n.LinkPort
				n.DialerProxyName = dialerProxyName
				n.Group = group
				n.SkipTLSVerify = skipTLSVerify
				n.Protocol = protocol.GetProtocolFromLink(proxyLink)

				// 生成 ContentHash
				contentHash := protocol.GenerateProxyContentHash(proxy)
				if contentHash != "" {
					n.ContentHash = contentHash
					// 检查是否已存在相同内容的节点（跨机场去重关闭时仅校验同来源）
					if enableCrossDedup {
						if _, exists := models.GetNodeByContentHashAndDialer(contentHash, n.DialerProxyName); exists {
							failedCount++
							continue
						}
					} else if _, exists := models.GetNodeByContentHashSourceAndDialer(contentHash, 0, n.DialerProxyName); exists {
						failedCount++
						continue
					}
				}

				n.ValidationStatus = models.ValidationStatusSuccess
				n.ValidationReason = "validated"
				n.ValidationCheckAt = time.Now().UTC().Format(time.RFC3339)
				if err := n.Add(); err != nil {
					failedCount++
					continue
				}
				addedCount++
			}

			if addedCount == 0 {
				utils.FailWithMsg(c, "Clash YAML 解析成功但无法添加任何节点（可能全部重复或格式不支持）")
				return
			}
			utils.OkWithMsg(c, "Clash YAML 导入完成，成功添加 "+strconv.Itoa(addedCount)+" 个节点")
			return
		}
	}

	if !strings.Contains(link, "://") {
		utils.FailWithMsg(c, "link 必须包含 :// 或者是有效的 WireGuard/Clash YAML 配置文件")
		return
	}
	Node.Name = name
	Node.Remark = remark
	Node.NameMode = nameMode
	identity, err := protocol.ExtractLinkIdentity(link)
	if err != nil {
		utils.Error("解析节点链接失败: %v", err)
		return
	}
	if strings.TrimSpace(Node.Name) == "" {
		Node.Name = identity.Name
	}
	if userProvidedName {
		if err := models.EnsureNodeNameAvailable(Node.Name, 0); err != nil {
			if !failNodeNameConflict(c, err) {
				utils.FailWithMsg(c, "检查节点备注名称失败")
			}
			return
		}
	} else {
		Node.Name = models.GenerateUniqueNodeName(Node.Name, 0, nil)
	}
	Node.LinkName = identity.Name
	Node.LinkAddress = identity.Address
	Node.LinkHost = identity.Host
	Node.LinkPort = identity.Port

	Node.Link = link
	Node.DialerProxyName = strings.TrimSpace(dialerProxyName)
	Node.Group = group
	Node.SkipTLSVerify = skipTLSVerify
	Node.Protocol = protocol.GetProtocolFromLink(link)
	if Node.Protocol == "" {
		utils.FailWithData(c, "不支持的节点协议", NodeAddResult{Mode: "link", Status: NodeAddStatusInvalid, Invalid: 1, Items: []NodeAddItem{{Status: NodeAddItemInvalid, Name: Node.Name, Reason: NodeAddReasonUnsupportedProtocol}}})
		return
	}
	Node.ValidationStatus = models.ValidationStatusSuccess
	Node.ValidationReason = "validated"
	Node.ValidationCheckAt = time.Now().UTC().Format(time.RFC3339)

	// 生成 ContentHash（用于全库去重）
	proxy, proxyErr := protocol.LinkToProxy(protocol.Urls{Url: link}, protocol.OutputConfig{})
	if proxyErr == nil {
		contentHash := protocol.GenerateProxyContentHash(proxy)
		if contentHash != "" {
			Node.ContentHash = contentHash
			// 检查是否已存在相同内容的节点（跨机场去重关闭时仅校验同来源）
			var existingNode *models.Node
			var exists bool
			if enableCrossDedup {
				existingNode, exists = models.GetNodeByContentHashAndDialer(contentHash, Node.DialerProxyName)
			} else {
				existingNode, exists = models.GetNodeByContentHashSourceAndDialer(contentHash, 0, Node.DialerProxyName)
			}
			if exists && existingNode != nil {
				// 重复节点：返回成功响应 + 跳过标记，不终止添加流程
				dupSource := existingNode.Source
				if dupSource == "" || dupSource == "manual" {
					dupSource = "手动添加"
				}
				dupGroup := existingNode.Group
				if dupGroup == "" {
					dupGroup = "未分组"
				}
				utils.OkDetailed(c, "节点重复已跳过", gin.H{
					"skipped": true,
					"duplicateInfo": gin.H{
						"name":         Node.Name,
						"source":       dupSource,
						"group":        dupGroup,
						"existingName": existingNode.Name,
					},
				})
				return
			}
		}
	}

	err = Node.Add()
	if err != nil {
		if failNodeNameConflict(c, err) {
			return
		}
		utils.FailWithMsg(c, "添加失败检查一下是否节点重复")
		return
	}

	// 处理标签
	tags := c.PostForm("tags")
	if tags != "" {
		tagNames := strings.Split(tags, ",")
		// 过滤空字符串
		var validTagNames []string
		for _, t := range tagNames {
			t = strings.TrimSpace(t)
			if t != "" {
				validTagNames = append(validTagNames, t)
			}
		}
		_ = Node.SetTagNames(validTagNames)
	}

	utils.OkWithMsg(c, "添加成功")
}

// 删除节点
func NodeDel(c *gin.Context) {
	var Node models.Node
	id := c.Query("id")
	if id == "" {
		utils.FailWithMsg(c, "id 不能为空")
		return
	}
	x, _ := strconv.Atoi(id)
	Node.ID = x
	err := Node.Del()
	if err != nil {
		utils.FailWithMsg(c, "删除失败")
		return
	}
	utils.OkWithMsg(c, "删除成功")
}

// 节点统计
func NodesTotal(c *gin.Context) {
	var Node models.Node
	nodes, err := Node.List()
	if err != nil {
		utils.FailWithMsg(c, "获取不到节点统计")
		return
	}

	total := len(nodes)
	available := 0
	delayPassCount := 0
	speedPassCount := 0
	for _, n := range nodes {
		if n.DelayStatus == "success" && n.DelayTime > 0 {
			delayPassCount++
		}
		if n.SpeedStatus == "success" && n.Speed > 0 {
			speedPassCount++
		}
		if n.Speed > 0 && n.DelayTime > 0 {
			available++
		}
	}

	utils.OkDetailed(c, "取得节点统计", gin.H{
		"total":          total,
		"available":      available,
		"delayPassCount": delayPassCount,
		"speedPassCount": speedPassCount,
	})
}

// NodeBatchDel 批量删除节点
func NodeBatchDel(c *gin.Context) {
	var req struct {
		IDs []int `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.FailWithMsg(c, "参数错误")
		return
	}
	if len(req.IDs) == 0 {
		utils.FailWithMsg(c, "请选择要删除的节点")
		return
	}
	err := models.BatchDel(req.IDs)
	if err != nil {
		utils.FailWithMsg(c, "批量删除失败")
		return
	}
	utils.OkWithMsg(c, "批量删除成功")
}

// NodeBatchUpdateGroup 批量更新节点分组
func NodeBatchUpdateGroup(c *gin.Context) {
	var req struct {
		IDs   []int  `json:"ids"`
		Group string `json:"group"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.FailWithMsg(c, "参数错误")
		return
	}
	if len(req.IDs) == 0 {
		utils.FailWithMsg(c, "请选择要修改的节点")
		return
	}
	err := models.BatchUpdateGroup(req.IDs, req.Group)
	if err != nil {
		utils.FailWithMsg(c, "批量更新分组失败")
		return
	}
	utils.OkWithMsg(c, "批量更新分组成功")
}

// NodeBatchUpdateDialerProxy 批量更新节点前置代理
func NodeBatchUpdateDialerProxy(c *gin.Context) {
	var req struct {
		IDs             []int  `json:"ids"`
		DialerProxyName string `json:"dialerProxyName"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.FailWithMsg(c, "参数错误")
		return
	}
	if len(req.IDs) == 0 {
		utils.FailWithMsg(c, "请选择要修改的节点")
		return
	}
	err := models.BatchUpdateDialerProxy(req.IDs, strings.TrimSpace(req.DialerProxyName))
	if err != nil {
		utils.FailWithMsg(c, "批量更新前置代理失败")
		return
	}
	utils.OkWithMsg(c, "批量更新前置代理成功")
}

// NodeBatchUpdateSource 批量更新节点来源
func NodeBatchUpdateSource(c *gin.Context) {
	var req struct {
		IDs    []int  `json:"ids"`
		Source string `json:"source"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.FailWithMsg(c, "参数错误")
		return
	}
	if len(req.IDs) == 0 {
		utils.FailWithMsg(c, "请选择要修改的节点")
		return
	}
	err := models.BatchUpdateSource(req.IDs, req.Source)
	if err != nil {
		utils.FailWithMsg(c, "批量更新来源失败")
		return
	}
	utils.OkWithMsg(c, "批量更新来源成功")
}

// NodeBatchUpdateCountry 批量更新节点国家代码
func NodeBatchUpdateCountry(c *gin.Context) {
	var req struct {
		IDs     []int  `json:"ids"`
		Country string `json:"country"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.FailWithMsg(c, "参数错误")
		return
	}
	if len(req.IDs) == 0 {
		utils.FailWithMsg(c, "请选择要修改的节点")
		return
	}
	// 国家代码转大写，保持一致性
	country := strings.ToUpper(strings.TrimSpace(req.Country))
	err := models.BatchUpdateCountry(req.IDs, country)
	if err != nil {
		utils.FailWithMsg(c, "批量更新国家代码失败")
		return
	}
	utils.OkWithMsg(c, "批量更新国家代码成功")
}

// NodeBatchFillCountry 批量填充节点国家信息
func NodeBatchFillCountry(c *gin.Context) {
	var req struct {
		AirportID int  `json:"airportId"` // 可选,指定机场
		OnlyEmpty bool `json:"onlyEmpty"` // 只填充国家为空的节点
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.FailWithMsg(c, "参数错误")
		return
	}

	// 构建查询条件
	query := database.DB.Model(&models.Node{})
	if req.AirportID > 0 {
		query = query.Where("source_id = ?", req.AirportID)
	}
	if req.OnlyEmpty {
		query = query.Where("link_country IS NULL OR link_country = ''")
	}

	var nodes []models.Node
	if err := query.Find(&nodes).Error; err != nil {
		utils.FailWithMsg(c, "查询节点失败")
		return
	}

	updated := 0
	for i := range nodes {
		if req.OnlyEmpty && nodes[i].LinkCountry != "" {
			continue
		}
		if country := models.ParseCountryFromNodeName(nodes[i].Name); country != "" {
			nodes[i].LinkCountry = country
			if err := database.DB.Model(&nodes[i]).Update("link_country", country).Error; err == nil {
				updated++
			}
		}
	}

	utils.OkDetailed(c, "批量填充完成", gin.H{
		"total":   len(nodes),
		"updated": updated,
	})
}

// 获取所有分组列表
func GetGroups(c *gin.Context) {
	var node models.Node
	groups, err := node.GetAllGroups()
	if err != nil {
		utils.FailWithMsg(c, "获取分组列表失败")
		return
	}
	utils.OkDetailed(c, "获取分组列表成功", groups)
}

// GetSources 获取所有来源列表
func GetSources(c *gin.Context) {
	var node models.Node
	sources, err := node.GetAllSources()
	if err != nil {
		utils.FailWithMsg(c, "获取来源列表失败")
		return
	}
	utils.OkDetailed(c, "获取来源列表成功", sources)
}

// FastestSpeedNode 获取最快速度节点
func FastestSpeedNode(c *gin.Context) {
	node := models.GetFastestSpeedNode()
	utils.OkDetailed(c, "获取最快速度节点成功", node)
}

// LowestDelayNode 获取最低延迟节点
func LowestDelayNode(c *gin.Context) {
	node := models.GetLowestDelayNode()
	utils.OkDetailed(c, "获取最低延迟节点成功", node)
}

// GetNodeCountries 获取所有节点的国家代码列表
func GetNodeCountries(c *gin.Context) {
	countries := models.GetAllCountries()
	utils.OkDetailed(c, "获取国家代码成功", countries)
}

// NodeCountryStats 获取按国家统计的节点数量
func NodeCountryStats(c *gin.Context) {
	stats := models.GetNodeCountryStats()
	utils.OkDetailed(c, "获取国家统计成功", stats)
}

func DashboardCountryStats(c *gin.Context) {
	stats := models.GetDashboardCountryStats()
	utils.OkDetailed(c, "获取仪表盘国家统计成功", stats)
}

// NodeProtocolStats 获取按协议统计的节点数量
func NodeProtocolStats(c *gin.Context) {
	stats := models.GetNodeProtocolStats()
	utils.OkDetailed(c, "获取协议统计成功", stats)
}

// NodeTagStats 获取按标签统计的节点数量
func NodeTagStats(c *gin.Context) {
	stats := models.GetNodeTagStats()
	utils.OkDetailed(c, "获取标签统计成功", stats)
}

// NodeGroupStats 获取按分组统计的节点数量
func NodeGroupStats(c *gin.Context) {
	stats := models.GetNodeGroupStats()
	utils.OkDetailed(c, "获取分组统计成功", stats)
}

// NodeSourceStats 获取按来源统计的节点数量
func NodeSourceStats(c *gin.Context) {
	stats := models.GetNodeSourceStats()
	utils.OkDetailed(c, "获取来源统计成功", stats)
}

func DashboardGroupedStats(c *gin.Context) {
	stats := models.GetDashboardGroupedStats()
	utils.OkDetailed(c, "获取仪表盘分组统计成功", stats)
}

func DashboardQualityStats(c *gin.Context) {
	stats := models.GetDashboardQualityStats()
	utils.OkDetailed(c, "获取仪表盘质量统计成功", stats)
}

// GetIPDetails 获取IP详细信息
// GET /api/v1/nodes/ip-info?ip=xxx.xxx.xxx.xxx
func GetIPDetails(c *gin.Context) {
	ip := c.Query("ip")
	if ip == "" {
		utils.FailWithMsg(c, "IP地址不能为空")
		return
	}

	// 调用模型层获取IP信息（多级缓存）
	ipInfo, err := models.GetIPInfo(ip)
	if err != nil {
		utils.FailWithMsg(c, "查询IP信息失败: "+err.Error())
		return
	}

	utils.OkDetailed(c, "获取成功", ipInfo)
}

// GetIPCacheStats 获取IP缓存统计
// GET /api/v1/nodes/ip-cache/stats
func GetIPCacheStats(c *gin.Context) {
	count := models.GetIPInfoCount()
	utils.OkDetailed(c, "获取成功", gin.H{
		"count": count,
	})
}

// ClearIPCache 清除所有IP缓存
// DELETE /api/v1/nodes/ip-cache
func ClearIPCache(c *gin.Context) {
	err := models.ClearAllIPInfo()
	if err != nil {
		utils.FailWithMsg(c, "清除失败: "+err.Error())
		return
	}
	utils.OkWithMsg(c, "IP缓存已清除")
}

// GetNodeProtocols 获取所有使用中的协议类型列表（用于过滤器选项）
// GET /api/v1/nodes/protocols
func GetNodeProtocols(c *gin.Context) {
	protocols := models.GetAllProtocols()
	utils.OkDetailed(c, "获取协议列表成功", protocols)
}
