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
		ChainConfig:  `[{"type":"specified_node","nodeId":2}]`,
		TargetConfig: `{"type":"specified_node","nodeId":1}`,
	}
	if err := ValidateChainRule(&rule, []Node{{ID: 1}, {ID: 2}}, nil); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestTargetConfigSpecifiedNodesParseAndMatch(t *testing.T) {
	rule := SubscriptionChainRule{TargetConfig: `{"type":"specified_nodes","nodeIds":[2,4]}`}
	target, err := rule.ParseTargetConfig()
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(target.NodeIDs) != 2 || target.NodeIDs[0] != 2 || target.NodeIDs[1] != 4 {
		t.Fatalf("unexpected node IDs: %#v", target.NodeIDs)
	}
	if !rule.MatchTargetCondition(Node{ID: 4}) || rule.MatchTargetCondition(Node{ID: 3}) {
		t.Fatal("specified_nodes matching returned unexpected result")
	}
}

func TestValidateChainRuleRejectsInvalidSpecifiedNodesTarget(t *testing.T) {
	base := SubscriptionChainRule{
		Name:        "chain-a",
		ChainConfig: `[{"type":"specified_node","nodeId":3}]`,
	}
	for name, target := range map[string]string{
		"empty":     `{"type":"specified_nodes","nodeIds":[]}`,
		"duplicate": `{"type":"specified_nodes","nodeIds":[1,1]}`,
		"outside":   `{"type":"specified_nodes","nodeIds":[1,9]}`,
		"self":      `{"type":"specified_nodes","nodeIds":[3]}`,
	} {
		t.Run(name, func(t *testing.T) {
			rule := base
			rule.TargetConfig = target
			if err := ValidateChainRule(&rule, []Node{{ID: 1}, {ID: 3}}, nil); err == nil {
				t.Fatal("expected target validation error")
			}
		})
	}
}

func TestApplyChainRulesSpecifiedNodesOverridesNodeDialer(t *testing.T) {
	rule := SubscriptionChainRule{
		Name: "chain-a", Enabled: true,
		ChainConfig:  `[{"type":"specified_node","nodeId":10}]`,
		TargetConfig: `{"type":"specified_nodes","nodeIds":[1,2]}`,
	}
	nodes := []Node{{ID: 1, DialerProxyName: "node-default"}, {ID: 2}, {ID: 3, DialerProxyName: "other-default"}, {ID: 10}}
	names := map[int]string{10: "front"}
	matched := ApplyChainRulesToNodeV2(nodes[0], []SubscriptionChainRule{rule}, nodes, names)
	if matched == nil || matched.FinalDialer != "front" {
		t.Fatalf("expected policy dialer, got %#v", matched)
	}
	fallback := ApplyChainRulesToNodeV2(nodes[2], []SubscriptionChainRule{rule}, nodes, names)
	if fallback == nil || fallback.FinalDialer != "other-default" {
		t.Fatalf("expected node fallback, got %#v", fallback)
	}
}
