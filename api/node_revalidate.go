package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
	"strings"
	"sublink/models"
	"sublink/node"
	"sublink/node/protocol"
	"sublink/services"
	"sublink/utils"
	"time"
)

func validNodeValidationStatus(status string) bool {
	switch status {
	case "", "all", models.ValidationStatusUntested, models.ValidationStatusSuccess, models.ValidationStatusPartial, models.ValidationStatusFailed, models.ValidationStatusDisabled:
		return true
	}
	return false
}

// Selection is deliberately strict: IDs XOR all:true + filters. Resolve and
// snapshot before task creation, not while the worker progresses.
func resolveNodeRevalidationTargets(raw map[string]json.RawMessage) ([]models.Node, error) {
	for key := range raw {
		if key != "ids" && key != "all" && key != "filters" {
			return nil, errors.New("unknown selection field")
		}
	}
	var ids []int
	_, hasIDs := raw["ids"]
	_, hasAll := raw["all"]
	_, hasFilters := raw["filters"]
	if hasIDs {
		if hasAll || hasFilters {
			return nil, errors.New("ids and all/filters are mutually exclusive")
		}
		if err := json.Unmarshal(raw["ids"], &ids); err != nil {
			return nil, errors.New("invalid ids")
		}
	} else {
		var all bool
		if !hasAll || !hasFilters || json.Unmarshal(raw["all"], &all) != nil || !all {
			return nil, errors.New("select ids or all:true with filters")
		}
		var values map[string]json.RawMessage
		if err := json.Unmarshal(raw["filters"], &values); err != nil || values == nil {
			return nil, errors.New("invalid filters")
		}
		// Reuse the exact query decoder used by list/count/IDs, including array and
		// unlock-rule semantics. Unknown keys fail rather than silently widen scope.
		allowed := map[string]bool{}
		for _, key := range strings.Fields("search group source protocol speedStatus delayStatus sortBy sortOrder maxDelay minSpeed maxFraudScore countries countries[] tags tags[] residentialType ipType qualityStatus validationStatus unlockRuleMode unlockRules unlockProvider unlockStatus unlockKeyword excludeIds excludeIds[] onlyResidential onlyNative") {
			allowed[key] = true
		}
		query := url.Values{}
		for key, value := range values {
			if !allowed[key] {
				return nil, errors.New("unknown filter")
			}
			if key == "unlockRules" {
				var encoded string
				if json.Unmarshal(value, &encoded) == nil {
					query.Set(key, encoded)
				} else {
					var rules []models.UnlockFilterRule
					if json.Unmarshal(value, &rules) != nil {
						return nil, errors.New("invalid unlock rules")
					}
					query.Set(key, string(value))
				}
				continue
			}
			if strings.HasSuffix(key, "Status") {
				var status string
				if json.Unmarshal(value, &status) != nil {
					return nil, errors.New("invalid status filter")
				}
				accepted := false
				switch key {
				case "validationStatus":
					accepted = validNodeValidationStatus(status)
				case "qualityStatus":
					accepted = status == "" || status == "all" || normalizeQualityStatus(status) == status
				case "unlockStatus":
					accepted = status == "" || normalizeUnlockStatus(status) == status
				case "speedStatus", "delayStatus":
					switch status {
					case "", "all", "untested", "success", "timeout", "error":
						accepted = true
					}
				}
				if !accepted {
					return nil, errors.New("unknown status filter")
				}
			}
			var array []any
			if json.Unmarshal(value, &array) == nil && array != nil {
				if key == "countries" || key == "tags" || key == "excludeIds" {
					key += "[]"
				}
				for _, item := range array {
					query.Add(key, fmt.Sprint(item))
				}
			} else {
				var scalar any
				if json.Unmarshal(value, &scalar) != nil || scalar == nil {
					return nil, errors.New("invalid filter value")
				}
				switch scalar.(type) {
				case string, float64, bool:
					query.Set(key, fmt.Sprint(scalar))
				default:
					return nil, errors.New("invalid filter value")
				}
			}
		}
		if !validNodeValidationStatus(query.Get("validationStatus")) {
			return nil, errors.New("unknown validation status")
		}
		c := &gin.Context{Request: nil}
		// A synthetic request only supplies the query; no request lifetime escapes.
		c.Request = (&http.Request{URL: &url.URL{RawQuery: query.Encode()}})
		var n models.Node
		var err error
		ids, err = n.GetFilteredNodeIDs(buildNodeFilterFromQuery(c))
		if err != nil {
			return nil, err
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("empty node selection")
	}
	seen := map[int]bool{}
	unique := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, errors.New("invalid node ID")
		}
		if !seen[id] {
			unique = append(unique, id)
			seen[id] = true
		}
	}
	return models.NodeValidationSnapshots(unique)
}

func NodeRevalidate(c *gin.Context) {
	var raw map[string]json.RawMessage
	if c.ShouldBindJSON(&raw) != nil || raw == nil {
		utils.FailWithMsg(c, "invalid selection")
		return
	}
	snapshots, err := resolveNodeRevalidationTargets(raw)
	if err != nil {
		utils.FailWithMsg(c, err.Error())
		return
	}
	task, ctx, err := services.GetTaskManager().CreateTask(models.TaskTypeNodeRevalidate, "节点结构重验证", models.TaskTriggerManual, len(snapshots))
	if err != nil {
		utils.FailWithMsg(c, "创建重验证任务失败")
		return
	}
	utils.OkWithData(c, gin.H{"taskId": task.ID, "task": task, "total": len(snapshots)})
	go runNodeRevalidationTask(task.ID, ctx, snapshots)
}

