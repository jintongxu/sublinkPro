package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"sublink/database"
	"sublink/internal/testutil"
	"sublink/models"
	"sublink/node/protocol"

	"github.com/gin-gonic/gin"
)

func TestNodeAddSafeFormatting(t *testing.T) {
	url := SafeNodeAddURL("trojan://user:secret@example.com:443/path?token=abc#fragment")
	if url != "trojan://example.com:443" {
		t.Fatalf("safe URL = %q", url)
	}
	if strings.Contains(url, "secret") || strings.Contains(url, "token") {
		t.Fatalf("safe URL leaked credential: %q", url)
	}

	errText := SafeNodeAddError(errors.New("failed trojan://u:p@example.com:443/x?token=abc password=secret"))
	if strings.Contains(errText, "u:p") || strings.Contains(errText, "token=abc") || strings.Contains(errText, "secret") {
		t.Fatalf("safe error leaked credential: %q", errText)
	}
	if got := TruncateNodeAddText(strings.Repeat("x", 300), 32); len([]rune(got)) > 32 || !strings.HasSuffix(got, "...") {
		t.Fatalf("truncation = %q", got)
	}
	if got := TruncateNodeAddText("abcdef", 2); got != "ab" {
		t.Fatalf("short truncation = %q", got)
	}
}

func TestNodeAddContracts(t *testing.T) {
	result := NodeAddResult{Status: NodeAddStatusPartial, Added: 1, Skipped: 1, Items: []NodeAddItem{{Status: NodeAddItemSkipped, Reason: NodeAddReasonDuplicate}}}
	if result.Status != NodeAddStatusPartial || result.Items[0].Reason != NodeAddReasonDuplicate {
		t.Fatal("unexpected node-add contract values")
	}
}

func TestSubscriptionDestinationRejectsPrivateAddresses(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1/sub", "http://10.0.0.1/sub", "http://100.64.0.1/sub", "http://198.18.0.1/sub", "http://[::1]/sub"} {
		if err := validateSubscriptionDestination(context.Background(), raw); err == nil {
			t.Fatalf("expected private destination rejection for %q", raw)
		}
	}
}

func TestSplitNodeAddLinks(t *testing.T) {
	links := splitNodeAddLinks("ss://one\nvmess://two, trojan://three")
	if len(links) != 3 || links[0] != "ss://one" || links[2] != "trojan://three" {
		t.Fatalf("split links = %#v", links)
	}
	clash := "proxies:\n  - name: demo\n    type: ss"
	if got := splitNodeAddLinks(clash); len(got) != 1 || got[0] != clash {
		t.Fatalf("clash payload split unexpectedly: %#v", got)
	}
}

func TestMergeNodeAddResult(t *testing.T) {
	result := NodeAddResult{Mode: "async"}
	mergeNodeAddResult(&result, NodeAddResult{Added: 1, Items: []NodeAddItem{{Status: NodeAddItemAdded, Name: "safe", Protocol: "ss"}}})
	mergeNodeAddResult(&result, NodeAddResult{Skipped: 1, Items: []NodeAddItem{{Status: NodeAddItemSkipped, Name: "duplicate", Reason: NodeAddReasonDuplicate}}})
	if result.Added != 1 || result.Skipped != 1 || len(result.Items) != 2 {
		t.Fatalf("merged task result = %#v", result)
	}
}

// setupNodeUpdateAPITestDB prepares an isolated database and caches for node update handler tests.
func setupNodeUpdateAPITestDB(t *testing.T) {
	t.Helper()

	oldDB := database.DB
	oldDialect := database.Dialect
	oldInitialized := database.IsInitialized
	db := testutil.OpenMemoryDB(t, "node_update_api_test")
	if err := db.AutoMigrate(&models.Node{}, &models.SystemSetting{}, &models.Task{}); err != nil {
		t.Fatalf("auto migrate node update tables: %v", err)
	}

	database.DB = db
	database.Dialect = database.DialectSQLite
	database.IsInitialized = false
	if err := models.InitNodeCache(); err != nil {
		t.Fatalf("init node cache: %v", err)
	}
	if err := models.InitSettingCache(); err != nil {
		t.Fatalf("init setting cache: %v", err)
	}
	if err := models.InitTaskCache(); err != nil {
		t.Fatalf("init task cache: %v", err)
	}

	t.Cleanup(func() {
		database.DB = oldDB
		database.Dialect = oldDialect
		database.IsInitialized = oldInitialized
		if oldDB != nil {
			_ = models.InitNodeCache()
			_ = models.InitSettingCache()
		}
		testutil.CloseDB(t, db)
	})
}

