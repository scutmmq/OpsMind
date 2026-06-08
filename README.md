# OpsMind — 运维数字员工系统

面向企业运维场景的 AI 数字员工系统，基于 Go + Vue 3 单体分层架构，集成 AnythingLLM RAG 实现智能问答、申告管理、知识库管理和 RBAC 权限控制。

## 当前进度

| 里程碑 | 状态 | 内容 |
|--------|------|------|
| M1 数据库与后端基础能力 | ✅ 完成 | Go 项目骨架、GORM 模型(16 表)、配置管理、中间件、路由注册 |
| M2 账号权限与后台框架 | ✅ 完成 | JWT 认证、RBAC 权限、用户 CRUD、角色管理、Vue 后台布局 |
| M3 知识库管理与 AI 服务 | 🔲 待开发 | 知识库 CRUD、审核发布流程、AnythingLLM 适配器 |
| M4 智能问答与申告处理 | 🔲 待开发 | RAG 集成、问答 API、申告状态机、站内消息 |
| M5 数据看板与日志审计 | 🔲 待开发 | 看板统计、审计日志、系统配置页面 |
| M6 联调测试与文档完善 | 🔲 待开发 | 集成测试、演示数据、Docker 完整编排 |

**当前可验收功能：** 用户认证登录、JWT 令牌刷新、修改密码、用户 CRUD（创建/编辑/冻结/恢复）、角色 CRUD（创建/编辑/删除/权限绑定）、基于 RBAC 的接口鉴权、Vue 后台管理框架（登录页 + 管理布局 + 动态菜单）。

---

## 技术栈

| 层级 | 技术 | 版本 |
|------|------|------|
| 后端框架 | Go + Gin | 1.22+ / 1.9+ |
| ORM | GORM | v1.25+ |
| 数据库 | PostgreSQL + pgvector | 18 / 0.7+ |
| 认证 | JWT (golang-jwt) | v5 |
| 前端框架 | Vue 3 + TypeScript | 3.4+ |
| UI 组件 | Radix Vue | 1.9+ |
| 状态管理 | Pinia | 2.1+ |
| 路由 | Vue Router | 4.3+ |

---

## 快速启动

### 前置条件

- **Go** 1.22+
- **Node.js** 18+
- **Docker Desktop**（提供 PostgreSQL）
- **Git Bash** 或 WSL（Windows 推荐）

### 1. 克隆项目

```bash
git clone <repo-url> OpsMind
cd OpsMind
```

### 2. 启动 PostgreSQL

```bash
docker compose up -d
```

验证数据库可用：

```bash
docker compose ps
# opsmind-postgres 状态应为 Up
```

### 3. 启动 Go 后端

```bash
cd server

# 安装依赖
go mod tidy

# 启动服务（自动建表）
go run ./cmd/main.go
```

看到以下日志表示启动成功：

```
INFO OpsMind 服务启动中...
INFO 数据库连接成功
INFO 数据库迁移完成
INFO HTTP 服务已启动 addr=:8080
```

### 4. 启动 Vue 前端

```bash
cd web

# 安装依赖（首次）
npm install

# 启动开发服务器
npm run dev
```

访问 `http://localhost:5173`

---

## 人工验收指南

> ⚠️ **当前 MVP 阶段：数据库无预置数据**。以下步骤从零开始验证 M1M2 全部功能，按顺序执行。

### 第一步：数据库初始化

启动后端后，GORM AutoMigrate 会自动创建 16 张表。验证：

```bash
docker compose exec postgres psql -U opsmind -d opsmind -c "\dt"
```

预期输出应包含：`users`, `roles`, `user_roles`, `menus`, `role_menus`, `tickets`, `ticket_records`, `knowledge_bases`, `knowledge_articles`, `knowledge_chunks`, `embedding_configs`, `chat_sessions`, `chat_messages`, `audit_logs`, `system_configs`, `messages`

### 第二步：创建基础数据

由于 seed.sql 尚未实现（T38），需要通过 API 手动创建角色和用户。按以下顺序执行：

#### 2.1 创建角色

```bash
# 创建系统管理员角色
curl -X POST http://localhost:8080/api/v1/admin/roles \
  -H "Content-Type: application/json" \
  -d '{"name":"系统管理员","description":"系统全局管理","permissions":["ticket:read","ticket:write","knowledge:read","knowledge:write","system:config","user:manage","audit:read"]}'

# 创建运维人员角色
curl -X POST http://localhost:8080/api/v1/admin/roles \
  -H "Content-Type: application/json" \
  -d '{"name":"运维人员","description":"处理申告和回访","permissions":["ticket:read","ticket:write","knowledge:read"]}'

# 创建报障人角色
curl -X POST http://localhost:8080/api/v1/admin/roles \
  -H "Content-Type: application/json" \
  -d '{"name":"报障人","description":"门户端用户","permissions":[]}'
```

