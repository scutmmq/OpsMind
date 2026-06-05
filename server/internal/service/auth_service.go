// Package service 实现认证业务逻辑。
//
// AuthService 处理登录、刷新令牌、修改密码等认证相关操作。
// 依赖 UserRepo 进行用户数据访问，依赖 pkg/jwt 生成令牌。
package service

import (
	"errors"
	"fmt"
	"os"
	"time"

	"opsmind/internal/dto/response"
	"opsmind/internal/model"
	"opsmind/internal/repository"
	"opsmind/pkg/hash"
	"opsmind/pkg/jwt"

	"gorm.io/gorm"
)

// AppError 业务错误，包含错误码。
//
// 为什么自定义 error 类型而非直接用 errcode 常量：
// Handler 层需要根据错误码返回不同 HTTP 状态码，AppError 携带 code 便于判断。
type AppError struct {
	Code    int
	Message string
}

func (e AppError) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// AuthService 认证业务逻辑
type AuthService struct {
	userRepo *repository.UserRepo
}

// NewAuthService 创建 AuthService 实例
func NewAuthService(userRepo *repository.UserRepo) *AuthService {
	return &AuthService{userRepo: userRepo}
}

// Login 用户登录。
//
// 流程：查用户 → bcrypt 校验 → 检查状态 → 生成令牌 → 组装返回。
// 为什么密码错误和用户不存在返回相同错误码（10003）：
// 避免用户名枚举攻击，不暴露"用户是否存在"信息。
func (s *AuthService) Login(username, password string) (*response.LoginResponse, error) {
	user, err := s.userRepo.GetByUsername(username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, AppError{Code: 10003, Message: "用户名或密码错误"}
		}
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}

	if !hash.CheckPassword(user.PasswordHash, password) {
		return nil, AppError{Code: 10003, Message: "用户名或密码错误"}
	}

	if user.Status == 2 {
		return nil, AppError{Code: 10002, Message: "账号已被冻结"}
	}

	return s.buildLoginResponse(user)
}

// RefreshToken 刷新令牌。
//
// 解析 refresh_token 后重新生成令牌对。
// 为什么不直接生成新 access_token：统一走令牌对刷新，客户端逻辑更简单。
func (s *AuthService) RefreshToken(refreshToken string) (*response.LoginResponse, error) {
	claims, err := jwt.ParseToken(refreshToken, jwtSecret())
	if err != nil {
		return nil, AppError{Code: 10001, Message: "刷新令牌无效或已过期"}
	}

	user, err := s.userRepo.GetByID(claims.UserID)
	if err != nil {
		return nil, AppError{Code: 10001, Message: "用户不存在"}
	}

	if user.Status == 2 {
		return nil, AppError{Code: 10002, Message: "账号已被冻结"}
	}

	return s.buildLoginResponse(user)
}

// ChangePassword 修改密码。
//
// 流程：查用户 → 校验旧密码 → 校验新密码策略 → 更新哈希 → 设置 first_login=false。
// 为什么先校验旧密码再校验新密码策略：旧密码错误是更常见的场景，先返回更有用的错误信息。
func (s *AuthService) ChangePassword(userID int64, oldPwd, newPwd string) error {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return fmt.Errorf("查询用户失败: %w", err)
	}

	if !hash.CheckPassword(user.PasswordHash, oldPwd) {
		return AppError{Code: 10003, Message: "旧密码错误"}
	}

	if err := hash.ValidatePassword(newPwd); err != nil {
		return AppError{Code: 10003, Message: err.Error()}
	}

	newHash, err := hash.HashPassword(newPwd)
	if err != nil {
		return fmt.Errorf("密码哈希失败: %w", err)
	}

	user.PasswordHash = newHash
	user.FirstLogin = false
	user.UpdatedAt = time.Now()

	if err := s.userRepo.Update(user); err != nil {
		return fmt.Errorf("更新密码失败: %w", err)
	}

	return nil
}

// buildLoginResponse 根据用户信息组装登录响应。
//
// 为什么提取为独立函数：Login 和 RefreshToken 共用相同的令牌生成和响应组装逻辑。
func (s *AuthService) buildLoginResponse(user *model.User) (*response.LoginResponse, error) {
	// MVP 阶段角色和权限从 UserRole/Role 表查询，暂时返回空
	// 后续 T15 实现 RoleRepo 后补充
	roles := []string{}
	permissions := []string{}

	accessToken, err := jwt.GenerateAccessToken(
		user.ID, user.Username, roles,
		jwtSecret(), 2*time.Hour,
	)
	if err != nil {
		return nil, fmt.Errorf("生成 access_token 失败: %w", err)
	}

	refreshToken, err := jwt.GenerateRefreshToken(
		user.ID, user.Username, roles,
		jwtSecret(), 7*24*time.Hour,
	)
	if err != nil {
		return nil, fmt.Errorf("生成 refresh_token 失败: %w", err)
	}

	return &response.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: response.UserInfo{
			ID:         user.ID,
			Username:   user.Username,
			RealName:   user.RealName,
			Phone:      user.Phone,
			Email:      user.Email,
			FirstLogin: user.FirstLogin,
		},
		Roles:       roles,
		Permissions: permissions,
		Menus:       []response.MenuItem{},
	}, nil
}

// jwtSecret 从环境变量读取 JWT 密钥。
//
// 为什么提供默认值：本地开发环境便利性。
// 生产环境必须通过环境变量 OPSMIND_JWT_SECRET 覆盖。
func jwtSecret() string {
	if s := os.Getenv("OPSMIND_JWT_SECRET"); s != "" {
		return s
	}
	return "opsmind_dev_secret_key_2024"
}
