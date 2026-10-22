package models

import (
	"sublink/database"
	"testing"
)

func TestSubscriptionValidationPolicyMatrix(t *testing.T) {
	resetChainRuleCacheForTest()
	statuses := []string{"", ValidationStatusUntested, ValidationStatusSuccess, ValidationStatusPartial, ValidationStatusFailed, ValidationStatusDisabled}
	for _, policy := range []string{"", "all", "exclude_failed", "only_success"} {
		for _, status := range statuses {
			t.Run(policy+"/"+status, func(t *testing.T) {
				sub := Subcription{ValidationPolicy: policy, Nodes: []Node{{ID: 1, Name: "node", LinkName: "node", Link: "ss://node", ValidationStatus: status}}}
				want := 1
				if policy == "exclude_failed" && status == ValidationStatusFailed || policy == "only_success" && status != ValidationStatusSuccess {
					want = 0
				}
				got, err := sub.PreviewSub()
				if err != nil {
					t.Fatal(err)
				}
				if len(got.Nodes) != want || got.ValidationExcludedCount != 1-want {
					t.Fatalf("result=%+v want=%d", got, want)
				}
			})
		}
	}
	if _, err := NormalizeValidationPolicy("unknown"); err == nil {
		t.Fatal("unknown accepted")
	}
}

type legacyPolicySubscription struct {
	ID   int
	Name string
}

func (legacyPolicySubscription) TableName() string { return "subcriptions" }

func TestSubscriptionValidationPolicyMigration(t *testing.T) {
	setupSubcriptionCopyTestDB(t)
	// Simulate a table predating the policy column.
	if err := database.DB.Migrator().DropColumn(&Subcription{}, "ValidationPolicy"); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Create(&legacyPolicySubscription{Name: "legacy"}).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := migrateSubscriptionValidationPolicy(database.DB); err != nil {
			t.Fatal(err)
		}
	}
	var sub Subcription
	if err := database.DB.First(&sub).Error; err != nil {
		t.Fatal(err)
	}
	if sub.ValidationPolicy != "all" {
		t.Fatal(sub.ValidationPolicy)
	}
	if err := database.DB.Model(&sub).Update("validation_policy", "only_success").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateSubscriptionValidationPolicy(database.DB); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.First(&sub).Error; err != nil {
		t.Fatal(err)
	}
	if sub.ValidationPolicy != "only_success" {
		t.Fatal("overwrote policy")
	}
	if err := database.DB.Model(&sub).Update("validation_policy", "").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateSubscriptionValidationPolicy(database.DB); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.First(&sub).Error; err != nil {
		t.Fatal(err)
	}
	if sub.ValidationPolicy != "all" {
		t.Fatal("blank not defaulted")
	}
}

func TestSubscriptionValidationPolicyPersistenceCopyCache(t *testing.T) {
	setupSubcriptionCopyTestDB(t)
	sub := Subcription{Name: "policy"}
	if err := sub.Add(); err != nil {
		t.Fatal(err)
	}
	if sub.ValidationPolicy != "all" {
		t.Fatal("missing default")
	}
	for _, policy := range []string{"only_success", "exclude_failed", "all"} {
		sub.ValidationPolicy = policy
		if err := sub.Update(); err != nil {
			t.Fatal(err)
		}
		cached, err := GetSubcriptionByID(sub.ID)
		if err != nil || cached.ValidationPolicy != policy {
			t.Fatalf("cache=%+v err=%v", cached, err)
		}
	}
	sub.ValidationPolicy = "only_success"
	if err := sub.Update(); err != nil {
		t.Fatal(err)
	}
	copied, err := sub.Copy()
	if err != nil {
		t.Fatal(err)
	}
	cached, err := GetSubcriptionByID(copied.ID)
	if err != nil || cached.ValidationPolicy != "only_success" {
		t.Fatalf("copy cache=%+v err=%v", cached, err)
	}
	sub.ValidationPolicy = "bad"
	if sub.Update() == nil {
		t.Fatal("unknown update accepted")
	}
	if _, err := sub.Copy(); err == nil {
		t.Fatal("unknown copy accepted")
	}
	if err := (&Subcription{Name: "bad", ValidationPolicy: "bad"}).Add(); err == nil {
		t.Fatal("unknown add accepted")
	}
}

