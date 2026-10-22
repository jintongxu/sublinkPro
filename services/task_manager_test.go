package services

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"sublink/database"
	"sublink/internal/testutil"
	"sublink/models"
	"testing"
)

func TestTaskTerminalPersistenceFailure(t *testing.T) {
	for _, operation := range []string{"complete", "fail", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			old := database.DB
			database.DB = testutil.OpenMemoryDB(t, "task_terminal")
			defer func() { testutil.CloseDB(t, database.DB); database.DB = old }()
			if err := database.DB.AutoMigrate(&models.Task{}); err != nil {
				t.Fatal(err)
			}
			tm := &TaskManager{runningTasks: make(map[string]*RunningTask)}
			task, ctx, err := tm.CreateTask(models.TaskTypeNodeAdd, "test", models.TaskTriggerManual, 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := database.DB.Callback().Update().Before("gorm:update").Register("reject", func(tx *gorm.DB) { _ = tx.AddError(errors.New("disk unavailable")) }); err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "complete":
				err = tm.CompleteTask(task.ID, "done", map[string]int{"added": 1})
			case "fail":
				err = tm.FailTask(task.ID, "failed")
			case "cancel":
				err = tm.CancelTask(task.ID)
			}
			if err == nil {
				t.Fatal("persistence failure reported as success")
			}
			if task.Status != models.TaskStatusError {
				t.Fatal("failure not visible")
			}
			select {
			case <-ctx.Done():
			default:
				t.Fatal("execution not stopped")
			}
			if err := tm.UpdateProgress(task.ID, 1, "late", nil); err == nil {
				t.Fatal("late progress overwrote terminal")
			}
			if err := tm.CompleteTask(task.ID, "late", nil); err == nil {
				t.Fatal("duplicate terminal accepted")
			}
		})
	}
}

func TestTaskTerminalPersistsResultAndCache(t *testing.T) {
	old := database.DB
	database.DB = testutil.OpenMemoryDB(t, "task_success")
	defer func() { testutil.CloseDB(t, database.DB); database.DB = old }()
	if err := database.DB.AutoMigrate(&models.Task{}); err != nil {
		t.Fatal(err)
	}
	tm := &TaskManager{runningTasks: make(map[string]*RunningTask)}
	task, _, err := tm.CreateTask(models.TaskTypeNodeAdd, "test", models.TaskTriggerManual, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := tm.CompleteTask(task.ID, "done", map[string]int{"added": 2}); err != nil {
		t.Fatal(err)
	}
	var cached, persisted models.Task
	if err := cached.GetByID(task.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.First(&persisted, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if cached.Status != models.TaskStatusCompleted || persisted.Result != cached.Result || persisted.Result != `{"added":2}` || cached.CompletedAt == nil {
		t.Fatalf("inconsistent terminal: %+v %+v", cached, persisted)
	}
}

func TestTaskProgressSummaryPersistsOnCancel(t *testing.T) {
	old := database.DB
	database.DB = testutil.OpenMemoryDB(t, "task_progress_summary")
	defer func() { testutil.CloseDB(t, database.DB); database.DB = old }()
	if err := database.DB.AutoMigrate(&models.Task{}); err != nil {
		t.Fatal(err)
	}
	tm := &TaskManager{runningTasks: make(map[string]*RunningTask)}
	task, _, err := tm.CreateTask(models.TaskTypeNodeRevalidate, "summary", models.TaskTriggerManual, 5)
	if err != nil {
		t.Fatal(err)
	}
	summary := map[string]int{"total": 5, "processed": 2, "validated": 1, "failed": 1}
	if err := tm.UpdateProgress(task.ID, 2, "node #2", summary); err != nil {
		t.Fatal(err)
	}
	encoded := task.Result
	if encoded == "" {
		t.Fatal("summary not retained")
	}
	// Intermediate updates remain in memory; no new write frequency is introduced.
	var before models.Task
	if err := database.DB.First(&before, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if before.Result != "" || before.Progress != 0 {
		t.Fatal("progress wrote DB", before)
	}
	if err := tm.UpdateProgress(task.ID, 2, "waiting", nil); err != nil {
		t.Fatal(err)
	}
	if task.Result != encoded {
		t.Fatal("nil erased summary")
	}
	// Failed encoding must change neither counts, current item, nor summary.
	if err := tm.UpdateProgress(task.ID, 3, "bad", make(chan int)); err == nil {
		t.Fatal("accepted unencodable result")
	}
	if task.Progress != 2 || task.CurrentItem != "waiting" || task.Result != encoded {
		t.Fatal("marshal failure mutated task")
	}
	if err := tm.CancelTask(task.ID); err != nil {
		t.Fatal(err)
	}
	var cached, persisted models.Task
	if err := persisted.GetByID(task.ID); err != nil {
		t.Fatal(err)
	}
	cached = persisted
	if err := database.DB.First(&persisted, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status != models.TaskStatusCancelled || persisted.Progress != 2 || persisted.Result != encoded || cached.Result != encoded {
		t.Fatalf("cancel summary lost: DB=%+v cache=%+v", persisted, cached)
	}
	var decoded map[string]int
	if err := json.Unmarshal([]byte(persisted.Result), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["processed"] != 2 || decoded["validated"] != 1 || decoded["failed"] != 1 || decoded["total"] != 5 {
		t.Fatal(decoded)
	}
	if err := tm.UpdateProgress(task.ID, 4, "late", summary); err == nil {
		t.Fatal("late progress accepted")
	}
	if task.Result != encoded || task.Progress != 2 {
		t.Fatal("late progress changed terminal")
	}
}
