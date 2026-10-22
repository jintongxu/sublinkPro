package protocol

import (
	"strings"
	"sublink/utils"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSSRMalformedQueryReturnsError(t *testing.T) {
	for _, query := range []string{"broken", "remarks=bmFtZQ&broken"} {
		link := "ssr://" + utils.Base64Encode("example.com:8388:origin:aes-256-cfb:plain:c2VjcmV0/?"+query)
		if _, err := DecodeSSRURL(link); err == nil {
			t.Fatalf("accepted malformed query %q", query)
		}
	}
}

// TestSSRClashYAMLParametersRoundTrip 验证 SSR 专用参数在 Clash YAML 与 SSR 链接之间不会丢失。
func TestSSRClashYAMLParametersRoundTrip(t *testing.T) {
	var config struct {
		Proxies []Proxy `yaml:"proxies"`
	}
	err := yaml.Unmarshal([]byte(`proxies:
  - name: imported-ssr
    type: ssr
    server: ssr.example.com
    port: 8388
    cipher: aes-256-cfb
    password: test-password
    protocol: auth_aes128_md5
    protocol-param: "30135:test-user-key"
    obfs: http_simple
    obfs-param: download.example.com
`), &config)
	if err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}
	if len(config.Proxies) != 1 {
		t.Fatalf("proxy count = %d, want 1", len(config.Proxies))
	}

	link, err := EncodeProxyLink(config.Proxies[0])
	if err != nil {
		t.Fatalf("EncodeProxyLink failed: %v", err)
	}
	decoded, err := DecodeSSRURL(link)
	if err != nil {
		t.Fatalf("DecodeSSRURL failed: %v", err)
	}
	assertEqualString(t, "ProtocolParam", "30135:test-user-key", decoded.Qurey.Protoparam)
	assertEqualString(t, "ObfsParam", "download.example.com", decoded.Qurey.Obfsparam)

	rebuilt, err := buildSSRProxy(Urls{Url: link}, OutputConfig{})
	if err != nil {
		t.Fatalf("buildSSRProxy failed: %v", err)
	}
	data, err := yaml.Marshal(rebuilt)
	if err != nil {
		t.Fatalf("yaml marshal failed: %v", err)
	}
	for _, want := range []string{
		"protocol-param: 30135:test-user-key",
		"obfs-param: download.example.com",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("Clash YAML output missing %q: %s", want, string(data))
		}
	}
}

func TestSSRQueryRejectsMalformedTokens(t *testing.T) {
	for _, query := range []string{"broken", "remarks=bm9kZQ==&broken", "broken&remarks=bm9kZQ=="} {
		t.Run(query, func(t *testing.T) {
			link := "ssr://" + utils.Base64Encode("example.com:8388:origin:aes-256-cfb:plain:c2VjcmV0/?"+query)
			if _, err := DecodeSSRURL(link); err == nil {
				t.Fatal("malformed query accepted")
			}
			if _, err := LinkToProxy(Urls{Url: link}, OutputConfig{}); err == nil {
				t.Fatal("malformed query converted")
			}
		})
	}
	link := "ssr://" + utils.Base64Encode("example.com:8388:origin:aes-256-cfb:plain:c2VjcmV0/?remarks=bm9kZQ==")
	decoded, err := DecodeSSRURL(link)
	if err != nil || decoded.Qurey.Remarks != "node" {
		t.Fatalf("padded value lost: %+v %v", decoded, err)
	}
}