func TestSubscriptionValidationPolicyDependencies(t *testing.T) {
	setupSubcriptionCopyTestDB(t)
	original := []Node{
		{ID: 1, Name: "entry", LinkName: "entry", ValidationStatus: ValidationStatusFailed},
		{ID: 2, Name: "middle", LinkName: "middle", ValidationStatus: ValidationStatusSuccess, DialerProxyName: "entry"},
		{ID: 3, Name: "exit", LinkName: "exit", ValidationStatus: ValidationStatusSuccess, DialerProxyName: "middle"},
	}
	sub := Subcription{ID: 1, ValidationPolicy: "exclude_failed"}
	got := sub.FilterValidationDependencies(sub.ApplyValidationPolicy(original), original)
	if len(got) != 0 || sub.ValidationExcludedCount != 1 || sub.ValidationDependencyExcludedCount != 2 {
		t.Fatalf("got=%+v sub=%+v", got, sub)
	}
	// A missing first matching chain cannot fall through to a weaker rule/direct.
	rule := SubscriptionChainRule{SubscriptionID: 1, Name: "mandatory", Enabled: true, ChainConfig: `[{"type":"specified_node","nodeId":1}]`, TargetConfig: `{"type":"specified_node","nodeId":3}`}
	if err := rule.Add(); err != nil {
		t.Fatal(err)
	}
	original[2].DialerProxyName = ""
	original[1].DialerProxyName = ""
	sub.ValidationDependencyExcludedCount = 0
	got = sub.FilterValidationDependencies(sub.ApplyValidationPolicy(original), original)
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("chain silently weakened: %+v", got)
	}
	// Dynamic custom-group rules must not generate an empty prerequisite group.
	rule.ChainConfig = `[{"type":"custom_group","groupName":"required","nodeConditions":{"logic":"and","conditions":[{"field":"name","operator":"equals","value":"absent"}]}}]`
	if err := rule.Update(); err != nil {
		t.Fatal(err)
	}
	got = sub.FilterValidationDependencies(sub.ApplyValidationPolicy(original), original)
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("empty custom group retained target: %+v", got)
	}
	rule.ChainConfig = `[{"type":"dynamic_node","nodeConditions":{"logic":"and","conditions":[{"field":"name","operator":"equals","value":"absent"}]}}]`
	if err := rule.Update(); err != nil {
		t.Fatal(err)
	}
	got = sub.FilterValidationDependencies(sub.ApplyValidationPolicy(original), original)
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("missing dynamic prerequisite retained target: %+v", got)
	}
}

func TestSubscriptionValidationPolicyRenamedDependencies(t *testing.T) {
	setupSubcriptionCopyTestDB(t)
	for _, tc := range []struct {
		name, rule, entryName, middleDialer, exitDialer string
	}{
		{"final alias", "renamed-$LinkName", "entry", "", "renamed-entry"},
		{"trimmed raw alias", "renamed-$LinkName", "entry", "", " entry "},
		{"transitive final aliases", "renamed-$LinkName", "entry", "renamed-entry", "renamed-middle"},
		// Removing the first duplicate transfers its old rendered name to the
		// survivor. That must not turn an unsafe exit into a different chain.
		{"duplicate index reassignment", "renamed-$LinkName$DuplicateIndex", "middle", "", "renamed-middle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := []Node{
				{ID: 1, Name: "entry", LinkName: tc.entryName, ValidationStatus: ValidationStatusFailed},
				{ID: 2, Name: "middle", LinkName: "middle", ValidationStatus: ValidationStatusSuccess, DialerProxyName: tc.middleDialer},
				{ID: 3, Name: "exit", LinkName: "exit", ValidationStatus: ValidationStatusSuccess, DialerProxyName: tc.exitDialer},
			}
			sub := Subcription{ValidationPolicy: "exclude_failed", NodeNameRule: tc.rule}
			got := sub.FilterValidationDependencies(sub.ApplyValidationPolicy(original), original)
			want := 1
			if tc.middleDialer != "" {
				want = 0
			}
			if len(got) != want || want == 1 && got[0].ID != 2 || sub.ValidationExcludedCount != 1 || sub.ValidationDependencyExcludedCount != 2-want {
				t.Fatalf("unsafe dependency survived: nodes=%+v sub=%+v", got, sub)
			}
			// Legacy all policy still keeps both dependency endpoints.
			sub.ValidationPolicy = "all"
			if got := sub.FilterValidationDependencies(original, original); len(got) != 3 {
				t.Fatalf("all policy changed: %+v", got)
			}
		})
	}
}
