package models

import "testing"

func TestBuildSubscriptionNodeNamePlanMarksAirportSource(t *testing.T) {
	nodes := []Node{
		{ID: 1, LinkName: "手动节点", Link: "ss://manual", Source: "manual"},
		{ID: 2, LinkName: "机场节点", Link: "ss://airport", Source: "机场A", SourceID: 7},
	}

	plan := BuildSubscriptionNodeNamePlan(nodes, "", "", nil, NodeNamePlanOptions{MarkAirportSource: true})
	if got := plan.NodeNameAt(0, nodes[0].ID); got != "手动节点" {
		t.Fatalf("manual node name = %q, want %q", got, "手动节点")
	}
	if got := plan.NodeNameAt(1, nodes[1].ID); got != "机场节点@机场A" {
		t.Fatalf("airport node name = %q, want %q", got, "机场节点@机场A")
	}
}

func TestBuildSubscriptionNodeNamePlanMarksSplitAirportLinks(t *testing.T) {
	node := Node{
		ID:       3,
		LinkName: "机场节点",
		Link:     "ss://one,ss://two",
		Source:   "机场B",
		SourceID: 8,
	}

	plan := BuildSubscriptionNodeNamePlan([]Node{node}, "", "", nil, NodeNamePlanOptions{MarkAirportSource: true})
	names := plan.SplitNamesAt(0)
	if len(names) != 2 || names[0] != "机场节点@机场B" || names[1] != "机场节点@机场B" {
		t.Fatalf("split airport names = %#v, want both names marked", names)
	}
}
