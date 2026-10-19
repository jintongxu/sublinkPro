package protocol

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sublink/cache"
	"sublink/utils"

	"gopkg.in/yaml.v3"
)

// FlexPort 是一个可以从 int 或 string 类型解析的端口类型
// 用于处理订阅源返回的 port 字段可能是 int 或 string 的情况
type FlexPort int

// UnmarshalYAML 实现 yaml.Unmarshaler 接口，支持从 int 或 string 解析
func (fp *FlexPort) UnmarshalYAML(value *yaml.Node) error {
	var intVal int
	if err := value.Decode(&intVal); err == nil {
		*fp = FlexPort(intVal)
		return nil
	}

	var strVal string
	if err := value.Decode(&strVal); err == nil {
		if strVal == "" {
			*fp = 0
			return nil
		}
		intVal, err := strconv.Atoi(strVal)
		if err != nil {
			return fmt.Errorf("无法将端口 '%s' 转换为整数: %w", strVal, err)
		}
		*fp = FlexPort(intVal)
		return nil
	}

	return fmt.Errorf("无法解析端口值")
}

// MarshalYAML 实现 yaml.Marshaler 接口，始终输出为 int
func (fp FlexPort) MarshalYAML() (any, error) {
	return int(fp), nil
}

// Int 返回端口的 int 值
func (fp FlexPort) Int() int {
	return int(fp)
}

// IsZero lets YAML omitempty suppress absent ports for protocols that use
// alternative port fields such as Mieru port-range.
func (fp FlexPort) IsZero() bool {
	return fp == 0
}

// Mbps 是 Clash 兼容的带宽数值类型。
// 兼容 provider 里把 up/down 之类字段写成 "11 Mbps" 这种字符串的情况，但序列化时仍输出整数。
type Mbps int

var bandwidthRatePattern = regexp.MustCompile(`^(\d+)\s*([KMGTkmgt]?)([Bb])ps$`)

// UnmarshalYAML 支持把带宽字段解析为整数、裸数字字符串或标准速率字符串。
// 这里保留对 Hysteria/Hysteria2 兼容格式的支持，但拒绝非标准后缀，避免把非法带宽静默降级。
func (m *Mbps) UnmarshalYAML(value *yaml.Node) error {
	var intVal int
	if err := value.Decode(&intVal); err == nil {
		*m = Mbps(intVal)
		return nil
	}

	var strVal string
	if err := value.Decode(&strVal); err != nil {
		return fmt.Errorf("无法解析带宽值")
	}
	strVal = strings.TrimSpace(strVal)
	if strVal == "" {
		*m = 0
		return nil
	}
	if directVal, err := strconv.Atoi(strVal); err == nil {
		*m = Mbps(directVal)
		return nil
	}
	matches := bandwidthRatePattern.FindStringSubmatch(strVal)
	if matches == nil {
		return fmt.Errorf("无法解析带宽值 %q", strVal)
	}
	base, err := strconv.Atoi(matches[1])
	if err != nil {
		return fmt.Errorf("无法将带宽值 %q 转换为整数: %w", strVal, err)
	}
	switch strings.ToUpper(matches[2]) {
	case "", "M":
		*m = Mbps(base)
		return nil
	case "G":
		*m = Mbps(base * 1000)
		return nil
	case "T":
		*m = Mbps(base * 1000 * 1000)
		return nil
	case "K":
		return fmt.Errorf("带宽值 %q 使用了 Kbps 单位，无法无损转换为 Mbps", strVal)
	default:
		return fmt.Errorf("无法解析带宽值 %q", strVal)
	}
}

// MarshalYAML 始终把带宽写成整数，避免把内部类型泄漏成字符串。
func (m Mbps) MarshalYAML() (any, error) {
	return int(m), nil
}

