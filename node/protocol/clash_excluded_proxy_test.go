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
