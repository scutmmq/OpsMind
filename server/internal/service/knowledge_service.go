// Package service 实现知识库管理业务逻辑。
//
// v2 迁移说明：KnowledgeService v1（依赖 AnythingLLM RagClient）中的 RAG 操作
// 已由 KnowledgeServiceV2（自建 pgvector 管道）替代。
// 本文件保留知识库 CRUD、文章 CRUD、审核流程等核心业务逻辑，
// RagClient 依赖已移除——Publish/Disable/RetrySync 的向量同步由 v2 Service 负责。
//
// v1 EmbeddingConfig 方法暂保留（M7 后续另行迁移至独立的 LlmConfigService）。
package service

import (
	"encoding/json"
	"strings"

	"opsmind/internal/dto/request"
	"opsmind/internal/dto/response"
	"opsmind/internal/model"
	"opsmind/internal/repository"
	"opsmind/pkg/errcode"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// KnowledgeService 知识库管理服务（v1 占位，RAG 操作已迁移至 KnowledgeServiceV2）。
type KnowledgeService struct {
	repo *repository.KnowledgeRepo
}

// NewKnowledgeService 创建 KnowledgeService 实例。
func NewKnowledgeService(repo *repository.KnowledgeRepo) *KnowledgeService {
	return &KnowledgeService{repo: repo}
}

// =============================================================================
// KnowledgeBase
// =============================================================================

// CreateKB 创建知识库（v2：不再调用 RagClient.CreateWorkspace，仅写 PostgreSQL）。
func (s *KnowledgeService) CreateKB(req request.CreateKBRequest, userID int64) error {
	kb := &model.KnowledgeBase{
		Name:        req.Name,
		Description: req.Description,
		CreatedBy:   userID,
	}

	return s.repo.CreateKB(kb)
}

// UpdateKB 更新知识库信息。
func (s *KnowledgeService) UpdateKB(id int64, req request.UpdateKBRequest) error {
	kb, err := s.repo.FindKBByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return AppError{Code: errcode.ErrNotFound, Message: "知识库不存在"}
		}
		return err
	}

	kb.Name = req.Name
	kb.Description = req.Description

	return s.repo.UpdateKB(kb)
}

// ListKBs 列出全部知识库。
func (s *KnowledgeService) ListKBs() ([]response.KBResponse, error) {
	kbs, err := s.repo.ListKBs()
	if err != nil {
		return nil, err
	}

	result := make([]response.KBResponse, len(kbs))
	for i, kb := range kbs {
		result[i] = response.KBResponse{
			ID:              kb.ID,
			Name:            kb.Name,
			Description:     kb.Description,
			EmbeddingModel:  kb.EmbeddingModel,
			VectorDimension: kb.VectorDimension,
			CreatedBy:       kb.CreatedBy,
			CreatedAt:       kb.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:       kb.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	return result, nil
}

// =============================================================================
// KnowledgeArticle
// =============================================================================

// CreateArticle 创建知识文章（草稿状态）。
func (s *KnowledgeService) CreateArticle(req request.CreateArticleRequest, userID int64) error {
	_, err := s.repo.FindKBByID(req.KBID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return AppError{Code: errcode.ErrNotFound, Message: "知识库不存在"}
		}
		return err
	}

	tagsJSON := marshalTags(req.Tags)
	article := &model.KnowledgeArticle{
		KBID:      req.KBID,
		Question:  req.Question,
		Answer:    req.Answer,
		Category:  req.Category,
		Tags:      tagsJSON,
		Status:    1, // 草稿
		CreatedBy: userID,
	}

	return s.repo.CreateArticle(article)
}

// UpdateArticle 更新文章（仅草稿/驳回状态可编辑）。
func (s *KnowledgeService) UpdateArticle(id int64, req request.UpdateArticleRequest, userID int64) error {
	article, err := s.repo.FindArticleByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return AppError{Code: errcode.ErrNotFound, Message: "文章不存在"}
		}
		return err
	}

	if article.Status != 1 && article.Status != 5 {
		return AppError{Code: errcode.ErrParam, Message: "仅草稿和驳回状态可编辑"}
	}

	article.Question = req.Question
	article.Answer = req.Answer
	article.Category = req.Category
	article.Tags = marshalTags(req.Tags)

	return s.repo.UpdateArticle(article)
}

