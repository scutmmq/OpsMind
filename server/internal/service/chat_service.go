// Package service 实现智能问答业务逻辑。
//
// 会话与对话分离：CreateSession 仅创建容器，StreamChat 在已有会话中流式返回 AI 答案。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"opsmind/internal/adapter"
	"opsmind/internal/dto/request"
	"opsmind/internal/dto/response"
	"opsmind/internal/model"
	"opsmind/internal/rag"
	"opsmind/pkg/errcode"

	"gorm.io/gorm"
)

const (
	defaultConfidenceThreshold = 0.6
	fallbackLowConfidence      = "暂未找到足够匹配的知识，建议提交申告由运维人员人工处理"
	fallbackAIUnavailable      = "当前 AI 服务暂不可用，请提交申告由人工处理"
)

// 消费者接口——ChatService 仅暴露它实际使用的依赖方法，
// 遵循 Go "accept interfaces, return structs" 惯例，便于测试 mock。
type chatKnowledgeRepo interface {
	FindKBByID(ctx context.Context, id int64) (*model.KnowledgeBase, error)
}

type chatSessionRepo interface {
	Create(ctx context.Context, session *model.ChatSession) error
	CreateBatch(ctx context.Context, messages []model.ChatMessage) error
	CreateMessage(ctx context.Context, m *model.ChatMessage) error
	UpdateMessage(ctx context.Context, m *model.ChatMessage) error
	MarkGeneratingFailed(ctx context.Context) (int64, error)
	FindByID(ctx context.Context, id int64) (*model.ChatSession, error)
	FindMessagesBySession(ctx context.Context, sessionID int64) ([]model.ChatMessage, error)
	UpdateFeedback(ctx context.Context, id int64, feedback int16) error
	UpdateSession(ctx context.Context, session *model.ChatSession) error
	ListByUser(ctx context.Context, userID int64, page, pageSize int) ([]model.ChatSession, int64, error)
	DeleteSession(ctx context.Context, id, userID int64) error
	CountMessagesBySession(ctx context.Context, sessionID int64) (int64, error)
	CountMessagesBySessions(ctx context.Context, sessionIDs []int64) (map[int64]int64, error)
}

// RAGDefaults RAG 管道默认开关（从 env 配置读取）。
type RAGDefaults struct {
	TopK         int
	QueryRewrite bool
	MultiRoute   bool
	Hybrid       bool
	Rerank       bool
}

// ragConfigReader ChatService 需要的运行时配置读取能力。
type ragConfigReader interface {
	GetInt(ctx context.Context, key string) (int, bool)
	GetFloat(ctx context.Context, key string) (float64, bool)
	GetBool(ctx context.Context, key string) (bool, bool)
}

// ChatService 智能问答服务。
type ChatService struct {
	ragDefaults   RAGDefaults
	configReader  ragConfigReader // 运行时读取 DB 配置覆盖 env 默认值
	knowledgeRepo chatKnowledgeRepo
	chatRepo      chatSessionRepo
	llmService    *LLMService
	hub           *GenerationHub
}

// NewChatService 创建 ChatService 实例。
func NewChatService(knowledgeRepo chatKnowledgeRepo, chatRepo chatSessionRepo, llmService *LLMService, ragDefaults RAGDefaults, configReader ragConfigReader, hub *GenerationHub) *ChatService {
	if ragDefaults.TopK <= 0 {
		ragDefaults.TopK = 5
	}
	return &ChatService{
		knowledgeRepo: knowledgeRepo,
		chatRepo:      chatRepo,
		llmService:    llmService,
		ragDefaults:   ragDefaults,
		configReader:  configReader,
		hub:           hub,
	}
}

// =============================================================================
// CreateSession — 创建会话容器
// =============================================================================

