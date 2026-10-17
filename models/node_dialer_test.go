package models

import (
	"testing"

	"sublink/database"
	"sublink/internal/testutil"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupNodeDialerTestDB(t *testing.T) {
	t.Helper()
	oldDB := database.DB
	oldDialect := database.Dialect
	db, err := gorm.Open(sqlite.Open(testutil.UniqueMemoryDSN(t, "node_dialer_test")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&Node{}); err != nil {
		t.Fatalf("auto migrate nodes: %v", err)
	}
	database.DB = db
	database.Dialect = database.DialectSQLite
	resetNodeCacheForTest()
	t.Cleanup(func() {
		database.DB = oldDB
		database.Dialect = oldDialect
		resetNodeCacheForTest()
		testutil.CloseDB(t, db)
	})
}

func TestNodeAddAllowsDifferentDialerForSameLink(t *testing.T) {
	setupNodeDialerTestDB(t)

	first := Node{Name: "target-a", Link: "ss://same-target", DialerProxyName: "front-a"}
	if err := first.Add(); err != nil {
		t.Fatalf("add first node: %v", err)
	}
	second := Node{Name: "target-b", Link: first.Link, DialerProxyName: "front-b"}
	if err := second.Add(); err != nil {
		t.Fatalf("add same target with different dialer: %v", err)
	}

	var count int64
	if err := database.DB.Model(&Node{}).Where("link_hash = ?", first.LinkHash).Count(&count).Error; err != nil {
		t.Fatalf("count nodes: %v", err)
	}
	if count != 2 {
		t.Fatalf("node count = %d, want 2", count)
	}
}

func TestNodeAddRejectsSameLinkAndDialer(t *testing.T) {
	setupNodeDialerTestDB(t)

	first := Node{Name: "target-a", Link: "ss://same-target", DialerProxyName: "front-a"}
	if err := first.Add(); err != nil {
		t.Fatalf("add first node: %v", err)
	}
	duplicate := Node{Name: "target-b", Link: first.Link, DialerProxyName: first.DialerProxyName}
	if err := duplicate.Add(); err == nil {
		t.Fatal("adding same target with same dialer should fail")
	}
}

func TestGetNodeByContentHashAndDialer(t *testing.T) {
	setupNodeDialerTestDB(t)

	first := Node{Name: "target-a", Link: "ss://same-target", ContentHash: "hash-a", DialerProxyName: "front-a"}
	if err := first.Add(); err != nil {
		t.Fatalf("add first node: %v", err)
	}
	second := Node{Name: "target-b", Link: "ss://same-target", ContentHash: "hash-a", DialerProxyName: "front-b"}
	if err := second.Add(); err != nil {
		t.Fatalf("add second node: %v", err)
	}

	if found, ok := GetNodeByContentHashAndDialer("hash-a", "front-a"); !ok || found.ID != first.ID {
		t.Fatalf("front-a lookup = %#v, %v; want first node", found, ok)
	}
	if found, ok := GetNodeByContentHashAndDialer("hash-a", "front-b"); !ok || found.ID != second.ID {
		t.Fatalf("front-b lookup = %#v, %v; want second node", found, ok)
	}
}
