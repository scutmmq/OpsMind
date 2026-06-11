// Package handler 实现 HTTP 请求处理。
//
// chat.go 提供智能问答相关接口（含 SSE 流式输出）。
// Handler 层职责：参数解析、调用 Service、格式化响应。
// 置信度判断和降级逻辑在 Service 层完成。
//
// 流式输出设计决策：
// 为什么在 Handler 层做 SSE 流式而非 Service 层：
// SSE 是 HTTP 协议层面的传输方式，属于表示层关注点。Service 层返回完整业务结果，
// Handler 层决定以 JSON 还是 SSE 方式交付给客户端，符合单一职责原则。
// 后续如果 AnythingLLM 原生支持流式，可在 RagClient 层实现真正的 token 级别流式。
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"opsmind/internal/dto/request"
	"opsmind/internal/service"
	"opsmind/pkg/errcode"
	"opsmind/pkg/response"

	"github.com/gin-gonic/gin"
)

// ChatHandler 智能问答接口。
type ChatHandler struct {
	svc *service.ChatService
}

// NewChatHandler 创建 ChatHandler 实例。
func NewChatHandler(svc *service.ChatService) *ChatHandler {
	return &ChatHandler{svc: svc}
}

// =============================================================================
// 门户端
// =============================================================================

// CreateChatSession 创建问答会话。
//
// POST /api/v1/portal/chat-sessions
func (h *ChatHandler) CreateChatSession(c *gin.Context) {
	var req request.CreateChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, errcode.ErrParam, "参数校验失败: "+err.Error())
		return
	}

	userID := getCurrentUserID(c)
	resp, err := h.svc.CreateChatSession(req, userID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	response.Success(c, resp)
}

// SubmitFeedback 提交问答反馈。
//
// POST /api/v1/portal/chat-sessions/:id/feedback
func (h *ChatHandler) SubmitFeedback(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.Error(c, errcode.ErrParam, "无效的会话 ID")
		return
	}

	// 解析反馈值（int16: 0=未评价, 1=已解决, 2=未解决）
	var body struct {
		Feedback int16 `json:"feedback"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, errcode.ErrParam, "参数校验失败: "+err.Error())
		return
	}

	if err := h.svc.SubmitFeedback(id, body.Feedback); err != nil {
		handleServiceError(c, err)
		return
	}

	response.Success(c, nil)
}

// GetChatDetail 查询问答会话详情。
//
// GET /api/v1/portal/chat-sessions/:id
func (h *ChatHandler) GetChatDetail(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.Error(c, errcode.ErrParam, "无效的会话 ID")
		return
	}

	resp, err := h.svc.GetChatDetail(id)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	response.Success(c, resp)
}

// =============================================================================
// SSE 流式输出
// =============================================================================

// StreamChatSession 创建问答会话并以 SSE 流式返回答案。
//
// POST /api/v1/portal/chat-sessions/stream
//
// 流式输出流程：
//  1. 解析请求参数并调用 ChatService.CreateChatSession 获取完整答案
//  2. 设置 SSE 响应头（text/event-stream）
//  3. 以字符块（每次 5 个 rune）流式发送答案文本
//  4. 流式发送完成后，发送 done 事件（含 session_id、sources、confidence 等元数据）
//  5. 发送期间检测客户端断开，及时终止
//
// 为什么在 Handler 层而非 Service 层做流式：
// SSE 是 HTTP 传输层关注点。Service 层返回完整业务结果，
// Handler 层决定以 JSON 还是 SSE 交付，符合单一职责原则。
// 后续 RagClient 支持原生流式时，可在 Service 层引入 channel 回调，
// Handler 层无需变更。
func (h *ChatHandler) StreamChatSession(c *gin.Context) {
	var req request.CreateChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, errcode.ErrParam, "参数校验失败: "+err.Error())
		return
	}

	userID := getCurrentUserID(c)

	// 调用 Service 层获取完整答案（业务逻辑不变）
	resp, err := h.svc.CreateChatSession(req, userID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	// 设置 SSE 响应头
	// X-Accel-Buffering: no 用于防止 nginx 缓冲 SSE 事件
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	// 检测是否支持 Flusher（所有主流 HTTP 实现都支持）
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		// 不支持的场景降级为普通 JSON 响应
		response.Success(c, resp)
		return
	}

	// 将答案文本按 rune 分块流式发送
	// 为什么每次 5 个 rune 而非按词分割：
	// 中文无空格分隔，按 rune 分块可以通用处理中英文混合场景
	// 30ms 间隔模拟人类阅读节奏，减少闪烁感
	answer := resp.Answer
	runes := []rune(answer)
	chunkSize := 5
	for i := 0; i < len(runes); i += chunkSize {
		// 检测客户端断开连接
		select {
		case <-c.Request.Context().Done():
			return
		default:
		}

		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunk := string(runes[i:end])

		// 对 JSON 字符串中的特殊字符做转义
		escaped := strings.ReplaceAll(chunk, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		escaped = strings.ReplaceAll(escaped, "\n", `\n`)
		escaped = strings.ReplaceAll(escaped, "\r", `\r`)

		fmt.Fprintf(c.Writer, "data: {\"type\":\"token\",\"content\":\"%s\"}\n\n", escaped)
		flusher.Flush()

		// 模拟流式阅读节奏，30ms 间隔近似人类逐句阅读速度
		time.Sleep(30 * time.Millisecond)
	}

	// 发送完成事件（含完整元数据）
	metadataJSON, err := json.Marshal(resp)
	if err != nil {
		// JSON 序列化失败：发送简化版 done 事件
		fmt.Fprintf(c.Writer, "data: {\"type\":\"done\",\"session_id\":%d}\n\n", resp.SessionID)
	} else {
		fmt.Fprintf(c.Writer, "data: {\"type\":\"done\",\"metadata\":%s}\n\n", string(metadataJSON))
	}
	flusher.Flush()
}
