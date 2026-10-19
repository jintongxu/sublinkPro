package models

import (
	"strings"
	"sublink/utils"
)

const duplicateIndexVariable = "$DuplicateIndex"

// NodeNamePlanOptions controls output-only subscription naming adjustments.
type NodeNamePlanOptions struct {
	MarkAirportSource bool
}

// NodeNamePlan records the final rendered names for subscription output.
type NodeNamePlan struct {
	NodeNames   []string
	SplitNames  [][]string
	NodeNameMap map[int]string
}

type nodeNameOccurrence struct {
	node              Node
	nodeIndex         int
	linkIndex         int
	link              string
	processedLinkName string
	protocol          string
	baseName          string
}

// BuildNodeNamePlan renders node names in output order and applies duplicate indexes only when requested.
func BuildNodeNamePlan(nodes []Node, nodeNamePreprocess string, nodeNameRule string, protocolFromLink func(string) string) NodeNamePlan {
	return buildNodeNamePlan(nodes, nodeNamePreprocess, nodeNameRule, protocolFromLink, NodeNamePlanOptions{})
}

// BuildSubscriptionNodeNamePlan builds final subscription names and optionally appends the airport source.
func BuildSubscriptionNodeNamePlan(nodes []Node, nodeNamePreprocess string, nodeNameRule string, protocolFromLink func(string) string, options NodeNamePlanOptions) NodeNamePlan {
	return buildNodeNamePlan(nodes, nodeNamePreprocess, nodeNameRule, protocolFromLink, options)
}

func buildNodeNamePlan(nodes []Node, nodeNamePreprocess string, nodeNameRule string, protocolFromLink func(string) string, options NodeNamePlanOptions) NodeNamePlan {
	if protocolFromLink == nil {
		protocolFromLink = func(string) string { return "" }
	}

	plan := NodeNamePlan{
		NodeNames:   make([]string, len(nodes)),
		SplitNames:  make([][]string, len(nodes)),
		NodeNameMap: make(map[int]string, len(nodes)),
	}
	occurrences := make([]nodeNameOccurrence, 0, len(nodes))

	for nodeIndex, node := range nodes {
		processedLinkName := utils.PreprocessNodeName(nodeNamePreprocess, node.LinkName)
		links := splitNodeOutputLinks(node.Link)
		plan.SplitNames[nodeIndex] = make([]string, len(links))

		for linkIndex, link := range links {
			protocolName := protocolFromLink(link)
			occurrences = append(occurrences, nodeNameOccurrence{
				node:              node,
				nodeIndex:         nodeIndex,
				linkIndex:         linkIndex,
				link:              link,
				processedLinkName: processedLinkName,
				protocol:          protocolName,
				baseName:          BuildNodeExportName(node, processedLinkName, nodeNameRule, protocolName, nodeIndex+1, 0),
			})
		}
	}

	useDuplicateIndex := NodeNameRuleUsesDuplicateIndex(nodeNameRule)
	nameTotals := make(map[string]int, len(occurrences))
	if useDuplicateIndex {
		for _, occurrence := range occurrences {
			nameTotals[occurrence.baseName]++
		}
	}

	nameIndexes := make(map[string]int, len(nameTotals))
	for _, occurrence := range occurrences {
		finalName := occurrence.baseName
		if useDuplicateIndex && nameTotals[occurrence.baseName] > 1 {
			duplicateIndex := nameIndexes[occurrence.baseName]
			nameIndexes[occurrence.baseName]++
			finalName = BuildNodeExportName(
				occurrence.node,
				occurrence.processedLinkName,
				nodeNameRule,
				occurrence.protocol,
				occurrence.nodeIndex+1,
				duplicateIndex,
			)
		}
		if options.MarkAirportSource {
			finalName = appendAirportSource(finalName, occurrence.node)
		}
		plan.setName(occurrence.nodeIndex, occurrence.linkIndex, occurrence.node.ID, finalName)
	}

	return plan
}

// BuildNodeExportName renders one node name with the current rename variables.
func BuildNodeExportName(node Node, processedLinkName string, nodeNameRule string, protocolName string, index int, duplicateIndex int) string {
	if nodeNameRule == "" {
		return node.EffectiveName()
	}
	return utils.RenameNode(nodeNameRule, BuildNodeRenameInfo(node, processedLinkName, protocolName, index, duplicateIndex))
}

// NodeNameRuleUsesDuplicateIndex reports whether the rename rule opts into duplicate numbering.
func NodeNameRuleUsesDuplicateIndex(nodeNameRule string) bool {
	return strings.Contains(nodeNameRule, duplicateIndexVariable)
}

// RenameOutputLink applies the planned names to every link in a node output.
func (plan NodeNamePlan) RenameOutputLink(index int, nodeID int, link string) string {
	if !strings.Contains(link, ",") {
		return utils.RenameNodeLink(link, plan.NodeNameAt(index, nodeID))
	}

	links := strings.Split(link, ",")
	names := plan.SplitNamesAt(index)
	for i, item := range links {
		name := plan.NodeNameAt(index, nodeID)
		if i < len(names) && names[i] != "" {
			name = names[i]
		}
		links[i] = utils.RenameNodeLink(item, name)
	}
	return strings.Join(links, ",")
}

// NodeNameAt returns the planned primary output name for a node occurrence.
func (plan NodeNamePlan) NodeNameAt(index int, nodeID int) string {
	if index >= 0 && index < len(plan.NodeNames) && plan.NodeNames[index] != "" {
		return plan.NodeNames[index]
	}
	return plan.NodeNameMap[nodeID]
}

// SplitNamesAt returns the planned output names for a comma-split node occurrence.
func (plan NodeNamePlan) SplitNamesAt(index int) []string {
	if index < 0 || index >= len(plan.SplitNames) {
		return nil
	}
	return plan.SplitNames[index]
}

func (plan NodeNamePlan) setName(nodeIndex int, linkIndex int, nodeID int, name string) {
	if nodeIndex < 0 || nodeIndex >= len(plan.SplitNames) || linkIndex < 0 || linkIndex >= len(plan.SplitNames[nodeIndex]) {
		return
	}
	plan.SplitNames[nodeIndex][linkIndex] = name
	if linkIndex == 0 {
		plan.NodeNames[nodeIndex] = name
		if _, exists := plan.NodeNameMap[nodeID]; !exists {
			plan.NodeNameMap[nodeID] = name
		}
	}
}

func appendAirportSource(name string, node Node) string {
	name = strings.TrimSpace(name)
	source := strings.TrimSpace(node.Source)
	if node.SourceID <= 0 || source == "" || source == "manual" || name == "" {
		return name
	}
	return name + "@" + source
}

func splitNodeOutputLinks(link string) []string {
	if strings.Contains(link, ",") {
		return strings.Split(link, ",")
	}
	return []string{link}
}
