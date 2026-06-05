// Package repository 提供用户的数据访问层。
//
// UserRepo 封装 users 表的查询和创建操作，供 UserService 调用。
// 为什么独立于 ConfigRepo：User 表的查询模式更丰富（按 ID/用户名/手机号），
// 且后续会扩展分页查询、角色关联等功能，独立 Repo 更利于演进。
package repository

import (
	"opsmind/internal/model"

	"gorm.io/gorm"
)

// UserRepo 用户数据访问
type UserRepo struct {
	db *gorm.DB
}

// NewUserRepo 创建 UserRepo 实例
func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

// GetByID 按 ID 查询用户。
//
// ID 是主键，查询不到时返回 gorm.ErrRecordNotFound。
func (r *UserRepo) GetByID(id int64) (*model.User, error) {
	var user model.User
	err := r.db.Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByUsername 按用户名查询用户。
//
// username 具有唯一索引，用于登录验证。
func (r *UserRepo) GetByUsername(username string) (*model.User, error) {
	var user model.User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByPhone 按手机号查询用户。
//
// phone 用于报障人身份识别和注册校验。
func (r *UserRepo) GetByPhone(phone string) (*model.User, error) {
	var user model.User
	err := r.db.Where("phone = ?", phone).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// ExistsByPhone 检查手机号是否已注册。
//
// 为什么单独封装而非复用 GetByPhone：
// 语义更清晰（布尔返回值 vs 结构体），且后续可优化为 SELECT 1 提升性能。
func (r *UserRepo) ExistsByPhone(phone string) (bool, error) {
	var count int64
	err := r.db.Model(&model.User{}).Where("phone = ?", phone).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Create 新增用户。
//
// 创建后 user.ID 会被 GORM 自动填充（autoIncrement）。
// 用户名唯一约束由数据库保证，重复时返回 PostgreSQL 错误。
func (r *UserRepo) Create(user *model.User) error {
	return r.db.Create(user).Error
}

// Update 更新用户全部字段。
//
// 为什么用 Save 而非 Updates：Save 会更新所有字段（包括零值），
// 适用于修改密码等需要更新 password_hash、first_login、updated_at 的场景。
func (r *UserRepo) Update(user *model.User) error {
	return r.db.Save(user).Error
}
