# PRD v2: RAG 对话模块重构 — 自建 RAG 引擎

| 项目 | 内容 |
| --- | --- |
| 文档版本 | v2.0 |
| 日期 | 2026-06-11 |
| 实施状态 | ✅ 已完成（M1-M7 全部交付） |
| 适用范围 | RAG 对话模块完整重构，覆盖 LLM/Embedding、文档解析、向量存储、RAG 管道、知识模型 |
| 关联文档 | [CHANGELOG](CHANGELOG.md) · [TECHv2](TECHv2.md) · [PLANv2](PLANv2.md) · [API 文档](../API/README.md) |

---

## 1. 概述

### 1.1 问题陈述

当前 OpsMind v1 的 RAG 链路完全依赖 AnythingLLM：

```
用户 → OpsMind Server → AnythingLLM → vLLM
                              │
                              ├─ 文档切分
                              ├─ Embedding 生成
                              ├─ LanceDB 向量存储
                              ├─ RAG 检索编排
                              └─ LLM 调用编排
```

**核心问题：**

- **不可控：** AnythingLLM 是黑盒 Node.js 服务，内部行为（切分策略、检索算法、embedding 质量控制）无法定制。
- **架构冗余：** 引入了一个额外的 Docker 服务（~2GB 镜像）、JWT 认证体系、API Key 管理流程，增加了部署和维护复杂度。
- **数据割裂：** 知识同步状态记录在 PostgreSQL 业务库，向量存储在 AnythingLLM LanceDB（本地文件），MinIO 存原始文档——三处数据难以保持一致性。
- **扩展受限：** 无法实现混合检索（BM25 + 向量）、查询改写、重排序等高级 RAG 技术。
- **开发体验差：** AnythingLLM 的 Node.js 技术栈与 OpsMind 的 Go 技术栈不一致，调试和问题定位困难。

### 1.2 解决方案

**移除 AnythingLLM，用 Go 自建 RAG 引擎替换整个 RAG 链路。**

v2 架构：

```
用户 → OpsMind Server (Go)
          │
          ├─ RAG 管道（自建 Go 模块）
          │   ├─ 查询改写 (Query Rewrite)
          │   ├─ 多路检索 (Multi-Route)
          │   ├─ 混合检索 (BM25 + pgvector + RRF)
          │   └─ 重排序 (Rerank)
          │
          ├─ 文档处理（自建 Go 模块）
          │   ├─ 多格式解析 (PDF/DOCX/MD/TXT)
          │   ├─ 智能分块 (RecursiveCharacterTextSplitter)
          │   ├─ Embedding 生成 (OpenAI-compatible API)
          │   └─ 向量写入 (pgvector)
          │
          ├─ LLM 客户端（自建 Go 适配层）
          │   ├─ llama.cpp server (OpenAI-compatible)
          │   └─ OpenAI API (或任意兼容 API)
          │
          ├─ PostgreSQL (pgvector)
          │   └─ 业务数据 + 向量存储（统一）
          │
          └─ MinIO
              └─ 原始文档存储
```

**核心变化：**

| 组件 | v1 | v2 |
| --- | --- | --- |
| RAG 服务 | AnythingLLM (Node.js Docker) | 自建 Go 模块 |
| LLM 推理 | vLLM (Docker) | llama.cpp server (可选 Docker) 或 OpenAI-compatible API |
| 向量存储 | AnythingLLM 内部 LanceDB | PostgreSQL + pgvector 扩展 |
| 文档处理 | AnythingLLM 内部处理 | 自建 Go 模块（PDF/DOCX/MD/TXT） |
| Embedding 生成 | AnythingLLM 调用 vLLM | OpsMind 直接调用 OpenAI-compatible API |
| Docker 服务数 | 6（含 anythingllm + vllm）| 4（pgvector + minio + server + web）+ 可选 llama.cpp |

### 1.3 不变的部分

以下 v1 模块不受本次重构影响：

- 用户/角色/RBAC 权限体系
- 申告管理（Ticket）及状态机
- 数据看板（Dashboard）
- 审计日志（AuditLog）
- 站内消息（Message）
- 系统配置（SystemConfig）
- 前端 Linear Design 设计系统
- JWT 认证流程

---

## 2. 目标

### 2.1 必须达成（P0）

- **G1：** 完全移除 AnythingLLM 依赖，Docker Compose 不再包含 anythingllm 服务
- **G2：** 自建 RAG 引擎支持完整检索管道：查询改写 → 多路检索 → 混合检索（BM25 + 向量 + RRF）→ 重排序
- **G3：** 向量存储使用 PostgreSQL + pgvector 扩展，业务数据和向量数据统一管理
- **G4：** 支持多格式文档上传解析（PDF/DOCX/MD/TXT），后端异步并行处理
- **G5：** LLM 和 Embedding 支持两种提供商：llama.cpp server（OpenAI-compatible）和 OpenAI API（或任意兼容 API）
- **G6：** 前端支持文档上传和知识库 UI 适配
- **G7：** SSE 流式输出从当前的「模拟分块」升级为「真正的 token 级别流式」

### 2.2 应该达成（P1）

- **G8：** BM25 检索在 Go 中自行实现（不含外部依赖，中文分词基于双字分词 bigram）
- **G9：** 文档异步解析支持进度反馈（通过 API 查询处理状态）
- **G10：** LLM/Embedding 提供商配置通过后台管理 UI 完成，支持多套配置

### 2.3 不做（P2/后续）

- 父子分块（parent-child chunking）— 参考项目有但 v2 不实现
- LLM 提供商的热切换（运行时动态切换模型）— 需重启服务
- 多知识库联合检索（跨 kb 向量索引查询）— MVP 单知识库检索

---

## 3. 用户故事

### US-V2-001: LLM 与 Embedding 提供商配置

**描述：** 作为系统管理员，我需要在后台配置 LLM 和 Embedding 的连接信息（llama.cpp server 或 OpenAI-compatible API），以便系统能够调用模型进行文本生成和向量生成。

**验收标准：**

- [ ] 系统管理员可在后台「模型配置」页面添加、编辑、删除 LLM/Embedding 提供商配置
- [ ] 每条配置包含：名称、提供商类型（llama.cpp / OpenAI-compatible）、API 地址（Base URL）、API Key（可选，密文存储）、模型名称、最大 Token 数、向量维度
- [ ] llama.cpp 和 OpenAI-compatible 两种类型共用同一个配置模型（两者 API 规范相同）
- [ ] Embedding 配置复用 LLM 提供商的连接参数，但可独立指定 embedding 模型名称（如 `bge-m3`）
- [ ] 支持「测试连接」按钮，验证 API 地址可达性和模型可用性
- [ ] 配置修改后即时生效（通过 `PUT /api/v1/admin/configs` 更新运行时配置）
- [ ] 至少有一条默认配置，系统首次启动时通过环境变量预填
- [ ] TypeScript 类型检查通过
- [ ] 在浏览器中使用 dev-browser skill 验证

