package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sublink/database"
	"sublink/models"
	"sublink/node/protocol"
	"sublink/utils"
	"testing"
)

func TestNodeAddRemoteSameNameDistinctContent(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	useLocalSubscriptionFetch(t)
	if result := processNodeAdd(context.Background(), nodeAddInput{Link: "trojan://secret@existing.example.com:443#Shared"}); result.Added != 1 {
		t.Fatalf("seed: %+v", result)
	}
	payload := strings.Join([]string{
		"trojan://secret@one.example.com:443#Shared",
		"trojan://secret@two.example.com:443#Shared",
		"trojan://secret@one.example.com:443#RenamedDuplicate",
	}, "\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(payload)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	result := processNodeAdd(context.Background(), nodeAddInput{Link: server.URL + "/subscription"})
	if result.Added != 2 || result.Skipped != 1 || result.Invalid != 0 || result.Failed != 0 {
		t.Fatalf("remote name collisions: %+v", result)
	}
	var nodes []models.Node
	if err := database.DB.Order("id").Find(&nodes).Error; err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("node count = %d, want 3", len(nodes))
	}
	names := make(map[string]bool)
	for _, n := range nodes {
		if n.Name == "" || names[n.Name] || n.LinkName != "Shared" || n.ContentHash == "" {
			t.Fatalf("unique Name / unchanged LinkName violated: %+v", n)
		}
		names[n.Name] = true
	}
	if result.Items[2].Reason != NodeAddReasonDuplicate {
		t.Fatalf("content duplicate not classified: %+v", result.Items)
	}
}

