// Package middleware 提供 Gin 中间件。
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// RequestIDKey 是请求 ID 在 Gin Context 和响应头中的键名。
	RequestIDKey = "X-Request-ID"
)

// RequestID 为每个请求生成唯一 ID 并写入响应头。
//
// 如果客户端已通过请求头传递 X-Request-ID，则直接复用（支持链路透传）。
// 否则生成 UUID v4 作为请求 ID。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(RequestIDKey)
		if rid == "" {
			rid = uuid.New().String()
		}

		c.Set(RequestIDKey, rid)
		c.Header(RequestIDKey, rid)
		c.Next()
	}
}