type Proxy struct {
	Name                  string         `yaml:"name,omitempty"`                        // 节点名称
	Type                  string         `yaml:"type,omitempty"`                        // 代理类型 (ss, vmess, trojan, etc.)
	Server                string         `yaml:"server,omitempty"`                      // 服务器地址
	Port                  FlexPort       `yaml:"port,omitempty"`                        // 服务器端口
	PortRange             string         `yaml:"port-range,omitempty"`                  // 端口范围 (Mieru)
	Ports                 string         `yaml:"ports,omitempty"`                       // hysteria2端口跳跃
	Transport             string         `yaml:"transport,omitempty"`                   // 传输协议 (Mieru TCP/UDP)
	Cipher                string         `yaml:"cipher,omitempty"`                      // 加密方式
	Username              string         `yaml:"username,omitempty"`                    // 用户名 (socks5 等)
	Password              string         `yaml:"password,omitempty"`                    // 密码
	Multiplexing          string         `yaml:"multiplexing,omitempty"`                // Mieru 多路复用级别
	TrafficPattern        string         `yaml:"traffic-pattern,omitempty"`             // Mieru 流量模式
	Client_fingerprint    string         `yaml:"client-fingerprint,omitempty"`          // 客户端指纹 (uTLS)
	Fingerprint           string         `yaml:"fingerprint,omitempty"`                 // 服务端证书 SHA-256 指纹
	Tfo                   bool           `yaml:"tfo,omitempty"`                         // TCP Fast Open
	Udp                   bool           `yaml:"udp,omitempty"`                         // 是否启用 UDP
	Skip_cert_verify      bool           `yaml:"skip-cert-verify,omitempty"`            // 跳过证书验证
	Tls                   bool           `yaml:"tls,omitempty"`                         // 是否启用 TLS
	Servername            string         `yaml:"servername,omitempty"`                  // TLS SNI
	Flow                  string         `yaml:"flow,omitempty"`                        // 流控 (xtls-rprx-vision 等)
	AlterId               string         `yaml:"alterId,omitempty"`                     // VMess AlterId
	Network               string         `yaml:"network,omitempty"`                     // 传输协议 (ws, grpc, etc.)
	Reality_opts          map[string]any `yaml:"reality-opts,omitempty"`                // Reality 选项
	Ws_opts               map[string]any `yaml:"ws-opts,omitempty"`                     // WebSocket 选项
	Grpc_opts             map[string]any `yaml:"grpc-opts,omitempty"`                   // gRPC 选项
	Auth_str              string         `yaml:"auth-str,omitempty"`                    // Hysteria 认证字符串
	Auth                  string         `yaml:"auth,omitempty"`                        // 认证信息
	Up                    Mbps           `yaml:"up,omitempty"`                          // 上行带宽限制
	Down                  Mbps           `yaml:"down,omitempty"`                        // 下行带宽限制
	AnyTLSIdleCheck       int            `yaml:"idle-session-check-interval,omitempty"` // AnyTLS 空闲会话检查间隔
	AnyTLSIdleTimeout     int            `yaml:"idle-session-timeout,omitempty"`        // AnyTLS 空闲会话超时时间
	AnyTLSMinIdle         int            `yaml:"min-idle-session,omitempty"`            // AnyTLS 最小空闲会话数
	Up_Speed              Mbps           `yaml:"up-speed,omitempty"`                    // 上行带宽限制兼容stash
	Down_Speed            Mbps           `yaml:"down-speed,omitempty"`                  // 下行带宽限制兼容stash
	Alpn                  []string       `yaml:"alpn,omitempty"`                        // ALPN
	Sni                   string         `yaml:"sni,omitempty"`                         // SNI
	Obfs                  string         `yaml:"obfs,omitempty"`                        // 混淆模式 (SSR/Hysteria2)
	Obfs_password         string         `yaml:"obfs-password,omitempty"`               // 混淆密码
	Psk                   string         `yaml:"psk,omitempty"`                         // Snell 预共享密钥
	Obfs_opts             map[string]any `yaml:"obfs-opts,omitempty"`                   // Snell 混淆选项 (mode/host)
	Protocol              string         `yaml:"protocol,omitempty"`                    // SSR 协议
	ProtocolParam         string         `yaml:"protocol-param,omitempty"`              // SSR 协议参数
	ObfsParam             string         `yaml:"obfs-param,omitempty"`                  // SSR 混淆参数
	Uuid                  string         `yaml:"uuid,omitempty"`                        // UUID (VMess/VLESS)
	Peer                  string         `yaml:"peer,omitempty"`                        // Peer (Hysteria)
	Congestion_controller string         `yaml:"congestion-controller,omitempty"`       // 拥塞控制 (Tuic)
	Udp_relay_mode        string         `yaml:"udp-relay-mode,omitempty"`              // UDP 转发模式 (Tuic)
	Disable_sni           bool           `yaml:"disable-sni,omitempty"`                 // 禁用 SNI (Tuic)
	Dialer_proxy          string         `yaml:"dialer-proxy,omitempty"`                // 前置代理
	// 通用 BasicOption 字段（mihomo 所有出站共用的连接层选项）
	Mptcp          bool   `yaml:"mptcp,omitempty"`          // 是否启用 MPTCP
	Interface_name string `yaml:"interface-name,omitempty"` // 绑定的本地网卡名称
	Routing_mark   int    `yaml:"routing-mark,omitempty"`   // Linux fwmark 路由标记
	Ip_version     string `yaml:"ip-version,omitempty"`     // IP 版本偏好 (dual/ipv4/ipv6/ipv4-prefer/ipv6-prefer)
	// SS 插件字段
	Plugin      string         `yaml:"plugin,omitempty"`      // SS 插件名称
	Plugin_opts map[string]any `yaml:"plugin-opts,omitempty"` // SS 插件选项
	// WireGuard 特有字段
	Private_key    string   `yaml:"private-key,omitempty"`    // WireGuard 私钥
	Public_key     string   `yaml:"public-key,omitempty"`     // WireGuard 公钥
	Pre_shared_key string   `yaml:"pre-shared-key,omitempty"` // WireGuard 预共享密钥（可选）
	Ip             string   `yaml:"ip,omitempty"`             // WireGuard 客户端 IPv4
	Ipv6           string   `yaml:"ipv6,omitempty"`           // WireGuard 客户端 IPv6
	Mtu            int      `yaml:"mtu,omitempty"`            // MTU 值
	Reserved       []int    `yaml:"reserved,omitempty"`       // 保留字段
	Allowed_ips    []string `yaml:"allowed-ips,omitempty"`    // 允许的 IP 段
	Version        int      `yaml:"version,omitempty"`        // 版本
	Token          string   `yaml:"token,omitempty"`          // Tuic 令牌v4
	// OpenVPN 特有字段
	Proto                string            `yaml:"proto,omitempty"`                 // OpenVPN 传输协议 (tcp/udp)
	Dev                  string            `yaml:"dev,omitempty"`                   // OpenVPN 设备类型（当前 Mihomo 支持 tun）
	Data_ciphers         []string          `yaml:"data-ciphers,omitempty"`          // OpenVPN 数据通道加密算法列表
	Data_cipher_fallback string            `yaml:"data-ciphers-fallback,omitempty"` // OpenVPN 数据通道回退加密算法
	Comp_lzo             string            `yaml:"comp-lzo,omitempty"`              // OpenVPN LZO 压缩模式
	Ca                   string            `yaml:"ca,omitempty"`                    // OpenVPN CA 证书
	Cert                 string            `yaml:"cert,omitempty"`                  // OpenVPN 客户端证书
	Key                  string            `yaml:"key,omitempty"`                   // OpenVPN 客户端私钥
	Tls_auth             string            `yaml:"tls-auth,omitempty"`              // OpenVPN tls-auth 静态密钥
	Key_direction        string            `yaml:"key-direction,omitempty"`         // OpenVPN 静态密钥方向
	Tls_crypt            string            `yaml:"tls-crypt,omitempty"`             // OpenVPN tls-crypt 静态密钥
	Tls_crypt_v2         string            `yaml:"tls-crypt-v2,omitempty"`          // OpenVPN tls-crypt-v2 客户端密钥
	Peer_info            map[string]string `yaml:"peer-info,omitempty"`             // OpenVPN Peer Info
	Ping                 int               `yaml:"ping,omitempty"`                  // OpenVPN keepalive ping 间隔
	Ping_restart         int               `yaml:"ping-restart,omitempty"`          // OpenVPN ping 超时重连时间
	Tran_window          *int              `yaml:"tran-window,omitempty"`           // OpenVPN 数据通道密钥转换窗口
	Handshake_timeout    int               `yaml:"handshake-timeout,omitempty"`     // OpenVPN 握手超时时间
	Ip_stack             map[string]any    `yaml:"ip-stack,omitempty"`              // OpenVPN 用户态 IP 栈配置
	Remote_dns_resolve   bool              `yaml:"remote-dns-resolve,omitempty"`    // 通过远端解析 DNS
	Dns                  []string          `yaml:"dns,omitempty"`                   // OpenVPN DNS 服务器
	// VLESS 特有字段
	Encryption      string         `yaml:"encryption,omitempty"`
	Packet_encoding string         `yaml:"packet-encoding,omitempty"` // VLESS packet-encoding (xudp/packetaddr)
	H2_opts         map[string]any `yaml:"h2-opts,omitempty"`         // HTTP/2 传输层选项
	Http_opts       map[string]any `yaml:"http-opts,omitempty"`       // HTTP 传输层选项
	XHTTP_opts      map[string]any `yaml:"xhttp-opts,omitempty"`
	ECH_opts        map[string]any `yaml:"ech-opts,omitempty"`
	ClashExtra      yaml.Node      `yaml:"-"` // 导入时未被模型识别的 Clash 顶层属性
}