// SubmitReview 提交审核（草稿→待审核）。
func (s *KnowledgeService) SubmitReview(id int64, userID int64) error {
	article, err := s.repo.FindArticleByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return AppError{Code: errcode.ErrNotFound, Message: "文章不存在"}
		}
		return err
	}

	if article.Status != 1 {
		return AppError{Code: errcode.ErrParam, Message: "仅草稿状态可提交审核"}
	}

	return s.repo.UpdateArticleStatus(id, 2)
}

// Review 审核文章（待审核→已通过/已驳回）。
func (s *KnowledgeService) Review(id int64, reviewerID int64, req request.ReviewRequest) error {
	article, err := s.repo.FindArticleByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return AppError{Code: errcode.ErrNotFound, Message: "文章不存在"}
		}
		return err
	}

	if article.Status != 2 {
		return AppError{Code: errcode.ErrParam, Message: "仅待审核状态可审核"}
	}

	if article.CreatedBy == reviewerID {
		return AppError{Code: errcode.ErrParam, Message: "审核人不能是创建人"}
	}

	if req.Approved {
		article.Status = 3
		article.ReviewedBy = &reviewerID
		return s.repo.UpdateArticle(article)
	}

	if strings.TrimSpace(req.ReviewComment) == "" {
		return AppError{Code: errcode.ErrParam, Message: "驳回时必须填写审核意见"}
	}

	article.Status = 5
	article.ReviewComment = req.ReviewComment
	article.ReviewedBy = &reviewerID
	return s.repo.UpdateArticle(article)
}

// Publish 发布已审核通过的文章（v2: 向量同步由 KnowledgeServiceV2.PublishV2 负责）。
func (s *KnowledgeService) Publish(id int64, publisherID int64) error {
	article, err := s.repo.FindArticleByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return AppError{Code: errcode.ErrNotFound, Message: "文章不存在"}
		}
		return err
	}

	if article.Status != 3 {
		return AppError{Code: errcode.ErrParam, Message: "仅已审核通过的文章可发布"}
	}

	article.Status = 4
	article.PublishedBy = &publisherID
	return s.repo.UpdateArticle(article)
}

// Disable 停用已发布文章（v2: 向量删除由 KnowledgeServiceV2.DisableV2 负责）。
func (s *KnowledgeService) Disable(id int64) error {
	article, err := s.repo.FindArticleByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return AppError{Code: errcode.ErrNotFound, Message: "文章不存在"}
		}
		return err
	}

	article.Status = 0
	return s.repo.UpdateArticle(article)
}

// Enable 恢复已停用文章为草稿状态。
func (s *KnowledgeService) Enable(id int64) error {
	article, err := s.repo.FindArticleByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return AppError{Code: errcode.ErrNotFound, Message: "文章不存在"}
		}
		return err
	}

	if article.Status != 0 {
		return AppError{Code: errcode.ErrParam, Message: "仅已停用状态的文章可恢复"}
	}

	article.Status = 1
	return s.repo.UpdateArticle(article)
}

// RetrySync 重试同步（v2 占位——由 KnowledgeServiceV2 负责文档处理重试）。
func (s *KnowledgeService) RetrySync(id int64) error {
	// v2: RagClient 已移除。文档上传使用 KnowledgeServiceV2.RetryDocument。
	article, err := s.repo.FindArticleByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return AppError{Code: errcode.ErrNotFound, Message: "文章不存在"}
		}
		return err
	}

	// 保留状态重置逻辑
	_ = s.repo.UpdateChunkSyncStatus(id, "pending", "")
	_ = article // 占位
	return nil
}

