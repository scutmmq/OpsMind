// Package router 负责注册 Gin 路由。
//
// 本文件注册门户端路由，与 TECH.md §5.2 门户端对齐。
// 所有路由需要 JWT 认证，当前使用占位 Handler 返回 501。
package router

import "github.com/gin-gonic/gin"

// registerPortalRoutes 注册门户端路由。
//
// 门户端面向报障人用户，提供智能问答、申告提交、进度查询等功能。
// 路由列表与 TECH.md §5.2 门户端对齐。
func registerPortalRoutes(rg *gin.RouterGroup) {
	// 智能问答
	rg.POST("/chat-sessions", placeholder())           // 创建问答会话
	rg.GET("/chat-sessions/:id", placeholder())        // 获取问答详情
	rg.POST("/chat-sessions/:id/feedback", placeholder()) // 提交问答反馈

	// 申告管理
	rg.POST("/tickets", placeholder())                 // 创建申告
	rg.GET("/tickets", placeholder())                  // 查询我的申告列表
	rg.GET("/tickets/:id", placeholder())              // 获取申告详情
	rg.PATCH("/tickets/:id/supplement", placeholder()) // 补充申告信息

	// 站内消息
	rg.GET("/messages", placeholder())                 // 获取站内消息
	rg.PATCH("/messages/:id/read", placeholder())      // 标记消息已读
	rg.GET("/messages/unread-count", placeholder())    // 获取未读消息数
}