---

### US-V2-002: 文档上传与异步解析

**描述：** 作为知识库管理员，我需要在知识库中上传 PDF / DOCX / MD / TXT 格式的文档，系统自动解析、分块、生成 embedding 并写入 pgvector，以便这些文档能被智能问答检索。

**验收标准：**

- [ ] 前端知识库编辑页提供文件上传区域，支持多文件同时选择（拖拽 + 点击选择）
- [ ] 支持 PDF / DOCX / MD / TXT 四种格式，上传时前端校验文件扩展名
- [ ] 文件大小限制为单文件 ≤ 20MB，上传前前端校验并提示
- [ ] `POST /api/v1/admin/knowledge-bases/:kb_id/documents/upload` 接收 multipart/form-data，返回 `document_id` 列表
- [ ] 上传完成后，MinIO 存储原始文件（bucket: `opsmind-knowledge`，路径: `kb_{kb_id}/{document_id}/{filename}`）
- [ ] 后端使用 Go 协程池（goroutine pool）并行解析多个文档，每个文档依次完成：解析文本 → 分块 → 生成 embedding → 写入 pgvector
- [ ] `GET /api/v1/admin/knowledge-bases/:kb_id/documents/:id/status` 返回文档处理状态：`pending` / `parsing` / `chunking` / `embedding` / `completed` / `failed`
- [ ] 处理失败时记录详细错误信息（`error_message` 字段），前端展示并可重试
- [ ] 支持批量上传：同时选中 10 个文件，后端并行处理
- [ ] TypeScript 类型检查通过
- [ ] 在浏览器中使用 dev-browser skill 验证上传和状态查询流程

---

### US-V2-003: 知识库统一文章模型

**描述：** 作为知识库管理员，我需要创建和管理知识文章（文章是统一的知识单位，可以是结构化 FAQ 文本或上传的文档），以便所有知识通过统一的「分块 → embedding → 检索」管线。

**验收标准：**

- [ ] `knowledge_articles` 表改造：移除 `question` / `answer` 分离字段，增加 `title`、`content`（全文）、`source_type`（`manual`=手动输入 / `upload`=文档上传）、`word_count`、`chunk_count`、`file_type`（仅文档上传）、`minio_path`（仅文档上传）
- [ ] 手动创建文章：管理员填写标题 + 正文内容，保存后走分块→embedding→pgvector 流程
- [ ] 文档上传创建文章：上传 PDF/DOCX/MD/TXT 自动生成文章记录（`source_type=upload`），解析后的文本存入 `content` 字段
- [ ] 文章列表展示：标题、来源类型（图标区分手动/文档）、字数、分块数、状态（草稿/待审核/已发布/已停用/驳回/同步失败）、更新时间
- [ ] 审核流程保持不变：提交审核 → 审核（不能是创建人）→ 发布（触发 embedding 写入 pgvector）
- [ ] 停用文章时，从 pgvector 删除该文章的所有向量分块，标记文章状态为「已停用」
- [ ] 已发布的文章修改后状态回退为「草稿」，需重新走审核→发布流程
- [ ] TypeScript 类型检查通过
- [ ] 在浏览器中使用 dev-browser skill 验证

---

### US-V2-004: pgvector 向量存储

**描述：** 作为开发者，我需要 PostgreSQL 数据库安装 pgvector 扩展，定义向量表结构，并实现高效的向量相似度检索（cosine distance），以便支撑 RAG 检索管道。

**验收标准：**

- [ ] PostgreSQL 使用 `pgvector/pgvector:pg18` Docker 镜像（或 `postgres:18` + 手动安装 pgvector）
- [ ] 数据库迁移脚本执行 `CREATE EXTENSION IF NOT EXISTS vector`
- [ ] `knowledge_chunks` 表增加 `embedding` 列：类型为 `halfvec(N)` 或 `vector(N)`，N 由知识库配置的向量维度决定
- [ ] 创建 IVFFlat 索引（列表数 = 文档数开方）或 HNSW 索引，用于加速向量检索
- [ ] 实现 Go 层向量 CRUD 操作：
  - 批量写入：`INSERT INTO knowledge_chunks (article_id, kb_id, content, embedding, ...) VALUES (...)`
  - 相似度检索：`SELECT ... FROM knowledge_chunks WHERE kb_id = ? ORDER BY embedding <=> ? LIMIT ?`
  - 按文章删除：`DELETE FROM knowledge_chunks WHERE article_id = ?`
- [ ] 向量检索查询耗时 < 100ms（知识库规模 ≤ 10000 分块）
- [ ] Go 测试覆盖向量写入和检索，使用 `pgvector-go` 或原生 SQL 方式
- [ ] `go test ./tests/... -tags=integration -v` 通过

---

### US-V2-005: RAG 检索管道

**描述：** 作为报障人，当我提交运维问题时，系统需要通过查询改写 → 多路检索 → 混合检索（BM25 + 向量）→ RRF 融合 → 重排序的完整管道，找到最相关的知识片段，以便生成高质量的答案。

**验收标准：**

#### 5.1 查询改写 (Query Rewrite)

- [ ] 使用 LLM 将用户问题结合对话历史改写为独立的检索查询（消除指代歧义）
- [ ] 输入：用户当前问题 + 最近 3 轮对话历史
- [ ] 输出：一个改写后的独立查询字符串
- [ ] 查询改写为可选步骤，可通过 API 参数 `rag_options.query_rewrite` 控制（默认 true）
- [ ] 改写失败时降级为原始问题（不阻塞后续流程）

#### 5.2 多路检索 (Multi-Route)

- [ ] 使用 LLM 从原始问题生成 2-4 个不同角度的子查询
- [ ] 每个子查询独立执行混合检索（向量 + BM25）
- [ ] 合并多路结果并去重
- [ ] 多路检索为可选步骤，可通过 API 参数 `rag_options.multi_route` 控制（默认 true）
- [ ] 多路检索失败时降级为单路检索

#### 5.3 向量检索 (Vector Retrieval)

- [ ] 使用 `pgvector` 的 cosine 距离算子 `<=>` 进行相似度检索
- [ ] 检索范围为当前知识库（`kb_id`）下的所有已发布分块
- [ ] 支持配置检索数量：`top_k`（默认 5，范围 1-20）
- [ ] 返回每个分块的 `(chunk_id, content, score)`，score 归一化到 [0, 1]

#### 5.4 BM25 稀疏检索

- [ ] 在 Go 中实现 BM25 算法（Okapi BM25），不依赖外部 Python/Java 库
- [ ] **中文分词**：支持中文分词（优先使用 `github.com/go-ego/gse` 或 `github.com/yanyiwu/gojieba`），英文使用空格分词；分词结果作为 BM25 的 terms
- [ ] 为什么需要中文分词而非 bigram：bigram 会产生大量无意义 token（如"么处"），影响 BM25 的 IDF 精度和检索质量；jieba 级别的分词能准确提取语义单元
- [ ] BM25 计算对象为当前知识库所有已发布分块的文本内容
- [ ] 参数配置：`k1 = 1.5`、`b = 0.75`
- [ ] 返回每个分块的 BM25 分数
- [ ] BM25 检索为可选步骤，可通过 API 参数 `rag_options.hybrid` 控制（默认 true）
- [ ] 首次加载时构建并缓存倒排索引（map[token][]posting），知识库发布新文章时增量更新倒排索引
- [ ] 全量 BM25 计分时记录性能日志；分块数 > 10000 时使用倒排索引加速

