package api

import (
	"testing"

	"sublink/models"
)

func TestBuildRulePreviewDataSpecifiedNodes(t *testing.T) {
	rule := models.SubscriptionChainRule{
		ID: 1, Name: "selected", Enabled: true,
		ChainConfig:  `[{"type":"dynamic_node","nodeConditions":{"conditions":[]}}]`,
		TargetConfig: `{"type":"specified_nodes","nodeIds":[2,99,1]}`,
	}
	nodes := []models.Node{{ID: 1, Name: "one"}, {ID: 2, Name: "two"}}
	names := map[int]string{1: nodes[0].EffectiveName(), 2: nodes[1].EffectiveName()}
	info := map[int]models.Node{1: nodes[0], 2: nodes[1]}

	preview := buildRulePreviewData(rule, nodes, names, info)
	if preview.TargetType != "specified_nodes" {
		t.Fatalf("expected specified_nodes target type, got %q", preview.TargetType)
	}
	if len(preview.TargetNodes) != 2 {
		t.Fatalf("expected two available target nodes, got %d", len(preview.TargetNodes))
	}
	if preview.TargetInfo != "two, (节点不存在: 99), one" {
		t.Fatalf("unexpected target info: %q", preview.TargetInfo)
	}
}