// CreateSession 创建问答会话（仅创建容器，不含 LLM 调用）。
// 与 StreamChat 分离的原因是：会话生命周期与 AI 调用解耦，避免 LLM 超时阻塞 HTTP 请求。
func (s *ChatService) CreateSession(ctx context.Context, req request.CreateSessionRequest, userID int64) (*model.ChatSession, error) {
	if s.knowledgeRepo != nil {
		if _, err := s.knowledgeRepo.FindKBByID(ctx, req.KBID); err != nil {
			return nil, errcode.AppError{Code: errcode.ErrNotFound, Message: "知识库不存在"}
		}
	}
	if s.chatRepo == nil {
		return nil, errcode.AppError{Code: errcode.ErrUnknown, Message: "服务未初始化"}
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "新会话"
	}

	session := &model.ChatSession{
		UserID:   userID,
		KBID:     req.KBID,
		Question: title,
	}
	if err := s.chatRepo.Create(ctx, session); err != nil {
		return nil, errcode.AppError{Code: errcode.ErrUnknown, Message: "创建会话失败"}
	}

	return session, nil
}

// =============================================================================
// StreamChat — 流式对话（在已有会话中）
// =============================================================================

// StreamChat 发起一次新生成：立即落库用户消息、建 generating 的 assistant 消息，
// 在 context.Background() 跑生成（脱离请求 ctx，客户端断开不影响），
// 返回 Hub 订阅（replay+实时）。完成时由后台 goroutine 落库 assistant 终稿。
// routeCount/rerankCount 为 0 时使用 RAG 管道默认值。
func (s *ChatService) StreamChat(ctx context.Context, sessionID int64, question string, userID int64, routeCount, rerankCount int) ([]StreamEvent, <-chan StreamEvent, func(), error) {
	if strings.TrimSpace(question) == "" {
		return nil, nil, nil, errcode.AppError{Code: errcode.ErrParam, Message: "问题不能为空"}
	}
	if s.llmService == nil {
		return nil, nil, nil, errcode.AppError{Code: errcode.ErrAIUnavailable, Message: fallbackAIUnavailable}
	}
	if s.chatRepo == nil {
		return nil, nil, nil, errcode.AppError{Code: errcode.ErrUnknown, Message: "服务未初始化"}
	}

	// 加载会话并校验归属
	session, err := s.chatRepo.FindByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, errcode.AppError{Code: errcode.ErrNotFound, Message: "会话不存在"}
		}
		return nil, nil, nil, errcode.AppError{Code: errcode.ErrUnknown, Message: "加载会话失败，请稍后重试"}
	}
	if session.UserID != userID {
		return nil, nil, nil, errcode.AppError{Code: errcode.ErrForbidden, Message: "无权访问该会话"}
	}

	// 加载历史消息（用于 LLM 上下文 + RAG 查询改写消歧）
	var history []adapter.ChatMessage
	msgs, msgErr := s.chatRepo.FindMessagesBySession(ctx, sessionID)
	if msgErr != nil {
		slog.Warn("加载会话历史消息失败，多轮上下文降级为单轮", "session_id", sessionID, "error", msgErr)
	}
	for _, m := range msgs {
		history = append(history, adapter.ChatMessage{Role: m.Role, Content: m.Content})
	}

	// 构建 RAG 查询改写所需的对话历史（格式：[]map[string]string）
	var ragHistory []map[string]string
	for _, m := range msgs {
		if m.Role == "user" || m.Role == "assistant" {
			ragHistory = append(ragHistory, map[string]string{"role": m.Role, "content": m.Content})
		}
	}

	// RAG 管道选项：env 默认值 → DB 配置覆盖 → 请求级参数
	opts := s.buildRAGOptions(routeCount, rerankCount, ragHistory)

	// 立即落库用户消息（解决导航/刷新后用户消息丢失）
	if err := s.chatRepo.CreateMessage(ctx, &model.ChatMessage{
		SessionID: sessionID, Role: "user", Content: question, Status: model.MessageStatusCompleted,
	}); err != nil {
		return nil, nil, nil, errcode.AppError{Code: errcode.ErrUnknown, Message: "保存用户消息失败"}
	}

	// 建 generating 的 assistant 占位消息，拿到 msgID
	assistant := &model.ChatMessage{SessionID: sessionID, Role: "assistant", Content: "", Status: model.MessageStatusGenerating}
	if err := s.chatRepo.CreateMessage(ctx, assistant); err != nil {
		return nil, nil, nil, errcode.AppError{Code: errcode.ErrUnknown, Message: "创建回复占位失败"}
	}

	// 脱离请求 ctx：用 background + 独立超时
	gctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	if err := s.hub.Start(sessionID, assistant.ID, cancel); err != nil {
		cancel()
		// 标记占位失败，避免残留 generating
		assistant.Status = model.MessageStatusFailed
		_ = s.chatRepo.UpdateMessage(context.Background(), assistant)
		return nil, nil, nil, errcode.AppError{Code: errcode.ErrParam, Message: err.Error()}
	}

	go s.runGeneration(gctx, sessionID, assistant.ID, question, session.KBID, opts, history)

	replay, ch, unsub, ok := s.hub.Subscribe(sessionID, 0)
	if !ok {
		return nil, nil, nil, errcode.AppError{Code: errcode.ErrUnknown, Message: "订阅生成失败"}
	}
	return replay, ch, unsub, nil
}