#### 5.5 RRF 融合 (Reciprocal Rank Fusion)

- [ ] 将向量检索结果和 BM25 检索结果通过 RRF 算法融合
- [ ] 公式：`RRF_score(d) = Σ(1 / (k + rank_i(d)))`，其中 k=60
- [ ] 融合后按 RRF 分数降序排列，截取 top_k 个结果
- [ ] 仅当混合检索模式启用且 BM25 和向量两路均有结果时执行

#### 5.6 重排序 (Rerank)

- [ ] 使用 LLM（与对话生成相同的提供商）对检索结果进行重排序
- [ ] 输入：用户问题 + top_k × 2（最多 10 个候选分块），输出：重新排序后的 top_k 分块
- [ ] 重排序 prompt 格式与 rag-engine 的 FlashRank rerank 效果对齐
- [ ] 重排序为可选步骤，可通过 API 参数 `rag_options.rerank` 控制（默认 true）
- [ ] 重排序失败时降级为原始检索排序（不阻塞后续流程）

#### 5.7 综合

- [ ] 所有 RAG 管道步骤通过统一接口 `RAGPipeline.Execute(ctx, query, opts)` 串联
- [ ] 每个步骤的可选开关通过 `RAGOptions` 结构体控制
- [ ] 检索结果中的分块内容作为 LLM 生成的上下文注入 system prompt
- [ ] Go 集成测试覆盖：向量检索、BM25 检索、RRF 融合三个核心步骤
- [ ] `go test ./tests/... -tags=integration -v` 通过

---

### US-V2-006: LLM 对话生成与 SSE 流式输出

**描述：** 作为报障人，当我提问后，系统应该通过 RAG 管道检索相关知识，构建带上下文的 prompt，调用 LLM 生成答案，并以 SSE 流式方式实时返回，让我看到逐 token 的输出过程。

**验收标准：**

- [ ] `POST /api/v1/portal/chat-sessions/stream` 接口升级：从「先获取完整答案再模拟分块」改为「真正的 token 级别流式输出」
- [ ] 后端调用 LLM 的 `/v1/chat/completions` 端点时设置 `stream: true`
- [ ] LLM 返回的每个 SSE chunk 经 OpsMind 后端透明转发给前端（不做缓冲）
- [ ] 流式输出格式：`data: {"type":"token","content":"...", "step": "..."}`，与前端现有解析逻辑兼容
- [ ] 流式输出期间发送管道步骤事件：`data: {"type":"step","id":"query_rewrite|vector_retrieval|bm25_retrieval|rerank|llm_generate","label":"..."}`
- [ ] LLM 生成完成后，发送 done 事件：`data: {"type":"done","metadata":{...}}`（含 session_id、sources、confidence）
- [ ] 处理客户端断连：检测 `context.Done()` 后立即取消 LLM 请求
- [ ] 流式输出期间如 LLM 调用失败，发送 error 事件并终止流
- [ ] 非流式接口 `POST /api/v1/portal/chat-sessions` 保持可用（同步返回完整答案）
- [ ] TypeScript 类型检查通过
- [ ] 在浏览器中使用 dev-browser skill 验证 SSE 流式输出

---

### US-V2-007: Docker Compose 简化

**描述：** 作为部署者，我只需要 `docker compose up -d --build` 即可启动整个系统，不再需要配置和管理 AnythingLLM。

**验收标准：**

- [ ] `docker-compose.yml` 移除以下服务：
  - `anythingllm`（容器 + volumes + 环境变量）
  - `vllm`（容器 + volumes + profiles 配置）
- [ ] `docker-compose.yml` 新增可选服务：
  - `llama-cpp`（profile: `ai-local`），使用 `ghcr.io/ggerganov/llama.cpp:server` 镜像或自构建镜像，暴露 OpenAI-compatible API 端口 8080
- [ ] `docker-compose.yml` 保留服务：
  - `opsmind-server`（自构建 Go 后端）
  - `opsmind-web`（自构建 Vue 前端）
  - `postgres`（使用 `pgvector/pgvector:pg18` 镜像）
  - `minio`（对象存储）
- [ ] `.env` 和 `.env.example` 移除 AnythingLLM 相关变量（`ANYTHINGLLM_*`），新增 `LLM_BASE_URL`、`LLM_API_KEY`、`LLM_MODEL`、`EMBEDDING_MODEL`
- [ ] `docker compose up -d --build` 只启动 4 个必须服务，均正常运行
- [ ] `docker compose --profile ai-local up -d --build` 额外启动 llama-cpp 服务
- [ ] 文档更新：`docs/v2/DEPLOY.md` 说明部署步骤和 llama.cpp 模型准备

---

### US-V2-008: 前端适配

**描述：** 作为前端用户，知识库编辑页面应该支持文档上传（替代纯文本输入），问答页面应该展示 RAG 管道的执行步骤。

**验收标准：**

- [ ] 知识库文章编辑页增加「上传文档」标签页，支持拖拽/点击上传 PDF/DOCX/MD/TXT
- [ ] 上传后展示文档列表，每个文档显示文件名、大小、处理状态（pending/parsing/chunking/embedding/completed/failed）
- [ ] 手动输入模式下，提供标题 + 正文的富文本或 Markdown 编辑器
- [ ] 问答页面（Chat.vue）在 SSE 流式输出时展示管道步骤进度条或标签（查询改写 → 检索 → 重排 → 生成）
- [ ] 问答结果展示引用的知识来源（文档名称 + 分块内容摘要 + 相似度分数）
- [ ] 模型配置页面适配新的提供商配置模型（llama.cpp / OpenAI-compatible 统一配置）
- [ ] 移除 AnythingLLM 相关 UI 元素（API Key 初始化引导、workspace 配置等）
- [ ] TypeScript 类型检查通过
- [ ] 在浏览器中使用 dev-browser skill 验证所有变更

---

## 4. 功能需求

### 4.1 移除（Remove）