> **注意：** 角色 API 当前无需认证（JWT 密钥为空时中间件跳过校验）。创建角色后，后续请求需要使用 JWT 令牌。

#### 2.2 创建用户

```bash
# 创建管理员账号（记下返回的 user ID）
curl -X POST http://localhost:8080/api/v1/admin/users \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Admin@123","real_name":"管理员","phone":"13800000001","role_ids":[1]}'

# 创建报障人账号
curl -X POST http://localhost:8080/api/v1/admin/users \
  -H "Content-Type: application/json" \
  -d '{"username":"reporter","password":"Reporter@123","real_name":"张三","phone":"13800000002","role_ids":[3]}'
```

> **密码策略：** 必须 8-32 位，含大写字母、小写字母和数字（正则 `^(?=.*[a-z])(?=.*[A-Z])(?=.*\d).{8,32}$`）

### 第三步：验收认证模块

#### 3.1 登录

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"Admin@123"}'
```

**预期响应：**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "access_token": "eyJhbGciOi...",
    "refresh_token": "eyJhbGciOi...",
    "user": {
      "id": 1,
      "username": "admin",
      "real_name": "管理员",
      "first_login": true
    },
    "roles": ["系统管理员"],
    "permissions": ["ticket:read", "ticket:write", ...],
    "menus": []
  }
}
```

**验证点：** `code=0`、`access_token` 非空、`roles` 含"系统管理员"、`first_login=true`。

保存返回的 `access_token` 为后续请求使用：

```bash
TOKEN="<access_token 的值>"
```

#### 3.2 修改密码（首次登录强制）

```bash
curl -X POST http://localhost:8080/api/v1/auth/change-password \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"old_password":"Admin@123","new_password":"OpsMind@2026"}'
```

**预期：** `{"code":0,"message":"success","data":null}`

#### 3.3 刷新令牌

```bash
# 用登录返回的 refresh_token
curl -X POST http://localhost:8080/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"<refresh_token 的值>"}'
```

**预期：** 返回新的 `access_token` 和 `refresh_token`。

#### 3.4 错误场景验证

```bash
# 错误密码 → 应返回 10003
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"WrongPass@123"}'

# 缺失参数 → 应返回 10003
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin"}'
```

### 第四步：验收用户管理

以下请求需要携带 JWT 令牌：

```bash
# 1. 用户列表（分页）
curl -X GET "http://localhost:8080/api/v1/admin/users?page=1&page_size=10" \
  -H "Authorization: Bearer $TOKEN"

# 2. 用户详情（含角色列表）
curl -X GET http://localhost:8080/api/v1/admin/users/1 \
  -H "Authorization: Bearer $TOKEN"

# 3. 搜索用户
curl -X GET "http://localhost:8080/api/v1/admin/users?keyword=admin" \
  -H "Authorization: Bearer $TOKEN"

# 4. 编辑用户
curl -X PUT http://localhost:8080/api/v1/admin/users/2 \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"real_name":"张三丰","phone":"13800000003","email":"zhang@example.com","role_ids":[3]}'

# 5. 冻结用户
curl -X PATCH http://localhost:8080/api/v1/admin/users/2/freeze \
  -H "Authorization: Bearer $TOKEN"

# 6. 恢复用户
curl -X PATCH http://localhost:8080/api/v1/admin/users/2/unfreeze \
  -H "Authorization: Bearer $TOKEN"

# 7. 重复冻结 → 应返回 10006
curl -X PATCH http://localhost:8080/api/v1/admin/users/2/freeze \
  -H "Authorization: Bearer $TOKEN"
```

### 第五步：验收角色管理

```bash
# 1. 角色列表
curl -X GET http://localhost:8080/api/v1/admin/roles \
  -H "Authorization: Bearer $TOKEN"

# 2. 角色详情
curl -X GET http://localhost:8080/api/v1/admin/roles/1 \
  -H "Authorization: Bearer $TOKEN"

# 3. 更新角色
curl -X PUT http://localhost:8080/api/v1/admin/roles/2 \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"运维工程师","description":"处理申告和回访","permissions":["ticket:read","ticket:write","knowledge:read","knowledge:write"]}'

# 4. 删除角色
curl -X DELETE http://localhost:8080/api/v1/admin/roles/2 \
  -H "Authorization: Bearer $TOKEN"
```

### 第六步：验收前端

1. 访问 `http://localhost:5173`
2. 输入用户名 `admin`，密码 `OpsMind@2026`（修改后的密码）
3. 登录成功后应跳转到 `/admin` 后台管理页面
4. 左侧应显示侧边栏，顶部显示用户名
5. 点击"用户管理"菜单 → 应显示用户列表
6. 点击"角色管理"菜单 → 应显示角色列表

### 第七步：验收权限控制

```bash
# 使用报障人账号登录
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"reporter","password":"Reporter@123"}'
```

