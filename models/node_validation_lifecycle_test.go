package models

import (
	"fmt"
	"gorm.io/gorm"
	"sublink/database"
	"sync"
	"testing"
)

func validatedLifecycleNode(t *testing.T) Node {
	t.Helper()
	n := Node{Name: "lifecycle", Link: "trojan://secret@example.com:443#life", ValidationStatus: ValidationStatusSuccess, ValidationReason: "validated", ValidationCheckAt: "2026-01-01T00:00:00Z"}
	if err := n.Add(); err != nil {
		t.Fatal(err)
	}
	return n
}
func assertLifecycleState(t *testing.T, id int, status string) Node {
	t.Helper()
	var persisted Node
	if err := database.DB.First(&persisted, id).Error; err != nil {
		t.Fatal(err)
	}
	cached, ok := GetNodeByID(id)
	if !ok || persisted.ValidationStatus != status || cached.ValidationStatus != status || persisted.ValidationReason != cached.ValidationReason || persisted.ValidationCheckAt != cached.ValidationCheckAt {
		t.Fatalf("DB/cache mismatch: DB=%+v cache=%+v", persisted, cached)
	}
	if status == ValidationStatusUntested && (persisted.ValidationReason != "" || persisted.ValidationCheckAt != "") {
		t.Fatal("reset retained metadata")
	}
	return persisted
}

func TestNodeValidationLifecycleMutations(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Node) error
		reset  bool
	}{
		{"update-link", func(n *Node) error { n.Link += "x"; return n.Update() }, true},
		{"update-extra", func(n *Node) error { n.ClashExtra = "{}"; return n.Update() }, true},
		{"update-tls", func(n *Node) error { n.SkipTLSVerify = true; return n.Update() }, true},
		{"update-dialer", func(n *Node) error { n.DialerProxyName = "proxy"; return n.Update() }, true},
		{"update-metadata", func(n *Node) error { n.Name = "renamed"; n.Remark = "note"; n.Group = "g"; return n.Update() }, false},
		{"update-unchanged", func(n *Node) error { return n.Update() }, false},
		{"fields-link", func(n *Node) error { return UpdateNodeFields(n.ID, map[string]any{"link": n.Link + "x"}) }, true},
		{"fields-extra", func(n *Node) error { return UpdateNodeFields(n.ID, map[string]any{"ClashExtra": "{}"}) }, true},
		{"fields-tls", func(n *Node) error { return UpdateNodeFields(n.ID, map[string]any{"skip_tls_verify": true}) }, true},
		{"fields-dialer", func(n *Node) error { return UpdateNodeFields(n.ID, map[string]any{"dialer_proxy_name": "proxy"}) }, true},
		{"fields-noop", func(n *Node) error {
			return UpdateNodeFields(n.ID, map[string]any{"link": n.Link, "skip_tls_verify": false})
		}, false},
		{"fields-metadata", func(n *Node) error { return UpdateNodeFields(n.ID, map[string]any{"remark": "note", "tags": "a"}) }, false},
		{"batch-dialer", func(n *Node) error { return BatchUpdateDialerProxy([]int{n.ID}, "proxy") }, true},
		{"batch-dialer-noop", func(n *Node) error { return BatchUpdateDialerProxy([]int{n.ID}, "") }, false},
		{"batch-info-link", func(n *Node) error {
			u := BuildNodeInfoUpdate(*n, n.LinkName, n.Link+"x", 1)
			_, e := BatchUpdateNodeInfo([]NodeInfoUpdate{u})
			return e
		}, true},
		{"batch-info-extra", func(n *Node) error {
			u := BuildNodeInfoUpdate(*n, n.LinkName, n.Link, 1)
			u.ClashExtra = "{}"
			_, e := BatchUpdateNodeInfo([]NodeInfoUpdate{u})
			return e
		}, true},
		{"batch-info-noop", func(n *Node) error {
			u := BuildNodeInfoUpdate(*n, n.LinkName, n.Link, 1)
			_, e := BatchUpdateNodeInfo([]NodeInfoUpdate{u})
			return e
		}, false},
		{"fallback-link", func(n *Node) error {
			u := BuildNodeInfoUpdate(*n, n.LinkName, n.Link+"x", 1)
			if fallbackToIndividualNodeInfoUpdate([]NodeInfoUpdate{u}) != 1 {
				t.Fatal("fallback failed")
			}
			return nil
		}, true},
		{"fallback-noop", func(n *Node) error {
			u := BuildNodeInfoUpdate(*n, n.LinkName, n.Link, 1)
			if fallbackToIndividualNodeInfoUpdate([]NodeInfoUpdate{u}) != 1 {
				t.Fatal("fallback failed")
			}
			return nil
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupValidationTestDB(t, true)
			n := validatedLifecycleNode(t)
			if err := tc.change(&n); err != nil {
				t.Fatal(err)
			}
			want := ValidationStatusSuccess
			if tc.reset {
				want = ValidationStatusUntested
			}
			assertLifecycleState(t, n.ID, want)
		})
	}
}

