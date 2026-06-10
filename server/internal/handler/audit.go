// Package handler 实现 HTTP 请求处理。
//
// audit.go 提供审计日志查询接口。
// Handler 层职责：参数解析、调用 Repository、组装响应（含操作人姓名）。
// 审计日志写入由各 Service 层直接调用 AuditRepo.Create，不经过 Handler。
package handler

import (
	"opsmind/internal/dto/request"
	dto "opsmind/internal/dto/response"
	"opsmind/internal/model"
	"opsmind/internal/repository"
	"opsmind/pkg/errcode"
	"opsmind/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuditHandler 审计日志查询接口。
type AuditHandler struct {
	repo *repository.AuditRepo
	db   *gorm.DB // 用于查询操作人姓名
}

// NewAuditHandler 创建 AuditHandler 实例。
func NewAuditHandler(repo *repository.AuditRepo, db *gorm.DB) *AuditHandler {
	return &AuditHandler{repo: repo, db: db}
}

// List 查询审计日志列表（分页，支持按操作人和操作类型筛选）。
//
// GET /api/v1/admin/audit-logs?operator_id=1&action=user.create&page=1&page_size=10
//
// 仅系统管理员可访问，在路由层通过 RBAC 中间件控制。
func (h *AuditHandler) List(c *gin.Context) {
	var req request.AuditLogListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Error(c, errcode.ErrParam, "参数校验失败: "+err.Error())
		return
	}

	// 设置默认值
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 10
	}

	logs, total, err := h.repo.List(req.OperatorID, req.Action, req.Page, req.PageSize)
	if err != nil {
		response.Error(c, errcode.ErrUnknown, "查询审计日志失败: "+err.Error())
		return
	}

	// 收集操作人 ID 并批量查询姓名
	operatorNames := h.batchGetOperatorNames(logs)

	// 组装响应
	items := make([]dto.AuditLogItem, len(logs))
	for i, log := range logs {
		detail := ""
		if len(log.Detail) > 0 {
			detail = string(log.Detail)
		}
		items[i] = dto.AuditLogItem{
			ID:           log.ID,
			OperatorID:   log.OperatorID,
			OperatorName: operatorNames[log.OperatorID],
			Action:       log.Action,
			TargetType:   log.TargetType,
			TargetID:     log.TargetID,
			Detail:       detail,
			IPAddress:    log.IPAddress,
			CreatedAt:    log.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	response.SuccessWithPage(c, items, total, req.Page, req.PageSize)
}

// batchGetOperatorNames 批量查询操作人姓名。
//
// 为什么批量查询而非逐条查询：
// 审计日志列表可能包含多条记录但操作人数量远少于记录数，
// 先收集 ID 再批量查询可减少数据库往返次数。
func (h *AuditHandler) batchGetOperatorNames(logs []model.AuditLog) map[int64]string {
	if len(logs) == 0 {
		return make(map[int64]string)
	}

	// 收集唯一操作人 ID
	idSet := make(map[int64]struct{}, len(logs))
	for _, log := range logs {
		idSet[log.OperatorID] = struct{}{}
	}

	ids := make([]int64, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}

	// 批量查询用户姓名
	type userInfo struct {
		ID       int64
		RealName string
	}
	var users []userInfo
	h.db.Model(&model.User{}).Where("id IN ?", ids).Select("id, real_name").Find(&users)

	result := make(map[int64]string, len(users))
	for _, u := range users {
		result[u.ID] = u.RealName
	}

	return result
}