func sanitizeCertificateFingerprint(fingerprint string) string {
	fingerprint = strings.ReplaceAll(fingerprint, ":", "")
	fingerprint = strings.ReplaceAll(fingerprint, "-", "")
	if len(fingerprint) != 64 {
		return ""
	}
	for _, c := range fingerprint {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return ""
	}
	fingerprint = strings.ToLower(fingerprint)
	return fingerprint
}

type ProxyGroup struct {
	Proxies []string `yaml:"proxies"`
}
type Config struct {
	Proxies      []Proxy      `yaml:"proxies"`
	Proxy_groups []ProxyGroup `yaml:"proxy-groups"`
}

// 代理链接的结构体
type Urls struct {
	Url             string
	DialerProxyName string
	ClashExtra      string
	// ProxyGroups optionally assigns this node to one or more template proxy groups.
	// Empty preserves the historical automatic group behavior.
	ProxyGroups []string
}

// 删除opts中的空值
func DeleteOpts(opts map[string]any) {
	for k, v := range opts {
		switch v := v.(type) {
		case string:
			if v == "" {
				delete(opts, k)
			}
		case map[string]any:
			DeleteOpts(v)
			if len(v) == 0 {
				delete(opts, k)
			}
		}
	}
}
func convertToInt(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("unexpected type %T", v)
	}
}

func isTruthyConfigValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		}
	}

	return false
}

// shouldPreserveProxyGroup 判断代理组是否应保留模板原始语义，而不是在服务端展开成固定节点列表。
func proxyAssignedToGroup(assignments map[string][]string, proxyName, groupName string) bool {
	for _, assignedGroup := range assignments[proxyName] {
		if strings.TrimSpace(assignedGroup) == strings.TrimSpace(groupName) {
			return true
		}
	}
	return false
}

func shouldPreserveProxyGroup(proxyGroup map[string]any) bool {
	for _, field := range []string{"include-all", "include-all-proxies", "include-all-providers"} {
		if isTruthyConfigValue(proxyGroup[field]) {
			return true
		}
	}

	// Provider 组和自动匹配组由客户端在运行时解析，不能在服务端展开为固定节点列表。
	for _, field := range []string{"use", "filter", "exclude-filter", "exclude-type", "expected-status"} {
		if _, exists := proxyGroup[field]; exists {
			return true
		}
	}

	return false
}

// convertSSPluginOpts 将 SsPlugin 转换为 Clash 格式的 plugin-opts
// 根据不同插件类型生成对应的配置
func convertSSPluginOpts(plugin SsPlugin) map[string]any {
	if plugin.Name == "" {
		return nil
	}

	opts := make(map[string]any)

	// 从结构体字段读取值
	if plugin.Mode != "" {
		opts["mode"] = plugin.Mode
	}
	if plugin.Host != "" {
		opts["host"] = plugin.Host
	}
	if plugin.Path != "" {
		opts["path"] = plugin.Path
	}
	if plugin.Tls {
		opts["tls"] = true
	}
	if plugin.Mux {
		opts["mux"] = true
	}
	if plugin.Password != "" {
		opts["password"] = plugin.Password
	}
	if plugin.Version > 0 {
		opts["version"] = plugin.Version
	}

	if len(opts) == 0 {
		return nil
	}
	return opts
}

