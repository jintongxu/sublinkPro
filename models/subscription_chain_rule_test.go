package models

import "testing"

func TestValidateChainRuleRejectsNodeOutsideSubscription(t *testing.T) {
	rule := SubscriptionChainRule{
		Name:         "chain-a",
		ChainConfig:  `[{"type":"specified_node","nodeId":99}]`,
		TargetConfig: `{"type":"all"}`,
	}
	if err := ValidateChainRule(&rule, []Node{{ID: 1}}, nil); err == nil {
		t.Fatal("expected missing subscription node validation error")
	}
}

func TestValidateChainRuleRejectsDuplicateCustomGroups(t *testing.T) {
	rule := SubscriptionChainRule{
		Name:         "chain-a",
		ChainConfig:  `[{"type":"custom_group","groupName":"front"},{"type":"custom_group","groupName":"front"}]`,
		TargetConfig: `{"type":"all"}`,
	}
	if err := ValidateChainRule(&rule, nil, nil); err == nil {
		t.Fatal("expected duplicate custom group validation error")
	}
}

func TestValidateChainRuleAcceptsSpecifiedNode(t *testing.T) {
	rule := SubscriptionChainRule{
		Name:         "chain-a",
		ChainConfig:  `[{"type":"specified_node","nodeId":1}]`,
		TargetConfig: `{"type":"specified_node","nodeId":1}`,
	}
	if err := ValidateChainRule(&rule, []Node{{ID: 1}}, nil); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
