// Package router 负责注册 Gin 路由。
//
// 本文件提供测试辅助函数，供 handler 测试使用。
package router

import (
	"github.com/gin-gonic/gin"
	"opsmind/internal/handler"
)

// SetupTestRouter 创建用于测试的 Gin 引擎。
//
// 注册认证路由组并绑定真实 Handler，省略中间件和数据库依赖。
func SetupTestRouter(authHandler *handler.AuthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	auth := r.Group("/api/v1/auth")
	auth.POST("/login", authHandler.Login)
	auth.POST("/refresh", authHandler.Refresh)
	auth.POST("/change-password", authHandler.ChangePassword)
	auth.POST("/logout", authHandler.Logout)

	return r
}
