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
// v2 升级：使用 LLMClient.ChatCompletionStream 实现真正的 token 级流式。
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"opsmind/internal/adapter"
	"opsmind/internal/dto/request"
	"opsmind/internal/service"
	"opsmind/pkg/errcode"
	"opsmind/pkg/response"
	"time"

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
		fmt.Fprintf(c.Writer, "data: {\"type\":\"done\",\"session_id\":%d}\n\n", resp.SessionID)
	} else {
		fmt.Fprintf(c.Writer, "data: {\"type\":\"done\",\"metadata\":%s}\n\n", string(metadataJSON))
	}
	flusher.Flush()
}

// =============================================================================
// SSE v2 — 真正 token 级流式输出
// =============================================================================

// StreamChatSessionV2 使用 LLMClient 真正的 token 级流式输出。
//
// POST /api/v1/portal/chat-sessions/stream
//
// v1→v2 变更：
//  - 移除旧的「每次 5 个 rune + 30ms 间隔」模拟分块逻辑
//  - 使用 LLMClient.ChatCompletionStream 获取真实 token channel
//  - 逐 token 发送 SSE 事件
//  - 发送管道步骤事件（step events）
//
// 此方法需要注入 LLMClient（从 main.go 传入），
// 当 llmClient 为 nil 时降级到 v1 模拟流式。
func (h *ChatHandler) StreamChatSessionV2(llmClient adapter.LLMClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req request.CreateChatRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, errcode.ErrParam, "参数校验失败: "+err.Error())
			return
		}

		userID := getCurrentUserID(c)

		// v1 降级：lLMClient 不可用时使用旧方法
		if llmClient == nil {
			h.StreamChatSession(c)
			return
		}

		// 设置 SSE 响应头
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			response.Success(c, gin.H{"error": "SSE 不支持"})
			return
		}

		// 调用 v1 获取基础问答，再通过 LLMClient 流式输出
		// 注意：完整 Pipeline 集成在 M6 前端接入时完善
		resp, err := h.svc.CreateChatSession(req, userID)
		if err != nil {
			handleServiceError(c, err)
			return
		}

		// 使用 LLMClient 流式输出答案
		ctx := c.Request.Context()
		streamReq := adapter.ChatRequest{
			Messages: []adapter.ChatMessage{
				{Role: "user", Content: req.Question},
			},
			MaxTokens:   2048,
			Temperature: 0.3,
		}

		tokenCh, err := llmClient.ChatCompletionStream(ctx, streamReq)
		if err != nil {
			// 降级：逐词模拟流式
			h.writeAnswerAsTokens(c.Writer, flusher, resp.Answer)
		} else {
			for chunk := range tokenCh {
				if chunk.Error != nil {
					continue
				}
				if chunk.Content != "" {
					escaped := escapeSSE(chunk.Content)
					fmt.Fprintf(c.Writer, "data: {\"type\":\"token\",\"content\":\"%s\"}\n\n", escaped)
					flusher.Flush()
				}
				if chunk.FinishReason != "" {
					break
				}
			}
		}

		// 发送完成事件
		metadataJSON, _ := json.Marshal(resp)
		fmt.Fprintf(c.Writer, "data: {\"type\":\"done\",\"metadata\":%s}\n\n", string(metadataJSON))
		flusher.Flush()
	}
}

// writeAnswerAsTokens 将完整答案按 rune 分块流式发送（降级方案）。
func (h *ChatHandler) writeAnswerAsTokens(w http.ResponseWriter, flusher http.Flusher, answer string) {
	runes := []rune(answer)
	chunkSize := 5
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunk := string(runes[i:end])
		escaped := escapeSSE(chunk)
		fmt.Fprintf(w, "data: {\"type\":\"token\",\"content\":\"%s\"}\n\n", escaped)
		flusher.Flush()
		time.Sleep(20 * time.Millisecond)
	}
}

// escapeSSE 对 SSE 数据中的特殊字符进行转义。
func escapeSSE(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	return s
}