// runGeneration 在后台跑 RAG 管道，逐事件 Publish 到 Hub；完成/失败时落库并 Finish。
func (s *ChatService) runGeneration(gctx context.Context, sessionID, msgID int64, question string, kbID int64, opts rag.RAGOptions, history []adapter.ChatMessage) {
	defer s.hub.Finish(sessionID)

	llmEvents, err := s.llmService.StreamChat(gctx, question, kbID, opts, history)
	if err != nil {
		s.hub.Publish(sessionID, StreamEvent{Type: "error", Error: err.Error()})
		s.failAssistant(msgID)
		return
	}

	var answer string
	for evt := range llmEvents {
		if evt.Type == "token" {
			answer += evt.Content
		}
		if evt.Type == "done" && evt.Metadata != nil {
			srcJSON, _ := json.Marshal(evt.Metadata.Sources)
			pipelineJSON, _ := json.Marshal(evt.Metadata.Pipeline)
			_ = s.chatRepo.UpdateSession(context.Background(), &model.ChatSession{
				ID: sessionID, Answer: evt.Metadata.Answer, Sources: srcJSON,
				Confidence: evt.Metadata.Confidence, DurationMs: evt.Metadata.DurationMS,
			})
			_ = s.chatRepo.UpdateMessage(context.Background(), &model.ChatMessage{
				ID: msgID, Content: evt.Metadata.Answer, Sources: srcJSON,
				PipelineMetrics: pipelineJSON, Confidence: evt.Metadata.Confidence,
				Status: model.MessageStatusCompleted,
			})
			evt.Metadata.SessionID = sessionID
			evt.Metadata.Question = question
			evt.Metadata.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
		}
		if evt.Type == "error" {
			s.failAssistant(msgID)
		}
		s.hub.Publish(sessionID, evt)
	}

	// gctx 被取消（用户停止/超时）：落库当前已生成内容为 failed
	if gctx.Err() != nil {
		_ = s.chatRepo.UpdateMessage(context.Background(), &model.ChatMessage{
			ID: msgID, Content: answer, Status: model.MessageStatusFailed,
		})
		s.hub.Publish(sessionID, StreamEvent{Type: "error", Error: "生成已停止"})
	}
}

// failAssistant 将 assistant 消息标记为 failed，用于生成失败时清理占位状态。
func (s *ChatService) failAssistant(msgID int64) {
	_ = s.chatRepo.UpdateMessage(context.Background(), &model.ChatMessage{ID: msgID, Status: model.MessageStatusFailed})
}

// ResumeStream 续传：校验会话归属后从 since 订阅 Hub。无活跃生成则返回 ErrNotFound。
func (s *ChatService) ResumeStream(ctx context.Context, sessionID, userID int64, since int) ([]StreamEvent, <-chan StreamEvent, func(), error) {
	session, err := s.chatRepo.FindByID(ctx, sessionID)
	if err != nil {
		return nil, nil, nil, errcode.AppError{Code: errcode.ErrNotFound, Message: "会话不存在"}
	}
	if session.UserID != userID {
		return nil, nil, nil, errcode.AppError{Code: errcode.ErrForbidden, Message: "无权访问该会话"}
	}
	replay, ch, unsub, ok := s.hub.Subscribe(sessionID, since)
	if !ok {
		return nil, nil, nil, errcode.AppError{Code: errcode.ErrNotFound, Message: "无进行中的生成"}
	}
	return replay, ch, unsub, nil
}

