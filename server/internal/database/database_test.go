//go:build integration

// Package database 的集成测试。
//
// 需要运行中的 PostgreSQL 实例。使用 integration build tag 跳过：
//
//	go test ./internal/database/...                    # 跳过（无 integration tag）
//	go test ./internal/database/... -tags=integration  # 执行（需要 PostgreSQL）
package database

import (
	"testing"

	"opsmind/internal/config"
)

// TestInit_Connection 验证使用配置建立数据库连接。
func TestInit_Connection(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "opsmind",
		Password: "opsmind_dev",
		DBName:   "opsmind",
		SSLMode:  "disable",
	}

	db, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init() 返回错误: %v", err)
	}

	// 验证连接可用
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层 sql.DB 失败: %v", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Ping 数据库失败: %v", err)
	}
}

// TestInit_PgvectorExtension 验证 pgvector 扩展已启用。
func TestInit_PgvectorExtension(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "opsmind",
		Password: "opsmind_dev",
		DBName:   "opsmind",
		SSLMode:  "disable",
	}

	db, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init() 返回错误: %v", err)
	}

	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	// 查询 pg_extension 验证 vector 扩展存在
	var exists bool
	err = db.Raw("SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'vector')").Scan(&exists).Error
	if err != nil {
		t.Fatalf("查询 pg_extension 失败: %v", err)
	}
	if !exists {
		t.Error("pgvector 扩展未启用，期望已启用")
	}
}

// TestInit_ConnectionPool 验证连接池配置。
func TestInit_ConnectionPool(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "opsmind",
		Password: "opsmind_dev",
		DBName:   "opsmind",
		SSLMode:  "disable",
	}

	db, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init() 返回错误: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层 sql.DB 失败: %v", err)
	}
	defer sqlDB.Close()

	// 验证连接池参数
	maxOpen := sqlDB.Stats().MaxOpenConnections
	if maxOpen != 25 {
		t.Errorf("MaxOpenConns = %d, 期望 25", maxOpen)
	}
}