| 编号 | 说明 |
| --- | --- |
| **RM-1** | 移除 `adapter/rag_client.go` — `AnythingLLMClient` 实现及 `AnythingLLMConfig` |
| **RM-2** | 移除 `knowledge_bases.rag_workspace_slug` 字段及相关逻辑 |
| **RM-3** | 移除 `knowledge_articles.rag_document_location` 字段及相关逻辑 |
| **RM-4** | 移除 `RagClient` 接口（`Query`/`SyncDocument`/`DisableDocument`/`CreateWorkspace`） |
| **RM-5** | 移除 `docker-compose.yml` 的 `anythingllm` 服务和 `vllm` 服务 |
| **RM-6** | 移除 `.env` 中所有 `ANYTHINGLLM_*` 和 `VLLM_*` 环境变量 |
| **RM-7** | 移除 `config.go` 和 `config.yaml` 中 `anythingllm` 配置段 |
| **RM-8** | 移除前端 `ModelConfig.vue` 中 AnythingLLM 相关配置项 |
| **RM-9** | 移除 `docs/v1/ANYTHINGLLM_AI_INTEGRATION.md`（归档至 v1 目录） |

### 4.2 新增（Add）

#### 4.2.1 RAG 引擎模块

| 编号 | 说明 |
| --- | --- |
| **FR-1** | 新增 `server/internal/rag/` 包，作为 RAG 引擎的核心模块，包含以下子包和文件 |
| **FR-2** | `rag/pipeline.go` — RAG 管道编排器，串联查询改写→多路检索→混合检索→重排序，暴露 `Execute(ctx, query, kbID, opts) (*RAGResult, error)` |
| **FR-3** | `rag/query_rewrite.go` — 查询改写，调用 LLM 消除对话历史中的指代歧义 |
| **FR-4** | `rag/multi_route.go` — 多路检索，生成子查询并并行执行混合检索 |
| **FR-5** | `rag/retrieval.go` — 检索核心，包含 `VectorRetrieve`（pgvector cosine）、`BM25Retrieve`（自建 Go BM25）、`HybridFuse`（RRF 融合） |
| **FR-6** | `rag/rerank.go` — 重排序，调用 LLM 对候选分块重新评分排序 |
| **FR-7** | `rag/bm25.go` — Go 原生的 BM25 实现（Okapi BM25，中文双字分词 bigram，k1=1.5, b=0.75） |
| **FR-8** | `rag/document_parser.go` — 文档解析器，支持 PDF/DOCX/MD/TXT 四种格式的文本提取 |
| **FR-9** | `rag/chunker.go` — 文本分块器，实现 `RecursiveCharacterTextSplitter`（chunk_size 默认 1000，chunk_overlap 默认 200） |
| **FR-10** | `rag/embedding.go` — Embedding 生成器，调用 OpenAI-compatible `/v1/embeddings` API |

#### 4.2.2 LLM 适配层

| 编号 | 说明 |
| --- | --- |
| **FR-11** | 新增 `server/internal/adapter/llm_client.go` — LLM 客户端接口和实现 |
| **FR-12** | 接口 `LLMClient`：`ChatCompletion(ctx, req) (*ChatResponse, error)` 和 `ChatCompletionStream(ctx, req) (<-chan StreamChunk, error)` |
| **FR-13** | 接口 `EmbeddingClient`：`CreateEmbeddings(ctx, req) (*EmbeddingResponse, error)` |
| **FR-14** | 统一实现 `OpenAIClient`：通过 `BaseURL` + `API Key` 调用任意 OpenAI-compatible API（llama.cpp server / OpenAI / DeepSeek / 等） |
| **FR-15** | 流式解析：`OpenAIClient.ChatCompletionStream` 解析 `/v1/chat/completions`（`stream: true`）的 SSE 响应，通过 channel 逐 token 输出 |

#### 4.2.3 向量存储适配层

| 编号 | 说明 |
| --- | --- |
| **FR-16** | 新增 `server/internal/adapter/vector_store.go` — pgvector 向量存储接口 `VectorStore` |
| **FR-17** | 方法：`AddDocuments(ctx, chunks)` — 批量写入分块向量；`SimilaritySearch(ctx, kbID, embedding, topK)` — 余弦相似度检索；`DeleteByArticle(ctx, articleID)` — 删除文章所有向量 |
| **FR-18** | pgvector 查询使用 `halfvec` 类型（节省 50% 存储），算子使用 `<=>`（cosine distance，需做 1-distance 转换）或 `1 - (embedding <=> query)` 作为相似度分数 |
| **FR-19** | 索引使用 HNSW（`CREATE INDEX ON knowledge_chunks USING hnsw (embedding halfvec_cosine_ops)`），比 IVFFlat 更快且无需重建 |

#### 4.2.4 文档处理管线

| 编号 | 说明 |
| --- | --- |
| **FR-20** | `POST /api/v1/admin/knowledge-bases/:kb_id/documents/upload` — 文档上传 API（multipart/form-data） |
| **FR-21** | `GET /api/v1/admin/knowledge-bases/:kb_id/documents/:id/status` — 查询文档处理状态 |
| **FR-22** | `POST /api/v1/admin/knowledge-bases/:kb_id/documents/:id/retry` — 重试失败的文档处理 |
| **FR-23** | 后端异步处理管线：`goroutine pool`（大小=CPU 核数）并行处理 `MinIO 读取 → 格式解析 → 文本分块 → embedding 生成 → pgvector 写入` |
| **FR-24** | 处理状态持久化在 `knowledge_articles` 表：`process_status`（pending/parsing/chunking/embedding/completed/failed）+ `process_error`（失败原因） |

#### 4.2.5 知识库 API 变更

| 编号 | 说明 |
| --- | --- |
| **FR-25** | `POST /api/v1/admin/knowledge-bases/:kb_id/articles` — 创建文章，请求体变为 `{title, content, source_type, tags}`（移除 `question`/`answer` 分离） |
| **FR-26** | `PUT /api/v1/admin/articles/:id` — 更新文章，请求体变为 `{title, content, tags}` |
| **FR-27** | `POST /api/v1/admin/articles/:id/publish` — 发布文章，触发 embedding 生成 → pgvector 写入（替代原来的 AnythingLLM SyncDocument） |
| **FR-28** | `POST /api/v1/admin/articles/:id/disable` — 停用文章，触发 `DELETE FROM knowledge_chunks WHERE article_id = ?`（替代原来的 AnythingLLM DisableDocument） |

#### 4.2.6 SSE 流式升级

| 编号 | 说明 |
| --- | --- |
| **FR-29** | `POST /api/v1/portal/chat-sessions/stream` 改为真正的 token 级流式：调用 `LLMClient.ChatCompletionStream`，通过 channel 接收 token 并即时写 SSE |
| **FR-30** | 流式期间持续检测 `gin.Context.Done()`（客户端断连），断连时调用取消函数终止 LLM 请求 |
| **FR-31** | 管道步骤进度通过 `{"type":"step",...}` SSE 事件通知前端 |

### 4.3 修改（Modify）

