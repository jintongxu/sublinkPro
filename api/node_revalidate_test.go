package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sublink/database"
	"sublink/models"
	"sublink/node/protocol"
	"sublink/services"
	"sublink/services/sse"
	"sublink/utils"
	"testing"
	"time"
)

func revalidationSelection(t *testing.T, text string) map[string]json.RawMessage {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}
func addRevalidationNode(t *testing.T, name, link string) models.Node {
	t.Helper()
	n := models.Node{Name: name, Link: link}
	if err := n.Add(); err != nil {
		t.Fatal(err)
	}
	return n
}
func waitRevalidationTask(t *testing.T, id string) models.Task {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var task models.Task
		if err := task.GetByID(id); err == nil && (task.Status == models.TaskStatusCompleted || task.Status == models.TaskStatusError || task.Status == models.TaskStatusCancelled) {
			return task
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("task timeout")
	return models.Task{}
}
func TestNodeVMessWithoutTransportHostImportAndRevalidate(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	link := protocol.EncodeVmessURL(protocol.Vmess{Add: "edge.example.com", Port: 443, Id: "12345678-1234-1234-1234-123456789abc", Net: "tcp", Ps: "vmess-no-host"})
	identity, _, err := ValidateNodeLink(protocol.Urls{Url: link})
	if err != nil || identity.Host != "edge.example.com" {
		t.Fatalf("VMess validation: %+v, %v", identity, err)
	}
	result := processNodeAdd(context.Background(), nodeAddInput{Link: link})
	if result.Added != 1 || result.Invalid != 0 {
		t.Fatalf("VMess import: %+v", result)
	}
	n, exists := models.GetNodeByID(result.Items[0].NodeID)
	if !exists {
		t.Fatal("imported VMess node missing")
	}
	status, reason := validateExistingNode(context.Background(), *n)
	if status != models.ValidationStatusSuccess || n.LinkHost != "edge.example.com" {
		t.Fatalf("VMess revalidation: %s %s, node=%+v", status, reason, n)
	}
}

func TestNodeRevalidationSelection(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	n := addRevalidationNode(t, "valid", "trojan://secret@example.com:443#valid")
	for _, text := range []string{`{}`, `{"ids":[]}`, `{"ids":[0]}`, `{"ids":[-1]}`, `{"ids":[999]}`, `{"ids":[1],"all":false}`, `{"ids":[1],"filters":{}}`, `{"all":true}`, `{"all":false,"filters":{}}`, `{"all":true,"filters":null}`, `{"all":true,"filters":{"validationStatus":"bogus"}}`, `{"all":true,"filters":{"typo":"x"}}`, `{"all":true,"filters":{"group":"missing"}}`} {
		if _, err := resolveNodeRevalidationTargets(revalidationSelection(t, text)); err == nil {
			t.Errorf("accepted %s", text)
		}
	}
	nodes, err := resolveNodeRevalidationTargets(revalidationSelection(t, `{"ids":[1,1]}`))
	if err != nil || len(nodes) != 1 || nodes[0].ID != n.ID {
		t.Fatal(nodes, err)
	}
	nodes, err = resolveNodeRevalidationTargets(revalidationSelection(t, `{"all":true,"filters":{"validationStatus":"untested","search":"valid"}}`))
	if err != nil || len(nodes) != 1 {
		t.Fatal(nodes, err)
	}
	addRevalidationNode(t, "later", "trojan://secret@later.example.com:443#later")
	if len(nodes) != 1 {
		t.Fatal("target set changed")
	}
}

func TestNodeRevalidationRemoteIsReadOnly(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	original := subscriptionFetch
	t.Cleanup(func() { subscriptionFetch = original })
	for _, tc := range []struct{ name, payload, status string }{
		{"all-valid", "trojan://secret@example.com:443#valid", models.ValidationStatusSuccess},
		{"mixed", "trojan://secret@example.com:443#valid\nnot a node", models.ValidationStatusPartial},
		{"none", "not a node", models.ValidationStatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subscriptionFetch = func(req *http.Request, client *http.Client) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.payload))}, nil
			}
			n := addRevalidationNode(t, tc.name, "https://example.com/sub?token="+tc.name)
			status, reason := validateExistingNode(context.Background(), n)
			if status != tc.status {
				t.Fatalf("%s %s", status, reason)
			}
			var count int64
			database.DB.Model(&models.Node{}).Count(&count)
			if count != int64(n.ID) {
				t.Fatal("validation inserted nodes")
			}
			var persisted models.Node
			database.DB.First(&persisted, n.ID)
			if persisted.ValidationStatus != models.ValidationStatusUntested || persisted.Link != n.Link {
				t.Fatal("pure validation mutated node")
			}
		})
	}
}

