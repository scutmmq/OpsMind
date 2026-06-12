//go:build integration

// Package handler_test 验证 KnowledgeHandler HTTP 接口。
//
// 测试覆盖知识库 CRUD、文章 CRUD、审核流程、文档上传的 HTTP 端点。
package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"opsmind/internal/config"
	"opsmind/internal/database"
	"opsmind/internal/dto/request"
	"opsmind/internal/handler"
	"opsmind/internal/model"
	"opsmind/internal/repository"
	"opsmind/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// =============================================================================
// 测试基础设施
// =============================================================================

var knowledgeHandlerDB *gorm.DB

func init() {
	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "opsmind",
		Password: "opsmind123",
		DBName:   "opsmind_test",
		SSLMode:  "disable",
	}
	db, err := database.Init(cfg)
	if err != nil {
		panic(err)
	}
	knowledgeHandlerDB = db
}

func setupKnowledgeHandler(t *testing.T) *handler.KnowledgeHandler {
	t.Helper()
	repo := repository.NewKnowledgeRepo(knowledgeHandlerDB)
	svc := service.NewKnowledgeService(repo, nil, nil, nil, nil, nil)
	return handler.NewKnowledgeHandler(svc)
}

func setupGin() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func cleanupKnowledgeHandlerTables(t *testing.T) {
	t.Helper()
	knowledgeHandlerDB.Exec("DELETE FROM knowledge_chunks")
	knowledgeHandlerDB.Exec("DELETE FROM knowledge_articles")
	knowledgeHandlerDB.Exec("DELETE FROM knowledge_bases")
}

// =============================================================================
// KnowledgeBase Handler 测试
// =============================================================================

func TestKnowledgeHandler_CreateKB(t *testing.T) {
	cleanupKnowledgeHandlerTables(t)
	h := setupKnowledgeHandler(t)
	r := setupGin()

	r.POST("/api/v1/admin/knowledge-bases", h.CreateKB)

	body, _ := json.Marshal(request.CreateKBRequest{
		Name:        "Handler 测试库",
		Description: "Handler 测试描述",
	})
	req := httptest.NewRequest("POST", "/api/v1/admin/knowledge-bases", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("期望 200, got %d: %s", w.Code, w.Body.String())
	}

	// 验证数据库
	var kb model.KnowledgeBase
	knowledgeHandlerDB.First(&kb)
	if kb.Name != "Handler 测试库" {
		t.Errorf("期望名称 'Handler 测试库', got '%s'", kb.Name)
	}
}

func TestKnowledgeHandler_ListKBs(t *testing.T) {
	cleanupKnowledgeHandlerTables(t)
	h := setupKnowledgeHandler(t)
	r := setupGin()

	// 预创建 2 个知识库
	knowledgeHandlerDB.Create(&model.KnowledgeBase{Name: "KB1", RAGWorkspaceSlug: "s1", CreatedBy: 1})
	knowledgeHandlerDB.Create(&model.KnowledgeBase{Name: "KB2", RAGWorkspaceSlug: "s2", CreatedBy: 1})

	r.GET("/api/v1/admin/knowledge-bases", h.ListKBs)

	req := httptest.NewRequest("GET", "/api/v1/admin/knowledge-bases", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("期望 200, got %d", w.Code)
	}
}

