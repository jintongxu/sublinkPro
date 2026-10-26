package protocol

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEncodeClashKeepsDialerDefinitionButHidesItFromGroups(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "clash-template.yaml")
	template := "proxies: []\nproxy-groups:\n  - name: Proxy\n    type: select\n    proxies: []\n"
	if err := os.WriteFile(templatePath, []byte(template), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	front := EncodeSSURL(Ss{Name: "front-node", Server: "front.example.com", Port: 443, Param: Param{Cipher: "aes-256-gcm", Password: "front-pass"}})
	target := EncodeSSURL(Ss{Name: "target-node", Server: "target.example.com", Port: 443, Param: Param{Cipher: "aes-256-gcm", Password: "target-pass"}})
	data, err := EncodeClash([]Urls{
		{Url: front},
		{Url: target, DialerProxyName: "front-node"},
	}, OutputConfig{Clash: templatePath, ExcludedProxyNames: []string{"front-node"}})
	if err != nil {
		t.Fatalf("encode clash: %v", err)
	}

	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	proxies, ok := config["proxies"].([]any)
	if !ok || len(proxies) != 2 {
		t.Fatalf("proxies = %#v, want two definitions", config["proxies"])
	}
	groups, ok := config["proxy-groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("proxy-groups = %#v", config["proxy-groups"])
	}
	group, ok := groups[0].(map[string]any)
	if !ok {
		t.Fatalf("proxy group type = %T, want map", groups[0])
	}
	members, ok := group["proxies"].([]any)
	if !ok {
		t.Fatalf("proxy group members type = %T, want []any", group["proxies"])
	}
	if len(members) != 1 || members[0] != "target-node" {
		t.Fatalf("group members = %#v, want only target-node", members)
	}
}

// TestEncodeClashKeepsExplicitlyAssignedFrontProxyInGroup verifies that a front
// (dialer) proxy stays hidden from automatic membership but can be assigned to a
// template proxy group on purpose.
func TestEncodeClashKeepsExplicitlyAssignedFrontProxyInGroup(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "explicit-front-strict.yaml")
	template := "proxies: []\nproxy-groups:\n  - name: Auto\n    type: select\n    proxies: []\n  - name: FrontOnly\n    type: select\n    proxies: []\n"
	if err := os.WriteFile(templatePath, []byte(template), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	front := EncodeSSURL(Ss{Name: "front-node", Server: "front.example.com", Port: 443, Param: Param{Cipher: "aes-256-gcm", Password: "front-pass"}})
	target := EncodeSSURL(Ss{Name: "target-node", Server: "target.example.com", Port: 443, Param: Param{Cipher: "aes-256-gcm", Password: "target-pass"}})
	data, err := EncodeClash([]Urls{
		{Url: front, ProxyGroups: []string{"FrontOnly"}},
		{Url: target, DialerProxyName: "front-node"},
	}, OutputConfig{Clash: templatePath, ExcludedProxyNames: []string{"front-node"}, NodeTemplateAssignmentsEnabled: true})
	if err != nil {
		t.Fatalf("encode clash: %v", err)
	}

	members := proxyGroupMembersFromBody(t, data)
	if len(members["FrontOnly"]) != 1 || !members["FrontOnly"]["front-node"] {
		t.Fatalf("FrontOnly members = %#v, want only front-node", members["FrontOnly"])
	}
	if members["Auto"]["front-node"] {
		t.Fatalf("Auto group must not gain the front proxy: %#v", members["Auto"])
	}
	if members["FrontOnly"]["target-node"] {
		t.Fatalf("FrontOnly must stay limited to its assigned node: %#v", members["FrontOnly"])
	}
}

// TestEncodeClashKeepsExplicitlyAssignedFrontProxyWithDynamicGroup covers the
// legacy (non-strict) template path where include-all still applies.
func TestEncodeClashKeepsExplicitlyAssignedFrontProxyWithDynamicGroup(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "explicit-front-template.yaml")
	template := "proxies: []\nproxy-groups:\n  - name: Auto\n    type: select\n    include-all: true\n  - name: FrontOnly\n    type: select\n    include-all: true\n"
	if err := os.WriteFile(templatePath, []byte(template), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	front := EncodeSSURL(Ss{Name: "front-node", Server: "front.example.com", Port: 443, Param: Param{Cipher: "aes-256-gcm", Password: "front-pass"}})
	target := EncodeSSURL(Ss{Name: "target-node", Server: "target.example.com", Port: 443, Param: Param{Cipher: "aes-256-gcm", Password: "target-pass"}})
	data, err := EncodeClash([]Urls{
		{Url: front, ProxyGroups: []string{"FrontOnly"}},
		{Url: target, DialerProxyName: "front-node"},
	}, OutputConfig{Clash: templatePath, ExcludedProxyNames: []string{"front-node"}})
	if err != nil {
		t.Fatalf("encode clash: %v", err)
	}

	members := proxyGroupMembersFromBody(t, data)
	if !members["FrontOnly"]["front-node"] {
		t.Fatalf("FrontOnly must keep the assigned front proxy: %#v", members["FrontOnly"])
	}
	if members["Auto"]["front-node"] {
		t.Fatalf("Auto group must not gain the front proxy: %#v", members["Auto"])
	}
}

func proxyGroupMembersFromBody(t *testing.T, data []byte) map[string]map[string]bool {
	t.Helper()
	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	groups, ok := config["proxy-groups"].([]any)
	if !ok {
		t.Fatalf("proxy-groups = %#v", config["proxy-groups"])
	}
	members := make(map[string]map[string]bool, len(groups))
	for _, rawGroup := range groups {
		group, ok := rawGroup.(map[string]any)
		if !ok {
			continue
		}
		name, _ := group["name"].(string)
		members[name] = make(map[string]bool)
		items, _ := group["proxies"].([]any)
		for _, item := range items {
			if member, ok := item.(string); ok {
				members[name][member] = true
			}
		}
	}
	return members
}