// validateExistingNode never inserts, expands remote records, changes business
// metadata, or dials proxy nodes. Only remote subscriptions are safely fetched.
func validateExistingNode(ctx context.Context, n models.Node) (string, string) {
	if parsed, err := parseRemoteSubscriptionURL(n.Link); err == nil && parsed != nil && !protocol.IsHTTPLink(n.Link) {
		payload, err := fetchNodeSubscriptionPayload(ctx, n.Link, n.SkipTLSVerify)
		if err != nil {
			return models.ValidationStatusFailed, "subscription_fetch_failed"
		}
		parsed, err := node.ParseSubscriptionPayloadWithDiagnostics(payload)
		valid, invalid := 0, parsed.Invalid
		for _, candidate := range parsed.URLs {
			if ctx.Err() != nil {
				return models.ValidationStatusFailed, "cancelled"
			}
			if _, _, err := ValidateNodeLink(candidate); err == nil {
				valid++
			} else {
				invalid++
			}
		}
		if valid == 0 {
			return models.ValidationStatusFailed, "no_valid_nodes"
		}
		if err != nil || invalid > 0 || len(parsed.Diagnostics) > 0 || parsed.OmittedDiagnostics > 0 {
			return models.ValidationStatusPartial, "mixed_subscription_entries"
		}
		return models.ValidationStatusSuccess, "validated"
	}
	for _, link := range splitNodeAddLinks(n.Link) {
		if _, _, err := ValidateNodeLink(protocol.Urls{Url: link, ClashExtra: n.ClashExtra, DialerProxyName: n.DialerProxyName}); err != nil {
			return models.ValidationStatusFailed, "invalid_node_structure"
		}
	}
	if len(splitNodeAddLinks(n.Link)) == 0 {
		return models.ValidationStatusFailed, "empty_node_link"
	}
	return models.ValidationStatusSuccess, "validated"
}

// NodeRevalidationResult reuses import detail bounds but exposes truthful counters:
// validated counts updated existing rows; partial is included in validated.
type NodeRevalidationResult struct {
	Mode               string              `json:"mode"`
	Status             NodeAddStatus       `json:"status"`
	Total              int                 `json:"total"`
	Processed          int                 `json:"processed"`
	Validated          int                 `json:"validated"`
	Partial            int                 `json:"partial"`
	Skipped            int                 `json:"skipped"`
	Failed             int                 `json:"failed"`
	Items              []NodeAddItem       `json:"items,omitempty"`
	Diagnostics        []NodeAddDiagnostic `json:"diagnostics,omitempty"`
	Truncated          bool                `json:"truncated,omitempty"`
	OmittedItems       int                 `json:"omittedItems,omitempty"`
	OmittedDiagnostics int                 `json:"omittedDiagnostics,omitempty"`
}

func nodeRevalidationResult(result NodeAddResult, total, processed, partial int, details bool) NodeRevalidationResult {
	output := NodeRevalidationResult{Mode: "revalidate", Status: result.Status, Total: total, Processed: processed, Validated: result.Added, Partial: partial, Skipped: result.Skipped, Failed: result.Failed, Truncated: result.Truncated, OmittedItems: result.OmittedItems, OmittedDiagnostics: result.OmittedDiagnostics}
	if details {
		output.Items, output.Diagnostics = result.Items, result.Diagnostics
	}
	return output
}

func runNodeRevalidationTask(taskID string, ctx context.Context, snapshots []models.Node) {
	tm := services.GetTaskManager()
	result := NodeAddResult{Mode: "revalidate"}
	partial := 0
	defer func() {
		if recover() != nil {
			_ = tm.FailTask(taskID, "重验证任务异常终止")
		}
	}()
	for index, snapshot := range snapshots {
		if ctx.Err() != nil || tm.IsTaskCancelled(taskID) {
			return
		}
		status, reason := validateExistingNode(ctx, snapshot)
		// Cancellation during fetch/parse MUST NOT persist a synthetic failure.
		if ctx.Err() != nil || tm.IsTaskCancelled(taskID) {
			return
		}
		applied, err := models.ApplyNodeValidationSnapshot(snapshot, status, reason, time.Now().UTC().Format(time.RFC3339Nano))
		item := NodeAddItem{NodeID: snapshot.ID, Status: NodeAddItemStatus(status)}
		switch {
		case err != nil:
			appendNodeAddFailure(&result, "", "", NodeAddReasonPersistenceError, errors.New("validation persistence failed"))
		case !applied:
			result.Skipped++
			item.Status = NodeAddItemSkipped
			result.Items = append(result.Items, item)
		case status == models.ValidationStatusFailed:
			result.Failed++
			item.Reason = NodeAddReasonInvalidInput
			result.Items = append(result.Items, item)
			result.Diagnostics = append(result.Diagnostics, NodeAddDiagnostic{Code: NodeAddReasonInvalidInput, Message: reason})
		default:
			result.Added++ // Existing bounded result counter means successfully revalidated; no rows added.
			if status == models.ValidationStatusPartial {
				partial++
				result.Warnings++
				result.Diagnostics = append(result.Diagnostics, NodeAddDiagnostic{Code: NodeAddReasonParseError, Message: reason})
			}
			result.Items = append(result.Items, item)
		}
		boundNodeAddResult(&result)
		classifyNodeAddResult(&result)
		summary := nodeRevalidationResult(result, len(snapshots), index+1, partial, false)
		if err := tm.UpdateProgress(taskID, index+1, fmt.Sprintf("节点 #%d", snapshot.ID), summary); err != nil {
			_ = tm.FailTask(taskID, "重验证进度写入失败")
			return
		}
	}
	if ctx.Err() == nil && !tm.IsTaskCancelled(taskID) {
		_ = tm.CompleteTask(taskID, "节点结构重验证完成（非测速）", nodeRevalidationResult(result, len(snapshots), len(snapshots), partial, true))
	}
}
