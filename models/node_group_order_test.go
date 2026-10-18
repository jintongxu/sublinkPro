package models

import "testing"

func TestAirportGroupAppearsBeforeNodesArePulled(t *testing.T) {
	oldAirports := airportCache.GetAll()
	oldNodes := nodeCache.GetAll()
	oldSorts := groupAirportSortCache.GetAll()
	t.Cleanup(func() {
		airportCache.LoadAll(oldAirports)
		nodeCache.LoadAll(oldNodes)
		groupAirportSortCache.LoadAll(oldSorts)
	})

	airportCache.LoadAll([]Airport{{ID: 7, Name: "机场A", Group: "香港"}})
	nodeCache.LoadAll(nil)
	groupAirportSortCache.LoadAll(nil)

	groups := GetAllGroupNames()
	if len(groups) != 1 || groups[0] != "香港" {
		t.Fatalf("groups = %#v, want [香港]", groups)
	}

	infos := GetAllGroupInfos()
	if len(infos) != 1 || infos[0].GroupName != "香港" || infos[0].NodeCount != 0 || infos[0].AirportCount != 1 {
		t.Fatalf("group infos = %#v, want one airport-only group", infos)
	}

	detail, err := GetGroupDetail("香港")
	if err != nil {
		t.Fatalf("GetGroupDetail() error = %v", err)
	}
	if len(detail.Airports) != 1 || detail.Airports[0].AirportID != 7 || detail.Airports[0].AirportName != "机场A" || detail.Airports[0].NodeCount != 0 {
		t.Fatalf("group detail = %#v, want airport-only detail", detail)
	}
}

func TestSortNodesByAirport_DefaultOrder(t *testing.T) {
	nodes := []Node{
		{ID: 1, SourceID: 2, Name: "A-3", SourceSort: 3},
		{ID: 2, SourceID: 2, Name: "A-1", SourceSort: 1},
		{ID: 3, SourceID: 4, Name: "B-1", SourceSort: 1},
		{ID: 4, SourceID: 3, Name: "C-1", SourceSort: 2},
		{ID: 5, SourceID: 3, Name: "C-0", SourceSort: 0},
		{ID: 6, SourceID: 4, Name: "B-2", SourceSort: 2},
		{ID: 7, SourceID: 2, Name: "A-2", SourceSort: 2},
	}

	got := SortNodesByAirport(nodes, nil)

	wantNames := []string{"A-1", "A-2", "A-3", "B-1", "B-2", "C-1", "C-0"}
	if len(got) != len(wantNames) {
		t.Fatalf("len(got)=%d, want=%d", len(got), len(wantNames))
	}
	for i, want := range wantNames {
		if got[i].Name != want {
			t.Fatalf("name mismatch at %d: got=%s want=%s", i, got[i].Name, want)
		}
	}
}

// 所有节点 SourceSort=0（纯历史数据），验证退化到按机场聚合 + ID 稳定排序
func TestSortNodesByAirport_AllZeroSourceSort(t *testing.T) {
	nodes := []Node{
		{ID: 1, SourceID: 2, Name: "A-1", SourceSort: 0},
		{ID: 2, SourceID: 4, Name: "B-1", SourceSort: 0},
		{ID: 3, SourceID: 2, Name: "A-2", SourceSort: 0},
		{ID: 4, SourceID: 3, Name: "C-1", SourceSort: 0},
		{ID: 5, SourceID: 4, Name: "B-2", SourceSort: 0},
		{ID: 6, SourceID: 3, Name: "C-2", SourceSort: 0},
	}

	got := SortNodesByAirport(nodes, nil)

	// 机场首次出现顺序：SourceID=2(A) → 4(B) → 3(C)
	// 同机场内 SourceSort 全为 0，SliceStable 保持输入顺序（即 ID 升序）
	wantNames := []string{"A-1", "A-2", "B-1", "B-2", "C-1", "C-2"}
	if len(got) != len(wantNames) {
		t.Fatalf("len(got)=%d, want=%d", len(got), len(wantNames))
	}
	for i, want := range wantNames {
		if got[i].Name != want {
			t.Fatalf("name mismatch at %d: got=%s want=%s", i, got[i].Name, want)
		}
	}
}

func TestSortNodesByAirport_CustomAirportSort(t *testing.T) {
	nodes := []Node{
		{ID: 1, SourceID: 2, Name: "A-3", SourceSort: 3},
		{ID: 2, SourceID: 2, Name: "A-1", SourceSort: 1},
		{ID: 3, SourceID: 4, Name: "B-1", SourceSort: 1},
		{ID: 4, SourceID: 3, Name: "C-1", SourceSort: 2},
		{ID: 5, SourceID: 3, Name: "C-0", SourceSort: 0},
		{ID: 6, SourceID: 4, Name: "B-2", SourceSort: 2},
		{ID: 7, SourceID: 2, Name: "A-2", SourceSort: 2},
	}

	airportSortMap := map[int]int{
		3: 0, // C 优先
		2: 1, // A 次之
		// B 未配置 -> 默认排最后
	}
	got := SortNodesByAirport(nodes, airportSortMap)

	wantNames := []string{"C-1", "C-0", "A-1", "A-2", "A-3", "B-1", "B-2"}
	if len(got) != len(wantNames) {
		t.Fatalf("len(got)=%d, want=%d", len(got), len(wantNames))
	}
	for i, want := range wantNames {
		if got[i].Name != want {
			t.Fatalf("name mismatch at %d: got=%s want=%s", i, got[i].Name, want)
		}
	}
}