// ListArticles 分页查询文章列表。
func (s *KnowledgeService) ListArticles(kbID int64, status int, page, pageSize int) (*response.ArticleListResponse, error) {
	articles, total, err := s.repo.ListArticles(kbID, status, page, pageSize)
	if err != nil {
		return nil, err
	}

	result := make([]response.ArticleResponse, len(articles))
	for i, a := range articles {
		syncStatus := getAggregateSyncStatus(&a)

		result[i] = response.ArticleResponse{
			ID:            a.ID,
			KBID:          a.KBID,
			KBName:        a.KnowledgeBase.Name,
			Question:      a.Question,
			Answer:        a.Answer,
			Category:      a.Category,
			Tags:          unmarshalTags(a.Tags),
			Status:        a.Status,
			StatusText:    statusText(a.Status),
			CreatedBy:     a.CreatedBy,
			ReviewedBy:    a.ReviewedBy,
			ReviewComment: a.ReviewComment,
			SyncStatus:    syncStatus,
			CreatedAt:     a.CreatedAt,
			UpdatedAt:     a.UpdatedAt,
		}
	}

	return &response.ArticleListResponse{
		Articles: result,
		Total:    total,
	}, nil
}

// GetArticleDetail 获取文章详情。
func (s *KnowledgeService) GetArticleDetail(id int64) (*response.ArticleDetailResponse, error) {
	article, err := s.repo.FindArticleByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, AppError{Code: errcode.ErrNotFound, Message: "文章不存在"}
		}
		return nil, err
	}

	chunks, err := s.repo.FindChunksByArticleID(id)
	if err != nil {
		return nil, err
	}

	chunkResponses := make([]response.ChunkResponse, len(chunks))
	for i, c := range chunks {
		chunkResponses[i] = response.ChunkResponse{
			ID:              c.ID,
			Content:         c.Content,
			EmbeddingModel:  c.EmbeddingModel,
			VectorDimension: c.VectorDimension,
			SyncStatus:      c.SyncStatus,
			SyncError:       c.SyncError,
			SyncedAt:        c.SyncedAt,
		}
	}

	syncStatus := "none"
	if len(chunks) > 0 {
		syncStatus = chunks[0].SyncStatus
	}

	return &response.ArticleDetailResponse{
		ArticleResponse: response.ArticleResponse{
			ID:            article.ID,
			KBID:          article.KBID,
			KBName:        article.KnowledgeBase.Name,
			Question:      article.Question,
			Answer:        article.Answer,
			Category:      article.Category,
			Tags:          unmarshalTags(article.Tags),
			Status:        article.Status,
			StatusText:    statusText(article.Status),
			CreatedBy:     article.CreatedBy,
			ReviewedBy:    article.ReviewedBy,
			ReviewComment: article.ReviewComment,
			SyncStatus:    syncStatus,
			CreatedAt:     article.CreatedAt,
			UpdatedAt:     article.UpdatedAt,
		},
		Chunks: chunkResponses,
	}, nil
}

// =============================================================================
// EmbeddingConfig（v1 保留）
// =============================================================================

// CreateEmbeddingConfig 创建 Embedding 配置。
func (s *KnowledgeService) CreateEmbeddingConfig(req request.CreateEmbeddingConfigRequest) error {
	if req.ModelType == 1 && req.APIEndpoint == "" {
		return AppError{Code: errcode.ErrParam, Message: "API 类型必须填写 api_endpoint"}
	}
	if req.ModelType == 2 && req.LocalPath == "" {
		return AppError{Code: errcode.ErrParam, Message: "本地类型必须填写 local_path"}
	}

	if req.IsDefault {
		if err := s.clearDefaultEmbedding(); err != nil {
			return err
		}
	}

	cfg := &model.EmbeddingConfig{
		Name:           req.Name,
		ModelType:      req.ModelType,
		APIEndpoint:    req.APIEndpoint,
		APIKey:         req.APIKey,
		LocalPath:      req.LocalPath,
		VectorDimension: req.VectorDimension,
		IsDefault:      req.IsDefault,
	}

	return s.repo.CreateEmbeddingConfig(cfg)
}

