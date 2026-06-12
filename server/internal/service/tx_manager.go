package service

import "gorm.io/gorm"

// TxManager 事务管理器接口。
//
// 为跨 Repository 的事务操作提供统一抽象，避免 Service 直接持有 *gorm.DB。
type TxManager interface {
	Transaction(fn func(tx *gorm.DB) error) error
}

// GormTxManager 基于 GORM 的 TxManager 实现。
type GormTxManager struct {
	db *gorm.DB
}

func NewGormTxManager(db *gorm.DB) *GormTxManager {
	return &GormTxManager{db: db}
}

func (m *GormTxManager) Transaction(fn func(tx *gorm.DB) error) error {
	return m.db.Transaction(fn)
}