func TestNodeAddAsyncReturnsTaskAndPersistsResult(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	link := protocol.EncodeSSURL(protocol.Ss{
		Name:   "async-node",
		Server: "edge.example.com",
		Port:   8388,
		Param:  protocol.Param{Cipher: "aes-256-gcm", Password: "password"},
	})
	form := url.Values{"link": {link}, "group": {"async-test"}}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/nodes/add-async", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	writer := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(writer)
	ctx.Request = req

	NodeAddAsync(ctx)
	if writer.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", writer.Code)
	}
	var response struct {
		Code int `json:"code"`
		Data struct {
			TaskID string `json:"taskId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode async response: %v", err)
	}
	if response.Code != 200 || response.Data.TaskID == "" {
		t.Fatalf("unexpected async response: %s", writer.Body.String())
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		var task models.Task
		if err := task.GetByID(response.Data.TaskID); err == nil && task.Status == models.TaskStatusCompleted {
			var result NodeAddResult
			if err := json.Unmarshal([]byte(task.Result), &result); err != nil {
				t.Fatalf("decode task result: %v", err)
			}
			if result.Added != 1 || result.Status != NodeAddStatusSuccess {
				t.Fatalf("unexpected task result: %#v", result)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("async task %s did not complete", response.Data.TaskID)
		}
		time.Sleep(20 * time.Millisecond)
	}

	var nodes []models.Node
	if err := database.DB.Find(&nodes).Error; err != nil {
		t.Fatalf("list async nodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Group != "async-test" {
		t.Fatalf("unexpected async nodes: %#v", nodes)
	}
}

// TestNodeUpdatePreservesClashExtraWhenLinkUnchanged verifies that metadata-only
// edits keep retained Clash fields, while replacing the link clears stale fields.
func TestNodeUpdateAllowsChangingDialerForSameLink(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	link := protocol.EncodeSSURL(protocol.Ss{
		Name:   "same-target",
		Server: "edge.example.com",
		Port:   443,
		Param:  protocol.Param{Cipher: "aes-256-gcm", Password: "password"},
	})
	proxy, err := protocol.LinkToProxy(protocol.Urls{Url: link}, protocol.OutputConfig{})
	if err != nil {
		t.Fatalf("parse test link: %v", err)
	}
	contentHash := protocol.GenerateProxyContentHash(proxy)
	first := models.Node{Name: "target-a", LinkName: "same-target", NameMode: models.NodeNameModeLink, Link: link, ContentHash: contentHash, DialerProxyName: "front-a", Protocol: "ss"}
	if err := first.Add(); err != nil {
		t.Fatalf("add first node: %v", err)
	}
	second := models.Node{Name: "target-b", LinkName: "same-target", NameMode: models.NodeNameModeLink, Link: link, ContentHash: contentHash, DialerProxyName: "front-b", Protocol: "ss"}
	if err := second.Add(); err != nil {
		t.Fatalf("add second node: %v", err)
	}

	form := url.Values{
		"remark":             {"来源备注测试"},
		"oldname":            {first.Name},
		"oldlink":            {link},
		"oldDialerProxyName": {first.DialerProxyName},
		"link":               {link},
		"name":               {first.Name},
		"nameMode":           {models.NodeNameModeLink},
		"dialerProxyName":    {"front-c"},
		"skipTLSVerify":      {"true"},
		"group":              {""},
	}
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/nodes/update", strings.NewReader(form.Encode()))
	ginContext.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	NodeUpdadte(ginContext)
	if recorder.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var stored models.Node
	if err := database.DB.First(&stored, first.ID).Error; err != nil {
		t.Fatalf("reload updated node: %v", err)
	}
	if stored.DialerProxyName != "front-c" {
		t.Fatalf("dialer proxy = %q, want front-c", stored.DialerProxyName)
	}
	if stored.Remark != "来源备注测试" {
		t.Fatalf("remark = %q, want 来源备注测试", stored.Remark)
	}
	if !stored.SkipTLSVerify {
		t.Fatal("skip TLS verify = false, want true")
	}
}

func TestNodeUpdatePreservesClashExtraWhenLinkUnchanged(t *testing.T) {
	for _, tt := range []struct {
		name        string
		updatedLink func(original string) string
		wantExtra   string
	}{
		{
			name:        "metadata-only edit",
			updatedLink: func(original string) string { return original },
			wantExtra:   "smux:\n  enabled: true\n",
		},
		{
			name: "link replacement",
			updatedLink: func(string) string {
				return protocol.EncodeSSURL(protocol.Ss{
					Name:   "replacement-node",
					Server: "replacement.example.com",
					Port:   8443,
					Param: protocol.Param{
						Cipher:   "aes-256-gcm",
						Password: "password",
					},
				})
			},
			wantExtra: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setupNodeUpdateAPITestDB(t)
			originalLink := protocol.EncodeSSURL(protocol.Ss{
				Name:   "source-node",
				Server: "edge.example.com",
				Port:   443,
				Param: protocol.Param{
					Cipher:   "aes-256-gcm",
					Password: "password",
				},
			})
			node := models.Node{
				Name:       "source-node",
				LinkName:   "source-node",
				NameMode:   models.NodeNameModeLink,
				Link:       originalLink,
				ClashExtra: "smux:\n  enabled: true\n",
				Protocol:   "ss",
			}
			if err := node.Add(); err != nil {
				t.Fatalf("add test node: %v", err)
			}

			updatedLink := tt.updatedLink(originalLink)
			form := url.Values{
				"oldname":         {node.Name},
				"oldlink":         {originalLink},
				"link":            {updatedLink},
				"name":            {"renamed-node"},
				"nameMode":        {models.NodeNameModeRemark},
				"dialerProxyName": {""},
				"group":           {"updated-group"},
			}
			recorder := httptest.NewRecorder()
			ginContext, _ := gin.CreateTestContext(recorder)
			ginContext.Request = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/nodes/update", strings.NewReader(form.Encode()))
			ginContext.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			NodeUpdadte(ginContext)
			if recorder.Code != http.StatusOK {
				t.Fatalf("update status = %d, body = %s", recorder.Code, recorder.Body.String())
			}

			var stored models.Node
			if err := database.DB.First(&stored, node.ID).Error; err != nil {
				t.Fatalf("reload updated node: %v", err)
			}
			if stored.ClashExtra != tt.wantExtra {
				t.Fatalf("ClashExtra = %q, want %q", stored.ClashExtra, tt.wantExtra)
			}
		})
	}
}
