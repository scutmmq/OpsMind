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

// CreateEmbeddingConfigRequest 创建 Embedding 配置请求。
type CreateEmbeddingConfigRequest struct {
	Name           string `json:"name" binding:"required"`       // 配置名称
	ModelType      int16  `json:"model_type" binding:"required"` // 模型类型（1=API, 2=本地）
	APIEndpoint    string `json:"api_endpoint"`                  // API 地址（model_type=1 时必填）
	APIKey         string `json:"api_key"`                       // API Key
	LocalPath      string `json:"local_path"`                    // 本地路径（model_type=2 时必填）
	VectorDimension int   `json:"vector_dimension" binding:"required"` // 向量维度
	IsDefault      bool   `json:"is_default"`                    // 是否默认
}

// UpdateEmbeddingConfigRequest 更新 Embedding 配置请求。
type UpdateEmbeddingConfigRequest struct {
	Name           string `json:"name" binding:"required"`       // 配置名称
	ModelType      int16  `json:"model_type" binding:"required"` // 模型类型
	APIEndpoint    string `json:"api_endpoint"`                  // API 地址
	APIKey         string `json:"api_key"`                       // API Key
	LocalPath      string `json:"local_path"`                    // 本地路径
	VectorDimension int   `json:"vector_dimension" binding:"required"` // 向量维度
	IsDefault      bool   `json:"is_default"`                    // 是否默认
}
