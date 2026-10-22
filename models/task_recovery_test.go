package models

import (
	"sublink/database"
	"sublink/internal/testutil"
	"testing"
)

func TestTaskRestartReconcilesDatabaseAndCache(t *testing.T) {
	old := database.DB
	database.DB = testutil.OpenMemoryDB(t, "task_recovery")
	defer func() { testutil.CloseDB(t, database.DB); database.DB = old; taskCache.Clear() }()
	if err := database.DB.AutoMigrate(&Task{}); err != nil {
		t.Fatal(err)
	}
	for _, status := range []TaskStatus{TaskStatusPending, TaskStatusRunning, TaskStatusCompleted} {
		task := Task{ID: string(status), Status: status}
		if err := task.Create(); err != nil {
			t.Fatal(err)
		}
	}
	if err := MarkRunningTasksAsError(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"pending", "running"} {
		var cached, persisted Task
		if err := cached.GetByID(id); err != nil {
			t.Fatal(err)
		}
		if err := database.DB.First(&persisted, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if cached.Status != TaskStatusError || persisted.Status != cached.Status || cached.CompletedAt == nil || persisted.CompletedAt == nil {
			t.Fatalf("inconsistent restart: cache=%+v db=%+v", cached, persisted)
		}
	}
	listed, total, err := ListTasks(TaskFilter{Status: "error"}, 1, 20)
	if err != nil || total != 2 || len(listed) != 2 {
		t.Fatalf("list mismatch: %v %d %v", listed, total, err)
	}
	running, err := GetRunningTasks()
	if err != nil || len(running) != 0 {
		t.Fatalf("stale running cache: %v %v", running, err)
	}
	pending, err := GetPendingTasks()
	if err != nil || len(pending) != 0 {
		t.Fatalf("stale pending cache: %v %v", pending, err)
	}

	var completed Task
	if err := completed.GetByID("completed"); err != nil || completed.Status != TaskStatusCompleted {
		t.Fatalf("terminal modified: %+v %v", completed, err)
	}
}
