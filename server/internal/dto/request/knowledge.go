// Package request 定义知识库管理相关的请求结构体。
//
// 与 TECH.md §5.2 知识库管理 API 对齐。
package request

// CreateKBRequest 创建知识库请求。
type CreateKBRequest struct {
	Name           string `json:"name" binding:"required"`     // 知识库名称
	Description    string `json:"description"`                  // 知识库描述
	EmbeddingModel string `json:"embedding_model"`             // Embedding 模型名称（可选，默认使用系统默认配置）
}

// UpdateKBRequest 更新知识库请求。
type UpdateKBRequest struct {
	Name        string `json:"name" binding:"required"` // 知识库名称
	Description string `json:"description"`              // 知识库描述
}

// CreateArticleRequest 创建知识文章请求。
type CreateArticleRequest struct {
	KBID     int64    `json:"kb_id" binding:"required"` // 所属知识库 ID
	Question string   `json:"question" binding:"required"` // 问题
	Answer   string   `json:"answer" binding:"required"`   // 答案
	Category string   `json:"category"`                    // 分类
	Tags     []string `json:"tags"`                       // 标签列表
}

// UpdateArticleRequest 更新知识文章请求。
type UpdateArticleRequest struct {
	Question string   `json:"question" binding:"required"` // 问题
	Answer   string   `json:"answer" binding:"required"`   // 答案
	Category string   `json:"category"`                    // 分类
	Tags     []string `json:"tags"`                       // 标签列表
}

// ReviewRequest 审核知识文章请求。
type ReviewRequest struct {
	Approved      bool   `json:"approved"`       // 是否通过
	ReviewComment string `json:"review_comment"`  // 审核意见
}
