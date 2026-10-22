package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"sublink/database"
	"sublink/models"
	"sublink/node/protocol"
	"sublink/services/substore"
	"sublink/utils"
)

func TestSubscriptionValidationPolicyAPIForms(t *testing.T) {
	setupClientsAPITestDB(t)
	gin.SetMode(gin.TestMode)
	call := func(handler gin.HandlerFunc, values url.Values) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(values.Encode()))
		c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		handler(c)
		return rec
	}
	for i, policy := range []string{"all", "exclude_failed", "only_success"} {
		name := fmt.Sprintf("policy-%d", i)
		rec := call(SubAdd, url.Values{"name": {name}, "groups": {"fixture-group"}, "config": {`{}`}, "ValidationPolicy": {policy}})
		if rec.Code != 200 {
			t.Fatalf("add %s: %d %s", policy, rec.Code, rec.Body)
		}
		var sub models.Subcription
		sub.Name = name
		if err := sub.Find(); err != nil {
			t.Fatal(err)
		}
		if sub.ValidationPolicy != policy {
			t.Fatalf("add policy=%s", sub.ValidationPolicy)
		}
		rec = call(SubUpdate, url.Values{"name": {name}, "oldname": {name}, "groups": {"fixture-group"}, "config": {`{}`}, "ValidationPolicy": {"only_success"}})
		if rec.Code != 200 {
			t.Fatalf("update: %s", rec.Body)
		}
		cached, err := models.GetSubcriptionByID(sub.ID)
		if err != nil || cached.ValidationPolicy != "only_success" {
			t.Fatalf("cache=%+v err=%v", cached, err)
		}
	}
	for _, handler := range []gin.HandlerFunc{SubAdd, SubUpdate} {
		if rec := call(handler, url.Values{"name": {"bad"}, "ValidationPolicy": {"invalid"}}); rec.Code != 400 {
			t.Fatalf("unknown accepted: %s", rec.Body)
		}
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequestWithContext(context.Background(), "POST", "/", strings.NewReader(`{"ValidationPolicy":"invalid"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	PreviewSubscriptionNodes(c)
	if rec.Code != 400 {
		t.Fatalf("preview unknown accepted: %s", rec.Body)
	}
}

func policyFixture(t *testing.T, policy string) models.Subcription {
	t.Helper()
	sub := models.Subcription{Name: "policy", ValidationPolicy: policy, Config: `{"clash":"` + writeTestClashTemplate(t) + `","surge":"` + writeTestSurgeTemplateWithManagedConfig(t) + `"}`}
	if err := sub.Add(); err != nil {
		t.Fatal(err)
	}
	statuses := []string{models.ValidationStatusUntested, models.ValidationStatusSuccess, models.ValidationStatusPartial, models.ValidationStatusFailed, models.ValidationStatusDisabled}
	for i, status := range statuses {
		name := fmt.Sprintf("policy-node-%d", i)
		n := models.Node{Name: name, LinkName: name, Link: "ss://YWVzLTEyOC1nY206cGFzc0BleGFtcGxlLmNvbTo0NDM=#" + name, Protocol: "ss", ValidationStatus: status}
		if err := n.Add(); err != nil {
			t.Fatal(err)
		}
		sub.Nodes = append(sub.Nodes, n)
	}
	if err := sub.AddNode(); err != nil {
		t.Fatal(err)
	}
	// Add() has an existing true default; disable request usage refresh explicitly.
	sub.RefreshUsageOnRequest = false
	if err := sub.Update(); err != nil {
		t.Fatal(err)
	}
	return sub
}

func TestSubscriptionValidationPolicyPreviewRenderShare(t *testing.T) {
	for _, policy := range []string{"all", "exclude_failed", "only_success"} {
		t.Run(policy, func(t *testing.T) {
			setupClientsAPITestDB(t)
			sub := policyFixture(t, policy)
			want := 5
			if policy == "exclude_failed" {
				want = 4
			}
			if policy == "only_success" {
				want = 1
			}
			saved, err := previewSavedSubscription(sub.ID)
			if err != nil {
				t.Fatal(err)
			}
			ids := []int{}
			for _, n := range sub.Nodes {
				ids = append(ids, n.ID)
			}
			form, err := previewFormSubscription(PreviewRequest{NodeIDs: ids, ValidationPolicy: policy})
			if err != nil {
				t.Fatal(err)
			}
			if len(saved.Nodes) != want || len(form.Nodes) != want || saved.ValidationExcludedCount != 5-want || form.ValidationExcludedCount != 5-want {
				t.Fatalf("saved=%+v form=%+v", saved, form)
			}
			for _, target := range []string{"clash", "mihomo", "v2ray", "surge"} {
				prepared, ok := buildPreparedResponseFromSubscription(sub, target, 0)
				if !ok {
					t.Fatal("prepare failed")
				}
				if len(prepared.Subscription.Nodes) != want {
					t.Fatal("prepare policy mismatch")
				}
				rec := renderPolicyPrepared(t, prepared)
				var got int
				switch target {
				case "clash", "mihomo":
					got = len(clashProxyNamesFromBody(t, rec.Body.Bytes()))
				case "surge":
					got = len(surgeProxyNamesFromBody(rec.Body.String()))
				case "v2ray":
					body, err := base64.StdEncoding.DecodeString(rec.Body.String())
					if err != nil {
						t.Fatal(err)
					}
					got = len(strings.Fields(string(body)))
				}
				if got != want {
					t.Fatalf("%s got=%d want=%d body=%s", target, got, want, rec.Body)
				}
				if rec.Header().Get("X-Subscription-Validation-Excluded") != fmt.Sprint(5-want) {
					t.Fatal("missing count")
				}
				if want < 5 && !strings.Contains(rec.Header().Get("X-Subscription-Diagnostics"), "validation policy excluded") {
					t.Fatalf("missing %s diagnostics", target)
				}
			}
			share := models.SubscriptionShare{SubscriptionID: sub.ID, Token: "policy-share", Enabled: true}
			if err := share.Add(); err != nil {
				t.Fatal(err)
			}
			rec := performClientRequest(t, "GET", "/c/?token=policy-share&client=clash")
			if got := len(clashProxyNamesFromBody(t, rec.Body.Bytes())); got != want {
				t.Fatalf("share got=%d body=%s", got, rec.Body)
			}
		})
	}
}

func renderPolicyPrepared(t *testing.T, p preparedClientResponse) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequestWithContext(context.Background(), "GET", "/c/", nil)
	switch p.ClientType {
	case "v2ray":
		renderPreparedV2ray(c, p)
	case "surge":
		renderPreparedSurge(c, p)
	default:
		renderPreparedClash(c, p)
	}
	return rec
}

func TestSubscriptionValidationPolicyEmptySyntheticAndHead(t *testing.T) {
	setupClientsAPITestDB(t)
	sub := policyFixture(t, "only_success")
	if err := database.DB.Model(&models.Node{}).Where("1=1").Update("validation_status", "failed").Error; err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"clash", "mihomo", "v2ray", "surge"} {
		p, ok := buildPreparedResponseFromSubscription(sub, target, 0)
		if !ok {
			t.Fatal("prepare")
		}
		if len(p.Subscription.Nodes) != 0 {
			t.Fatal("fell back to all")
		}
		rec := renderPolicyPrepared(t, p)
		if strings.Contains(rec.Body.String(), "policy-node") {
			t.Fatal("empty fell back")
		}
		if rec.Header().Get("X-Subscription-Validation-Excluded") != "5" {
			t.Fatal("lost count")
		}
		p.Mode = clientResponseSyntheticFallback
		p.FallbackName = "policy-expired"
		synthetic := renderPolicyPrepared(t, p)
		if target == "v2ray" {
			body, _ := base64.StdEncoding.DecodeString(synthetic.Body.String())
			if !strings.Contains(string(body), "policy-expired") {
				t.Fatal("synthetic filtered")
			}
		} else if !strings.Contains(synthetic.Body.String(), "policy-expired") {
			t.Fatal("synthetic filtered")
		}
		head := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(head)
		c.Request = httptest.NewRequestWithContext(context.Background(), "HEAD", "/", nil)
		_, write := prepareRendererResponse(c, p)
		if write || head.Header().Get("X-Subscription-Validation-Excluded") != "5" {
			t.Fatal("HEAD diagnostics lost")
		}
	}
}

func TestSubscriptionValidationPolicyConvertedBridge(t *testing.T) {
	setupClientsAPITestDB(t)
	sub := policyFixture(t, "only_success")
	var source string
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Data string `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		source = body.Data
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"par_res":"converted"}}`))
	}))
	defer sidecar.Close()
	saveSubStoreSettings(t, sidecar.URL, substore.SupportedTargets())
	for _, target := range substore.SupportedTargets() {
		p, ok := buildPreparedResponseFromSubscription(sub, target, 0)
		if !ok {
			t.Fatal("prepare")
		}
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
		renderPreparedConvertedClient(c, p)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "converted") {
			t.Fatalf("conversion %s: %d %s", target, rec.Code, rec.Body)
		}
		if got := len(clashProxyNamesFromBody(t, []byte(source))); got != 1 {
			t.Fatalf("sidecar %s got %d", target, got)
		}
		if rec.Header().Get("X-Subscription-Validation-Excluded") != "4" {
			t.Fatal("converted lost diagnostic")
		}
	}
}

func TestSubscriptionValidationPolicyChainRender(t *testing.T) {
	setupClientsAPITestDB(t)
	sub := policyFixture(t, "exclude_failed")
	// Failed node #3 is a mandatory prerequisite of the successful node #1.
	rule := models.SubscriptionChainRule{SubscriptionID: sub.ID, Name: "mandatory", Enabled: true, ChainConfig: fmt.Sprintf(`[{"type":"specified_node","nodeId":%d}]`, sub.Nodes[3].ID), TargetConfig: fmt.Sprintf(`{"type":"specified_node","nodeId":%d}`, sub.Nodes[1].ID)}
	if err := rule.Add(); err != nil {
		t.Fatal(err)
	}
	p, ok := buildPreparedResponseFromSubscription(sub, "clash", 0)
	if !ok {
		t.Fatal("prepare")
	}
	if len(p.Subscription.Nodes) != 3 || p.Subscription.ValidationDependencyExcludedCount != 1 {
		t.Fatalf("chain mismatch: %+v", p.Subscription)
	}
	rec := renderPolicyPrepared(t, p)
	if strings.Contains(rec.Body.String(), "policy-node-1") || strings.Contains(rec.Body.String(), "policy-node-3") {
		t.Fatalf("chain leaked/weakened: %s", rec.Body)
	}
	if rec.Header().Get("X-Subscription-Validation-Dependency-Excluded") != "1" {
		t.Fatal("lost chain diagnostic")
	}
	preview, err := previewSavedSubscription(sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Nodes) != 3 || preview.ValidationDependencyExcludedCount != 1 {
		t.Fatal("preview chain mismatch")
	}
}

func TestSubscriptionValidationPolicyScriptsAgree(t *testing.T) {
	setupClientsAPITestDB(t)
	sub := policyFixture(t, "only_success")
	// Modify speed after filtering; preview must not apply pre-script filters twice.
	sub.MinSpeed = 1
	if err := sub.Update(); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Model(&models.Node{}).Where("id = ?", sub.Nodes[1].ID).Update("speed", 2).Error; err != nil {
		t.Fatal(err)
	}
	if err := models.InitNodeCache(); err != nil {
		t.Fatal(err)
	}
	script := models.Script{Name: "policy-script", Content: `function filterNode(nodes, clientType) { return nodes.map(function(n) { n.Speed=0; return n; }); }`}
	if err := script.Add(); err != nil {
		t.Fatal(err)
	}
	if err := sub.AddScripts([]int{script.ID}); err != nil {
		t.Fatal(err)
	}
	ids := []int{}
	for _, n := range sub.Nodes {
		ids = append(ids, n.ID)
	}
	form, err := previewFormSubscription(PreviewRequest{NodeIDs: ids, Scripts: []int{script.ID}, ValidationPolicy: "only_success", MinSpeed: 1})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := previewSavedSubscription(sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(form.Nodes) != 1 || len(saved.Nodes) != 1 || form.Nodes[0].Speed != 0 || saved.Nodes[0].Speed != 0 {
		t.Fatalf("form=%+v saved=%+v", form, saved)
	}
}

func TestSubscriptionValidationPolicyBoundedDiagnostics(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequestWithContext(context.Background(), "HEAD", "/", nil)
	p := preparedClientResponse{Subscription: models.Subcription{ValidationExcludedCount: 42, ValidationDependencyExcludedCount: 7}}
	_, write := prepareRendererResponse(c, p)
	if write {
		t.Fatal("HEAD body")
	}
	var d clientDiagnostics
	for i := 0; i < 20; i++ {
		d.add("safe remote parse diagnostic")
	}
	d.apply(c)
	if rec.Header().Get("X-Subscription-Diagnostics-Count") != "8" || len(strings.Split(rec.Header().Get("X-Subscription-Diagnostics"), " | ")) != 8 {
		t.Fatal("unbounded diagnostics")
	}
	if !strings.Contains(rec.Header().Get("X-Subscription-Diagnostics"), "excluded 42") || !strings.Contains(rec.Header().Get("X-Subscription-Diagnostics"), "excluded 7") {
		t.Fatal("policy diagnostics overwritten")
	}
}

func TestSubscriptionValidationPolicyRenamedDependencyRender(t *testing.T) {
	setupClientsAPITestDB(t)
	utils.SetProtocolLinkFuncs(protocol.GetProtocolLabelFromLink, protocol.RenameNodeLink)
	t.Cleanup(func() { utils.SetProtocolLinkFuncs(nil, nil) })
	sub := policyFixture(t, "all")
	sub.NodeNameRule = "renamed-$LinkName"
	entry, exit := sub.Nodes[3], sub.Nodes[1]
	for _, n := range []struct {
		id   int
		name string
	}{{entry.ID, "entry"}, {exit.ID, "exit"}} {
		if err := database.DB.Model(&models.Node{}).Where("id = ?", n.id).Updates(map[string]any{"name": n.name, "link_name": n.name}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := database.DB.Model(&models.Node{}).Where("id = ?", exit.ID).Update("dialer_proxy_name", "renamed-entry").Error; err != nil {
		t.Fatal(err)
	}
	if err := models.InitNodeCache(); err != nil {
		t.Fatal(err)
	}
	if err := sub.Update(); err != nil {
		t.Fatal(err)
	}
	// Establish that the renderer supports the stored final-name alias with all.
	p, ok := buildPreparedResponseFromSubscription(sub, "clash", 0)
	if !ok {
		t.Fatal("prepare all")
	}
	body := renderPolicyPrepared(t, p).Body.String()
	if !strings.Contains(body, "dialer-proxy: renamed-entry") || !strings.Contains(body, "name: renamed-exit") {
		t.Fatalf("final alias not rendered under all: %s", body)
	}
	sub.ValidationPolicy = "exclude_failed"
	if err := sub.Update(); err != nil {
		t.Fatal(err)
	}
	preview, err := previewSavedSubscription(sub.ID)
	if err != nil || len(preview.Nodes) != 3 || preview.ValidationExcludedCount != 1 || preview.ValidationDependencyExcludedCount != 1 {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	for _, target := range []string{"clash", "mihomo", "surge", "v2ray"} {
		p, ok := buildPreparedResponseFromSubscription(sub, target, 0)
		if !ok || len(p.Subscription.Nodes) != 3 || p.Subscription.ValidationDependencyExcludedCount != 1 {
			t.Fatalf("%s prepared=%+v ok=%v", target, p.Subscription, ok)
		}
		rec := renderPolicyPrepared(t, p)
		body := rec.Body.String()
		if target == "v2ray" {
			decoded, err := base64.StdEncoding.DecodeString(body)
			if err != nil {
				t.Fatal(err)
			}
			body = string(decoded)
		}
		if strings.Contains(body, "renamed-entry") || strings.Contains(body, "renamed-exit") {
			t.Fatalf("%s restored or weakened dependency: %s", target, body)
		}
		if rec.Header().Get("X-Subscription-Validation-Dependency-Excluded") != "1" {
			t.Fatalf("%s lost dependency diagnostic", target)
		}
	}
}