| 编号 | 说明 |
| --- | --- |
| **FR-32** | `ChatService` 不再依赖 `RagClient`，改为依赖 `rag.Pipeline`（RAG 管道）+ `adapter.LLMClient`（LLM 生成） |
| **FR-33** | `KnowledgeService` 不再依赖 `RagClient`，改为依赖 `rag.Pipeline`（分块+embedding）+ `adapter.VectorStore`（向量写入）+ `adapter.StorageClient`（MinIO） |
| **FR-34** | `config.go` 新增 `LLMConfig`（`base_url`/`api_key`/`model`/`max_tokens`）和 `EmbeddingConfig`（`model`/`dimension`） |
| **FR-35** | `docker-compose.yml` 的 `postgres` 服务镜像改为 `pgvector/pgvector:pg18` |
| **FR-36** | `server/Dockerfile` 需要安装文档解析所需的系统依赖（如 PDF 解析可能需要的 C 库） |

---

## 5. 非目标（v2 不做）

| 编号 | 说明 | 原因 |
| --- | --- | --- |
| **NG-1** | 父子分块（parent-child chunking） | 复杂度高，MVP 先用简单分块；预留扩展点 |
| **NG-2** | LLM 运行时热切换 | 需重启服务；后续通过配置热加载实现 |
| **NG-3** | 跨知识库联合检索 | 单知识库检索已满足 MVP 需求 |
| **NG-4** | 向量检索的量化加速（PQ/OPQ） | pgvector halfvec 已节省 50% 存储，精度损失可接受 |
| **NG-5** | 对话历史的多轮上下文管理 | 当前已有基础支持（v1 的 `ChatSession` → `ChatMessage`），够用 |
| **NG-6** | 语义分块（按段落/章节切分） | 复杂，MVP 使用固定大小的递归字符分块 |
| **NG-7** | RAG 评估（RAGAS 等自动打分） | v1 依赖用户反馈评分（已解决/未解决），够用 |
| **NG-8** | 图片/表格中的文字提取 | PDF 解析只提取文本层，图片 OCR 和表格结构化不在范围 |

---

## 6. 技术设计

### 6.1 新增模块结构

```
server/internal/
├── rag/                            # RAG 引擎（全新模块）
│   ├── pipeline.go                 # 管道编排 Execute()
│   ├── query_rewrite.go            # 查询改写
│   ├── multi_route.go              # 多路检索
│   ├── retrieval.go                # 检索核心：VectorRetrieve / BM25Retrieve / HybridFuse
│   ├── rerank.go                   # 重排序
│   ├── bm25.go                     # BM25 Okapi 算法实现
│   ├── document_parser.go          # 文档解析：PDF/DOCX/MD/TXT
│   ├── chunker.go                  # 文本分块：RecursiveCharacterTextSplitter
│   ├── embedding.go                # Embedding 生成
│   └── types.go                    # 共享类型定义（RAGOptions, RAGResult, RetrievalResult 等）
├── adapter/
│   ├── llm_client.go               # LLM 客户端（新增，替换 rag_client.go）
│   ├── vector_store.go             # pgvector 向量存储（新增）
│   ├── storage_client.go           # MinIO 客户端（保持不变）
│   └── rag_client.go               # 删除（AnythingLLM）
├── model/
│   ├── knowledge.go                # 修改：KnowledgeArticle 字段调整, KnowledgeChunk 增加 embedding
│   ├── chat.go                     # 修改：ChatMessage 增加 RAG 管道步骤信息
│   └── llm_config.go               # 新增：LLM/Embedding 提供商配置模型
├── service/
│   ├── chat_service.go             # 修改：依赖 rag.Pipeline + LLMClient
│   ├── knowledge_service.go        # 修改：依赖 rag 管线 + VectorStore
│   └── llm_config_service.go       # 新增：LLM 配置管理
└── handler/
    ├── chat.go                     # 修改：StreamChatSession 升级为真正流式
    ├── knowledge.go                # 修改：新增加文档上传、状态查询 API
    └── llm_config.go               # 新增：LLM 配置管理 API
```

### 6.2 数据库变更

#### 6.2.1 knowledge_bases 表变更

```sql
-- 删除字段
ALTER TABLE knowledge_bases DROP COLUMN rag_workspace_slug;

-- 新增字段
ALTER TABLE knowledge_bases ADD COLUMN llm_config_id bigint;  -- 关联 llm_configs.id
```

#### 6.2.2 knowledge_articles 表变更

```sql
-- 删除字段
ALTER TABLE knowledge_articles DROP COLUMN question;
ALTER TABLE knowledge_articles DROP COLUMN rag_document_location;

-- 重命名/新增字段
ALTER TABLE knowledge_articles RENAME COLUMN answer TO content;
ALTER TABLE knowledge_articles ADD COLUMN title varchar(255) NOT NULL DEFAULT '';
ALTER TABLE knowledge_articles ADD COLUMN source_type smallint NOT NULL DEFAULT 1;  -- 1=manual, 2=upload
ALTER TABLE knowledge_articles ADD COLUMN word_count integer NOT NULL DEFAULT 0;
ALTER TABLE knowledge_articles ADD COLUMN chunk_count integer NOT NULL DEFAULT 0;
ALTER TABLE knowledge_articles ADD COLUMN file_type varchar(16);       -- pdf/docx/md/txt
ALTER TABLE knowledge_articles ADD COLUMN minio_path varchar(512);     -- MinIO 对象路径
ALTER TABLE knowledge_articles ADD COLUMN process_status varchar(16) NOT NULL DEFAULT 'pending';  -- 处理状态
ALTER TABLE knowledge_articles ADD COLUMN process_error text;          -- 处理失败原因
```

#### 6.2.3 knowledge_chunks 表变更

```sql
-- 新增向量字段（使用 pgvector 扩展）
CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE knowledge_chunks ADD COLUMN kb_id bigint NOT NULL;                            -- 知识库 ID（用于检索过滤）
ALTER TABLE knowledge_chunks ADD COLUMN embedding halfvec(1536);                           -- 向量（维度由 kb 配置决定，表结构用最大维度）
ALTER TABLE knowledge_chunks ADD COLUMN chunk_index integer NOT NULL DEFAULT 0;             -- 分块序号
ALTER TABLE knowledge_chunks DROP COLUMN sync_status;
ALTER TABLE knowledge_chunks DROP COLUMN sync_error;
ALTER TABLE knowledge_chunks DROP COLUMN synced_at;

-- 向量索引
CREATE INDEX idx_chunks_embedding ON knowledge_chunks USING hnsw (embedding halfvec_cosine_ops);
```

#### 6.2.4 新增 llm_configs 表

```sql
CREATE TABLE llm_configs (
    id bigint PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    name varchar(128) NOT NULL,                          -- 配置名称
    provider_type smallint NOT NULL DEFAULT 1,           -- 1=llama.cpp, 2=OpenAI-compatible
    base_url varchar(512) NOT NULL,                      -- API 地址（如 http://llama-cpp:8080/v1）
    api_key varchar(512),                                -- API Key（可选，llama.cpp 不需要；数据库加密存储）
    llm_model varchar(128) NOT NULL,                     -- LLM 模型名称
    embedding_model varchar(128) NOT NULL,               -- Embedding 模型名称
    max_tokens integer NOT NULL DEFAULT 8192,            -- 最大生成 Token 数
    vector_dimension integer NOT NULL DEFAULT 1536,      -- 向量维度
    is_default boolean NOT NULL DEFAULT false,           -- 是否默认配置
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- 唯一索引：最多一个默认配置
CREATE UNIQUE INDEX idx_llm_configs_default ON llm_configs (is_default) WHERE is_default = true;
```