// LinkToProxy 将单个节点链接转换为 Proxy 结构体
// 支持 ss, ssr, trojan, vmess, vless, hysteria, hysteria2, tuic, anytls, socks5, http, https 等协议
func LinkToProxy(link Urls, config OutputConfig) (Proxy, error) {
	protocol := detectProtocol(link.Url)
	if protocol == nil {
		return Proxy{}, fmt.Errorf("unsupported scheme: %s", strings.ToLower(strings.Split(link.Url, "://")[0]))
	}
	proxyCapable, ok := protocol.(ProxyCapable)
	if !ok {
		return Proxy{}, fmt.Errorf("protocol %s does not support proxy export", protocol.Name())
	}
	proxy, err := proxyCapable.ToProxy(link, config)
	if err != nil {
		return Proxy{}, err
	}
	if err := ApplyClashExtra(&proxy, link.ClashExtra); err != nil {
		return Proxy{}, err
	}
	return proxy, nil
}

// EncodeClash 用于生成 Clash 配置文件
// 输入: 节点链接列表, SQL配置
// 输出: Clash 配置文件的 YAML 字节流
func EncodeClash(urls []Urls, config OutputConfig) ([]byte, error) {
	// 传入urls，解析urls，生成proxys
	// yamlfile 为模板文件
	var proxys []Proxy

	for _, link := range urls {
		proxy, err := LinkToProxy(link, config)
		if err != nil {
			utils.Error("链接转换失败: %s", err.Error())
			continue
		}
		proxys = append(proxys, proxy)
	}

	// 根据配置执行 Host 替换
	if config.ReplaceServerWithHost && len(config.HostMap) > 0 {
		for i := range proxys {
			if ip, exists := config.HostMap[proxys[i].Server]; exists {
				proxys[i].Server = ip
			}
		}
	}

	// 生成Clash配置文件
	assignments := make(map[string][]string)
	seenAssignments := make(map[string][]string)
	for _, link := range urls {
		// LinkToProxy uses the final protocol name; derive it again so assignment
		// remains independent of the caller's URL ordering.
		if proxy, err := LinkToProxy(link, config); err == nil {
			groups := make([]string, 0, len(link.ProxyGroups))
			seen := make(map[string]struct{}, len(link.ProxyGroups))
			for _, group := range link.ProxyGroups {
				group = strings.TrimSpace(group)
				if group == "" {
					continue
				}
				if _, exists := seen[group]; exists {
					continue
				}
				groups = append(groups, group)
				seen[group] = struct{}{}
			}
			sort.Strings(groups)
			if existingGroups, exists := seenAssignments[proxy.Name]; exists {
				if strings.Join(existingGroups, "\x00") != strings.Join(groups, "\x00") {
					return nil, fmt.Errorf("节点名称 %q 重复且代理组分配不同，请使用命名规则生成唯一名称", proxy.Name)
				}
			} else {
				seenAssignments[proxy.Name] = groups
			}
			if len(groups) > 0 {
				assignments[proxy.Name] = groups
			}
		}
	}
	return decodeClashWithOptions(proxys, config.Clash, [][]CustomProxyGroup{config.CustomProxyGroups}, config.ExcludedProxyNames, assignments, config.NodeTemplateAssignmentsEnabled)
}

// DecodeClash 用于解析 Clash 配置文件并合并新节点
// proxys: 新增的节点列表
// yamlfile: 模板文件路径或 URL
// customGroups: 自定义代理组列表（可选，由链式代理规则生成）
func DecodeClash(proxys []Proxy, yamlfile string, customGroups ...[]CustomProxyGroup) ([]byte, error) {
	return decodeClashWithOptions(proxys, yamlfile, customGroups, nil, nil, false)
}

