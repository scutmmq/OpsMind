// Package main 是 OpsMind 后端服务的入口。
//
// 负责初始化配置、数据库连接、路由注册和 HTTP 服务启动。
// MVP 阶段采用单体分层架构，所有模块在同一进程内运行。
package main

import (
	"fmt"
	"log/slog"
	"os"

	"opsmind/internal/config"
	"opsmind/internal/router"
)

func main() {
	slog.Info("OpsMind 服务启动中...")

	// 加载配置
	cfg, err := config.Load("")
	if err != nil {
		slog.Error("加载配置失败", "error", err)
		os.Exit(1)
	}

	// 生产模式下 JWT 密钥必须非空，否则拒绝启动。
	// 调试模式仅输出警告，便于本地开发不配置 JWT 也能启动。
	if cfg.JWT.Secret == "" {
		if cfg.Server.Mode == "release" {
			slog.Error("JWT 密钥为空，生产模式不允许启动，请设置环境变量 OPSMIND_JWT_SECRET")
			os.Exit(1)
		}
		slog.Warn("JWT 密钥为空，JWT 认证功能不可用（仅调试模式允许）")
	}

	// 初始化路由（数据库参数暂传 nil，后续任务补充）
	r := router.Setup(cfg, nil)

	// 启动 HTTP 服务
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	slog.Info("HTTP 服务已启动", "addr", addr)

	if err := r.Run(addr); err != nil {
		slog.Error("HTTP 服务启动失败", "error", err)
		os.Exit(1)
	}
}
