package models

import (
	"strconv"
	"testing"

	"sublink/cache"
	"sublink/database"
	"sublink/internal/testutil"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupValidationTestDB uses an isolated in-memory database and restores the
// package globals so these tests do not leak state into other model tests.
func setupValidationTestDB(t *testing.T, migrateNode bool) *gorm.DB {
	t.Helper()
	oldDB, oldDialect, oldInitialized := database.DB, database.Dialect, database.IsInitialized
	db, err := gorm.Open(sqlite.Open(testutil.UniqueMemoryDSN(t, "node_validation_test")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if migrateNode {
		if err := db.AutoMigrate(&Node{}); err != nil {
			t.Fatalf("migrate node table: %v", err)
		}
	}
	database.DB = db
	database.Dialect = database.DialectSQLite
	database.IsInitialized = false
	resetNodeValidationCache()
	t.Cleanup(func() {
		database.DB, database.Dialect, database.IsInitialized = oldDB, oldDialect, oldInitialized
		resetNodeValidationCache()
		testutil.CloseDB(t, db)
	})
	return db
}

func resetNodeValidationCache() {
	nodeCache = cache.NewMapCache(func(n Node) int { return n.ID })
	nodeCache.AddIndex("group", func(n Node) string { return n.Group })
	nodeCache.AddIndex("source", func(n Node) string { return n.Source })
	nodeCache.AddIndex("country", func(n Node) string { return n.LinkCountry })
	nodeCache.AddIndex("protocol", func(n Node) string { return n.Protocol })
	nodeCache.AddIndex("sourceID", func(n Node) string { return strconv.Itoa(n.SourceID) })
	nodeCache.AddIndex("name", func(n Node) string { return n.Name })
	nodeCache.AddIndex("contentHash", func(n Node) string { return n.ContentHash })
}

func TestNodeValidationFieldsPersistAcrossReloadAndCache(t *testing.T) {
	db := setupValidationTestDB(t, true)
	node := &Node{Name: "validation-node", Link: "ss://validation", Protocol: "ss"}
	if err := node.Add(); err != nil {
		t.Fatalf("add node: %v", err)
	}
	if node.ValidationStatus != ValidationStatusUntested {
		t.Fatalf("new node validation status = %q, want %q", node.ValidationStatus, ValidationStatusUntested)
	}
	loaded := Node{}
	if err := db.First(&loaded, node.ID).Error; err != nil {
		t.Fatalf("reload added node: %v", err)
	}
	if loaded.ValidationStatus != ValidationStatusUntested || loaded.ValidationReason != "" || loaded.ValidationCheckAt != "" {
		t.Fatalf("new node validation fields = %#v, want untested and empty metadata", loaded)
	}

	snapshot := *node
	node.ValidationStatus = ValidationStatusPartial
	node.ValidationReason = "one endpoint returned 502"
	node.ValidationCheckAt = "2025-01-02T03:04:05Z"
	if applied, err := ApplyNodeValidationSnapshot(snapshot, node.ValidationStatus, node.ValidationReason, node.ValidationCheckAt); err != nil || !applied {
		t.Fatalf("update validation fields: applied=%v err=%v", applied, err)
	}
	var reloaded Node
	if err := db.First(&reloaded, node.ID).Error; err != nil {
		t.Fatalf("reload updated node: %v", err)
	}
	if reloaded.ValidationStatus != node.ValidationStatus || reloaded.ValidationReason != node.ValidationReason || reloaded.ValidationCheckAt != node.ValidationCheckAt {
		t.Fatalf("reloaded validation fields = %#v, want %#v", reloaded, *node)
	}
	cached, ok := nodeCache.Get(node.ID)
	if !ok || cached.ValidationStatus != node.ValidationStatus || cached.ValidationReason != node.ValidationReason || cached.ValidationCheckAt != node.ValidationCheckAt {
		t.Fatalf("cached validation fields = %#v, ok=%v, want %#v", cached, ok, *node)
	}
}

type legacyValidationNode struct {
	ID       int `gorm:"primaryKey"`
	Link     string
	LinkHash string
	Name     string
}

func (legacyValidationNode) TableName() string { return "nodes" }

func TestNodeValidationMigrationAddsColumnsAndDefaultsLegacyRows(t *testing.T) {
	db := setupValidationTestDB(t, false)
	if err := db.AutoMigrate(&legacyValidationNode{}); err != nil {
		t.Fatalf("create legacy node table: %v", err)
	}
	if err := db.Create(&legacyValidationNode{Link: "ss://legacy", Name: "legacy"}).Error; err != nil {
		t.Fatalf("seed legacy node: %v", err)
	}
	if err := RunMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	var node Node
	if err := db.First(&node).Error; err != nil {
		t.Fatalf("reload migrated node: %v", err)
	}
	if node.ValidationStatus != ValidationStatusUntested || node.ValidationReason != "" || node.ValidationCheckAt != "" {
		t.Fatalf("migrated legacy validation fields = %#v, want untested and empty metadata", node)
	}
	for _, column := range []string{"validation_status", "validation_reason", "validation_check_at"} {
		if !db.Migrator().HasColumn(&Node{}, column) {
			t.Fatalf("migration did not add %s", column)
		}
	}
}
