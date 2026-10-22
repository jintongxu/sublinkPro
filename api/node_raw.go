package api

import (
	"encoding/json"
	"errors"
	"strconv"
	"sublink/models"
	"sublink/node/protocol"
	"sublink/utils"

	"github.com/gin-gonic/gin"
)

// GetProtocolUIMeta 获取协议 UI 元数据（包含颜色、图标等）
// GET /api/v1/nodes/protocol-ui-meta
func GetProtocolUIMeta(c *gin.Context) {
	metas := protocol.GetAllProtocolMeta()
	utils.OkWithData(c, metas)
}

// ParseNodeLinkAPI 解析节点链接
// GET /api/v1/nodes/parse-link?link=xxx
func ParseNodeLinkAPI(c *gin.Context) {
	link := c.Query("link")
	if link == "" {
		utils.FailWithMsg(c, "链接不能为空")
		return
	}

	info, err := protocol.ParseNodeLink(link)
	if err != nil {
		utils.FailWithMsg(c, err.Error())
		return
	}

	utils.OkWithData(c, info)
}

// UpdateNodeRawRequest 更新节点原始信息请求
type UpdateNodeRawRequest struct {
	NodeID int            `json:"nodeId"` // 节点 ID
	Fields map[string]any `json:"fields"` // 要更新的字段
}

// UpdateNodeRawInfo 更新节点原始信息
// POST /api/v1/nodes/update-raw
func UpdateNodeRawInfo(c *gin.Context) {
	var req UpdateNodeRawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.FailWithMsg(c, "请求参数错误")
		return
	}

	if req.NodeID <= 0 {
		utils.FailWithMsg(c, "节点ID无效")
		return
	}

	// 获取节点
	var node models.Node
	node.ID = req.NodeID
	if err := node.GetByID(); err != nil {
		utils.FailWithMsg(c, "节点不存在")
		return
	}

	// 将字段转为 JSON
	fieldsJSON, err := json.Marshal(req.Fields)
	if err != nil {
		utils.FailWithMsg(c, "字段序列化失败")
		return
	}

	// 更新链接
	newLink, err := protocol.UpdateNodeLinkFields(node.Link, string(fieldsJSON))
	if err != nil {
		utils.FailWithMsg(c, err.Error())
		return
	}

	conflictNode, conflict, err := models.FindNodeLinkConflict(newLink, req.NodeID)
	if err != nil {
		utils.FailWithMsg(c, "检查节点冲突失败")
		return
	}
	if conflict {
		utils.FailWithMsg(c, "已存在相同连接的节点: "+conflictNode.Name)
		return
	}

	// 解析新链接以获取可能更新的名称
	newInfo, err := protocol.ParseNodeLink(newLink)
	if err != nil {
		utils.FailWithMsg(c, "解析新链接失败")
		return
	}

	// 获取节点名称（不同协议名称字段不同）
	newLinkName := protocol.ExtractNodeNameFromFields(newInfo.Protocol, newInfo.Fields)

	// 更新数据库
	updates := map[string]any{
		"link": newLink,
	}
	if newLinkName != "" {
		updates["link_name"] = newLinkName
		// link 模式或历史“备注=原始名称”的节点继续同步备注；remark 模式下保留用户自定义备注。
		if node.ShouldSyncNameFromLink() {
			updates["name"] = newLinkName
		}
	}

	if err := models.UpdateNodeFields(req.NodeID, updates); err != nil {
		if errors.Is(err, models.ErrNodeNameExists) {
			utils.FailWithMsg(c, "节点备注名称已存在，请换一个备注")
			return
		}
		utils.FailWithMsg(c, "更新数据库失败")
		return
	}

	// UpdateNodeFields owns the DB/cache write; never replay a stale local snapshot.

	utils.OkWithData(c, gin.H{
		"link":     newLink,
		"linkName": newLinkName,
	})
}

// GetNodeRawInfo 获取节点原始信息
// GET /api/v1/nodes/raw-info?id=xxx
func GetNodeRawInfo(c *gin.Context) {
	idStr := c.Query("id")
	if idStr == "" {
		utils.FailWithMsg(c, "节点ID不能为空")
		return
	}

	nodeID, err := strconv.Atoi(idStr)
	if err != nil {
		utils.FailWithMsg(c, "节点ID格式错误")
		return
	}

	// 获取节点
	var node models.Node
	node.ID = nodeID
	if err := node.GetByID(); err != nil {
		utils.FailWithMsg(c, "节点不存在")
		return
	}

	// 解析链接
	info, err := protocol.ParseNodeLink(node.Link)
	if err != nil {
		utils.FailWithMsg(c, err.Error())
		return
	}

	utils.OkWithData(c, info)
}
