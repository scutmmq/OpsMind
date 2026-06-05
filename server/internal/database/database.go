// Package database 负责初始化 PostgreSQL 数据库连接和 pgvector 扩展。
//
// 使用 GORM 作为 ORM 框架，通过 gorm.io/driver/postgres 连接 PostgreSQL 18。
// 初始化时自动启用 pgvector 扩展，为后续知识向量存储做准备。
package database

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"opsmind/internal/config"
)

// Init 初始化数据库连接并启用 pgvector 扩展。
//
// 为什么在 Init 中启用 pgvector 而非迁移脚本：
// pgvector 扩展是系统级对象，只需执行一次，放在连接初始化阶段
// 可以确保每次服务启动时扩展都可用，无需额外的迁移步骤。
//
// 连接池参数选择依据：
// - MaxOpenConns=25：MVP 阶段单实例部署，25 连接足够支撑并发请求
// - MaxIdleConns=10：保持空闲连接减少建连开销
// - ConnMaxLifetime=5min：避免长时间空闲连接被服务端关闭
func Init(cfg config.DatabaseConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	// 启用 pgvector 扩展
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return nil, fmt.Errorf("启用 pgvector 扩展失败: %w", err)
	}

	// 配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层 sql.DB 失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}
