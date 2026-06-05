//go:build integration

// Package service_test 验证系统配置 Repository 的数据访问功能。
//
// 需要运行中的 PostgreSQL 实例。运行方式：
//
//	go test ./tests/service/... -tags=integration -v -run TestConfigRepo
package service_test

import (
	"encoding/json"
	"testing"

	"opsmind/internal/config"
	"opsmind/internal/database"
	"opsmind/internal/model"
	"opsmind/internal/repository"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// jsonEqual 语义比较两个 JSON 字符串是否等价。
//
// 为什么不用字符串直接比较：PostgreSQL JSONB 会规范化格式
// （如 {"a":1} 存储后读出为 {"a": 1}），字符串比较会误判。
func jsonEqual(t *testing.T, got datatypes.JSON, expected string) {
	t.Helper()
	var a, b interface{}
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatalf("解析实际 JSON 失败: %v, 原始值: %s", err, string(got))
	}
	if err := json.Unmarshal([]byte(expected), &b); err != nil {
		t.Fatalf("解析期望 JSON 失败: %v, 原始值: %s", err, expected)
	}
	gotBytes, _ := json.Marshal(a)
	expectedBytes, _ := json.Marshal(b)
	if string(gotBytes) != string(expectedBytes) {
		t.Errorf("Value = %s, 期望 %s", string(got), expected)
	}
}

// setupTestDB 初始化测试数据库连接并自动迁移 SystemConfig 表。
//
// 为什么在每个 TestMain 中初始化而非全局复用：
// 集成测试需要隔离的数据库状态，TestMain 级别初始化可以确保
// 每个测试包有独立的连接，避免测试间相互干扰。
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbCfg := config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "opsmind",
		Password: "opsmind123",
		DBName:   "opsmind_test",
		SSLMode:  "disable",
	}

	db, err := database.Init(dbCfg)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}

	// 自动迁移 SystemConfig 表
	if err := db.AutoMigrate(&model.SystemConfig{}); err != nil {
		t.Fatalf("自动迁移 SystemConfig 失败: %v", err)
	}

	// 清理测试数据
	db.Exec("DELETE FROM system_configs")

	return db
}

// TestConfigRepo_GetByKey_Existing 验证按 key 查询已存在的配置。
func TestConfigRepo_GetByKey_Existing(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewConfigRepo(db)

	// 插入测试数据
	value := datatypes.JSON(`{"threshold":0.6}`)
	cfg := &model.SystemConfig{
		Key:         "ai.confidence_threshold",
		Value:       value,
		Description: "AI 置信度阈值",
		UpdatedBy:   1,
	}
	if err := db.Create(cfg).Error; err != nil {
		t.Fatalf("插入测试数据失败: %v", err)
	}

	// 查询
	result, err := repo.GetByKey("ai.confidence_threshold")
	if err != nil {
		t.Fatalf("GetByKey 失败: %v", err)
	}
	if result.Key != "ai.confidence_threshold" {
		t.Errorf("Key = %q, 期望 ai.confidence_threshold", result.Key)
	}
	if result.Description != "AI 置信度阈值" {
		t.Errorf("Description = %q, 期望 AI 置信度阈值", result.Description)
	}
	if result.UpdatedBy != 1 {
		t.Errorf("UpdatedBy = %d, 期望 1", result.UpdatedBy)
	}
}

// TestConfigRepo_GetByKey_NotFound 验证按 key 查询不存在的配置返回错误。
func TestConfigRepo_GetByKey_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewConfigRepo(db)

	_, err := repo.GetByKey("nonexistent.key")
	if err == nil {
		t.Fatal("期望返回错误, 实际为 nil")
	}
	if err != gorm.ErrRecordNotFound {
		t.Errorf("错误类型 = %v, 期望 gorm.ErrRecordNotFound", err)
	}
}

// TestConfigRepo_Upsert_UpdateExisting 验证更新已存在的配置。
func TestConfigRepo_Upsert_UpdateExisting(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewConfigRepo(db)

	// 插入初始数据
	initialValue := datatypes.JSON(`{"threshold":0.6}`)
	db.Create(&model.SystemConfig{
		Key:       "ai.confidence_threshold",
		Value:     initialValue,
		UpdatedBy: 1,
	})

	// 更新
	newValue := datatypes.JSON(`{"threshold":0.8}`)
	err := repo.Upsert("ai.confidence_threshold", newValue, 2)
	if err != nil {
		t.Fatalf("Upsert 更新失败: %v", err)
	}

	// 验证更新结果
	result, err := repo.GetByKey("ai.confidence_threshold")
	if err != nil {
		t.Fatalf("查询更新后的配置失败: %v", err)
	}
	jsonEqual(t, result.Value, `{"threshold":0.8}`)
	if result.UpdatedBy != 2 {
		t.Errorf("UpdatedBy = %d, 期望 2", result.UpdatedBy)
	}
}

// TestConfigRepo_Upsert_InsertNew 验证插入新配置。
func TestConfigRepo_Upsert_InsertNew(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewConfigRepo(db)

	newValue := datatypes.JSON(`{"max_retries":3}`)
	err := repo.Upsert("system.max_retries", newValue, 1)
	if err != nil {
		t.Fatalf("Upsert 插入失败: %v", err)
	}

	// 验证插入结果
	result, err := repo.GetByKey("system.max_retries")
	if err != nil {
		t.Fatalf("查询新插入的配置失败: %v", err)
	}
	if result.Key != "system.max_retries" {
		t.Errorf("Key = %q, 期望 system.max_retries", result.Key)
	}
	jsonEqual(t, result.Value, `{"max_retries":3}`)
	if result.UpdatedBy != 1 {
		t.Errorf("UpdatedBy = %d, 期望 1", result.UpdatedBy)
	}
}

// TestConfigRepo_List 验证列出全部配置。
func TestConfigRepo_List(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewConfigRepo(db)

	// 插入多条测试数据
	configs := []model.SystemConfig{
		{Key: "ai.confidence_threshold", Value: datatypes.JSON(`{"threshold":0.6}`), UpdatedBy: 1},
		{Key: "ai.default_top_k", Value: datatypes.JSON(`{"top_k":5}`), UpdatedBy: 1},
		{Key: "system.max_retries", Value: datatypes.JSON(`{"max_retries":3}`), UpdatedBy: 2},
	}
	for i := range configs {
		if err := db.Create(&configs[i]).Error; err != nil {
			t.Fatalf("插入测试数据失败: %v", err)
		}
	}

	// 查询全部
	result, err := repo.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("List 返回 %d 条, 期望 3", len(result))
	}

	// 验证包含所有 key
	keys := make(map[string]bool)
	for _, c := range result {
		keys[c.Key] = true
	}
	for _, expected := range []string{"ai.confidence_threshold", "ai.default_top_k", "system.max_retries"} {
		if !keys[expected] {
			t.Errorf("List 结果缺少 key: %s", expected)
		}
	}
}

// TestConfigRepo_List_Empty 验证空表时 List 返回空切片而非 nil。
func TestConfigRepo_List_Empty(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewConfigRepo(db)

	result, err := repo.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if result == nil {
		t.Fatal("List 返回 nil, 期望空切片")
	}
	if len(result) != 0 {
		t.Errorf("List 返回 %d 条, 期望 0", len(result))
	}
}
