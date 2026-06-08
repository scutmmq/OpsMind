// Package adapter_test 验证 RagClient 适配器的请求构造和响应映射。
//
// 使用 httptest 模拟 AnythingLLM 服务端，验证：
// - 正常响应 → 字段映射正确
// - 错误响应 → Error 字段非空
// - 网络超时 → 返回错误
// - 请求体构造 → 各方法参数映射正确
package adapter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"opsmind/internal/adapter"
)

// anythingLLMResponse 模拟 AnythingLLM API 的 chat 响应结构。
type anythingLLMResponse struct {
	ID           string              `json:"id"`
	Type         string              `json:"type"`
	Close        bool                `json:"close"`
	Error        *string             `json:"error"`
	TextResponse string              `json:"textResponse"`
	Sources      []anythingLLMSource `json:"sources"`
	Metrics      map[string]any      `json:"metrics"`
}

type anythingLLMSource struct {
	Title   string  `json:"title"`
	Text    string  `json:"text"`
	Score   float64 `json:"score"`
	URL     string  `json:"url"`
	DocName string  `json:"doc_name"`
}

// setupMockServer 启动模拟 AnythingLLM 服务的 httptest server。
func setupMockServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *adapter.AnythingLLMClient) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := adapter.NewAnythingLLMClient(adapter.AnythingLLMConfig{
		BaseURL:        server.URL,
		APIKey:         "test-api-key",
		TimeoutSeconds: 5,
	})

	return server, client
}

// =============================================================================
// Query 测试
// =============================================================================

// TestRagClient_Query_Success 正常 Query 响应 → 字段映射正确。
func TestRagClient_Query_Success(t *testing.T) {
	server, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		// 验证请求路径
		if !strings.Contains(r.URL.Path, "/workspace/") || !strings.Contains(r.URL.Path, "/chat") {
			t.Errorf("期望路径包含 /workspace/.../chat, got %s", r.URL.Path)
		}

		// 验证 Authorization header
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("期望 Authorization: Bearer test-api-key, got %s", r.Header.Get("Authorization"))
		}

		// 验证 Content-Type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("期望 Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
		}

		resp := anythingLLMResponse{
			ID:           "chat-123",
			Type:         "textResponse",
			Close:        true,
			TextResponse: "根据知识库，密码重置流程是...",
			Sources: []anythingLLMSource{
				{Title: "密码重置指南", Text: "用户可以通过设置页面重置密码。", Score: 0.92, DocName: "FAQ-001"},
				{Title: "账号管理FAQ", Text: "密码需包含大小写字母和数字。", Score: 0.78, DocName: "FAQ-002"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	_ = server

	ctx := context.Background()
	result, err := client.Query(ctx, adapter.RAGQueryRequest{
		WorkspaceSlug: "test-workspace",
		Question:      "如何重置密码？",
		TopK:          5,
	})
	if err != nil {
		t.Fatalf("期望无错误, got %v", err)
	}

	// 验证字段映射
	if result.Answer != "根据知识库，密码重置流程是..." {
		t.Errorf("期望 Answer, got '%s'", result.Answer)
	}
	if result.Confidence != 0.92 {
		t.Errorf("期望 Confidence=0.92(max score), got %f", result.Confidence)
	}
	if len(result.Sources) != 2 {
		t.Errorf("期望 2 个 sources, got %d", len(result.Sources))
	}
	if result.Sources[0].DocName != "FAQ-001" {
		t.Errorf("期望 DocName='FAQ-001', got '%s'", result.Sources[0].DocName)
	}
	if result.Sources[0].ChunkContent != "用户可以通过设置页面重置密码。" {
		t.Errorf("期望 ChunkContent, got '%s'", result.Sources[0].ChunkContent)
	}
}

// TestRagClient_Query_Error 服务器返回 error → Error 字段非空。
func TestRagClient_Query_Error(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		errMsg := "workspace not found"
		resp := anythingLLMResponse{
			ID:    "chat-err",
			Error: &errMsg,
		}
		json.NewEncoder(w).Encode(resp)
	})

	ctx := context.Background()
	result, err := client.Query(ctx, adapter.RAGQueryRequest{
		WorkspaceSlug: "nonexistent",
		Question:      "问题",
	})
	if err != nil {
		t.Fatalf("Query 不应返回 error（error 封装在响应中）, got %v", err)
	}
	if result.Error == "" {
		t.Error("期望 result.Error 非空")
	}
	if result.Error != "workspace not found" {
		t.Errorf("期望 'workspace not found', got '%s'", result.Error)
	}
}