#### 6.2.5 chat_messages 表增加 RAG 管道信息

```sql
ALTER TABLE chat_messages ADD COLUMN rag_pipeline jsonb;
-- rag_pipeline 结构示例：
-- {
--   "steps": [
--     {"id": "query_rewrite", "label": "查询改写", "duration_ms": 120},
--     {"id": "multi_route", "label": "多路检索", "duration_ms": 350},
--     {"id": "rerank", "label": "重排序", "duration_ms": 80},
--     {"id": "llm_generate", "label": "LLM 生成", "duration_ms": 2800}
--   ],
--   "total_duration_ms": 3350
-- }
```

### 6.3 RAG 检索管道数据流

```
用户问题: "账号冻结怎么处理？"
     │
     ▼
┌──────────────────────┐
│ 1. 查询改写 (可选)     │  LLM: "用户询问企业账号被冻结后如何处理和恢复"
│    QueryRewrite       │  失败降级：使用原始问题
└──────┬───────────────┘
       │ 改写后的查询
       ▼
┌──────────────────────┐
│ 2. 多路检索 (可选)     │  LLM 生成 2-4 个子查询：
│    MultiRoute         │  "账号冻结恢复步骤"、"账号冻结解锁方法"、"冻结账号怎么办"
└──────┬───────────────┘
       │ 子查询列表 (并行处理)
       ▼
┌──────────────────────┐
│ 3. 混合检索            │  每个子查询独立执行：
│    HybridRetrieval    │  ┌──────────────┐  ┌──────────────┐
│                       │  │ pgvector cos │  │ BM25 (bigram)│
│                       │  │ 相似度检索    │  │ 稀疏检索     │
│                       │  └──────┬───────┘  └──────┬───────┘
│                       │         │                  │
│                       │         └────────┬─────────┘
│                       │                  ▼
│                       │         ┌──────────────┐
│                       │         │ RRF 融合      │
│                       │         │ k=60          │
│                       │         └──────────────┘
└──────┬───────────────┘
       │ top_k × 2 (过采样，最多 10 个候选)
       ▼
┌──────────────────────┐
│ 4. 重排序 (可选)       │  LLM 对候选分块重新评分排序，截取 top_k
│    Rerank             │  失败降级：使用 RRF 排序结果
└──────┬───────────────┘
       │ top_k 个相关分块
       ▼
┌──────────────────────┐
│ 5. LLM 生成            │  System prompt: 你是企业运维数字员工...
│    LLMGenerate        │  Context: [分块1]...[分块N]
│                       │  User: 账号冻结怎么处理？
│                       │  → SSE 流式输出答案
└──────────────────────┘
```

### 6.4 RAGOptions 配置结构

```go
// rag/types.go

type RAGOptions struct {
    TopK         int   `json:"top_k"`          // 最终返回的分块数，默认 5
    QueryRewrite bool  `json:"query_rewrite"`  // 是否启用查询改写，默认 true
    MultiRoute   bool  `json:"multi_route"`    // 是否启用多路检索，默认 true
    Hybrid       bool  `json:"hybrid"`         // 是否启用 BM25 混合检索，默认 true
    Rerank       bool  `json:"rerank"`         // 是否启用重排序，默认 true
    RouteCount   int   `json:"route_count"`    // 多路子查询数，默认 3
    RerankCount  int   `json:"rerank_count"`   // 进入重排序的候选数（top_k × 倍数），默认 10
}
```

### 6.5 文档处理管线

```
POST /api/v1/admin/knowledge-bases/:kb_id/documents/upload
     │
     │ multipart/form-data (多个文件)
     ▼
┌─────────────────────────┐
│ 1. 接收文件              │
│    - 校验格式/大小       │
│    - 上传到 MinIO        │
│    - 创建 article 记录   │
│      (source_type=upload,│
│       process_status=    │
│       pending)           │
│    - 返回 document_id 列表│
└────────┬────────────────┘
         │ 发送到处理队列 (channel)
         ▼
┌─────────────────────────┐
│ 2. 异步并行处理           │  goroutine pool (CPU 核数)
│                          │
│  For each document:      │
│  2a. process_status      │
│      = "parsing"         │
│  2b. 读取 MinIO 文件     │
│  2c. 按扩展名选择解析器: │
│      - PDF  → ledongthuc/pdf 或 pdfcpu
│      - DOCX → docx2txt (Go 实现)
│      - MD   → 直接读取
│      - TXT  → 直接读取
│  2d. process_status      │
│      = "chunking"        │
│  2e. RecursiveCharText-  │
│      Splitter 分块       │
│      (chunk_size=1000,   │
│       overlap=200)       │
│  2f. process_status      │
│      = "embedding"       │
│  2g. 调用 Embedding API  │
│      (批处理，每批 20 块) │
│  2h. 写入 pgvector       │
│      (批量 INSERT)       │
│  2i. process_status      │
│      = "completed"       │
│                          │
│  失败 → process_status   │
│  = "failed",             │
│  记录 process_error      │
└─────────────────────────┘
```

### 6.6 Go 依赖选型

| 用途 | 推荐库 | 替代方案 | 选择理由 |
| --- | --- | --- | --- |
| PDF 解析 | `github.com/ledongthuc/pdf` 或 `github.com/pdfcpu/pdfcpu` | `github.com/unidoc/unipdf` (商业) | 前者轻量免费，后者功能更全但 AGPL；仅做文本提取无需商业库 |
| DOCX 解析 | 自行实现（基于 `archive/zip` + `encoding/xml`）| `github.com/nguyenthenguyen/docx` | DOCX 本质是 ZIP 包中的 XML，标准库足够，避免引入不稳定第三方依赖 |
| 中文分词 | `github.com/go-ego/gse` | `github.com/yanyiwu/gojieba` | gse 是纯 Go 实现无需 CGO，词典可嵌入式加载；gojieba 需要 CGO 编译，Docker 构建更复杂 |
| pgvector | 原生 SQL + `github.com/pgvector/pgvector-go` | - | pgvector 官方提供 Go SDK，支持 halfvec 类型 |
| HTTP 客户端 | `net/http` 标准库 | `github.com/go-resty/resty` | 标准库足够，减少依赖 |
| 并发控制 | `golang.org/x/sync/errgroup` + buffered channel | `sourcegraph/conc` | errgroup 是官方扩展库 |

### 6.7 SSE 流式输出格式

```
data: {"type":"step","id":"query_rewrite","label":"查询改写"}

data: {"type":"step","id":"retrieval","label":"知识检索"}

data: {"type":"token","content":"账号冻结"}

data: {"type":"token","content":"的处理"}

data: {"type":"token","content":"步骤如下"}

...

data: {"type":"done","metadata":{"session_id":12345,"sources":[...],"confidence":0.85,"pipeline":{"steps":[...],"total_duration_ms":3200}}}

```

