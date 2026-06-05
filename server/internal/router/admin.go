// Package router 负责注册 Gin 路由。
//
// 本文件注册后台管理路由，与 TECH.md §5.2 后台管理对齐。
// 所有路由需要 JWT 认证 + RBAC 权限，当前使用占位 Handler 返回 501。
package router

import "github.com/gin-gonic/gin"

// registerAdminRoutes 注册后台管理路由。
//
// 后台管理面向运维人员和管理员，提供申告处理、知识库管理、用户管理等功能。
// 路由列表与 TECH.md §5.2 后台管理对齐。
func registerAdminRoutes(rg *gin.RouterGroup) {
	// 申告管理
	rg.GET("/tickets", placeholder())                           // 申告列表
	rg.GET("/tickets/:id", placeholder())                       // 申告详情
	rg.PATCH("/tickets/:id/status", placeholder())              // 更新申告状态
	rg.POST("/tickets/:id/records", placeholder())              // 添加处理记录
	rg.POST("/tickets/:id/knowledge-candidate", placeholder())  // 生成知识候选

	// 知识库管理
	rg.GET("/knowledge-bases", placeholder())                   // 知识库列表
	rg.POST("/knowledge-bases", placeholder())                  // 创建知识库
	rg.PUT("/knowledge-bases/:id", placeholder())               // 更新知识库
	rg.GET("/knowledge-articles", placeholder())                // 知识条目列表
	rg.POST("/knowledge-articles", placeholder())               // 创建知识条目
	rg.PUT("/knowledge-articles/:id", placeholder())            // 更新知识条目
	rg.POST("/knowledge-articles/:id/review", placeholder())    // 审核知识
	rg.POST("/knowledge-articles/:id/publish", placeholder())   // 发布知识
	rg.POST("/knowledge-articles/:id/disable", placeholder())   // 停用知识
	rg.POST("/knowledge-articles/:id/retry-sync", placeholder()) // 重试同步

	// 用户管理
	rg.GET("/users", placeholder())                             // 用户列表
	rg.POST("/users", placeholder())                            // 创建用户
	rg.PUT("/users/:id", placeholder())                         // 更新用户
	rg.PATCH("/users/:id/freeze", placeholder())                // 冻结用户
	rg.PATCH("/users/:id/unfreeze", placeholder())              // 恢复用户

	// 角色权限
	rg.GET("/roles", placeholder())                             // 角色列表
	rg.POST("/roles", placeholder())                            // 创建角色
	rg.PUT("/roles/:id", placeholder())                         // 更新角色
	rg.GET("/menus", placeholder())                             // 菜单列表
	rg.PUT("/roles/:id/menus", placeholder())                   // 更新角色菜单

	// 数据看板
	rg.GET("/dashboard/stats", placeholder())                   // 统计数据
	rg.GET("/dashboard/trends", placeholder())                  // 趋势数据

	// 操作日志
	rg.GET("/audit-logs", placeholder())                        // 审计日志列表

	// 系统配置
	rg.GET("/configs/:key", placeholder())                      // 获取配置
	rg.PUT("/configs/:key", placeholder())                      // 更新配置
	rg.GET("/embedding-configs", placeholder())                 // Embedding 配置列表
	rg.POST("/embedding-configs", placeholder())                // 创建 Embedding 配置
	rg.PUT("/embedding-configs/:id", placeholder())             // 更新 Embedding 配置
}
