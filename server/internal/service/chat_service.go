// Package service 实现智能问答业务逻辑。
//
// v2 迁移说明：ChatService v1（依赖 AnythingLLM RagClient）已被 ChatServiceV2 替代。
// 本文件保留 CreateChatSession / SubmitFeedback / GetChatDetail 方法签名以确保
// Handler 层编译通过，但 RagClient 依赖已移除——运行时实际由 ChatServiceV2 提供服务。
//
// TODO M7: 完成 Handler 层到 ChatServiceV2 的切换后，可删除本文件。
package service

import (
	"encoding/json"
	"strings"
	"time"

	"opsmind/internal/dto/request"
	"opsmind/internal/dto/response"
	"opsmind/internal/model"
	"opsmind/internal/repository"
	"opsmind/pkg/errcode"
)

const (
	// 置信度阈值：低于此值判定为低置信度，引导用户提交申告。
	// MVP 阶段硬编码，后续从 system_configs 表读取。
	defaultConfidenceThreshold = 0.6

	// 降级兜底文本（与 ANYTHINGLLM_AI_INTEGRATION.md §7.1 对齐）
	fallbackLowConfidence = "暂未找到足够匹配的知识，建议提交申告由运维人员人工处理"
	fallbackAIUnavailable  = "当前 AI 服务暂不可用，请提交申告由人工处理"
)

// ChatService 智能问答服务（v1 占位，已由 ChatServiceV2 替代）。
type ChatService struct {
	knowledgeRepo *repository.KnowledgeRepo
	chatRepo      *repository.ChatRepo
}

// NewChatService 创建 ChatService 实例（v1 占位）。
func NewChatService(knowledgeRepo *repository.KnowledgeRepo, chatRepo *repository.ChatRepo) *ChatService {
	return &ChatService{
		knowledgeRepo: knowledgeRepo,
		chatRepo:      chatRepo,
	}
}

// =============================================================================
// CreateChatSession
// =============================================================================

// CreateChatSession 创建问答会话（v1 占位，v2 中由 ChatServiceV2 替代）。
//
// v1 依赖 RagClient.Query（AnythingLLM），v2 已迁移到自建 RAG 管道。
func (s *ChatService) CreateChatSession(req request.CreateChatRequest, userID int64) (*response.ChatSessionResponse, error) {
	// 参数校验
	if strings.TrimSpace(req.Question) == "" {
		return nil, AppError{Code: errcode.ErrParam, Message: "问题不能为空"}
	}

	// 查询知识库
	if s.knowledgeRepo == nil {
		return nil, AppError{Code: errcode.ErrAIUnavailable, Message: fallbackAIUnavailable}
	}
	_, err := s.knowledgeRepo.FindKBByID(req.KBID)
	if err != nil {
		return nil, AppError{Code: errcode.ErrNotFound, Message: "知识库不存在"}
	}

	// v2: RagClient 已移除，由 ChatServiceV2 + Pipeline 提供 RAG 能力。
	// 本方法仅作占位——实际通过 SSE 流式端点（StreamChatSession）调用 LLMClient。
	now := time.Now()
	answer := fallbackAIUnavailable

	session := &model.ChatSession{
		UserID:     userID,
		KBID:       req.KBID,
		Question:   req.Question,
		Answer:     answer,
		Confidence: 0,
		DurationMs: 0,
	}
	if s.chatRepo != nil {
		if err := s.chatRepo.Create(session); err != nil {
			return nil, AppError{Code: errcode.ErrUnknown, Message: "保存会话失败"}
		}
	}

	return &response.ChatSessionResponse{
		SessionID:       session.ID,
		Question:        req.Question,
		Answer:          answer,
		Confidence:      0,
		CanSubmitTicket: true,
		DurationMS:      0,
		Feedback:        0,
		CreatedAt:       now.Format("2006-01-02 15:04:05"),
	}, nil
}

// =============================================================================
// SubmitFeedback
// =============================================================================

// SubmitFeedback 提交问答反馈。
func (s *ChatService) SubmitFeedback(sessionID int64, feedback int16) error {
	if s.chatRepo == nil {
		return AppError{Code: errcode.ErrUnknown, Message: "服务未初始化"}
	}
	if _, err := s.chatRepo.FindByID(sessionID); err != nil {
		return AppError{Code: errcode.ErrNotFound, Message: "会话不存在"}
	}
	return s.chatRepo.UpdateFeedback(sessionID, feedback)
}

// GetChatDetail 查询问答会话详情。
func (s *ChatService) GetChatDetail(sessionID int64) (*response.ChatSessionResponse, error) {
	if s.chatRepo == nil {
		return nil, AppError{Code: errcode.ErrUnknown, Message: "服务未初始化"}
	}
	session, err := s.chatRepo.FindByID(sessionID)
	if err != nil {
		return nil, AppError{Code: errcode.ErrNotFound, Message: "会话不存在"}
	}

	var sources []response.SourceItem
	if len(session.Sources) > 0 {
		json.Unmarshal(session.Sources, &sources)
	}

	return &response.ChatSessionResponse{
		SessionID:       session.ID,
		Question:        session.Question,
		Answer:          session.Answer,
		Sources:         sources,
		Confidence:      session.Confidence,
		CanSubmitTicket: session.Confidence < defaultConfidenceThreshold,
		DurationMS:      session.DurationMs,
		Feedback:        session.Feedback,
		CreatedAt:       session.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}