// TestRagClient_Query_Timeout 网络超时 → 返回错误。
func TestRagClient_Query_Timeout(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		// 模拟超时：延迟 2 秒响应，但客户端超时设置为 0
		time.Sleep(2 * time.Second)
	})

	// 使用 1 纳秒超时的客户端（立即超时）
	timeoutClient := adapter.NewAnythingLLMClient(adapter.AnythingLLMConfig{
		BaseURL:        client.BaseURL(),
		APIKey:         "test-key",
		TimeoutSeconds: 0, // 会导致 context deadline
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := timeoutClient.Query(ctx, adapter.RAGQueryRequest{
		WorkspaceSlug: "test",
		Question:      "问题",
	})
	if err == nil {
		t.Error("期望超时错误, got nil")
	}
}

// TestRagClient_Query_EmptySources 无 sources → confidence=0。
func TestRagClient_Query_EmptySources(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		resp := anythingLLMResponse{
			ID:           "chat-empty",
			Type:         "textResponse",
			TextResponse: "未找到相关信息。",
			Sources:      []anythingLLMSource{},
		}
		json.NewEncoder(w).Encode(resp)
	})

	ctx := context.Background()
	result, err := client.Query(ctx, adapter.RAGQueryRequest{
		WorkspaceSlug: "test",
		Question:      "问题",
	})
	if err != nil {
		t.Fatalf("期望无错误, got %v", err)
	}
	if result.Confidence != 0 {
		t.Errorf("期望 Confidence=0（无 sources）, got %f", result.Confidence)
	}
}

// TestRagClient_Query_HTTPError HTTP 错误状态码 → 返回错误。
func TestRagClient_Query_HTTPError(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	})

	ctx := context.Background()
	_, err := client.Query(ctx, adapter.RAGQueryRequest{
		WorkspaceSlug: "test",
		Question:      "问题",
	})
	if err == nil {
		t.Error("期望错误, got nil")
	}
}

// =============================================================================
// CreateWorkspace 测试
// =============================================================================

// TestRagClient_CreateWorkspace 创建工作区。
func TestRagClient_CreateWorkspace(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/workspace/new") {
			t.Errorf("期望路径 /workspace/new, got %s", r.URL.Path)
		}

		// 验证请求体
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "测试工作区" {
			t.Errorf("期望 name='测试工作区', got '%s'", body["name"])
		}

		resp := map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":   10,
				"slug": "test-workspace-slug",
				"name": "测试工作区",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	ctx := context.Background()
	result, err := client.CreateWorkspace(ctx, adapter.RAGCreateWorkspaceRequest{
		Name: "测试工作区",
	})
	if err != nil {
		t.Fatalf("期望无错误, got %v", err)
	}
	if result.Slug != "test-workspace-slug" {
		t.Errorf("期望 slug='test-workspace-slug', got '%s'", result.Slug)
	}
}

// =============================================================================
// SyncDocument 测试
// =============================================================================

// TestRagClient_SyncDocument_RawText raw-text 模式 → 请求体构造正确。
func TestRagClient_SyncDocument_RawText(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/document/raw-text") {
			t.Errorf("期望路径 /document/raw-text, got %s", r.URL.Path)
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["title"] != "Q: 问题" {
			t.Errorf("期望 title, got '%v'", body["title"])
		}

		resp := map[string]interface{}{
			"success": true,
			"document": map[string]interface{}{
				"location": "custom-documents/faq-test.json",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	ctx := context.Background()
	result, err := client.SyncDocument(ctx, adapter.RAGSyncRequest{
		WorkspaceSlug: "test-ws",
		Title:         "Q: 问题",
		Content:       "A: 答案",
		Mode:          "raw-text",
	})
	if err != nil {
		t.Fatalf("期望无错误, got %v", err)
	}
	if result.DocumentLocation != "custom-documents/faq-test.json" {
		t.Errorf("期望 location, got '%s'", result.DocumentLocation)
	}
}

// =============================================================================
// DisableDocument 测试
// =============================================================================

// TestRagClient_DisableDocument 停用文档 → 请求体包含 deletes。
func TestRagClient_DisableDocument(t *testing.T) {
	_, client := setupMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/update-embeddings") {
			t.Errorf("期望路径包含 /update-embeddings, got %s", r.URL.Path)
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		deletes, ok := body["deletes"].([]interface{})
		if !ok || len(deletes) != 1 {
			t.Errorf("期望 deletes 数组有 1 个元素, got %v", body["deletes"])
		}

		resp := map[string]interface{}{
			"success": true,
		}
		json.NewEncoder(w).Encode(resp)
	})

	ctx := context.Background()
	err := client.DisableDocument(ctx, adapter.RAGDisableRequest{
		WorkspaceSlug:    "test-ws",
		DocumentLocations: []string{"custom-documents/old-faq.json"},
	})
	if err != nil {
		t.Fatalf("期望无错误, got %v", err)
	}
}
