// Package router_test 测试路由注册骨架。
//
// 验证路由组正确注册，占位 Handler 返回 501 Not Implemented。
// 测试覆盖公开路由、门户端路由、后台管理路由。
package router_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"opsmind/internal/config"
	"opsmind/internal/router"
)

// setupRouter 创建用于测试的路由引擎
func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	cfg := &config.AppConfig{
		Server: config.ServerConfig{
			Port: 8080,
			Mode: "test",
		},
	}
	return router.Setup(cfg, nil)
}

// TestSetup_ReturnsEngine 测试 Setup 返回有效的 Gin 引擎
func TestSetup_ReturnsEngine(t *testing.T) {
	r := setupRouter()
	if r == nil {
		t.Fatal("Setup 应该返回非 nil 的 Gin 引擎")
	}
}

// TestPublicRoutes_Exist 测试公开路由组存在
func TestPublicRoutes_Exist(t *testing.T) {
	r := setupRouter()

	// 测试登录路由存在
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/login", nil)
	r.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Error("公开路由 /api/v1/auth/login 应该存在")
	}
}

// TestPublicRoutes_Return501 测试公开路由占位返回 501
func TestPublicRoutes_Return501(t *testing.T) {
	tests := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/auth/login"},
		{"POST", "/api/v1/auth/refresh"},
		{"POST", "/api/v1/auth/change-password"},
		{"POST", "/api/v1/auth/logout"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			r := setupRouter()
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.path, nil)
			r.ServeHTTP(w, req)

			if w.Code != http.StatusNotImplemented {
				t.Errorf("期望状态码 501，实际 %d", w.Code)
			}

			// 验证响应格式
			var resp map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("解析响应失败: %v", err)
			}
		})
	}
}

// TestPortalRoutes_Exist 测试门户端路由存在
func TestPortalRoutes_Exist(t *testing.T) {
	tests := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/portal/chat-sessions"},
		{"GET", "/api/v1/portal/chat-sessions/1"},
		{"POST", "/api/v1/portal/chat-sessions/1/feedback"},
		{"POST", "/api/v1/portal/tickets"},
		{"GET", "/api/v1/portal/tickets"},
		{"GET", "/api/v1/portal/tickets/1"},
		{"PATCH", "/api/v1/portal/tickets/1/supplement"},
		{"GET", "/api/v1/portal/messages"},
		{"PATCH", "/api/v1/portal/messages/1/read"},
		{"GET", "/api/v1/portal/messages/unread-count"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			r := setupRouter()
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.path, nil)
			r.ServeHTTP(w, req)

			if w.Code == http.StatusNotFound {
				t.Errorf("门户端路由 %s %s 应该存在", tt.method, tt.path)
			}
		})
	}
}

// TestAdminRoutes_Exist 测试后台管理路由存在
func TestAdminRoutes_Exist(t *testing.T) {
	tests := []struct {
		method string
		path   string
	}{
		// 申告管理
		{"GET", "/api/v1/admin/tickets"},
		{"GET", "/api/v1/admin/tickets/1"},
		{"PATCH", "/api/v1/admin/tickets/1/status"},
		{"POST", "/api/v1/admin/tickets/1/records"},

		// 知识库管理
		{"GET", "/api/v1/admin/knowledge-bases"},
		{"POST", "/api/v1/admin/knowledge-bases"},
		{"GET", "/api/v1/admin/knowledge-articles"},
		{"POST", "/api/v1/admin/knowledge-articles"},

		// 用户管理
		{"GET", "/api/v1/admin/users"},
		{"POST", "/api/v1/admin/users"},
		{"PUT", "/api/v1/admin/users/1"},

		// 角色权限
		{"GET", "/api/v1/admin/roles"},
		{"POST", "/api/v1/admin/roles"},
		{"GET", "/api/v1/admin/menus"},

		// 数据看板
		{"GET", "/api/v1/admin/dashboard/stats"},
		{"GET", "/api/v1/admin/dashboard/trends"},

		// 操作日志
		{"GET", "/api/v1/admin/audit-logs"},

		// 系统配置
		{"GET", "/api/v1/admin/configs/app_name"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			r := setupRouter()
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tt.method, tt.path, nil)
			r.ServeHTTP(w, req)

			if w.Code == http.StatusNotFound {
				t.Errorf("后台路由 %s %s 应该存在", tt.method, tt.path)
			}
		})
	}
}

// TestPlaceholderHandler_Returns501 测试占位 Handler 返回 501
func TestPlaceholderHandler_Returns501(t *testing.T) {
	r := setupRouter()

	// 测试多个路由都返回 501
	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/portal/tickets"},
		{"GET", "/api/v1/admin/users"},
		{"GET", "/api/v1/admin/dashboard/stats"},
	}

	for _, rt := range routes {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(rt.method, rt.path, nil)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotImplemented {
			t.Errorf("%s %s: 期望 501，实际 %d", rt.method, rt.path, w.Code)
		}
	}
}
