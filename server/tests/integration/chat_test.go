//go:build integration

// Package integration_test 验证智能问答模块的端到端完整流程。
//
// 测试覆盖 PLAN.md Task36 定义的场景：
//   - 创建问答会话 → 返回答案和来源
//   - 低置信度 → can_submit_ticket=true
//   - 提交反馈 → 反馈状态验证
//   - AI 服务不可用降级
//
// RagClient 使用 mock（因为测试环境无可用的 AnythingLLM 实例），
// 数据库使用真实 PostgreSQL opsmind_test 库。
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"opsmind/internal/adapter"
	"opsmind/internal/config"
	"opsmind/internal/database"
	"opsmind/internal/dto/response"
	"opsmind/internal/handler"
	"opsmind/internal/middleware"
	"opsmind/internal/model"
	"opsmind/internal/repository"
	"opsmind/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// =============================================================================
// Mock RagClient（集成测试用）
// =============================================================================

// chatIntMockRAG 集成测试用的 RagClient mock。
//
// 与 handler_test 中的 mock 不同，此 mock 支持更复杂的场景注入：
// 正常回答、低置信度、服务不可达等。
type chatIntMockRAG struct {
	queryFunc func(ctx context.Context, req adapter.RAGQueryRequest) (*adapter.RAGQueryResponse, error)
}

func (m *chatIntMockRAG) Query(ctx context.Context, req adapter.RAGQueryRequest) (*adapter.RAGQueryResponse, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, req)
	}
	return &adapter.RAGQueryResponse{Answer: "默认回答", Confidence: 0.9}, nil
}
func (m *chatIntMockRAG) CreateWorkspace(ctx context.Context, req adapter.RAGCreateWorkspaceRequest) (*adapter.RAGCreateWorkspaceResponse, error) {
	return &adapter.RAGCreateWorkspaceResponse{Slug: "itg-ws"}, nil
}
func (m *chatIntMockRAG) SyncDocument(ctx context.Context, req adapter.RAGSyncRequest) (*adapter.RAGSyncResponse, error) {
	return &adapter.RAGSyncResponse{DocumentLocation: "itg-doc-loc"}, nil
}
func (m *chatIntMockRAG) DisableDocument(ctx context.Context, req adapter.RAGDisableRequest) error {
	return nil
}

// =============================================================================
// 测试环境
// =============================================================================

// chatIntEnv 封装问答集成测试环境。
type chatIntEnv struct {
	r       *gin.Engine
	db      *gorm.DB
	mockRAG *chatIntMockRAG
	kb      *model.KnowledgeBase
}

// setupChatIntegration 创建问答集成测试环境。
func setupChatIntegration(t *testing.T) *chatIntEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dbCfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "opsmind",
		Password: "opsmind123",
		DBName:   "opsmind_test",
		SSLMode:  "disable",
	}
	db, err := database.Init(dbCfg)
	require.NoError(t, err, "初始化数据库失败")

	// 建表
	db.Exec(`CREATE TABLE IF NOT EXISTS knowledge_bases (
		id BIGSERIAL PRIMARY KEY,
		name VARCHAR(128) NOT NULL,
		description TEXT,
		rag_workspace_slug VARCHAR(128),
		embedding_model VARCHAR(128) NOT NULL DEFAULT '',
		vector_dimension INT NOT NULL DEFAULT 0,
		created_by BIGINT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS knowledge_articles (
		id BIGSERIAL PRIMARY KEY,
		kb_id BIGINT NOT NULL,
		question TEXT NOT NULL,
		answer TEXT NOT NULL,
		category VARCHAR(64) DEFAULT '',
		tags JSONB,
		status SMALLINT NOT NULL DEFAULT 1,
		review_comment TEXT,
		rag_document_location VARCHAR(255),
		created_by BIGINT NOT NULL,
		reviewed_by BIGINT,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS chat_sessions (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL,
		kb_id BIGINT NOT NULL,
		question TEXT NOT NULL,
		answer TEXT,
		sources JSONB,
		confidence DOUBLE PRECISION DEFAULT 0,
		feedback SMALLINT DEFAULT 0,
		duration_ms INT DEFAULT 0,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS chat_messages (
		id BIGSERIAL PRIMARY KEY,
		session_id BIGINT NOT NULL,
		role VARCHAR(16) NOT NULL,
		content TEXT NOT NULL,
		sources JSONB,
		confidence DOUBLE PRECISION DEFAULT 0,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`)

	// 清理
	db.Exec("DELETE FROM chat_messages")
	db.Exec("DELETE FROM chat_sessions")
	db.Exec("DELETE FROM knowledge_articles")
	db.Exec("DELETE FROM knowledge_bases")

	// 创建测试知识库
	kb := &model.KnowledgeBase{
		Name:             "集成测试知识库",
		RAGWorkspaceSlug: "itg-test-workspace",
		EmbeddingModel:   "text-embedding-ada-002",
		VectorDimension:  1536,
		CreatedBy:        1,
	}
	require.NoError(t, db.Create(kb).Error)

	// 组装依赖链
	knowledgeRepo := repository.NewKnowledgeRepo(db)
	chatRepo := repository.NewChatRepo(db)
	mockRAG := &chatIntMockRAG{}
	chatSvc := service.NewChatService(knowledgeRepo, chatRepo, mockRAG)
	chatH := handler.NewChatHandler(chatSvc)

	// 路由（模拟认证中间件注入 user_id=1）
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(func(c *gin.Context) {
		c.Set("currentUser", map[string]interface{}{"user_id": float64(1)})
		c.Next()
	})

	portal := r.Group("/api/v1/portal")
	{
		portal.POST("/chat-sessions", chatH.CreateChatSession)
		portal.POST("/chat-sessions/:id/feedback", chatH.SubmitFeedback)
		portal.GET("/chat-sessions/:id", chatH.GetChatDetail)
	}

	return &chatIntEnv{r: r, db: db, mockRAG: mockRAG, kb: kb}
}