// UpdateEmbeddingConfig 更新 Embedding 配置。
func (s *KnowledgeService) UpdateEmbeddingConfig(id int64, req request.UpdateEmbeddingConfigRequest) error {
	cfg, err := s.repo.ListEmbeddingConfigs()
	if err != nil {
		return err
	}

	found := false
	for _, c := range cfg {
		if c.ID == id {
			found = true
			break
		}
	}
	if !found {
		return AppError{Code: errcode.ErrNotFound, Message: "Embedding 配置不存在"}
	}

	if req.IsDefault {
		if err := s.clearDefaultEmbedding(); err != nil {
			return err
		}
	}

	updated := &model.EmbeddingConfig{
		ID:             id,
		Name:           req.Name,
		ModelType:      req.ModelType,
		APIEndpoint:    req.APIEndpoint,
		APIKey:         req.APIKey,
		LocalPath:      req.LocalPath,
		VectorDimension: req.VectorDimension,
		IsDefault:      req.IsDefault,
	}

	return s.repo.UpdateEmbeddingConfig(updated)
}

// ListEmbeddingConfigs 列出全部 Embedding 配置。
func (s *KnowledgeService) ListEmbeddingConfigs() ([]response.EmbeddingConfigResponse, error) {
	configs, err := s.repo.ListEmbeddingConfigs()
	if err != nil {
		return nil, err
	}

	result := make([]response.EmbeddingConfigResponse, len(configs))
	for i, c := range configs {
		result[i] = response.EmbeddingConfigResponse{
			ID:             c.ID,
			Name:           c.Name,
			ModelType:      c.ModelType,
			APIEndpoint:    c.APIEndpoint,
			APIKey:         c.APIKey,
			LocalPath:      c.LocalPath,
			VectorDimension: c.VectorDimension,
			IsDefault:      c.IsDefault,
			CreatedAt:      c.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	return result, nil
}

// DeleteEmbeddingConfig 删除 Embedding 配置。
func (s *KnowledgeService) DeleteEmbeddingConfig(id int64) error {
	if err := s.repo.DeleteEmbeddingConfig(id); err != nil {
		if err == gorm.ErrRecordNotFound {
			return AppError{Code: errcode.ErrNotFound, Message: "Embedding 配置不存在"}
		}
		return err
	}
	return nil
}

// clearDefaultEmbedding 清空所有配置的 is_default 标志。
func (s *KnowledgeService) clearDefaultEmbedding() error {
	configs, err := s.repo.ListEmbeddingConfigs()
	if err != nil {
		return err
	}
	for _, c := range configs {
		if c.IsDefault {
			c.IsDefault = false
			if err := s.repo.UpdateEmbeddingConfig(&c); err != nil {
				return err
			}
		}
	}
	return nil
}

// =============================================================================
// 辅助函数
// =============================================================================

func marshalTags(tags []string) datatypes.JSON {
	if len(tags) == 0 {
		return datatypes.JSON(`[]`)
	}
	data, _ := json.Marshal(tags)
	return datatypes.JSON(data)
}

func unmarshalTags(data datatypes.JSON) []string {
	if len(data) == 0 {
		return []string{}
	}
	var tags []string
	_ = json.Unmarshal(data, &tags)
	if tags == nil {
		return []string{}
	}
	return tags
}

func statusText(status int16) string {
	switch status {
	case 0:
		return "已停用"
	case 1:
		return "草稿"
	case 2:
		return "待审核"
	case 3:
		return "已通过"
	case 4:
		return "已发布"
	case 5:
		return "已驳回"
	default:
		return "未知"
	}
}

func getAggregateSyncStatus(article *model.KnowledgeArticle) string {
	switch article.Status {
	case 4:
		return "synced"
	case 0:
		return "disabled"
	default:
		return "pending"
	}
}