func TestNodeValidationSnapshotInterleaving(t *testing.T) {
	for _, scenario := range []string{"validation-then-edit", "edit-then-validation", "edit-revert", "delete-then-validation", "two-results", "missing-cache"} {
		t.Run(scenario, func(t *testing.T) {
			db := setupValidationTestDB(t, true)
			n := validatedLifecycleNode(t)
			snapshots, err := NodeValidationSnapshots([]int{n.ID})
			if err != nil {
				t.Fatal(err)
			}
			snapshot := snapshots[0]
			apply := func() bool {
				t.Helper()
				ok, e := ApplyNodeValidationSnapshot(snapshot, ValidationStatusFailed, "invalid_node_structure", "now")
				if e != nil {
					t.Fatal(e)
				}
				return ok
			}
			switch scenario {
			case "validation-then-edit":
				if !apply() {
					t.Fatal("not applied")
				}
				n.Link += "x"
				if err := n.Update(); err != nil {
					t.Fatal(err)
				}
				assertLifecycleState(t, n.ID, ValidationStatusUntested)
			case "edit-then-validation":
				if err := UpdateNodeFields(n.ID, map[string]any{"skip_tls_verify": true}); err != nil {
					t.Fatal(err)
				}
				if apply() {
					t.Fatal("stale applied")
				}
				assertLifecycleState(t, n.ID, ValidationStatusUntested)
			case "edit-revert":
				original := n.Link
				n.Link += "x"
				if err := n.Update(); err != nil {
					t.Fatal(err)
				}
				n.Link = original
				if err := n.Update(); err != nil {
					t.Fatal(err)
				}
				if apply() {
					t.Fatal("ABA stale applied")
				}
			case "delete-then-validation":
				if err := db.Delete(&Node{}, n.ID).Error; err != nil {
					t.Fatal(err)
				}
				nodeCache.Delete(n.ID)
				if apply() {
					t.Fatal("deleted applied")
				}
				if _, ok := GetNodeByID(n.ID); ok {
					t.Fatal("resurrected cache")
				}
			case "two-results":
				if !apply() {
					t.Fatal("not applied")
				}
				if ok, err := ApplyNodeValidationSnapshot(snapshot, ValidationStatusSuccess, "validated", "later"); err != nil || ok {
					t.Fatal("older overlapping snapshot was accepted", ok, err)
				}
				assertLifecycleState(t, n.ID, ValidationStatusFailed)
			case "missing-cache":
				nodeCache.Delete(n.ID)
				if !apply() {
					t.Fatal("not applied")
				}
				assertLifecycleState(t, n.ID, ValidationStatusFailed)
			}
		})
	}
}

func TestNodeValidationSnapshotPersistenceFailure(t *testing.T) {
	db := setupValidationTestDB(t, true)
	n := validatedLifecycleNode(t)
	if err := db.Callback().Update().Before("gorm:update").Register("validation_fail", func(tx *gorm.DB) { _ = tx.AddError(gorm.ErrInvalidDB) }); err != nil {
		t.Fatal(err)
	}
	ok, err := ApplyNodeValidationSnapshot(n, ValidationStatusFailed, "invalid", "now")
	if err == nil || ok {
		t.Fatal("expected failure")
	}
	assertLifecycleState(t, n.ID, ValidationStatusSuccess)
}