// =============================================================================
// 完整问答流程
// =============================================================================

// TestChatIntegration_FullFlow 验证完整问答流程。
//
// 流程：创建问答会话（高置信度）→ 验证答案和来源 → 提交反馈 → 查询验证反馈
func TestChatIntegration_FullFlow(t *testing.T) {
	env := setupChatIntegration(t)

	// 配置 mock 返回高置信度答案
	env.mockRAG.queryFunc = func(ctx context.Context, req adapter.RAGQueryRequest) (*adapter.RAGQueryResponse, error) {
		return &adapter.RAGQueryResponse{
			Answer:     "请尝试重启路由器并检查网线连接。",
			Confidence: 0.85,
			Sources: []adapter.RAGSource{
				{DocName: "网络故障排查指南", ChunkContent: "步骤一：重启设备...", Confidence: 0.85},
				{DocName: "常见问题 FAQ", ChunkContent: "网络连接异常时...", Confidence: 0.72},
			},
		}, nil
	}

	// 1. 创建问答会话
	askBody, _ := json.Marshal(map[string]interface{}{
		"question": "网络连不上怎么办？",
		"kb_id":    env.kb.ID,
	})
	askReq := httptest.NewRequest("POST", "/api/v1/portal/chat-sessions",
		bytes.NewReader(askBody))
	askReq.Header.Set("Content-Type", "application/json")
	askW := httptest.NewRecorder()
	env.r.ServeHTTP(askW, askReq)

	assert.Equal(t, 200, askW.Code, "创建问答应返回 200")

	var createResp struct {
		Code int                        `json:"code"`
		Data response.ChatSessionResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(askW.Body.Bytes(), &createResp))
	assert.Equal(t, 0, createResp.Code)

	sessionID := createResp.Data.SessionID
	assert.NotZero(t, sessionID, "应返回 SessionID")
	assert.Equal(t, "请尝试重启路由器并检查网线连接。", createResp.Data.Answer)
	assert.False(t, createResp.Data.CanSubmitTicket, "高置信度时不应引导提交申告")
	assert.Len(t, createResp.Data.Sources, 2, "应返回 2 条来源")
	assert.Equal(t, "网络故障排查指南", createResp.Data.Sources[0].DocName)
	// DurationMS 在 mock 环境下可能为 0（RagClient 瞬间返回），
	// 生产环境中会有网络延迟，此处仅验证字段存在即可。
	assert.GreaterOrEqual(t, createResp.Data.DurationMS, 0, "DurationMS 应 >= 0")
	t.Logf("✅ 步骤1: 问答创建成功，答案='%s'，来源=%d，置信度=%.2f",
		createResp.Data.Answer, len(createResp.Data.Sources), createResp.Data.Confidence)

	// 2. 提交反馈（已解决）
	feedbackBody, _ := json.Marshal(map[string]interface{}{"feedback": 1}) // 1=resolved
	fbReq := httptest.NewRequest("POST",
		"/api/v1/portal/chat-sessions/"+itoa64(sessionID)+"/feedback",
		bytes.NewReader(feedbackBody))
	fbReq.Header.Set("Content-Type", "application/json")
	fbW := httptest.NewRecorder()
	env.r.ServeHTTP(fbW, fbReq)

	assert.Equal(t, 200, fbW.Code, "提交反馈应返回 200")
	t.Logf("✅ 步骤2: 反馈提交成功 (resolved)")

	// 3. 查询会话详情 → 验证反馈已保存
	detailReq := httptest.NewRequest("GET",
		"/api/v1/portal/chat-sessions/"+itoa64(sessionID), nil)
	detailW := httptest.NewRecorder()
	env.r.ServeHTTP(detailW, detailReq)

	assert.Equal(t, 200, detailW.Code)

	var detailResp struct {
		Data response.ChatSessionResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(detailW.Body.Bytes(), &detailResp))
	assert.Equal(t, int16(1), detailResp.Data.Feedback, "反馈状态应为 1(resolved)")
	t.Logf("✅ 步骤3: 反馈状态验证通过")

	// 4. 验证数据库中会话和消息都已持久化
	var sessionCount int64
	env.db.Model(&model.ChatSession{}).Where("id = ?", sessionID).Count(&sessionCount)
	assert.Equal(t, int64(1), sessionCount, "数据库应有 1 条会话记录")

	var msgCount int64
	env.db.Model(&model.ChatMessage{}).Where("session_id = ?", sessionID).Count(&msgCount)
	assert.Equal(t, int64(2), msgCount, "数据库应有 2 条消息记录（用户问题 + AI 回答）")
	t.Logf("✅ 步骤4: 数据持久化验证通过（会话=%d, 消息=%d）", sessionCount, msgCount)
}