// CancelGeneration 校验归属后真正取消后端生成。
func (s *ChatService) CancelGeneration(ctx context.Context, sessionID, userID int64) error {
	session, err := s.chatRepo.FindByID(ctx, sessionID)
	if err != nil {
		return errcode.AppError{Code: errcode.ErrNotFound, Message: "会话不存在"}
	}
	if session.UserID != userID {
		return errcode.AppError{Code: errcode.ErrForbidden, Message: "无权访问该会话"}
	}
	if !s.hub.Cancel(sessionID) {
		return errcode.AppError{Code: errcode.ErrParam, Message: "无进行中的生成"}
	}
	return nil
}

// CleanupStaleGenerating 启动时把残留 generating 标记 failed。
func (s *ChatService) CleanupStaleGenerating(ctx context.Context) error {
	_, err := s.chatRepo.MarkGeneratingFailed(ctx)
	return err
}

// =============================================================================
// SubmitFeedback
// =============================================================================

// SubmitFeedback 提交问答反馈。
//
// 校验规则在 Service 层集中管理，不依赖 Handler 层参数校验。
func (s *ChatService) SubmitFeedback(ctx context.Context, sessionID int64, userID int64, feedback int16) error {
	if s.chatRepo == nil {
		return errcode.AppError{Code: errcode.ErrUnknown, Message: "服务未初始化"}
	}
	if feedback < 0 || feedback > 2 {
		return errcode.AppError{Code: errcode.ErrParam, Message: "反馈值无效，请输入 0（未反馈）/1（已解决）/2（未解决）"}
	}
	// 仅允许从「未反馈」(0) 更新为「已解决」(1) 或「未解决」(2)，禁止用 0 覆盖已有反馈。
	if feedback == 0 {
		return errcode.AppError{Code: errcode.ErrParam, Message: "反馈值无效，请输入 1（已解决）或 2（未解决）"}
	}
	session, err := s.chatRepo.FindByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.AppError{Code: errcode.ErrNotFound, Message: "会话不存在"}
		}
		return errcode.AppError{Code: errcode.ErrUnknown, Message: "加载会话失败，请稍后重试"}
	}
	if session.UserID != userID {
		return errcode.AppError{Code: errcode.ErrForbidden, Message: "无权操作该会话"}
	}
	return s.chatRepo.UpdateFeedback(ctx, sessionID, feedback)
}

// =============================================================================
// GetChatDetail
// =============================================================================