func decodeClashWithOptions(proxys []Proxy, yamlfile string, customGroups [][]CustomProxyGroup, excludedNames []string, assignments map[string][]string, strictAssignments bool) ([]byte, error) {
	// 读取 YAML 文件
	var data []byte
	var err error
	if strings.Contains(yamlfile, "://") {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, yamlfile, nil)
		if err != nil {
			utils.Error("http.Get error: %v", err)
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			utils.Error("http.Get error: %v", err)
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			utils.Error("error: %v", err)
			return nil, err
		}
	} else {
		// 优先从缓存读取模板内容（本地文件使用缓存）
		filename := filepath.Base(yamlfile)
		if cached, ok := cache.GetTemplateContent(filename); ok {
			data = []byte(cached)
		} else {
			data, err = os.ReadFile(yamlfile)
			if err != nil {
				utils.Error("error: %v", err)
				return nil, err
			}
			// 写入缓存
			cache.SetTemplateContent(filename, string(data))
		}
	}
	// 解析 YAML 文件
	config := make(map[string]any)
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		utils.Error("error: %v", err)
		return nil, err
	}

	// 检查 "proxies" 键是否存在于 config 中
	proxies, ok := config["proxies"].([]any)
	if !ok {
		// 如果 "proxies" 键不存在，创建一个新的切片
		proxies = []any{}
	}
	// 定义一个代理列表名字。前置节点保留在 proxies 中，但不加入自动生成的代理组。
	ProxiesNameList := []string{}
	proxyNameSet := make(map[string]struct{})
	excludedProxyNames := make(map[string]bool, len(excludedNames))
	for _, name := range excludedNames {
		excludedProxyNames[strings.TrimSpace(name)] = true
	}
	for _, p := range proxys {
		proxyNameSet[p.Name] = struct{}{}
		if !excludedProxyNames[p.Name] {
			ProxiesNameList = append(ProxiesNameList, p.Name)
		}
		proxies = append(proxies, p)
	}
	// proxies = append(proxies, newProxy)
	config["proxies"] = proxies
	// 往ProxyGroup中插入代理列表
	proxyGroups, ok := config["proxy-groups"].([]any)
	if !ok {
		proxyGroups = []any{}
	}
	// Ignore unknown group names when building membership. In strict mode, a node
	// whose assignments are all unknown therefore remains out of ordinary groups.
	validGroupNames := make(map[string]struct{}, len(proxyGroups))
	for _, pg := range proxyGroups {
		if group, ok := pg.(map[string]any); ok {
			if name, ok := group["name"].(string); ok {
				validGroupNames[strings.TrimSpace(name)] = struct{}{}
			}
		}
	}
	if len(customGroups) > 0 {
		for _, customGroup := range customGroups[0] {
			if name := strings.TrimSpace(customGroup.Name); name != "" {
				validGroupNames[name] = struct{}{}
			}
		}
	}
	for proxyName, groupNames := range assignments {
		validGroups := make([]string, 0, len(groupNames))
		for _, groupName := range groupNames {
			if _, ok := validGroupNames[strings.TrimSpace(groupName)]; ok {
				validGroups = append(validGroups, strings.TrimSpace(groupName))
			}
		}
		if len(validGroups) == 0 {
			delete(assignments, proxyName)
		} else {
			assignments[proxyName] = validGroups
		}
	}

	// 插入自定义代理组（在模板组之后）
	// 使用 _custom_group 标记来标识自定义代理组，后续循环时跳过节点追加
	if len(customGroups) > 0 && len(customGroups[0]) > 0 {
		for _, cg := range customGroups[0] {
			// 构建代理组 map
			groupMap := map[string]any{
				"name":          cg.Name,
				"type":          cg.Type,
				"proxies":       cg.Proxies,
				"_custom_group": true, // 标记为自定义代理组，不追加所有节点
			}

			// 根据组类型添加相应配置
			switch cg.Type {
			case "url-test", "fallback":
				// url-test 和 fallback 类型需要 url、interval、tolerance
				if cg.URL != "" {
					groupMap["url"] = cg.URL
				} else {
					groupMap["url"] = "http://www.gstatic.com/generate_204"
				}
				if cg.Interval > 0 {
					groupMap["interval"] = cg.Interval
				} else {
					groupMap["interval"] = 300 // 默认 300 秒
				}
				if cg.Tolerance > 0 {
					groupMap["tolerance"] = cg.Tolerance
				} else {
					groupMap["tolerance"] = 50 // 默认 50 毫秒
				}

			case "load-balance":
				// load-balance 类型需要 url、interval、strategy
				if cg.URL != "" {
					groupMap["url"] = cg.URL
				} else {
					groupMap["url"] = "http://www.gstatic.com/generate_204"
				}
				if cg.Interval > 0 {
					groupMap["interval"] = cg.Interval
				} else {
					groupMap["interval"] = 300 // 默认 300 秒
				}
				if cg.Strategy != "" {
					groupMap["strategy"] = cg.Strategy
				} else {
					groupMap["strategy"] = "consistent-hashing" // 默认一致性哈希
				}
			}

			proxyGroups = append(proxyGroups, groupMap)
		}
	}

	for i, pg := range proxyGroups {
		proxyGroup, ok := pg.(map[string]any)
		if !ok {
			continue
		}

		// 链式代理不处理
		if proxyGroup["type"] == "relay" {
			continue
		}

		// 自定义代理组（由链式代理规则生成）拥有链路解析后的权威成员列表。
		// 不要对它执行普通模板组的前置节点排除，否则链式组会丢失必要成员。
		if isCustom, ok := proxyGroup["_custom_group"].(bool); ok && isCustom {
			delete(proxyGroup, "_custom_group")
			proxyGroups[i] = proxyGroup
			continue
		}

		// 前置节点可以保留在 proxies 定义中供 dialer-proxy 引用，
		// 但不应出现在普通显式节点代理组中。
		if existingProxies, ok := proxyGroup["proxies"].([]any); ok && len(excludedProxyNames) > 0 {
			filteredProxies := make([]any, 0, len(existingProxies))
			for _, proxy := range existingProxies {
				name, isName := proxy.(string)
				if !isName || !excludedProxyNames[strings.TrimSpace(name)] {
					filteredProxies = append(filteredProxies, proxy)
				}
			}
			proxyGroup["proxies"] = filteredProxies
		}

		// Explicit assignments are authoritative in strict mode. Mihomo appends
		// include-all results to proxies and applies exclude-filter to both, so
		// materialize membership rather than mixing explicit and dynamic sources.
		if strictAssignments {
			for _, field := range []string{"include-all", "include-all-proxies", "include-all-providers", "use", "filter", "exclude-filter", "exclude-type"} {
				delete(proxyGroup, field)
			}
		}

		// Disabled assignment mode preserves the template's dynamic semantics.
		if !strictAssignments && shouldPreserveProxyGroup(proxyGroup) {
			groupName, _ := proxyGroup["name"].(string)
			existing, _ := proxyGroup["proxies"].([]any)
			filteredExisting := make([]any, 0, len(existing)+len(ProxiesNameList))
			seen := make(map[string]struct{}, len(existing)+len(ProxiesNameList))
			for _, item := range existing {
				if name, ok := item.(string); ok {
					if _, assigned := assignments[name]; assigned && !proxyAssignedToGroup(assignments, name, groupName) {
						continue
					}
					if _, exists := seen[name]; exists {
						continue
					}
					seen[name] = struct{}{}
				}
				filteredExisting = append(filteredExisting, item)
			}
			for _, name := range ProxiesNameList {
				if proxyAssignedToGroup(assignments, name, groupName) {
					if _, exists := seen[name]; !exists {
						filteredExisting = append(filteredExisting, name)
						seen[name] = struct{}{}
					}
				}
			}
			if len(filteredExisting) > 0 {
				proxyGroup["proxies"] = filteredExisting
			}

			patternNames := make([]string, 0, len(excludedProxyNames)+len(assignments))
			for name := range excludedProxyNames {
				patternNames = append(patternNames, regexp.QuoteMeta(strings.TrimSpace(name)))
			}
			for name := range assignments {
				patternNames = append(patternNames, regexp.QuoteMeta(strings.TrimSpace(name)))
			}
			if len(patternNames) > 0 {
				excludedPattern := "^(?:" + strings.Join(patternNames, "|") + ")$"
				if existingFilter, ok := proxyGroup["exclude-filter"].(string); ok && strings.TrimSpace(existingFilter) != "" {
					proxyGroup["exclude-filter"] = "(?:" + existingFilter + ")|" + excludedPattern
				} else {
					proxyGroup["exclude-filter"] = excludedPattern
				}
			}
			proxyGroups[i] = proxyGroup
			continue
		}

		// In strict mode, only explicitly assigned nodes are emitted in ordinary groups;
		// otherwise assigned nodes are filtered while unassigned nodes retain legacy behavior.
		groupNodeNames := ProxiesNameList
		if groupName, ok := proxyGroup["name"].(string); ok && (len(assignments) > 0 || strictAssignments) {
			groupNodeNames = make([]string, 0, len(ProxiesNameList))
			for _, name := range ProxiesNameList {
				if strictAssignments {
					if !proxyAssignedToGroup(assignments, name, groupName) {
						continue
					}
				} else if _, exists := assignments[name]; exists && !proxyAssignedToGroup(assignments, name, groupName) {
					continue
				}
				groupNodeNames = append(groupNodeNames, name)
			}
		}

		// 获取现有的 proxies 列表
		var existingProxies []any
		if proxyGroup["proxies"] != nil {
			existingProxies, _ = proxyGroup["proxies"].([]any)
		}
		// Explicit assignments override generated membership while preserving
		// unrelated template entries. Add assigned nodes to their target group
		// even when that group already has static entries.
		if groupName, ok := proxyGroup["name"].(string); ok && (len(assignments) > 0 || strictAssignments) {
			filtered := make([]any, 0, len(existingProxies)+len(ProxiesNameList))
			seen := make(map[string]struct{}, len(existingProxies)+len(ProxiesNameList))
			for _, item := range existingProxies {
				name, isName := item.(string)
				if isName {
					if strictAssignments {
						if _, isNode := proxyNameSet[name]; isNode && !proxyAssignedToGroup(assignments, name, groupName) {
							continue
						}
					} else if _, exists := assignments[name]; exists && !proxyAssignedToGroup(assignments, name, groupName) {
						continue
					}
					if _, exists := seen[name]; exists {
						continue
					}
					seen[name] = struct{}{}
				}
				filtered = append(filtered, item)
			}
			existingProxies = filtered
			for _, name := range ProxiesNameList {
				if proxyAssignedToGroup(assignments, name, groupName) {
					if _, exists := seen[name]; exists {
						continue
					}
					seen[name] = struct{}{}
					existingProxies = append(existingProxies, name)
				}
			}
			proxyGroup["proxies"] = existingProxies
		}

		// 检查是否包含 __ALL_PROXIES__ 占位符（与 subconverter 行为一致）
		// 如果有占位符，将其替换为所有节点
		hasPlaceholder := false
		placeholderIndex := -1
		for idx, proxy := range existingProxies {
			if proxyStr, ok := proxy.(string); ok && proxyStr == "__ALL_PROXIES__" {
				hasPlaceholder = true
				placeholderIndex = idx
				break
			}
		}

		if hasPlaceholder {
			// 构建新的 proxies 列表：占位符之前的元素 + 非前置节点，且不重复。
			var newProxies []any
			seen := make(map[string]struct{})
			appendProxy := func(value any) {
				if name, ok := value.(string); ok {
					if _, exists := seen[name]; exists {
						return
					}
					seen[name] = struct{}{}
				}
				newProxies = append(newProxies, value)
			}
			// 添加占位符之前的元素（组引用如 🔯 故障转移、♻️ 自动选择、DIRECT 等）
			for j := 0; j < placeholderIndex; j++ {
				appendProxy(existingProxies[j])
			}
			// 添加所有节点
			for _, newProxy := range groupNodeNames {
				appendProxy(newProxy)
			}
			// 添加占位符之后的元素（如果有的话）
			for j := placeholderIndex + 1; j < len(existingProxies); j++ {
				appendProxy(existingProxies[j])
			}
			proxyGroup["proxies"] = newProxies
			proxyGroups[i] = proxyGroup
			continue
		}

		// 原有逻辑：只有当 proxies 列表为空时才追加所有节点
		// 如果已有 proxies（组引用如 🚀 节点选择、DIRECT 等），保持不变
		if len(existingProxies) == 0 {
			// 没有任何 proxies，追加非前置节点
			var validProxies []any
			for _, newProxy := range groupNodeNames {
				validProxies = append(validProxies, newProxy)
			}
			// 如果仍然为空，插入 DIRECT 作为后备
			if len(validProxies) == 0 {
				validProxies = append(validProxies, "DIRECT")
			}
			proxyGroup["proxies"] = validProxies
			proxyGroups[i] = proxyGroup
		}
		// 已有 proxies 的组保持不变
	}

	config["proxy-groups"] = proxyGroups

	// 将修改后的内容写回文件
	newData, err := yaml.Marshal(config)
	if err != nil {
		utils.Error("error: %v", err)
	}
	return newData, nil
}
