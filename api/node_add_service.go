package api

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sublink/models"
	"sublink/node"
	"sublink/node/protocol"
	"time"

	"gopkg.in/yaml.v3"
)

type nodeAddInput struct {
	Link            string
	Name            string
	NameMode        string
	Remark          string
	DialerProxyName string
	Group           string
	SkipTLSVerify   bool
	Tags            string
}

func parseRemoteSubscriptionURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return nil, err
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return nil, errors.New("not a remote subscription URL")
	}
	return parsed, nil
}

func parseNodeAddTags(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func appendNodeAddFailure(result *NodeAddResult, name, protocolName string, reason NodeAddReason, err error) {
	item := NodeAddItem{Status: NodeAddItemFailed, Name: name, Protocol: protocolName, Reason: reason}
	if err != nil {
		diagnostic := NodeAddDiagnostic{Code: reason, Message: SafeNodeAddError(err)}
		item.Diagnostic = &diagnostic
		result.Diagnostics = append(result.Diagnostics, diagnostic)
	}
	result.Items = append(result.Items, item)
	result.Failed++
}

func processNodeAdd(ctx context.Context, input nodeAddInput) NodeAddResult {
	result := NodeAddResult{Mode: "link"}
	link := strings.TrimSpace(input.Link)
	if link == "" {
		result.Invalid = 1
		result.Items = []NodeAddItem{{Status: NodeAddItemInvalid, Reason: NodeAddReasonInvalidInput}}
		classifyNodeAddResult(&result)
		return result
	}
	tags := parseNodeAddTags(input.Tags)

	if parsedURL, err := parseRemoteSubscriptionURL(link); err == nil && parsedURL != nil && !protocol.IsHTTPLink(link) {
		result.Mode = "subscription"
		payload, fetchErr := fetchNodeSubscriptionPayload(ctx, link, input.SkipTLSVerify)
		if fetchErr != nil {
			result.Failed = 1
			result.Retryable = true
			result.Diagnostics = []NodeAddDiagnostic{{Code: NodeAddReasonNetworkError, Message: SafeNodeAddError(fetchErr), Retryable: true}}
			classifyNodeAddResult(&result)
			return result
		}
		parsed, _ := node.ParseSubscriptionPayloadWithDiagnostics(payload)
		for _, diagnostic := range parsed.Diagnostics {
			result.Diagnostics = append(result.Diagnostics, NodeAddDiagnostic{Code: NodeAddReasonParseError, Message: TruncateNodeAddText(diagnostic.Message, maxNodeAddDiagnostic)})
		}
		for _, candidate := range parsed.URLs {
			if ctx.Err() != nil {
				break
			}
			identity, identityErr := protocol.ExtractLinkIdentity(candidate.Url)
			protocolName := protocol.GetProtocolFromLink(candidate.Url)
			item := NodeAddItem{Name: identity.Name, Protocol: protocolName}
			if identityErr != nil || protocolName == "" {
				item.Status, item.Reason = NodeAddItemInvalid, NodeAddReasonInvalidInput
				result.Invalid++
				result.Items = append(result.Items, item)
				continue
			}
			n := models.Node{Name: identity.Name, Remark: input.Remark, NameMode: models.NodeNameModeLink, Link: candidate.Url, LinkName: identity.Name, LinkAddress: identity.Address, LinkHost: identity.Host, LinkPort: identity.Port, Protocol: protocolName, DialerProxyName: input.DialerProxyName, Group: input.Group, SkipTLSVerify: input.SkipTLSVerify, ClashExtra: candidate.ClashExtra, ValidationStatus: models.ValidationStatusSuccess, ValidationReason: "validated", ValidationCheckAt: time.Now().UTC().Format(time.RFC3339)}
			if err := n.Add(); err != nil {
				if errors.Is(err, models.ErrNodeVariantExists) || errors.Is(err, models.ErrNodeNameExists) {
					item.Status, item.Reason = NodeAddItemSkipped, NodeAddReasonDuplicate
					result.Skipped++
					result.Items = append(result.Items, item)
				} else {
					appendNodeAddFailure(&result, identity.Name, protocolName, NodeAddReasonPersistenceError, err)
				}
				continue
			}
			if len(tags) > 0 {
				if err := n.SetTagNames(tags); err != nil {
					result.Warnings++
					result.Diagnostics = append(result.Diagnostics, NodeAddDiagnostic{Code: NodeAddReasonPersistenceError, Message: SafeNodeAddError(err)})
				}
			}
			item.Status, item.NodeID = NodeAddItemAdded, n.ID
			result.Added++
			result.Items = append(result.Items, item)
		}
		classifyNodeAddResult(&result)
		return result
	}

	crossAirportDedupVal, _ := models.GetSetting("cross_airport_dedup_enabled")
	enableCrossDedup := crossAirportDedupVal != "false"
	if protocol.IsWireGuardConfig(link) {
		wg, err := protocol.ParseWireGuardConfig(link)
		if err != nil {
			result.Invalid = 1
			result.Items = []NodeAddItem{{Status: NodeAddItemInvalid, Reason: NodeAddReasonInvalidInput}}
			result.Diagnostics = []NodeAddDiagnostic{{Code: NodeAddReasonParseError, Message: SafeNodeAddError(err)}}
			classifyNodeAddResult(&result)
			return result
		}
		link = protocol.EncodeWireGuardURL(wg)
	}

	if strings.Contains(link, "proxies:") {
		result.Mode = "clash"
		var clashConfig node.ClashConfig
		if err := yaml.Unmarshal([]byte(link), &clashConfig); err != nil || len(clashConfig.Proxies) == 0 {
			result.Invalid = 1
			result.Items = []NodeAddItem{{Status: NodeAddItemInvalid, Reason: NodeAddReasonParseError}}
			classifyNodeAddResult(&result)
			return result
		}
		for _, proxy := range clashConfig.Proxies {
			if ctx.Err() != nil {
				break
			}
			proxyLink := node.GenerateProxyLink(proxy)
			protocolName := protocol.GetProtocolFromLink(proxyLink)
			if proxyLink == "" || protocolName == "" {
				result.Invalid++
				result.Items = append(result.Items, NodeAddItem{Status: NodeAddItemInvalid, Name: proxy.Name, Reason: NodeAddReasonUnsupportedProtocol})
				continue
			}
			clashExtra, err := protocol.EncodeClashExtra(proxy)
			if err != nil {
				appendNodeAddFailure(&result, proxy.Name, protocolName, NodeAddReasonParseError, err)
				continue
			}
			n := models.Node{Name: models.GenerateUniqueNodeName(proxy.Name, 0, nil), Remark: input.Remark, NameMode: models.NodeNameModeLink, Link: proxyLink, ClashExtra: clashExtra, LinkName: proxy.Name, LinkHost: proxy.Server, LinkPort: strconv.Itoa(proxy.Port.Int()), DialerProxyName: input.DialerProxyName, Group: input.Group, SkipTLSVerify: input.SkipTLSVerify, Protocol: protocolName, ValidationStatus: models.ValidationStatusSuccess, ValidationReason: "validated", ValidationCheckAt: time.Now().UTC().Format(time.RFC3339)}
			n.LinkAddress = n.LinkHost + ":" + n.LinkPort
			contentHash := protocol.GenerateProxyContentHash(proxy)
			if contentHash != "" {
				n.ContentHash = contentHash
				var exists bool
				if enableCrossDedup {
					_, exists = models.GetNodeByContentHashAndDialer(contentHash, n.DialerProxyName)
				} else {
					_, exists = models.GetNodeByContentHashSourceAndDialer(contentHash, 0, n.DialerProxyName)
				}
				if exists {
					result.Skipped++
					result.Items = append(result.Items, NodeAddItem{Status: NodeAddItemSkipped, Name: proxy.Name, Protocol: protocolName, Reason: NodeAddReasonDuplicate})
					continue
				}
			}
			if err := n.Add(); err != nil {
				appendNodeAddFailure(&result, proxy.Name, protocolName, NodeAddReasonPersistenceError, err)
				continue
			}
			if len(tags) > 0 {
				if err := n.SetTagNames(tags); err != nil {
					result.Warnings++
					result.Diagnostics = append(result.Diagnostics, NodeAddDiagnostic{Code: NodeAddReasonPersistenceError, Message: SafeNodeAddError(err)})
				}
			}
			result.Added++
			result.Items = append(result.Items, NodeAddItem{Status: NodeAddItemAdded, Name: proxy.Name, Protocol: protocolName, NodeID: n.ID})
		}
		classifyNodeAddResult(&result)
		return result
	}

	if !strings.Contains(link, "://") {
		result.Invalid = 1
		result.Items = []NodeAddItem{{Status: NodeAddItemInvalid, Reason: NodeAddReasonInvalidInput}}
		classifyNodeAddResult(&result)
		return result
	}
	identity, err := protocol.ExtractLinkIdentity(link)
	if err != nil {
		result.Invalid = 1
		result.Items = []NodeAddItem{{Status: NodeAddItemInvalid, Reason: NodeAddReasonInvalidInput}}
		classifyNodeAddResult(&result)
		return result
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = models.GenerateUniqueNodeName(identity.Name, 0, nil)
	} else if err := models.EnsureNodeNameAvailable(name, 0); err != nil {
		result.Skipped = 1
		result.Items = []NodeAddItem{{Status: NodeAddItemSkipped, Name: name, Reason: NodeAddReasonDuplicate}}
		classifyNodeAddResult(&result)
		return result
	}
	protocolName := protocol.GetProtocolFromLink(link)
	if protocolName == "" {
		result.Invalid = 1
		result.Items = []NodeAddItem{{Status: NodeAddItemInvalid, Name: name, Reason: NodeAddReasonUnsupportedProtocol}}
		classifyNodeAddResult(&result)
		return result
	}
	n := models.Node{Name: name, Remark: input.Remark, NameMode: models.NormalizeNodeNameMode(input.NameMode), Link: link, LinkName: identity.Name, LinkAddress: identity.Address, LinkHost: identity.Host, LinkPort: identity.Port, DialerProxyName: input.DialerProxyName, Group: input.Group, SkipTLSVerify: input.SkipTLSVerify, Protocol: protocolName, ValidationStatus: models.ValidationStatusSuccess, ValidationReason: "validated", ValidationCheckAt: time.Now().UTC().Format(time.RFC3339)}
	if proxy, err := protocol.LinkToProxy(protocol.Urls{Url: link}, protocol.OutputConfig{}); err == nil {
		n.ContentHash = protocol.GenerateProxyContentHash(proxy)
		if n.ContentHash != "" {
			var exists bool
			if enableCrossDedup {
				_, exists = models.GetNodeByContentHashAndDialer(n.ContentHash, n.DialerProxyName)
			} else {
				_, exists = models.GetNodeByContentHashSourceAndDialer(n.ContentHash, 0, n.DialerProxyName)
			}
			if exists {
				result.Skipped = 1
				result.Items = []NodeAddItem{{Status: NodeAddItemSkipped, Name: name, Protocol: protocolName, Reason: NodeAddReasonDuplicate}}
				classifyNodeAddResult(&result)
				return result
			}
		}
	}
	if err := n.Add(); err != nil {
		appendNodeAddFailure(&result, name, protocolName, NodeAddReasonPersistenceError, err)
		classifyNodeAddResult(&result)
		return result
	}
	if len(tags) > 0 {
		if err := n.SetTagNames(tags); err != nil {
			result.Warnings++
			result.Diagnostics = append(result.Diagnostics, NodeAddDiagnostic{Code: NodeAddReasonPersistenceError, Message: SafeNodeAddError(err)})
		}
	}
	result.Added = 1
	result.Items = []NodeAddItem{{Status: NodeAddItemAdded, Name: name, Protocol: protocolName, NodeID: n.ID}}
	classifyNodeAddResult(&result)
	return result
}