// =============================================================================
// 低置信度 → 引导转申告
// =============================================================================

// TestChatIntegration_LowConfidenceToTicket 验证低置信度时引导用户提交申告。
func TestChatIntegration_LowConfidenceToTicket(t *testing.T) {
	env := setupChatIntegration(t)

	// 模拟低置信度回答
	env.mockRAG.queryFunc = func(ctx context.Context, req adapter.RAGQueryRequest) (*adapter.RAGQueryResponse, error) {
		return &adapter.RAGQueryResponse{
			Answer:     "暂未找到足够匹配的知识，建议提交申告由运维人员人工处理",
			Confidence: 0.35,
			Sources:    nil,
		}, nil
	}

	body, _ := json.Marshal(map[string]interface{}{
		"question": "非常专业的运维问题",
		"kb_id":    env.kb.ID,
	})
	req := httptest.NewRequest("POST", "/api/v1/portal/chat-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp struct {
		Data response.ChatSessionResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Data.CanSubmitTicket, "低置信度时 CanSubmitTicket 应为 true")
	assert.NotEmpty(t, resp.Data.Answer, "低置信度时也应有兜底回答")
	t.Logf("✅ 低置信度 (%.2f) → CanSubmitTicket=true, 兜底回答='%s'",
		resp.Data.Confidence, resp.Data.Answer)
}

// =============================================================================
// AI 服务降级
// =============================================================================

// TestChatIntegration_AIServiceUnavailable 验证 AI 服务不可用时的降级处理。
func TestChatIntegration_AIServiceUnavailable(t *testing.T) {
	env := setupChatIntegration(t)

	// 模拟 RagClient 返回网络错误
	env.mockRAG.queryFunc = func(ctx context.Context, req adapter.RAGQueryRequest) (*adapter.RAGQueryResponse, error) {
		return nil, errors.New("connection refused")
	}

	body, _ := json.Marshal(map[string]interface{}{
		"question": "任何问题",
		"kb_id":    env.kb.ID,
	})
	req := httptest.NewRequest("POST", "/api/v1/portal/chat-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.r.ServeHTTP(w, req)

	// AI 不可用时，ChatService 返回 AppError{Code: 20001}，
	// Handler 层将 AppError 映射为 HTTP 500 + 错误码 20001。
	// 这是当前设计的降级策略：AI 不可达时返回明确错误码供前端识别。
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 20001, resp.Code, "AI 不可用错误码应为 20001")
	assert.NotEmpty(t, resp.Message, "应有错误消息")
	t.Logf("✅ AI 不可用 → 错误码 20001, message='%s'", resp.Message)
}

// =============================================================================
// 参数校验
// =============================================================================

// TestChatIntegration_Validation 验证问答接口参数校验。
func TestChatIntegration_Validation(t *testing.T) {
	env := setupChatIntegration(t)

	// 场景1: 空问题
	body, _ := json.Marshal(map[string]interface{}{
		"question": "",
		"kb_id":    env.kb.ID,
	})
	req := httptest.NewRequest("POST", "/api/v1/portal/chat-sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.r.ServeHTTP(w, req)

	var resp struct{ Code int }
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotEqual(t, 0, resp.Code, "空问题应返回错误码")
	t.Logf("✅ 空问题被拒绝 (code=%d)", resp.Code)

	// 场景2: kb_id 不存在
	body2, _ := json.Marshal(map[string]interface{}{
		"question": "有效问题",
		"kb_id":    99999,
	})
	req2 := httptest.NewRequest("POST", "/api/v1/portal/chat-sessions", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	env.r.ServeHTTP(w2, req2)

	var resp2 struct{ Code int }
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	assert.NotEqual(t, 0, resp2.Code, "不存在的知识库应返回错误码")
	t.Logf("✅ 不存在知识库被拒绝 (code=%d)", resp2.Code)
}

// =============================================================================
// 工具函数
// =============================================================================

// itoa64 将 int64 转为字符串（用于 URL 拼接）
func itoa64(n int64) string {
	return fmt.Sprintf("%d", n)
}