// GetChatDetail 查询问答会话详情（含多轮对话消息历史 + 归属校验）。
func (s *ChatService) GetChatDetail(ctx context.Context, sessionID int64, userID int64) (*response.ChatSessionResponse, error) {
	if s.chatRepo == nil {
		return nil, errcode.AppError{Code: errcode.ErrUnknown, Message: "服务未初始化"}
	}
	session, err := s.chatRepo.FindByID(ctx, sessionID)
	if err != nil {
		return nil, errcode.AppError{Code: errcode.ErrNotFound, Message: "会话不存在"}
	}
	if session.UserID != userID {
		return nil, errcode.AppError{Code: errcode.ErrForbidden, Message: "无权查看该会话"}
	}

	var sources []response.SourceItem
	if len(session.Sources) > 0 {
		if err := json.Unmarshal(session.Sources, &sources); err != nil {
			slog.Warn("解析会话 Sources JSON 失败", "session_id", sessionID, "error", err)
		}
	}

	// 加载消息历史
	var messages []response.MessageItem
	if msgs, msgErr := s.chatRepo.FindMessagesBySession(ctx, sessionID); msgErr == nil {
		for _, m := range msgs {
			var msgSources []response.SourceItem
			if len(m.Sources) > 0 {
				if err := json.Unmarshal(m.Sources, &msgSources); err != nil {
					slog.Warn("解析消息 Sources JSON 失败", "message_id", m.ID, "error", err)
				}
			}
			messages = append(messages, response.MessageItem{
				ID:         m.ID,
				Role:       m.Role,
				Content:    m.Content,
				Sources:    msgSources,
				Confidence: m.Confidence,
				Status:     m.Status,
				CreatedAt:  m.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}
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
		Messages:        messages,
	}, nil
}

// =============================================================================
// ListSessions — 会话列表
// =============================================================================

// ListSessions 分页查询用户的问答会话列表。
//
// 每条会话返回首轮问题标题 + 最后一条回复摘要 + 消息总数。
func (s *ChatService) ListSessions(ctx context.Context, userID int64, page, pageSize int) ([]response.SessionListItem, int64, error) {
	if s.chatRepo == nil {
		return nil, 0, errcode.AppError{Code: errcode.ErrUnknown, Message: "服务未初始化"}
	}
	sessions, total, err := s.chatRepo.ListByUser(ctx, userID, page, pageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("查询会话列表失败: %w", err)
	}

	// 批量获取消息数量，避免 N+1 查询
	sessionIDs := make([]int64, len(sessions))
	for i, sess := range sessions {
		sessionIDs[i] = sess.ID
	}
	msgCounts, countErr := s.chatRepo.CountMessagesBySessions(ctx, sessionIDs)
	if countErr != nil {
		slog.Warn("批量获取会话消息数失败", "error", countErr)
		msgCounts = map[int64]int64{}
	}

	items := make([]response.SessionListItem, 0, len(sessions))
	for _, sess := range sessions {
		lastAnswer := truncateText(sess.Answer, 100)
		items = append(items, response.SessionListItem{
			ID:           sess.ID,
			Question:     sess.Question,
			LastAnswer:   lastAnswer,
			MessageCount: msgCounts[sess.ID],
			CreatedAt:    sess.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:    sess.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return items, total, nil
}

// DeleteSession 删除会话及其全部消息（含归属校验）。
func (s *ChatService) DeleteSession(ctx context.Context, sessionID, userID int64) error {
	if s.chatRepo == nil {
		return errcode.AppError{Code: errcode.ErrUnknown, Message: "服务未初始化"}
	}
	session, err := s.chatRepo.FindByID(ctx, sessionID)
	if err != nil {
		return errcode.AppError{Code: errcode.ErrNotFound, Message: "会话不存在"}
	}
	if session.UserID != userID {
		return errcode.AppError{Code: errcode.ErrForbidden, Message: "无权删除该会话"}
	}
	return s.chatRepo.DeleteSession(ctx, sessionID, userID)
}

// buildRAGOptions 合并多层配置：env 默认 → DB 运行时配置 → 请求参数。
func (s *ChatService) buildRAGOptions(routeCount, rerankCount int, history []map[string]string) rag.RAGOptions {
	ragEnabled := s.readBool("ai.rag_enabled", true)
	return rag.RAGOptions{
		TopK:             s.readInt("ai.top_k", s.ragDefaults.TopK),
		QueryRewrite:     s.readBool("ai.rag_query_rewrite", s.ragDefaults.QueryRewrite),
		MultiRoute:       s.readBool("ai.rag_multi_route", s.ragDefaults.MultiRoute),
		Hybrid:           s.readBool("ai.rag_hybrid", s.ragDefaults.Hybrid),
		Rerank:           s.readBool("ai.rag_rerank", s.ragDefaults.Rerank),
		DisableRetrieval: !ragEnabled,
		RouteCount:       routeCount,
		RerankCount:      rerankCount,
		History:          history,
	}
}

func (s *ChatService) readInt(key string, fallback int) int {
	if s.configReader == nil {
		return fallback
	}
	if v, ok := s.configReader.GetInt(context.Background(), key); ok {
		return v
	}
	return fallback
}

func (s *ChatService) readBool(key string, fallback bool) bool {
	if s.configReader == nil {
		return fallback
	}
	if v, ok := s.configReader.GetBool(context.Background(), key); ok {
		return v
	}
	return fallback
}

// truncateText 截断文本到 maxRunes 个字符，超出加 "..."
func truncateText(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