与 v1 前端 `web/src/api/chat.ts` 的 `streamChatSession` 函数兼容，前端只需扩展 `StreamCallbacks` 类型增加 `onStep` 回调。

---

## 7. Docker Compose 变更

### 7.1 变更后的服务列表

```yaml
services:
  # ===== 必须服务 =====
  opsmind-server:    # Go 后端（自构建）
  opsmind-web:       # Vue 前端（自构建）
  postgres:          # pgvector/pgvector:pg18（业务数据 + 向量存储）
  minio:             # minio/minio:latest（对象存储）

  # ===== 可选服务 =====
  llama-cpp:         # llama.cpp server（profile: ai-local）
                     # 暴露 OpenAI-compatible API 在 8080 端口
```

### 7.2 .env 变量变更

```diff
# 移除
- ANYTHINGLLM_BASE_URL=http://anythingllm:3001/api
- ANYTHINGLLM_API_KEY=xxx
- ANYTHINGLLM_JWT_SECRET=xxx
- ANYTHINGLLM_SIG_KEY=xxx
- ANYTHINGLLM_SIG_SALT=xxx
- VLLM_BASE_URL=http://vllm:8000/v1
- LLM_BASE_URL=http://vllm:8000/v1   (旧的 vLLM 变量)
- LLM_MODEL=qwen3-4b
- LLM_TOKEN_LIMIT=8192
- LLM_API_KEY=dummy-key

# 新增/修改
+ LLM_BASE_URL=http://llama-cpp:8080/v1   # llama.cpp server 地址（或外部 OpenAI 地址如 https://api.openai.com/v1）
+ LLM_API_KEY=                             # 外部 API 需要（OpenAI/DeepSeek 等）；本地 llama.cpp 留空
+ LLM_MODEL=qwen3-4b                       # LLM 模型名称（llama.cpp 启动时通过 --model 参数指定）
+ LLM_MAX_TOKENS=8192                      # 最大生成 Token 数
+ EMBEDDING_MODEL=bge-m3                   # Embedding 模型名称
+ EMBEDDING_DIMENSION=1024                 # 向量维度（bge-m3 为 1024）

# llama.cpp 模型挂载路径（Docker Compose 中配置）
+ LLAMA_MODELS_DIR=./models                # 模型文件存放目录，挂载到容器的 /models

# 保留
  POSTGRES_PASSWORD=opsmind_dev
  MINIO_ROOT_USER=minioadmin
  MINIO_ROOT_PASSWORD=minioadmin
  JWT_SECRET=xxx
  AI_CONFIDENCE_THRESHOLD=0.6
  AI_DEFAULT_TOP_K=5
```

---

## 8. API 变更汇总

### 8.1 新增 API

| 方法 | 路径 | 说明 | 权限 |
| --- | --- | --- | --- |
| POST | `/api/v1/admin/knowledge-bases/:kb_id/documents/upload` | 上传文档（multipart） | 知识库管理员 |
| GET | `/api/v1/admin/knowledge-bases/:kb_id/documents/:id/status` | 查询文档处理状态 | 知识库管理员 |
| POST | `/api/v1/admin/knowledge-bases/:kb_id/documents/:id/retry` | 重试失败的文档 | 知识库管理员 |

### 8.2 修改 API

| 方法 | 路径 | 变更说明 |
| --- | --- | --- |
| POST | `/api/v1/admin/knowledge-bases/:kb_id/articles` | 请求体改为 `{title, content, source_type, tags}`（移除 question/answer） |
| PUT | `/api/v1/admin/articles/:id` | 请求体改为 `{title, content, tags}` |
| POST | `/api/v1/admin/articles/:id/publish` | 内部逻辑：触发 embedding 生成 → pgvector 写入（替代 AnythingLLM 同步） |
| POST | `/api/v1/admin/articles/:id/disable` | 内部逻辑：从 pgvector 删除向量（替代 AnythingLLM disable） |
| POST | `/api/v1/portal/chat-sessions/stream` | 升级为真正的 token 级 SSE 流式，增加管道步骤事件 |

### 8.3 不变 API

以下 v1 API 路径和请求/响应格式不变（仅内部实现变更）：

- `POST /api/v1/portal/chat-sessions` — 同步问答
- `POST /api/v1/portal/chat-sessions/:id/feedback` — 问答反馈
- `GET /api/v1/portal/chat-sessions/:id` — 问答详情
- `GET/POST /api/v1/admin/knowledge-bases` — 知识库 CRUD
- `POST /api/v1/admin/articles/:id/submit-review` — 提交审核
- `POST /api/v1/admin/articles/:id/review` — 审核知识

---

## 9. 风险与缓解

| 风险 | 影响 | 概率 | 缓解措施 |
| --- | --- | --- | --- |
| **Go PDF 解析质量不如 Python (pypdf)** | 中文 PDF 文本提取不完整 | 中 | 选择成熟的 Go PDF 库；对于扫描版 PDF 不做 OCR 要求（NG-8） |
| **BM25 Go 自实现性能差** | 分块数 > 5000 时检索变慢 | 中 | 实现倒排索引缓存，每次发布后重建索引；预留性能测试 |
| **llama.cpp 模型推理速度慢** | 对话响应超时 | 中 | 支持外部 LLM API（OpenAI 等），降级方案 |
| **pgvector 大规模向量检索性能** | > 10000 向量时检索 > 1s | 低 | HNSW 索引在 10000 规模下仍 < 50ms；后续可加近似检索 |
| **文档并行处理内存溢出** | 大文件（> 20MB）并发处理 OOM | 低 | 限制单个文件处理协程的内存使用；并发池控制在 CPU 核数 |
| **v1 数据迁移兼容性** | 现有知识数据丢失或无法使用 | 中 | 编写迁移脚本，将 v1 的 FAQ 数据转换为 v2 文章模型；保留 v1 数据备份 |

---

## 10. 成功指标

| 指标 | 目标 | 测量方式 |
| --- | --- | --- |
| **M1: AnythingLLM 完全移除** | `docker compose ps` 不含 anythingllm | 部署后验证 |
| **M2: 文档上传解析成功率** | ≥ 95%（非扫描版 PDF/DOCX/MD/TXT） | 上传 20 个测试文档，≥ 19 个成功 |
| **M3: 向量检索响应时间** | 10000 分块下 < 100ms | `EXPLAIN ANALYZE` + Go benchmark |
| **M4: 端到端问答响应时间** | 不含 LLM 推理 ≤ 500ms（检索+重排） | 仪表盘统计 |
| **M5: SSE 首 token 延迟** | 从请求到第一个 token ≤ 2s | `StreamChatSession` 计时 |
| **M6: BM25 + 向量混合检索优于纯向量** | 人工评估 10 个查询，混合检索 MRR ≥ 纯向量 | A/B 对比测试 |
| **M7: 知识审核发布流程完整** | 手动创建 → 审核 → 发布 → 可通过问答检索到 | 端到端手动测试 |
| **M8: 现有测试通过** | v1 的非 RAG 测试（auth/ticket/user/role 等）全部通过 | `go test ./tests/... -tags=integration` |
| **M9: Docker 一键启动** | `docker compose up -d --build` 启动 4 个必须服务 | 冷启动后全部 healthy |

