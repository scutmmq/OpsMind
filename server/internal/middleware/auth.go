// Package middleware 提供 Gin 中间件。
//
// 本文件实现 JWT 认证中间件，从 Authorization 头提取 Bearer 令牌，
// 解析后将用户信息写入 Gin context，供下游 Handler 使用。
package middleware

import (
	"strings"

	"opsmind/pkg/errcode"
	"opsmind/pkg/jwt"
	"opsmind/pkg/response"

	"github.com/gin-gonic/gin"
)

// CurrentUser JWT 解析后的用户信息，写入 Gin context。
type CurrentUser struct {
	UserID   int64    `json:"user_id"`
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
}

// JWTAuth 返回 JWT 认证中间件。
//
// 为什么返回 gin.HandlerFunc 而非直接使用闭包：
// 函数签名更清晰，调用方通过参数传入 secret，便于测试和配置。
func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			abortWithError(c, errcode.ErrAuth, "缺失 Authorization 头")
			return
		}

		// 提取 Bearer 令牌
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			abortWithError(c, errcode.ErrAuth, "Authorization 格式错误，应为 Bearer <token>")
			return
		}

		claims, err := jwt.ParseToken(parts[1], secret)
		if err != nil {
			abortWithError(c, errcode.ErrAuth, "令牌无效或已过期")
			return
		}

		// 写入 context，供下游 Handler 使用
		c.Set("currentUser", CurrentUser{
			UserID:   claims.UserID,
			Username: claims.Username,
			Roles:    claims.Roles,
		})
		c.Set("userID", claims.UserID)

		c.Next()
	}
}

// abortWithError 中断请求并返回统一错误响应。
func abortWithError(c *gin.Context, code int, msg string) {
	response.Error(c, code, msg)
	c.Abort()
}