func TestNodeValidationFilterAllConsumers(t *testing.T) {
	setupValidationTestDB(t, true)
	n := validatedLifecycleNode(t)
	for _, status := range []string{ValidationStatusSuccess, ValidationStatusFailed} {
		f := NodeFilter{ValidationStatus: status}
		want := 0
		if status == ValidationStatusSuccess {
			want = 1
		}
		nodes, err := new(Node).ListWithFilters(f)
		if err != nil || len(nodes) != want {
			t.Fatal(nodes, err)
		}
		ids, err := new(Node).GetFilteredNodeIDs(f)
		if err != nil || len(ids) != want {
			t.Fatal(ids, err)
		}
		paged, count, err := new(Node).ListWithFiltersPaginated(f, 1, 10)
		if err != nil || int(count) != want || len(paged) != want {
			t.Fatal(count, err)
		}
	}
	if err := UpdateNodeFields(n.ID, map[string]any{"link": n.Link + "x"}); err != nil {
		t.Fatal(err)
	}
	ids, _ := new(Node).GetFilteredNodeIDs(NodeFilter{ValidationStatus: ValidationStatusUntested})
	if len(ids) != 1 {
		t.Fatal(ids)
	}
}

func TestNodeValidationConcurrentEditAndDelete(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(fmt.Sprint(deleted), func(t *testing.T) {
			db := setupValidationTestDB(t, true)
			if err := db.Exec("CREATE TABLE subcription_nodes (subcription_id integer, node_id integer)").Error; err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 15; i++ {
				n := Node{Name: fmt.Sprintf("concurrent-%d", i), Link: fmt.Sprintf("trojan://secret@example.com:%d", 443+i), ValidationStatus: ValidationStatusSuccess}
				if err := n.Add(); err != nil {
					t.Fatal(err)
				}
				start := make(chan struct{})
				errors := make(chan error, 2)
				var group sync.WaitGroup
				group.Add(2)
				go func() {
					defer group.Done()
					<-start
					_, err := ApplyNodeValidationSnapshot(n, ValidationStatusFailed, "invalid", "now")
					errors <- err
				}()
				go func() {
					defer group.Done()
					<-start
					if deleted {
						errors <- n.Del()
					} else {
						errors <- UpdateNodeFields(n.ID, map[string]any{"skip_tls_verify": true})
					}
				}()
				close(start)
				group.Wait()
				close(errors)
				for err := range errors {
					if err != nil {
						t.Fatal(err)
					}
				}
				if deleted {
					var count int64
					db.Model(&Node{}).Where("id = ?", n.ID).Count(&count)
					if count != 0 {
						t.Fatal("row resurrected")
					}
					if _, ok := GetNodeByID(n.ID); ok {
						t.Fatal("cache resurrected")
					}
				} else {
					assertLifecycleState(t, n.ID, ValidationStatusUntested)
				}
			}
		})
	}
}

func TestNodeValidationMetadataCannotReplayStaleResult(t *testing.T) {
	setupValidationTestDB(t, true)
	n := validatedLifecycleNode(t)
	stale := n
	if ok, err := ApplyNodeValidationSnapshot(n, ValidationStatusFailed, "invalid", "now"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	stale.Remark = "changed note"
	if err := stale.Update(); err != nil {
		t.Fatal(err)
	}
	assertLifecycleState(t, n.ID, ValidationStatusFailed)
	// Legacy external cache injection also reloads persisted truth.
	UpdateNodeCache(n.ID, n)
	assertLifecycleState(t, n.ID, ValidationStatusFailed)
}

func TestNodeValidationBatchMissingCacheAndFallback(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprint(fallback), func(t *testing.T) {
			setupValidationTestDB(t, true)
			n := validatedLifecycleNode(t)
			nodeCache.Delete(n.ID)
			u := BuildNodeInfoUpdate(n, n.LinkName, n.Link+"x", 1)
			if fallback {
				if fallbackToIndividualNodeInfoUpdate([]NodeInfoUpdate{u}) != 1 {
					t.Fatal("fallback")
				}
			} else {
				if _, err := BatchUpdateNodeInfo([]NodeInfoUpdate{u}); err != nil {
					t.Fatal(err)
				}
			}
			if err := n.GetByID(); err != nil {
				t.Fatal(err)
			}
			assertLifecycleState(t, n.ID, ValidationStatusUntested)
		})
	}
}