报障人无 `user:manage` 权限，访问管理接口应返回 403：

```bash
# 使用报障人的 token
REPORTER_TOKEN="<报障人的 access_token>"

# 尝试访问用户列表 → 应返回 403
curl -X GET http://localhost:8080/api/v1/admin/users \
  -H "Authorization: Bearer $REPORTER_TOKEN"
```

---

## 项目结构

```
OpsMind/
├── docs/                           # 项目文档
│   ├── PRD.md                      # 产品需求文档
│   ├── TECH.md                     # 技术架构文档
│   ├── PLAN.md                     # 实施计划（38 任务，6 里程碑）
│   └── ANYTHINGLLM_AI_INTEGRATION.md
│
├── server/                         # Go 后端
│   ├── cmd/main.go                 # 入口（DB→Repo→Service→Handler→Router）
│   ├── internal/
│   │   ├── config/                 # Viper 配置管理
│   │   ├── database/               # GORM 连接 + AutoMigrate
│   │   ├── model/                  # 16 张表 GORM 模型 + 枚举常量
│   │   ├── middleware/             # JWT 认证 / RBAC 权限 / CORS / Logger / RequestID
│   │   ├── router/                 # Gin 路由注册（public / portal / admin 三组）
│   │   ├── handler/                # Handler 层（auth / user / role）
│   │   ├── service/                # Service 层（auth / user / role）
│   │   ├── repository/             # Repository 层（user / role / config）
│   │   ├── adapter/                # 外部服务适配层（RagClient / StorageClient — M3/M4 实现）
│   │   └── dto/                    # 请求/响应 DTO
│   ├── pkg/                        # 公共工具包（response / errcode / jwt / hash）
│   └── tests/                      # 测试代码（按子目录组织）
│
├── web/                            # Vue 3 前端
│   └── src/
│       ├── api/                    # Axios API 封装（auth / user）
│       ├── stores/                 # Pinia 状态（auth / app）
│       ├── router/                 # Vue Router + 路由守卫
│       ├── views/                  # 页面（auth / admin / portal）
│       ├── components/             # 通用组件（AdminLayout / PortalLayout / Pagination...）
│       ├── utils/                  # 工具函数（request / auth）
│       └── styles/                 # Linear Design 暗色主题
│
├── docker-compose.yml              # Docker Compose（当前: PostgreSQL; T37: 完整 6 服务）
├── .env.example                    # 环境变量模板
└── README.md                       # 本文件
```

---

## 常用命令

### 后端

```bash
cd server
go build ./cmd/...            # 编译
go run ./cmd/main.go           # 运行
go test ./tests/... -v         # 单元测试
go test ./tests/... -v -tags=integration  # 集成测试（需数据库）
```

### 前端

```bash
cd web
npm install                    # 安装依赖
npm run dev                    # 开发服务器 (localhost:5173)
npm run build                  # 生产构建
npm run type-check             # TypeScript 类型检查
npm run test                   # 单元测试
```

### Docker

```bash
docker compose up -d           # 启动
docker compose down            # 停止
docker compose down -v         # 停止并清除数据
```

---

## 文档索引

| 文档 | 说明 |
|------|------|
| [PRD.md](docs/PRD.md) | 产品需求文档 v2.2 |
| [TECH.md](docs/TECH.md) | 技术架构文档 v1.2 |
| [PLAN.md](docs/PLAN.md) | 实施计划（38 任务，6 里程碑） |
| [ANYTHINGLLM_AI_INTEGRATION.md](docs/ANYTHINGLLM_AI_INTEGRATION.md) | AnythingLLM 集成方案 v1.1 |
| [CLAUDE.md](CLAUDE.md) | 项目上下文指令（AI 开发辅助） |

---

## 预设角色与权限

| 角色 | 典型权限 |
|------|---------|
| 系统管理员 | ticket:read/write/assign, knowledge:read/write/review, system:config, user:manage, audit:read |
| 运维人员 | ticket:read/write, knowledge:read/write |
| 知识库管理员 | knowledge:read/write/review |
| 报障人 | 无后台权限，仅门户端 |

## 错误码速查

| 错误码 | HTTP 状态 | 说明 |
|--------|----------|------|
| 0 | 200 | 成功 |
| 10001 | 401 | 未登录或令牌过期 |
| 10002 | 403 | 无权限 |
| 10003 | 400 | 参数校验失败 |
| 10004 | 404 | 资源不存在 |
| 10005 | 409 | 资源冲突（如用户名重复） |
| 10006 | 409 | 用户已被冻结 |
| 10007 | 409 | 用户已处于正常状态 |
| 20001 | 500 | AI 服务不可用 |
| 20002 | 500 | RAG 服务不可用 |
| 20003 | 500 | 存储服务不可用 |
| 99999 | 500 | 未知错误 |