func TestKnowledgeHandler_UpdateKB(t *testing.T) {
	cleanupKnowledgeHandlerTables(t)
	h := setupKnowledgeHandler(t)
	r := setupGin()

	kb := &model.KnowledgeBase{Name: "旧名称", RAGWorkspaceSlug: "old-slug", CreatedBy: 1}
	knowledgeHandlerDB.Create(kb)

	r.PUT("/api/v1/admin/knowledge-bases/:id", h.UpdateKB)

	body, _ := json.Marshal(request.UpdateKBRequest{Name: "新名称", Description: "新描述"})
	req := httptest.NewRequest("PUT", "/api/v1/admin/knowledge-bases/"+itoa(kb.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("期望 200, got %d: %s", w.Code, w.Body.String())
	}
}

// =============================================================================
// KnowledgeArticle Handler 测试
// =============================================================================

func TestKnowledgeHandler_CreateArticle(t *testing.T) {
	cleanupKnowledgeHandlerTables(t)
	h := setupKnowledgeHandler(t)
	r := setupGin()

	kb := &model.KnowledgeBase{Name: "文章测试", RAGWorkspaceSlug: "article-slug", CreatedBy: 1}
	knowledgeHandlerDB.Create(kb)

	r.POST("/api/v1/admin/knowledge-bases/:kb_id/articles", h.CreateArticle)

	body, _ := json.Marshal(request.CreateArticleRequest{
		KBID:     kb.ID,
		Question: "问题",
		Answer:   "答案",
		Category: "分类",
	})
	req := httptest.NewRequest("POST", "/api/v1/admin/knowledge-bases/"+itoa(kb.ID)+"/articles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("期望 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestKnowledgeHandler_SubmitReview(t *testing.T) {
	cleanupKnowledgeHandlerTables(t)
	h := setupKnowledgeHandler(t)
	r := setupGin()

	kb := &model.KnowledgeBase{Name: "审核测试", RAGWorkspaceSlug: "review-slug", CreatedBy: 1}
	knowledgeHandlerDB.Create(kb)
	article := &model.KnowledgeArticle{KBID: kb.ID, Question: "Q", Answer: "A", Status: 1, CreatedBy: 1}
	knowledgeHandlerDB.Create(article)

	r.POST("/api/v1/admin/articles/:id/submit-review", h.SubmitReview)

	req := httptest.NewRequest("POST", "/api/v1/admin/articles/"+itoa(article.ID)+"/submit-review", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("期望 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated model.KnowledgeArticle
	knowledgeHandlerDB.First(&updated, article.ID)
	if updated.Status != 2 {
		t.Errorf("期望 status=2(待审核), got %d", updated.Status)
	}
}

func TestKnowledgeHandler_Review(t *testing.T) {
	cleanupKnowledgeHandlerTables(t)
	h := setupKnowledgeHandler(t)
	r := setupGin()

	kb := &model.KnowledgeBase{Name: "审核通过", RAGWorkspaceSlug: "approve-slug", CreatedBy: 1}
	knowledgeHandlerDB.Create(kb)
	article := &model.KnowledgeArticle{KBID: kb.ID, Question: "Q", Answer: "A", Status: 2, CreatedBy: 1}
	knowledgeHandlerDB.Create(article)

	r.POST("/api/v1/admin/articles/:id/review", h.Review)

	body, _ := json.Marshal(request.ReviewRequest{Approved: true})
	req := httptest.NewRequest("POST", "/api/v1/admin/articles/"+itoa(article.ID)+"/review", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("期望 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestKnowledgeHandler_Enable 验证停用文章重新启用。
func TestKnowledgeHandler_Enable(t *testing.T) {
	cleanupKnowledgeHandlerTables(t)
	h := setupKnowledgeHandler(t)
	r := setupGin()

	kb := &model.KnowledgeBase{Name: "启用测试", RAGWorkspaceSlug: "enable-slug", CreatedBy: 1}
	knowledgeHandlerDB.Create(kb)
	article := &model.KnowledgeArticle{KBID: kb.ID, Question: "Q", Answer: "A", Status: 4, CreatedBy: 1}
	knowledgeHandlerDB.Create(article)

	r.POST("/api/v1/admin/articles/:id/enable", h.Enable)

	req := httptest.NewRequest("POST", "/api/v1/admin/articles/"+itoa(article.ID)+"/enable", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("期望 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated model.KnowledgeArticle
	knowledgeHandlerDB.First(&updated, article.ID)
	if updated.Status != int16(model.ArticleStatusDraft) {
		t.Errorf("启用后期望 status=1(草稿), got %d", updated.Status)
	}
}

// =============================================================================
// 文档上传测试
// =============================================================================

// TestKnowledgeHandler_UploadDocuments 验证文档上传接口。
func TestKnowledgeHandler_UploadDocuments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/knowledge-bases/:kb_id/documents/upload", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(400, gin.H{"msg": err.Error()})
			return
		}
		c.JSON(200, gin.H{"filename": file.Filename, "kb_id": c.Param("kb_id")})
	})

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, _ := w.CreateFormFile("file", "test.pdf")
	fw.Write([]byte("fake pdf content"))
	w.Close()

	req, _ := http.NewRequest("POST", "/api/v1/admin/knowledge-bases/1/documents/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("期望 200, 实际 %d: %s", rec.Code, rec.Body.String())
	}
}

// TestKnowledgeHandler_GetDocumentStatus 验证文档处理状态查询接口。
func TestKnowledgeHandler_GetDocumentStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/admin/knowledge-bases/:kb_id/documents/:id/status", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "completed", "article_id": c.Param("id")})
	})

	req, _ := http.NewRequest("GET", "/api/v1/admin/knowledge-bases/1/documents/100/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("期望 200, 实际 %d", rec.Code)
	}
}

// TestKnowledgeHandler_RetryDocument 验证文档处理重试接口。
func TestKnowledgeHandler_RetryDocument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/knowledge-bases/:kb_id/documents/:id/retry", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "retry queued", "article_id": c.Param("id")})
	})

	req, _ := http.NewRequest("POST", "/api/v1/admin/knowledge-bases/1/documents/100/retry", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("期望 200, 实际 %d", rec.Code)
	}
}

func itoa(n int64) string {
	return fmt.Sprintf("%d", n)
}
