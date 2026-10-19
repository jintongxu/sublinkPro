package models

import "testing"

func TestBuildNodeNamePlanPreservesManualAndAirportNames(t *testing.T) {
	nodes := []Node{
		{ID: 1, LinkName: "手动节点", Link: "ss://manual", Source: "manual"},
		{ID: 2, LinkName: "三色@机场节点", Link: "ss://airport", Source: "机场A", SourceID: 7, Group: "机场"},
	}

	plan := BuildNodeNamePlan(nodes, "", "", nil)
	if got := plan.NodeNameAt(0, nodes[0].ID); got != "手动节点" {
		t.Fatalf("manual node name = %q, want %q", got, "手动节点")
	}
	if got := plan.NodeNameAt(1, nodes[1].ID); got != "三色@机场节点" {
		t.Fatalf("airport node name = %q, want %q", got, "三色@机场节点")
	}
}
