// Package main 是 OpsMind 后端服务的入口。
//
// 负责初始化配置、数据库连接、路由注册和 HTTP 服务启动。
// MVP 阶段采用单体分层架构，所有模块在同一进程内运行。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"opsmind/internal/adapter"
	"opsmind/internal/config"
	"opsmind/internal/database"
	"opsmind/internal/handler"
	"opsmind/internal/rag"
	"opsmind/internal/repository"
	"opsmind/internal/router"
	"opsmind/internal/service"
)

func main() {
	slog.Info("OpsMind 服务启动中...")

	// 1. 加载配置
	cfg, err := config.Load("")
	if err != nil {
		slog.Error("加载配置失败", "error", err)
		os.Exit(1)
	}

	// 生产模式下 JWT 密钥必须非空，否则拒绝启动。
	if cfg.JWT.Secret == "" {
		if cfg.Server.Mode == "release" {
			slog.Error("JWT 密钥为空，生产模式不允许启动，请设置环境变量 OPSMIND_JWT_SECRET")
			os.Exit(1)
		}
		slog.Warn("JWT 密钥为空，JWT 认证功能不可用（仅调试模式允许）")
	}

	// 2. 初始化数据库连接
	db, err := database.Init(cfg.Database)
	if err != nil {
		slog.Error("数据库连接失败", "error", err)
		os.Exit(1)
	}
	slog.Info("数据库连接成功")

	// 3. 自动迁移（开发/测试阶段自动建表，生产环境建议用独立迁移脚本）
	if err := database.AutoMigrate(db); err != nil {
		slog.Error("数据库迁移失败", "error", err)
		os.Exit(1)
	}
	slog.Info("数据库迁移完成")

	// 4. 初始化 v2 Adapter 层（LLMClient / EmbeddingClient / VectorStore）
	// LLM 客户端超时：同步 60s，流式 SSE 长连接通过 ctx 控制
	llmTimeout := 60 * time.Second
	llmClient := adapter.NewOpenAIClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, llmTimeout)
	embeddingClient := adapter.NewOpenAIEmbeddingClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, 30*time.Second)
	slog.Info("v2 LLM/Embedding 客户端已初始化", "base_url", cfg.LLM.BaseURL, "model", cfg.LLM.Model)

	// pgvector 向量存储（使用与 GORM 相同的 DSN）
	pgDSN := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName, cfg.Database.SSLMode)
	vectorStore, err := adapter.NewPgvectorStore(pgDSN)
	if err != nil {
		slog.Warn("pgvector 连接失败，向量检索功能不可用", "error", err)
	} else {
		slog.Info("pgvector VectorStore 已连接")
	}

	// 5. 初始化 Repository 层
	configRepo := repository.NewConfigRepo(db)
	userRepo := repository.NewUserRepo(db)
	roleRepo := repository.NewRoleRepo(db)
	ticketRepo := repository.NewTicketRepo(db)
	knowledgeRepo := repository.NewKnowledgeRepo(db)
	chatRepo := repository.NewChatRepo(db)
	messageRepo := repository.NewMessageRepo(db)
	auditRepo := repository.NewAuditRepo(db)

	// 6. 初始化 Service 层
	authService := service.NewAuthService(userRepo, db)
	userService := service.NewUserService(userRepo, db)
	roleService := service.NewRoleService(roleRepo, userRepo, db)
	ticketService := service.NewTicketService(ticketRepo)

	// v2: LLM 配置管理（从 DB 加载默认配置）
	llmConfigRepo := repository.NewLlmConfigRepo(db)
	llmConfigSvc := service.NewLLMConfigService(llmConfigRepo)
	slog.Info("v2 LLM 配置服务已初始化")

	// v2: RAG 引擎组件
	embedder := rag.NewEmbedder(embeddingClient, 20)
	bm25Seg := rag.NewGseSegmenter()
	bm25Retriever := rag.NewBM25Retriever(bm25Seg, 30*time.Minute)
	if vectorStore != nil {
		slog.Info("v2 RAG 引擎组件已就绪（pgvector 已连接）")
	}

	// v2: KnowledgeServiceV2（自建 pgvector 发布管道）
	knowledgeServiceV2 := service.NewKnowledgeServiceV2(knowledgeRepo, nil, embedder, vectorStore, nil)
	_ = knowledgeServiceV2 // M5 Handler 接入时使用

	// v2: ChatServiceV2（自建 Pipeline 检索）
	chatServiceV2 := service.NewChatServiceV2(knowledgeRepo, chatRepo, nil, llmClient, llmConfigSvc.GetManager())
	_ = chatServiceV2 // M5 Handler 接入时使用

	// v1 兼容：占位（后续 M5/M7 清理）
	knowledgeService := &service.KnowledgeService{}
	chatService := &service.ChatService{}
	_ = knowledgeService
	_ = chatService

	messageService := service.NewMessageService(messageRepo)
	dashboardService := service.NewDashboardService(db)
	configService := service.NewConfigService(configRepo)

	_ = embedder
	_ = bm25Retriever
	_ = llmConfigSvc
	_ = vectorStore

	// 7. 初始化 Handler 层
	authHandler := handler.NewAuthHandler(authService)
	userHandler := handler.NewUserHandler(userService)
	roleHandler := handler.NewRoleHandler(roleService)
	ticketHandler := handler.NewTicketHandler(ticketService, knowledgeService)
	knowledgeHandler := handler.NewKnowledgeHandler(knowledgeService)
	chatHandler := handler.NewChatHandler(chatService)
	messageHandler := handler.NewMessageHandler(messageService)
	dashboardHandler := handler.NewDashboardHandler(dashboardService)
	auditHandler := handler.NewAuditHandler(auditRepo, db)
	configHandler := handler.NewConfigHandler(configService)

	// 8. 初始化后台调度器
	scheduler := service.NewScheduler(ticketRepo)
	slog.Info("后台调度器已创建")

	// 9. 设置路由
	r := router.Setup(cfg, &router.Handlers{
		Auth:      authHandler,
		User:      userHandler,
		Role:      roleHandler,
		Ticket:    ticketHandler,
		Knowledge: knowledgeHandler,
		Chat:      chatHandler,
		Message:   messageHandler,
		Dashboard: dashboardHandler,
		Audit:     auditHandler,
		Config:    configHandler,
	})

	// 10. 创建 HTTP Server（支持优雅关闭）
	// WriteTimeout 设为 0 以支持 SSE 长连接；应用层通过 context.WithTimeout 控制超时
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // SSE 流式响应无固定超时
		IdleTimeout:  60 * time.Second,
	}

	// 11. 启动调度器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler.Start(ctx)

	// 12. 启动 HTTP 服务（goroutine）
	go func() {
		slog.Info("HTTP 服务已启动", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP 服务启动失败", "error", err)
			os.Exit(1)
		}
	}()

	// 13. 等待退出信号（SIGINT / SIGTERM）
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("收到退出信号，开始优雅关闭...", "signal", sig)

	// 14. 优雅关闭
	// 先停止调度器
	scheduler.Stop()
	cancel()

	// 再关闭 HTTP 服务（最多等待 10 秒）
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP 服务关闭失败", "error", err)
	} else {
		slog.Info("HTTP 服务已关闭")
	}

	slog.Info("OpsMind 服务已停止")
}
