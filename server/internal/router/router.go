// Package router 负责注册 Gin 路由。
//
// 路由分为三组：
// - /api/v1/auth — 公开路由（登录、刷新令牌等）
// - /api/v1/portal — 门户端路由（需要 JWT 认证）
// - /api/v1/admin — 后台管理路由（需要 JWT 认证 + RBAC 权限）
//
// MVP 阶段所有路由 Handler 返回 501 Not Implemented，后续任务逐步实现。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"opsmind/internal/config"
	"opsmind/internal/middleware"
)

// Setup 初始化 Gin 引擎并注册所有路由。
//
// cfg 用于设置 Gin 模式（debug/release）和中间件配置。
// db 参数保留给后续任务（JWT 中间件需要数据库查询用户角色）。
func Setup(cfg *config.AppConfig, db interface{}) *gin.Engine {
	// 设置 Gin 模式
	gin.SetMode(cfg.Server.Mode)

	r := gin.New()

	// 注册全局中间件
	r.Use(middleware.RequestID())
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())
	r.Use(gin.Recovery())

	// 健康检查端点（无需认证，供 Docker/K8s 存活探针使用）
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// 公开路由组（无需认证）
	public := r.Group("/api/v1/auth")
	registerPublicRoutes(public)

	// 门户端路由组（需要 JWT 认证）
	portal := r.Group("/api/v1/portal")
	registerPortalRoutes(portal)

	// 后台管理路由组（需要 JWT 认证 + RBAC 权限）
	admin := r.Group("/api/v1/admin")
	registerAdminRoutes(admin)

	return r
}

// placeholder 返回一个占位 Handler，返回 501 Not Implemented。
// 用于路由骨架注册，后续任务替换为真实 Handler。
func placeholder() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{
			"code":    501,
			"message": "Not Implemented",
			"data":    nil,
		})
	}
}

// registerPublicRoutes 注册公开路由（无需认证）。
//
// 包括登录、刷新令牌、修改密码、退出登录等认证相关接口。
func registerPublicRoutes(rg *gin.RouterGroup) {
	rg.POST("/login", placeholder())           // 用户登录
	rg.POST("/refresh", placeholder())         // 刷新令牌
	rg.POST("/change-password", placeholder()) // 修改密码
	rg.POST("/logout", placeholder())          // 退出登录
}
