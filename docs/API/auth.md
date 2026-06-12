# 认证接口

> 基础路径：`/api/v1/auth` | 认证：公开（login/refresh）或 JWT（change-password/logout）

## 1. 登录

```http
POST /api/v1/auth/login
```

**请求体：**

```json
{
  "username": "admin",
  "password": "Admin@123"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| username | string | ✓ | 用户名 |
| password | string | ✓ | 密码 |

**成功响应 (200)：**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "user": {
      "id": 1,
      "username": "admin",
      "real_name": "系统管理员",
      "phone": "13800000001",
      "email": "admin@opsmind.local",
      "first_login": false
    },
    "roles": ["系统管理员"],
    "permissions": ["user:manage", "ticket:manage", "knowledge:manage"],
    "menus": [
      {
        "id": 1,
        "name": "仪表盘",
        "path": "/admin/dashboard",
        "icon": "dashboard",
        "children": []
      }
    ]
  }
}
```

**错误响应：**

| code | 说明 |
|------|------|
| 10003 | 用户名或密码错误 |
| 10002 | 账号已被冻结 |

---

## 2. 刷新令牌

```http
POST /api/v1/auth/refresh
```

**请求体：**

```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

**成功响应 (200)：**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "user": { },
    "roles": [],
    "permissions": [],
    "menus": []
  }
}
```

> 刷新令牌有效期 7 天。过期需重新登录。

---

## 3. 修改密码

```http
POST /api/v1/auth/me/change-password
Authorization: Bearer <token>
```

**请求体：**

```json
{
  "old_password": "Admin@123",
  "new_password": "NewPass456"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| old_password | string | ✓ | 旧密码 |
| new_password | string | ✓ | 新密码（8-32 位，须含大小写字母+数字） |

**密码策略：**
- 长度 8-32 位
- 必须包含大写字母、小写字母、数字
- 正则：`^(?=.*[a-z])(?=.*[A-Z])(?=.*\d).{8,32}$`

**成功响应 (200)：**

```json
{
  "code": 0,
  "message": "success",
  "data": null
}
```

---

## 4. 登出

```http
POST /api/v1/auth/me/logout
Authorization: Bearer <token>
```

**成功响应 (200)：**

```json
{
  "code": 0,
  "message": "success",
  "data": null
}
```

> MVP 阶段登出为客户端行为（清除本地 token），后端不做 token 黑名单。