---

## 11. 已确认决策

以下为 2026-06-11 与需求方确认的技术决策，已体现在本 PRD 各章节中：

| 编号 | 问题 | 最终决策 | 影响章节 |
| --- | --- | --- | --- |
| **D1** | PDF 解析深度 | **仅文本提取**，不做表格结构化提取。扫描版 PDF（纯图片）不在 MVP 支持范围。 | §4.2.4 FR-23, §5 NG-8 |
| **D2** | BM25 中文分词 | **使用中文分词库**（`gojieba` 或 `gse`），而非仅 bigram 双字分词。分词结果作为 BM25 terms，提升 IDF 精度。 | §3 US-V2-005 5.4 |
| **D3** | 向量索引类型 | **硬编码 HNSW**，不对用户暴露索引配置。HNSW 在 10000 级向量规模下比 IVFFlat 更快（查询 < 50ms），且无需定期重建。 | §3 US-V2-004, §6.2.3 |
| **D4** | 向量精度 | **使用 halfvec（半精度 float16）**。精度损失可忽略（对 cosine 相似度排序影响 < 0.1%），存储空间减半。 | §3 US-V2-004, §6.2.3 |
| **D5** | llama.cpp 镜像 | **使用官方 `ghcr.io/ggerganov/llama.cpp:server` 镜像**。模型文件通过 volume 挂载（`./models:/models`），模型下载作为部署文档中的准备步骤。 | §7.1, §7.2 |
| **D6** | 迁移策略 | **模块完成后一次性切换**（Big Bang cutover），不在同一代码库保留两套 RAG 实现。`RagClient` 接口直接删除，所有引用同步改为 `rag.Pipeline` + `LLMClient` + `VectorStore`。 | §4.1 RM-1~RM-4, §12 |

### 11.1 架构重构原则

本次 v2 不仅仅是替换 AnythingLLM，更是对 RAG/知识/LLM 三个核心模块的**全面重构**：

- **`rag/` 包**是全新的 RAG 引擎模块，不继承 v1 任何代码；所有算法（BM25、RRF、分块）在 Go 中自行实现
- **`adapter/` 包**从「封装外部服务」转变为「封装外部协议」— `llm_client.go` 封装 OpenAI-compatible 协议而非特定服务，`vector_store.go` 封装 pgvector 而非 AnythingLLM
- **`model/knowledge.go`** 的知识模型从 FAQ 结构变为统一文章模型，与 v1 不兼容；需要数据迁移脚本
- **`handler/chat.go`** 的 SSE 流式从「模拟分块」升级为「真正 token 级流式」，代码逻辑显著变化
- 前端知识库页面从「QA 编辑器」变为「文档管理 + 编辑器」，UI 组件需重写

**不做的：** v1 的用户/角色/申告/看板/审计/消息模块完全不动——这些与 RAG 无关。

---

## 12. 附录：v1 → v2 文件变更清单

### 删除的文件

| 文件 | 说明 |
| --- | --- |
| `server/internal/adapter/rag_client.go` | AnythingLLM 客户端实现 |
| `server/tests/adapter/rag_client_test.go` | AnythingLLM 客户端测试 |
| `docs/v1/ANYTHINGLLM_AI_INTEGRATION.md` | 归档至 v1 |

### 新增的文件

| 文件 | 说明 |
| --- | --- |
| `server/internal/rag/pipeline.go` | RAG 管道编排 |
| `server/internal/rag/types.go` | RAG 共享类型 |
| `server/internal/rag/query_rewrite.go` | 查询改写 |
| `server/internal/rag/multi_route.go` | 多路检索 |
| `server/internal/rag/retrieval.go` | 混合检索核心 |
| `server/internal/rag/bm25.go` | BM25 算法 |
| `server/internal/rag/rerank.go` | 重排序 |
| `server/internal/rag/document_parser.go` | 文档解析 |
| `server/internal/rag/chunker.go` | 文本分块 |
| `server/internal/rag/embedding.go` | Embedding 生成 |
| `server/internal/adapter/llm_client.go` | LLM 客户端 |
| `server/internal/adapter/vector_store.go` | pgvector 适配器 |
| `server/internal/model/llm_config.go` | LLM 配置模型 |
| `server/internal/service/llm_config_service.go` | LLM 配置服务 |
| `server/internal/handler/llm_config.go` | LLM 配置 API |
| `server/tests/rag/` | RAG 模块测试 |
| `server/tests/adapter/llm_client_test.go` | LLM 客户端测试 |
| `server/tests/adapter/vector_store_test.go` | 向量存储测试 |
| `server/migrations/v2/` | v2 数据库迁移脚本 |
| `docs/v2/TECHv2.md` | v2 技术架构文档 |

### 修改的文件

| 文件 | 变更内容 |
| --- | --- |
| `server/internal/model/knowledge.go` | KnowledgeArticle 字段调整，KnowledgeChunk 增加 embedding |
| `server/internal/model/chat.go` | ChatMessage 增加 rag_pipeline 字段 |
| `server/internal/service/chat_service.go` | 依赖从 RagClient 改为 rag.Pipeline + LLMClient |
| `server/internal/service/knowledge_service.go` | 发布/停用逻辑改为 pgvector 操作 |
| `server/internal/handler/chat.go` | StreamChatSession 升级为真正流式 |
| `server/internal/handler/knowledge.go` | 新增加文档上传/状态查询 handler |
| `server/internal/config/config.go` | 新增 LLMConfig/EmbeddingConfig，移除 AnythingLLMConfig |
| `server/cmd/main.go` | 初始化 rag 模块、LLMClient、VectorStore 替代 RagClient |
| `docker-compose.yml` | 移除 anythingllm + vllm，新增可选 llama-cpp，postgres 换 pgvector 镜像 |
| `.env.example` | 移除 ANYTHINGLLM_*，新增 LLM/Embedding 变量 |
| `web/src/views/admin/KnowledgeEdit.vue` | 增加文档上传区域 |
| `web/src/views/admin/KnowledgeList.vue` | 展示文章来源类型和处理状态 |
| `web/src/views/admin/ModelConfig.vue` | 适配新 LLM 配置模型 |
| `web/src/views/portal/Chat.vue` | 展示 RAG 管道步骤 |
| `web/src/api/knowledge.ts` | 新增加文档上传/状态查询 API |
| `web/src/api/chat.ts` | SSE 解析增加 onStep 回调 |
| `web/src/stores/chat.ts` | 增加管道步骤状态管理 |