func TestNodeAddBoundedResults(t *testing.T) {
	result := NodeAddResult{}
	for i := 0; i < 500; i++ {
		mergeNodeAddResult(&result, NodeAddResult{Added: 1, Items: []NodeAddItem{{Status: NodeAddItemAdded}}, Diagnostics: []NodeAddDiagnostic{{Code: NodeAddReasonParseError}}})
	}
	if result.Added != 500 || len(result.Items) != 200 || len(result.Diagnostics) != 32 || result.OmittedItems != 300 || result.OmittedDiagnostics != 468 || !result.Truncated {
		t.Fatalf("unexpected bounded result: %+v", result)
	}
}
func TestNodeAddSharedValidationAndWarnings(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	link := protocol.EncodeSSURL(protocol.Ss{Name: "contract", Server: "edge.example.com", Port: 8388, Param: protocol.Param{Cipher: "aes-256-gcm", Password: "password"}})
	if _, _, err := ValidateNodeLink(protocol.Urls{Url: link}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidateNodeLink(protocol.Urls{Url: "not-a-node"}); err == nil {
		t.Fatal("invalid identity accepted")
	}
	// Inject a test-local update failure to exercise non-fatal tag warnings.
	if err := database.DB.Callback().Update().Before("gorm:update").Register("test:tag_failure", func(tx *gorm.DB) { _ = tx.AddError(errors.New("tag persistence failed")) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.DB.Callback().Update().Remove("test:tag_failure"); err != nil {
			t.Error(err)
		}
	})
	result := processNodeAdd(context.Background(), nodeAddInput{Link: link, Tags: "tag"})
	if result.Added != 1 || result.Warnings != 1 || result.Status != NodeAddStatusPartial {
		t.Fatalf("tag warning: %+v", result)
	}
	duplicate := processNodeAdd(context.Background(), nodeAddInput{Link: link})
	if duplicate.Skipped != 1 || duplicate.DuplicateInfo["existingName"] == "" {
		t.Fatalf("duplicate contract: %+v", duplicate)
	}
}
func TestNodeAddHTTPProxyClassification(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	result := processNodeAdd(context.Background(), nodeAddInput{Link: "http://user:pass@proxy.example.com:8080#http-node"})
	if result.Mode != "link" || result.Added != 1 {
		t.Fatalf("HTTP proxy misclassified: %+v", result)
	}
}

func TestNodeAddWireGuardAndClash(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	wg := `[Interface]
Address = 172.16.0.2/32
PrivateKey = OOrigZsSjw2YaY4urjbbU4/BNOZKXqW6EYNm8XKLtkU=
[Peer]
PublicKey = bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=
Endpoint = wg.example.com:7152
AllowedIPs = 0.0.0.0/0`
	if r := processNodeAdd(context.Background(), nodeAddInput{Link: wg}); r.Added != 1 {
		t.Fatalf("WG: %+v", r)
	}
	clash := `proxies:
  - name: clash-contract
    type: ss
    server: clash.example.com
    port: 8388
    cipher: aes-256-gcm
    password: secret`
	if r := processNodeAdd(context.Background(), nodeAddInput{Link: clash}); r.Added != 1 {
		t.Fatalf("Clash: %+v", r)
	}
}

func TestNodeAddSyncAliasAndRemote(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	useLocalSubscriptionFetch(t)
	link := protocol.EncodeSSURL(protocol.Ss{Name: "remote-contract", Server: "remote.example.com", Port: 8388, Param: protocol.Param{Cipher: "aes-256-gcm", Password: "password"}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(link + "\ninvalid-line")); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	r := processNodeAdd(context.Background(), nodeAddInput{Link: server.URL + "/subscription"})
	if r.Added != 1 || r.Invalid != 1 || r.Status != NodeAddStatusPartial {
		t.Fatalf("remote: %+v", r)
	}
	form := url.Values{"link": {link}, "NameMode": {"custom"}}
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	writer := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(writer)
	ctx.Request = req
	if nodeAddForm(ctx).NameMode != "custom" {
		t.Fatal("legacy alias lost")
	}
	NodeAdd(ctx)
	var response struct {
		Code int
		Data struct {
			Skipped       bool
			DuplicateInfo map[string]string
		}
	}
	if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || !response.Data.Skipped {
		t.Fatalf("duplicate envelope: %s", writer.Body.String())
	}
}

func TestNodeAddVMessWithoutTransportHost(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	link := protocol.EncodeVmessURL(protocol.Vmess{Add: "example.com", Port: "443", Id: "88888888-9999-7777-5555-777777777777", Net: "tcp", V: "2"})
	identity, canonical, err := ValidateNodeLink(protocol.Urls{Url: link})
	if err != nil || identity.Host != "example.com" || canonical.Server != "example.com" {
		t.Fatalf("validation: %+v %+v %v", identity, canonical, err)
	}
	result := processNodeAdd(context.Background(), nodeAddInput{Link: link})
	if result.Added != 1 || result.Invalid != 0 {
		t.Fatalf("import: %+v", result)
	}
}

func TestNodeAddRemoteMalformedSSRPreservesNeighbors(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	useLocalSubscriptionFetch(t)
	broken := "ssr://" + utils.Base64Encode("example.com:8388:origin:aes-256-cfb:plain:c2VjcmV0/?broken")
	payload := "trojan://secret@one.example.com:443#one\n" + broken + "\ntrojan://secret@two.example.com:443#two"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(payload)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	result := processNodeAdd(context.Background(), nodeAddInput{Link: server.URL + "/subscription"})
	if result.Added != 2 || result.Invalid != 1 || result.Failed != 0 || result.Status != NodeAddStatusPartial {
		t.Fatalf("import: %+v", result)
	}
	var nodes []models.Node
	if err := database.DB.Order("id").Find(&nodes).Error; err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].LinkHost != "one.example.com" || nodes[1].LinkHost != "two.example.com" {
		t.Fatalf("neighbors: %+v", nodes)
	}
}

func TestNodeAddRemoteUniqueNamesAndContentDuplicates(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			setupNodeUpdateAPITestDB(t)
			useLocalSubscriptionFetch(t)
			if existing {
				n := models.Node{Name: "Shared", Link: "trojan://other@existing.example.com:443#Shared"}
				if err := n.Add(); err != nil {
					t.Fatal(err)
				}
			}
			payload := "trojan://secret@one.example.com:443#Shared\ntrojan://secret@two.example.com:443#Shared\ntrojan://secret@one.example.com:443#Renamed"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := w.Write([]byte(payload)); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			result := processNodeAdd(context.Background(), nodeAddInput{Link: server.URL + "/subscription"})
			if result.Added != 2 || result.Skipped != 1 || result.Invalid != 0 || result.Failed != 0 {
				t.Fatalf("import: %+v", result)
			}
			var nodes []models.Node
			if err := database.DB.Where("link_host IN ?", []string{"one.example.com", "two.example.com"}).Order("id").Find(&nodes).Error; err != nil {
				t.Fatal(err)
			}
			if len(nodes) != 2 || nodes[0].Name == nodes[1].Name {
				t.Fatalf("names: %+v", nodes)
			}
			for _, n := range nodes {
				if n.LinkName != "Shared" || (existing && n.Name == "Shared") {
					t.Fatalf("link name not preserved: %+v", n)
				}
			}
			again := processNodeAdd(context.Background(), nodeAddInput{Link: server.URL + "/subscription"})
			if again.Added != 0 || again.Skipped != 3 || again.Failed != 0 {
				t.Fatalf("repeat: %+v", again)
			}
		})
	}
}