func TestNodeRevalidationTaskResultAndNoDuplicates(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	good := addRevalidationNode(t, "good", "trojan://secret@example.com:443#good")
	bad := addRevalidationNode(t, "bad", "ss://invalid")
	response := performJSONRequest(t, NodeRevalidate, http.MethodPost, map[string]any{"ids": []int{good.ID, bad.ID}})
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			TaskID string `json:"taskId"`
			Total  int    `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Data.TaskID == "" || envelope.Data.Total != 2 {
		t.Fatal(response.Body.String(), err)
	}
	task := waitRevalidationTask(t, envelope.Data.TaskID)
	if task.Status != models.TaskStatusCompleted || task.Progress != 2 || !strings.Contains(task.Result, `"status":"partial"`) {
		t.Fatalf("%+v", task)
	}
	var count int64
	database.DB.Model(&models.Node{}).Count(&count)
	if count != 2 {
		t.Fatal("inserted nodes", count)
	}
	got, _ := models.GetNodeByID(good.ID)
	if got.ValidationStatus != models.ValidationStatusSuccess {
		t.Fatal(got)
	}
	got, _ = models.GetNodeByID(bad.ID)
	if got.ValidationStatus != models.ValidationStatusFailed {
		t.Fatal(got)
	}
	// Every result is safe and bounded; input link/credentials never enter details.
	if strings.Contains(task.Result, "secret") || strings.Contains(task.Result, "trojan://") {
		t.Fatal(task.Result)
	}
}

func TestNodeRevalidationCancelDuringFetch(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	n := addRevalidationNode(t, "remote", "https://example.com/sub?token=secret")
	original := subscriptionFetch
	t.Cleanup(func() { subscriptionFetch = original })
	entered := make(chan struct{})
	subscriptionFetch = func(req *http.Request, client *http.Client) (*http.Response, error) {
		close(entered)
		<-req.Context().Done()
		return nil, req.Context().Err()
	}
	snapshots, err := models.NodeValidationSnapshots([]int{n.ID})
	if err != nil {
		t.Fatal(err)
	}
	tm := services.GetTaskManager()
	task, ctx, err := tm.CreateTask(models.TaskTypeNodeRevalidate, "cancel", models.TaskTriggerManual, 1)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); runNodeRevalidationTask(task.ID, ctx, snapshots) }()
	<-entered
	if err := tm.CancelTask(task.ID); err != nil {
		t.Fatal(err)
	}
	<-done
	persisted := waitRevalidationTask(t, task.ID)
	if persisted.Status != models.TaskStatusCancelled {
		t.Fatal(persisted)
	}
	got, _ := models.GetNodeByID(n.ID)
	if got.ValidationStatus != models.ValidationStatusUntested {
		t.Fatal("cancel wrote failure", got)
	}
}

func TestNodeRevalidationQueryRejectsUnknownStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, handler := range []gin.HandlerFunc{NodeGet, NodeGetIDs, NodeSelector} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/?validationStatus=typo", nil)
		handler(c)
		if !strings.Contains(w.Body.String(), "unknown validation status") {
			t.Fatal(w.Body.String())
		}
	}
}

func TestNodeRevalidationBoundedFullCounters(t *testing.T) {
	// Production starts SSEBroker.Listen; this isolated task test drains the
	// bounded notifier too, avoiding an artificial full-channel producer block.
	stop, drained := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(drained)
		for {
			select {
			case <-sse.GetSSEBroker().Notifier:
			case <-stop:
				return
			}
		}
	}()
	defer func() { close(stop); <-drained }()

	setupNodeUpdateAPITestDB(t)
	var ids []int
	for i := 0; i < 205; i++ {
		n := addRevalidationNode(t, fmt.Sprintf("bad-%d", i), fmt.Sprintf("invalid://%d", i))
		ids = append(ids, n.ID)
	}
	snapshots, err := models.NodeValidationSnapshots(ids)
	if err != nil {
		t.Fatal(err)
	}
	task, ctx, err := services.GetTaskManager().CreateTask(models.TaskTypeNodeRevalidate, "bounds", models.TaskTriggerManual, len(snapshots))
	if err != nil {
		t.Fatal(err)
	}
	runNodeRevalidationTask(task.ID, ctx, snapshots)
	persisted := waitRevalidationTask(t, task.ID)
	var result NodeRevalidationResult
	if err := json.Unmarshal([]byte(persisted.Result), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 205 || result.Processed != 205 || result.Failed != 205 || len(result.Items) != 200 || len(result.Diagnostics) != 32 || result.OmittedItems != 5 || result.OmittedDiagnostics != 173 || !result.Truncated || result.Validated != 0 {
		t.Fatalf("%+v", result)
	}
}

func TestNodeRevalidationStaleTargetsAreSkipped(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	n := addRevalidationNode(t, "stale", "trojan://secret@example.com:443#stale")
	snapshots, err := models.NodeValidationSnapshots([]int{n.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.UpdateNodeFields(n.ID, map[string]any{"skip_tls_verify": true}); err != nil {
		t.Fatal(err)
	}
	task, ctx, err := services.GetTaskManager().CreateTask(models.TaskTypeNodeRevalidate, "stale", models.TaskTriggerManual, 1)
	if err != nil {
		t.Fatal(err)
	}
	runNodeRevalidationTask(task.ID, ctx, snapshots)
	persisted := waitRevalidationTask(t, task.ID)
	var result NodeRevalidationResult
	if err := json.Unmarshal([]byte(persisted.Result), &result); err != nil {
		t.Fatal(err)
	}
	if result.Skipped != 1 || result.Validated != 0 {
		t.Fatal(persisted.Result)
	}
	got, _ := models.GetNodeByID(n.ID)
	if got.ValidationStatus != models.ValidationStatusUntested {
		t.Fatal(got)
	}
}

func TestNodeRevalidationRawEditDoesNotReplayCache(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	n := addRevalidationNode(t, "raw", "trojan://secret@example.com:443#raw")
	if ok, err := models.ApplyNodeValidationSnapshot(n, models.ValidationStatusSuccess, "validated", "now"); err != nil || !ok {
		t.Fatal(err)
	}
	response := performJSONRequest(t, UpdateNodeRawInfo, http.MethodPost, map[string]any{"nodeId": n.ID, "fields": map[string]any{"Hostname": "changed.example.com"}})
	got, _ := models.GetNodeByID(n.ID)
	if got.ValidationStatus != models.ValidationStatusUntested || got.ValidationReason != "" || got.ValidationCheckAt != "" {
		t.Fatal(response.Body.String(), got)
	}
}

func TestNodeRevalidationUnlockFilterJSONCompatibility(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	n := addRevalidationNode(t, "unlock", "trojan://secret@example.com:443#unlock")
	if err := models.UpdateNodeFields(n.ID, map[string]any{"unlock_summary": `{"netflix":{"status":"unlocked"}}`}); err != nil {
		t.Fatal(err)
	}
	// Empty list is encoded as a JSON string by the existing frontend; both
	// serialized and native array forms use the same decoder as list endpoints.
	for _, rules := range []any{`[]`, []any{}} {
		body, _ := json.Marshal(map[string]any{"all": true, "filters": map[string]any{"unlockRules": rules, "validationStatus": "untested"}})
		nodes, err := resolveNodeRevalidationTargets(revalidationSelection(t, string(body)))
		if err != nil || len(nodes) != 1 {
			t.Fatal(nodes, err)
		}
	}
}

func TestNodeRevalidationRemoteFetchAndNestedRemoteFailure(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	original := subscriptionFetch
	t.Cleanup(func() { subscriptionFetch = original })
	calls := 0
	subscriptionFetch = func(req *http.Request, client *http.Client) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("https://nested.example.com/secret"))}, nil
	}
	status, _ := validateExistingNode(context.Background(), models.Node{Link: "https://example.com/sub"})
	if status == "" || calls != 1 {
		t.Fatal(status, calls)
	}
	subscriptionFetch = func(req *http.Request, client *http.Client) (*http.Response, error) {
		return nil, fmt.Errorf("failed token=secret https://example.com/sub?token=hidden")
	}
	status, reason := validateExistingNode(context.Background(), models.Node{Link: "https://example.com/sub"})
	if status != models.ValidationStatusFailed || strings.Contains(reason, "secret") || strings.Contains(reason, "hidden") {
		t.Fatal(status, reason)
	}
	// Ordinary HTTP proxy identity is structural and must not be fetched.
	status, _ = validateExistingNode(context.Background(), models.Node{Link: "http://user:password@example.com:8080#proxy"})
	if status != models.ValidationStatusSuccess {
		t.Fatal(status)
	}
}

func TestNodeRevalidationEndpointStrictSelection(t *testing.T) {
	setupNodeUpdateAPITestDB(t)
	addRevalidationNode(t, "one", "trojan://secret@example.com:443#one")
	for _, body := range []string{`{"ids":[]}`, `{"ids":[1],"all":true,"filters":{}}`, `{"ids":[999]}`, `{"all":true,"filters":{"validationStatus":"typo"}}`} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		NodeRevalidate(c)
		if strings.Contains(w.Body.String(), "taskId") {
			t.Fatal("created task for invalid selection", w.Body.String())
		}
	}
}

func TestNodeRevalidationVMessWithoutTransportHost(t *testing.T) {
	link := protocol.EncodeVmessURL(protocol.Vmess{Add: "example.com", Port: "443", Id: "88888888-9999-7777-5555-777777777777", Net: "tcp", V: "2"})
	status, reason := validateExistingNode(context.Background(), models.Node{Link: link})
	if status != models.ValidationStatusSuccess {
		t.Fatalf("status=%s reason=%s", status, reason)
	}
}

func TestNodeRevalidationRemoteMalformedSSRPreservesNeighbors(t *testing.T) {
	original := subscriptionFetch
	t.Cleanup(func() { subscriptionFetch = original })
	broken := "ssr://" + utils.Base64Encode("example.com:8388:origin:aes-256-cfb:plain:c2VjcmV0/?broken")
	payload := "trojan://secret@one.example.com:443#one\n" + broken + "\ntrojan://secret@two.example.com:443#two"
	subscriptionFetch = func(req *http.Request, client *http.Client) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload))}, nil
	}
	status, reason := validateExistingNode(context.Background(), models.Node{Link: "https://example.com/sub"})
	if status != models.ValidationStatusPartial || reason != "mixed_subscription_entries" {
		t.Fatalf("status=%s reason=%s", status, reason)
	}
}
