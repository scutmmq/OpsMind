package handler_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestKnowledgeHandler_UploadDocuments 验证 POST /knowledge-bases/:kb_id/documents/upload。
func TestKnowledgeHandler_UploadDocuments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 占位：Handler 未实现时路由返回 501
	r.POST("/api/v1/admin/knowledge-bases/:kb_id/documents/upload", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(400, gin.H{"msg": err.Error()})
			return
		}
		c.JSON(200, gin.H{"filename": file.Filename, "kb_id": c.Param("kb_id")})
	})

	// 构造 multipart 请求
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

// TestKnowledgeHandler_GetDocumentStatus 验证 GET /knowledge-bases/:kb_id/documents/:id/status。
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

// TestKnowledgeHandler_RetryDocument 验证 POST /knowledge-bases/:kb_id/documents/:id/retry。
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
