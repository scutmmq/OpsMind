# OpsMind v2 实施计划

> **状态：** ✅ 全部完成（M1-M7 已交付，133 测试通过）  
> **目标：** 移除 AnythingLLM，自建 Go RAG 引擎 + pgvector 向量存储，升级 SSE 流式，新增文档上传和 LLM 配置管理。
>
> **架构：** 自下而上 7 阶段实施 — 环境准备 → 适配层 → RAG 引擎 → Service 层 → Handler/路由 → 前端 → 清理。每个里程碑完成后可独立验证。
>
> **技术栈：** Go 1.22+ / Gin / GORM / pgvector / gse(中文分词) / llama.cpp / Vue 3 / TypeScript / Radix Vue
>
> **关联文档：** [CHANGELOG](CHANGELOG.md) · [PRDv2](PRDv2.md) · [TECHv2](TECHv2.md) · [API 文档](../API/README.md)
>
> **约定：**
> - 每个 Task 对应一次 git commit，commit message 格式 `类型: 描述`（中文）
> - 所有新文件必须包含文件头注释（说明模块存在原因）
> - 所有函数必须包含注释（说明为什么这样实现）
> - 测试文件命名 `*_test.go`，使用外部测试包 `_test`（集成测试使用 `-tags=integration`）

---

## 里程碑总览

| 里程碑 | 内容 | 新增文件 | 修改文件 | 删除文件 |
|--------|------|----------|----------|----------|
| M1 环境准备 | pgvector 扩展、迁移脚本、种子数据 | 2 | 3 | 0 |
| M2 适配层 | LLMClient、EmbeddingClient、VectorStore | 4 | 2 | 0 |
| M3 RAG 引擎 | 分块、BM25、文档解析、管道编排、异步处理 | 11 | 0 | 0 |
| M4 Service 层 | LLM配置服务、Chat/Knowledge 服务改造 | 1 | 3 | 0 |
| M5 Handler+路由 | LLM配置API、文档上传API、SSE升级、移除embedding-configs | 1 | 4 | 0 |
| M6 前端 | 文档上传UI、LLM配置UI、知识库UI适配、问答管道步骤 | 2 | 7 | 0 |
| M7 清理 | 删除 AnythingLLM 代码、Docker/ENV 清理、文档定稿 | 0 | 4 | 3 |

---

## M1: 环境准备 + pgvector

### Task 1.1: 切换 PostgreSQL 镜像为 pgvector

**文件：**
- 修改：`docker-compose.yml` — postgres 服务镜像从 `postgres:18` 改为 `pgvector/pgvector:pg18`
- 修改：`.env.example` — 移除 `ANYTHINGLLM_*`、`VLLM_*` 变量，新增 `LLM_BASE_URL`、`LLM_API_KEY`、`LLM_MODEL`、`LLM_MAX_TOKENS`、`EMBEDDING_MODEL`、`EMBEDDING_DIMENSION`、`LLAMA_MODELS_DIR`

**验证：** `docker compose up -d postgres && docker compose exec postgres psql -U opsmind -d opsmind -c "SELECT 1"`

**Commit：** `chore: 切换 PostgreSQL 镜像为 pgvector/pgvector:pg18`

---

### Task 1.2: 编写 v2 数据库迁移脚本

**文件：**
- 新增：`server/migrations/v2/001_pgvector_extension.sql` — `CREATE EXTENSION IF NOT EXISTS vector`
- 新增：`server/migrations/v2/002_alter_knowledge_bases.sql` — 删除 `rag_workspace_slug`，新增 `llm_config_id`
- 新增：`server/migrations/v2/003_alter_knowledge_articles.sql` — 删除 `question`/`rag_document_location`，`answer`→`content`，新增 `title`/`source_type`/`word_count`/`chunk_count`/`file_type`/`minio_path`/`process_status`/`process_error`
- 新增：`server/migrations/v2/004_alter_knowledge_chunks.sql` — 删除 `sync_status`/`sync_error`/`synced_at`，新增 `kb_id`/`chunk_index`/`embedding halfvec(N)`
- 新增：`server/migrations/v2/005_create_llm_configs.sql` — 创建 `llm_configs` 表（id/name/provider_type/base_url/api_key/llm_model/embedding_model/max_tokens/vector_dimension/is_default/created_at/updated_at）
- 新增：`server/migrations/v2/006_create_indexes.sql` — HNSW 向量索引、kb_id 索引、article_id 索引
- 新增：`server/migrations/v2/007_alter_chat_messages.sql` — 新增 `rag_pipeline` jsonb 字段

**验证：** 在 Docker pgvector 环境下依次执行迁移 SQL，确认所有表结构符合 TECHv2 §3.2 DDL

**Commit：** `feat: 编写 v2 数据库迁移脚本 — pgvector 扩展 + 知识表/LLM配置表/chunks向量列`

---

### Task 1.3: 更新种子数据脚本适配 v2 模型

**文件：**
- 修改：`server/migrations/seed.sql` — 所有 `knowledge_articles` INSERT 从 `(question, answer)` 改为 `(title, content, source_type)`；新增 `llm_configs` 默认配置 INSERT（llama.cpp localhost 默认值）；`knowledge_chunks` INSERT 新增 `kb_id`/`chunk_index`，移除 `sync_*`；预置角色/用户的 INSERT 不变

**验证：** `make seed` 执行成功，查询 `knowledge_articles` 确认 `title`/`content` 字段非空且格式正确

**Commit：** `feat: 种子数据适配 v2 统一文章模型 + LLM 配置默认值`

---

## M2: 适配层 (server/internal/adapter/)

### Task 2.1: 定义 LLMClient 接口 + OpenAI-compatible 实现

**文件：**
- 新增：`server/internal/adapter/llm_client.go` — `LLMClient` 接口（`ChatCompletion` / `ChatCompletionStream`）+ `OpenAIClient` 结构体 + `ChatRequest`/`ChatResponse`/`StreamChunk` 类型定义。流式方法解析 SSE 的 `data:` 行并通过 channel 输出
- 新增：`server/tests/adapter/llm_client_test.go` — mock HTTP server 测试：同步 ChatCompletion 正确返回、流式 ChatCompletionStream 正确逐 token 输出、HTTP 4xx 错误处理、超时错误处理、context 取消后 channel 关闭

**验证：** `go test ./tests/adapter/ -v -run TestLLM` 全部通过

**Commit：** `feat: 实现 LLMClient — OpenAI-compatible 协议的同步/流式适配器`

---

### Task 2.2: 定义 EmbeddingClient 接口 + 实现

**文件：**
- 新增：`server/internal/adapter/embedding_client.go` — `EmbeddingClient` 接口（`CreateEmbeddings`）+ `OpenAIEmbeddingClient` 结构体 + `EmbeddingRequest`/`EmbeddingResponse` 类型
- 新增：`server/tests/adapter/embedding_client_test.go` — mock HTTP server 测试：单条文本向量化、批量文本向量化、API 返回维度校验、错误处理

**验证：** `go test ./tests/adapter/ -v -run TestEmbedding` 全部通过

**Commit：** `feat: 实现 EmbeddingClient — OpenAI-compatible /v1/embeddings 适配器`

---

### Task 2.3: 定义 VectorStore 接口 + pgvector 实现

**文件：**
- 新增：`server/internal/adapter/vector_store.go` — `VectorStore` 接口（`BatchInsert`/`CosineSearch`/`DeleteByArticle`/`DeleteByKB`/`CountByKB`/`GetChunksByArticle`）+ `PgvectorStore` 结构体 + `VectorChunk`/`SearchResult`/`ChunkContent` 类型。使用 `database/sql` 原生参数化查询 + halfvec 类型写入
- 新增：`server/tests/adapter/vector_store_test.go`（`-tags=integration`）— 真实 pgvector 测试：BatchInsert 写入 10 个向量、CosineSearch 检索 TopK、相同向量相似度=1.0 验证、DeleteByArticle 后检索为空、DeleteByKB 全部清除

**验证：** `go test ./tests/adapter/ -v -tags=integration -run TestVectorStore` 全部通过（需 Docker pgvector 运行中）

**Commit：** `feat: 实现 VectorStore — pgvector halfvec 写入 + HNSW cosine 检索`

---

### Task 2.4: 更新 Config 结构体添加 LLM/Embedding 配置

**文件：**
- 修改：`server/internal/config/config.go` — 新增 `LLMConfig` 结构体（`BaseURL`/`APIKey`/`Model`/`MaxTokens`）+ `EmbeddingConfig` 结构体（`Model`/`Dimension`）。Viper 从环境变量 `LLM_BASE_URL`/`LLM_API_KEY`/`LLM_MODEL`/`LLM_MAX_TOKENS`/`EMBEDDING_MODEL`/`EMBEDDING_DIMENSION` 读取。删除 AnythingLLM 相关配置方法和字段

**验证：** `go test ./tests/config/ -v` 确认配置解析正确

**Commit：** `feat: 新增 LLM/Embedding 配置结构体，移除 AnythingLLM 配置`

---

## M3: RAG 引擎 (server/internal/rag/)

### Task 3.1: 定义 RAG 引擎共享类型

**文件：**
- 新增：`server/internal/rag/types.go` — `RAGOptions`（TopK/QueryRewrite/MultiRoute/Hybrid/Rerank/RouteCount/RerankCount）、`RAGResult`（Chunks/Metrics）、`RetrievalResult`（ChunkID/ArticleID/Content/Score/Source）、`PipelineMetrics`（Steps/TotalDurationMS）、`StepMetric`（StepID/Label/DurationMS/Success/Error）、`Retriever` 接口（`Retrieve(ctx, query, kbID, topK)`）

**验证：** `go build ./internal/rag/...` 编译通过

**Commit：** `feat: 定义 RAG 引擎共享类型 — Retriever 接口 + RAGOptions + RAGResult`

---

### Task 3.2: 实现 RecursiveCharacterTextSplitter 分块器

**文件：**
- 新增：`server/internal/rag/chunker.go` — `Chunker` 结构体（`ChunkSize`/`ChunkOverlap`），`Split(text)` 方法：按 `\n\n` → `\n` → `。` → `.` → 空格 → 字符级优先级递归分割
- 新增：`server/tests/rag/chunker_test.go` — 测试：短文本（< chunkSize）不分块、长文本按分隔符分块、overlap 正确重叠、中英文混合文本、空字符串返回空

**验证：** `go test ./tests/rag/ -v -run TestChunker` 全部通过

**Commit：** `feat: 实现 RecursiveCharacterTextSplitter 文本分块器`

---

### Task 3.3: 实现中文分词 + BM25 倒排索引

**文件：**
- 新增：`server/internal/rag/bm25.go` — `Segmenter` 接口 + `gseSegmenter` 实现（封装 gse 分词库）+ `BM25Index` 结构体（倒排索引 map[token][]posting）+ `BM25Retriever`（实现 `Retriever` 接口，管理多 kb 索引的懒加载和 TTL 过期）+ 后台清理 goroutine
- 新增：`server/tests/rag/bm25_test.go` — 测试：中文分词正确性（"账号冻结" → ["账号", "冻结"]）、倒排索引构建、Okapi 分数计算、多知识库隔离、索引失效后重建、TTL 过期释放

**验证：** `go test ./tests/rag/ -v -run TestBM25` 全部通过

**Commit：** `feat: 实现 BM25 倒排索引 + 中文分词（gse）+ 懒加载 + TTL`

---

### Task 3.4: 实现 Embedder 批量向量生成

**文件：**
- 新增：`server/internal/rag/embedder.go` — `Embedder` 结构体（封装 `EmbeddingClient`，批处理大小默认 20），`Embed(texts []string)` 方法：分批调用 API，处理部分失败的批并返回成功结果 + 失败计数
- 新增：`server/tests/rag/embedder_test.go` — mock `EmbeddingClient` 测试：单批处理、多批分页、部分批次失败后继续、维度校验

**验证：** `go test ./tests/rag/ -v -run TestEmbedder` 全部通过

**Commit：** `feat: 实现 Embedder 批量向量生成 — 分批调用 + 部分失败处理`

---

### Task 3.5: 实现文档解析器

**文件：**
- 新增：`server/internal/rag/document_parser.go` — `DocParser` 结构体，`Parse(reader, fileType)` 方法：根据扩展名选择 PDF（`ledongthuc/pdf`）/DOCX（`archive/zip` + `encoding/xml`）/MD（直接读取）/TXT（直接读取）解析器，返回纯文本和错误
- 新增：`server/tests/rag/document_parser_test.go` — 测试：PDF 文本提取、DOCX 文本提取、MD 保留换行、TXT UTF-8、不支持的文件类型报错、空文件处理

**验证：** `go test ./tests/rag/ -v -run TestDocParser` 全部通过

**Commit：** `feat: 实现多格式文档解析器 — PDF/DOCX/MD/TXT`

---

### Task 3.6: 实现 RRF 混合融合算法

**文件：**
- 新增：`server/internal/rag/hybrid.go` — `HybridFuse(vectorResults, bm25Results, k, topK)` 函数：按 chunk_id 合并两路分数，计算 RRF_score(d) = Σ(1/(k+rank_i(d)))，降序截取 topK
- 新增：`server/tests/rag/hybrid_test.go` — 测试：两路均有结果时融合、仅一路有结果时降级、相同文档在不同排行位置的正确 RRF 分数、topK 截取、空输入

**验证：** `go test ./tests/rag/ -v -run TestHybrid` 全部通过

**Commit：** `feat: 实现 RRF 混合融合算法 — Reciprocal Rank Fusion`

---

### Task 3.7: 实现 RAG 管道编排器

**文件：**
- 新增：`server/internal/rag/pipeline.go` — `Pipeline` 结构体（组装 `VectorRetriever`/`BM25Retriever`/`LLMClient`/`Embedder`）。`Execute(ctx, query, history, kbID, opts)` 方法：按序执行 QueryRewrite→MultiRoute→HybridRetrieve→Rerank，每步失败按降级矩阵处理，返回 `RAGResult` + `PipelineMetrics`。SSE 步骤事件通过回调函数 `onStep(stepID, label)` 通知
- 新增：`server/tests/rag/pipeline_test.go` — mock 所有依赖的管道编排测试：完整管道成功、查询改写失败降级、多路检索失败降级、BM25 失败仅用向量、重排序失败降级、向量检索失败返回错误（不降级）、步骤跳过（RAGOptions 开关）、管道耗时指标正确

**验证：** `go test ./tests/rag/ -v -run TestPipeline` 全部通过

**Commit：** `feat: 实现 RAG 管道编排器 — 查询改写→多路检索→混合检索→重排序`

---

### Task 3.8: 实现查询改写 + 多路检索 + 重排序

**文件：**
- 新增：`server/internal/rag/query_rewrite.go` — `QueryRewrite(ctx, query, history)` 函数：构造 LLM prompt（含最近 3 轮对话），调用 `LLMClient.ChatCompletion`，失败降级返回原始 query
- 新增：`server/internal/rag/multi_route.go` — `MultiRoute(ctx, query)` 函数：构造 LLM prompt 生成 2-4 个子查询，失败降级返回 `[query]`
- 新增：`server/internal/rag/rerank.go` — `Rerank(ctx, query, candidates)` 函数：构造 rerank prompt，调用 LLM 对候选重新评分排序，失败降级返回原始排序
- 新增：`server/tests/rag/pipeline_steps_test.go` — mock LLMClient 测试：查询改写正确性、多路检索子查询生成、重排序结果变换、各步骤超时降级

**验证：** `go test ./tests/rag/ -v -run TestPipelineSteps` 全部通过

**Commit：** `feat: 实现查询改写 + 多路检索 + 重排序 — LLM 驱动的 RAG 增强步骤`

---

### Task 3.9: 实现文档处理管线（异步 goroutine pool）

**文件：**
- 新增：`server/internal/rag/processor.go` — `Processor` 结构体（`poolSize`/`taskCh`/`DocParser`/`Chunker`/`Embedder`/`VectorStore`/`KnowledgeRepo`）。`Submit(task)` 不阻塞入队。`worker()` goroutine 执行：MinIO 下载→Parse→Split→Embed→BatchInsert，每阶段更新 `process_status`。`Stop()` 优雅关闭
- 新增：`server/tests/rag/processor_test.go`（`-tags=integration`）— mock MinIO + 真实 pgvector：提交单文档任务、处理状态流转正确、失败重试、优雅关闭、并发提交多文档

**验证：** `go test ./tests/rag/ -v -tags=integration -run TestProcessor` 全部通过

**Commit：** `feat: 实现文档异步处理管线 — goroutine pool + 状态持久化`

---

## M4: Service 层改造

### Task 4.1: 实现 LLMConfigService

**文件：**
- 新增：`server/internal/service/llm_config_service.go` — `LLMConfigService` 结构体 + `LLMConfigManager`（`atomic.Value` 热替换）。方法：`GetConfig()` 零锁读取、`UpdateConfig(cfg)` 写 DB + `atomic.Store`、`ListConfigs()`/`CreateConfig()`/`DeleteConfig()` CRUD、`TestConnection(cfg)` 调用 LLMClient 验证连接
- 新增：`server/tests/service/llm_config_service_test.go` — mock DB + mock LLMClient 测试：默认配置读写、`is_default` 唯一性、删除默认配置拒绝、测试连接成功/超时、配置更新后 `GetConfig()` 即时返回新值

**验证：** `go test ./tests/service/ -v -run TestLLMConfig` 全部通过

**Commit：** `feat: 实现 LLMConfigService — atomic.Value 热替换 + CRUD + 测试连接`

---

### Task 4.2: 改造 ChatService — 依赖 RAG Pipeline + LLMClient

**文件：**
- 修改：`server/internal/service/chat_service.go` — `ChatService` 结构体：移除 `RagClient` 依赖，新增 `rag.Pipeline` + `adapter.LLMClient` + `adapter.Embedder` + `adapter.VectorStore` + `LLMConfigManager`。`CreateChatSession()` 方法：调用 `rag.Pipeline.Execute()` 获取分块→构造带 context 的 LLM prompt→调用 `LLMClient.ChatCompletion()` 生成答案→保存会话→返回；SSE 流式方法：调用 `LLMClient.ChatCompletionStream()` 通过 channel 返回给 Handler
- 新增：`server/tests/service/chat_service_test.go` — mock Pipeline + mock LLMClient 测试：正常问答流程、低置信度兜底、AI 服务不可用降级、RAG 管道失败降级、SSE 流式 channel 正确关闭

**验证：** `go test ./tests/service/ -v -run TestChatService` 全部通过

**Commit：** `feat: 改造 ChatService — 依赖 RAG Pipeline + LLMClient，移除 RagClient`

---

### Task 4.3: 改造 KnowledgeService — 统一文章模型 + pgvector

**文件：**
- 修改：`server/internal/service/knowledge_service.go` — `KnowledgeService` 结构体：移除 `RagClient` 依赖，新增 `rag.Chunker` + `rag.Embedder` + `adapter.VectorStore` + `rag.Processor`。方法改造：
  - `CreateKB()` — 不再调 AnythingLLM workspace 创建，仅写 PostgreSQL
  - `Publish()` — 调用 `Chunker.Split()`→`Embedder.Embed()`→`VectorStore.BatchInsert()`→`BM25Retriever.Invalidate()`，替代原 AnythingLLM 同步
  - `Disable()` — 调用 `VectorStore.DeleteByArticle()` + `BM25Retriever.Invalidate()`
  - `Enable()` — 重新执行发布流程
  - `RetrySync()` — 从 AnythingLLM 重试改为重试 embedding+pgvector 写入
  - 新增：`UploadDocuments()` — 接收 multipart 文件，保存 MinIO，创建 article，提交到 Processor
  - 新增：`GetDocumentStatus()` — 查询 process_status
  - 新增：`RetryDocument()` — 重置 process_status 为 pending，重新入队
- 修改：`server/tests/service/knowledge_service_test.go` — mock Chunker/Embedder/VectorStore/Processor 测试：发布触发分块+embedding+写入流程、停用调用 DeleteByArticle、启用重新写入、文档上传创建文章+入队、重试重置状态

**验证：** `go test ./tests/service/ -v -run TestKnowledge` 全部通过

**Commit：** `feat: 改造 KnowledgeService — 统一文章模型 + pgvector 发布/停用 + 文档上传`

---

### Task 4.4: 更新 main.go 初始化流程

**文件：**
- 修改：`server/cmd/main.go` — 初始化 `LLMConfigManager`（从 DB 或环境变量加载默认配置）→ 创建 `LLMClient`/`EmbeddingClient`→ 创建 `VectorStore`→ 创建 `BM25Retriever`→ 创建 `Pipeline`→ 创建 `Processor`→ 创建 `Service`→ 创建 `Handler`→ 注入 `Router`。移除 `RagClient` 初始化。启动 `Processor` 的 goroutine pool，通过 `context.WithCancel` 管理生命周期

**验证：** `go build ./cmd/...` 编译通过，`go run ./cmd/main.go` 启动无 panic（config 正确即可）

**Commit：** `feat: 更新 main.go — 初始化 v2 RAG 引擎 + 适配层，移除 RagClient`

---

## M5: Handler + 路由

### Task 5.1: 实现 LLMConfigHandler + 注册路由

**文件：**
- 新增：`server/internal/handler/llm_config.go` — `LLMConfigHandler` 结构体。方法：`ListConfigs`/`CreateConfig`/`GetConfig`/`UpdateConfig`/`DeleteConfig`/`TestConnection`。请求/响应使用 `server/internal/dto/request/llm_config.go` 和 `server/internal/dto/response/llm_config.go` 定义
- 新增：`server/internal/dto/request/llm_config.go` — `CreateLLMConfigRequest`/`UpdateLLMConfigRequest`
- 新增：`server/internal/dto/response/llm_config.go` — `LLMConfigResponse`（含 `api_key_masked` 字段）
- 修改：`server/internal/router/admin.go` — 注册 6 个 LLM 配置路由：`GET/POST /llm-configs`、`GET/PUT/DELETE /llm-configs/:id`、`POST /llm-configs/:id/test`
- 修改：`server/internal/router/router.go` — `Handlers` 结构体新增 `LLMConfig *handler.LLMConfigHandler` 字段

**验证：** `go build ./cmd/...` 编译通过，mock HTTP 测试 6 个端点可访问

**Commit：** `feat: 实现 LLMConfigHandler + 6 个 LLM 配置 API 路由`

---

### Task 5.2: 新增文档上传/状态/重试 Handler + 路由

**文件：**
- 修改：`server/internal/handler/knowledge.go` — 在 `KnowledgeHandler` 中新增方法：
  - `UploadDocuments` — 接收 multipart form，校验格式和大小，调用 `KnowledgeService.UploadDocuments()`
  - `GetDocumentStatus` — 查询 article 的 `process_status` 和进度
  - `RetryDocument` — 重置状态并重新入队
- 修改：`server/internal/router/admin.go` — 在知识库路由组中新增：`POST /knowledge-bases/:kb_id/documents/upload`、`GET /knowledge-bases/:kb_id/documents/:id/status`、`POST /knowledge-bases/:kb_id/documents/:id/retry`、新增 `DELETE /knowledge-bases/:id`
- 修改：`server/internal/dto/request/knowledge.go` — 新增 `UploadDocumentRequest`、`CreateArticleRequest` 改为 `{title, content, source_type, category, tags}`（移除 question/answer）

**验证：** `go build ./cmd/...` 编译通过

**Commit：** `feat: 新增文档上传/状态/重试 API + 改造 CreateArticle 请求体`

---

### Task 5.3: 升级 SSE 流式 Handler — 真正 token 级流式 + 管道步骤

**文件：**
- 修改：`server/internal/handler/chat.go` — `StreamChatSession` 方法改造：
  - 解析 `rag_options` 请求参数
  - 调用 `ChatService.CreateChatSessionStream()` 获取 token channel
  - `for chunk := range tokenChan` 循环：检测 `ctx.Done()` 断连→写 `{"type":"token","content":"..."}` SSE 事件→flush
  - 发送步骤事件 `{"type":"step","id":"...","label":"..."}`（通过 Service 层回调）
  - 流式结束后发送 done 事件（含 pipeline 耗时）
  - 移除旧的「每次 5 字符 + 30ms 间隔」模拟分块逻辑
- 修改：`server/internal/dto/request/chat.go` — `CreateChatRequest` 新增 `RAGOptions` 字段

**验证：** `go build ./cmd/...` 编译通过

**Commit：** `feat: 升级 SSE 流式为真正 token 级流式 + RAG 管道步骤事件`

---

### Task 5.4: 移除 embedding-configs 路由

**文件：**
- 修改：`server/internal/router/admin.go` — 删除 `GET/POST/PUT/DELETE /embedding-configs` 四行路由注册（Task 5.1 的 llm-configs 已替代）

**验证：** `grep -r "embedding-configs" server/internal/router/` 无结果

**Commit：** `feat: 移除 embedding-configs 路由 — 已被 llm-configs 替代`

---

## M6: 前端

### Task 6.1: 新增 LLM 配置页面

**文件：**
- 新增：`web/src/api/llm_config.ts` — API 封装：`getLLMConfigs()`/`createLLMConfig()`/`getLLMConfig()`/`updateLLMConfig()`/`deleteLLMConfig()`/`testLLMConnection()`。请求/响应的 TypeScript 类型定义
- 新增：`web/src/views/admin/LLMConfig.vue` — 配置列表页：表格展示（名称、提供商类型标签、Base URL、模型名称、默认标记）、新建/编辑弹窗（含测试连接按钮）、删除确认。替代原 `EmbeddingConfig.vue`
- 修改：`web/src/router/index.ts` — 新增 `/admin/llm-config` 路由（替代 `/admin/embedding-config`）

**验证：** `npm run type-check` 通过，dev-browser 验证 CRUD + 测试连接流程

**Commit：** `feat: 新增 LLM 配置管理页面 — llama.cpp / OpenAI-compatible 双支持`

---

### Task 6.2: 新增文档上传功能到知识库编辑页

**文件：**
- 修改：`web/src/views/admin/KnowledgeEdit.vue` — 新增「上传文档」标签页：拖拽/点击选择区域、文件列表（文件名、大小、格式图标）、上传进度（pending→parsing→...→completed 状态轮询）、处理失败提示和重试按钮。手动输入模式：`title` + `content`（Markdown 编辑器）替代原 `question`/`answer` 字段
- 修改：`web/src/api/knowledge.ts` — 新增 `uploadDocuments()`/`getDocumentStatus()`/`retryDocument()` API 封装，更新 `createArticle()`/`updateArticle()` 请求体类型（title/content/source_type）

**验证：** `npm run type-check` 通过，dev-browser 验证上传 4 种格式文档 + 处理状态查询

**Commit：** `feat: 知识库编辑页增加文档上传 + v2 统一文章模型字段`

---

### Task 6.3: 更新知识库列表页适配 v2 字段

**文件：**
- 修改：`web/src/views/admin/KnowledgeList.vue` — 表格列更新：
  - 「标题」列（替代原「问题」列）
  - 「来源」列（图标区分手动/上传）
  - 「字数」列
  - 「处理状态」标签（仅文档上传文章显示）
  - 筛选器增加 `source_type` 下拉

**验证：** `npm run type-check` 通过，dev-browser 验证列表展示 + 筛选

**Commit：** `feat: 知识库列表页适配 v2 统一文章模型 — 来源类型、处理状态`

---

### Task 6.4: 更新问答页面 — RAG 管道步骤 + rag_options

**文件：**
- 修改：`web/src/views/portal/Chat.vue` — 新增管道步骤指示器（step 事件渲染成标签/进度条）、`rag_options` 高级设置面板（折叠的折叠面板，含 TopK/查询改写/混合检索/重排序开关）
- 修改：`web/src/api/chat.ts` — `StreamCallbacks` 类型新增 `onStep` 回调函数签名，`streamChatSession()` 解析 `type: "step"` 事件
- 修改：`web/src/stores/chat.ts` — 新增 `currentStep`/`pipelineMetrics` 状态字段

**验证：** `npm run type-check` 通过，dev-browser 验证 SSE 流式 + 步骤显示 + options 控制

**Commit：** `feat: 问答页面增加 RAG 管道步骤显示 + rag_options 高级设置`

---

### Task 6.5: 移除 Embedding 配置页面 + 更新模型配置页

**文件：**
- 删除：`web/src/views/admin/EmbeddingConfig.vue`（被 LLMConfig.vue 替代）
- 修改：`web/src/views/admin/ModelConfig.vue` — 移除 AnythingLLM 相关配置项（API Key 初始化引导、workspace 配置）、新增 LLM 配置入口（链接到 `/admin/llm-config`）
- 修改：`web/src/router/index.ts` — 移除 `/admin/embedding-config` 路由，重定向到 `/admin/llm-config`
- 修改：`web/src/api/knowledge.ts` — 移除 `getEmbeddingConfigs()` 等旧 API 调用

**验证：** `npm run type-check` 通过，dev-browser 验证旧路由重定向

**Commit：** `feat: 移除 EmbeddingConfig 页面 — 已被 LLMConfig 替代`

---

## M7: 清理 + 文档

### Task 7.1: 删除 AnythingLLM 相关代码

**文件：**
- 删除：`server/internal/adapter/rag_client.go` — `AnythingLLMClient` 实现 + `RAGClient` 接口 + `RAGQueryRequest`/`RAGQueryResponse`/`RAGSyncRequest`/`RAGSyncResponse`/`RAGDisableRequest`/`RAGCreateWorkspaceRequest`/`RAGCreateWorkspaceResponse` 类型
- 删除：`server/tests/adapter/rag_client_test.go` — AnythingLLM mock 测试
- 修改：`server/internal/service/chat_service.go` — 移除所有 `RagClient` 类型引用（import 清理）
- 修改：`server/internal/service/knowledge_service.go` — 移除所有 `RagClient` 类型引用

**验证：** `grep -r "RagClient\|AnythingLLM\|anythingllm" server/internal/ --include="*.go" | grep -v "_test.go" | grep -v "//"` 无结果；`go build ./cmd/...` 编译通过

**Commit：** `chore: 删除 AnythingLLM RagClient 适配器 — v2 已全部替换为自建 RAG 引擎`

---

### Task 7.2: Docker Compose + 环境变量清理

**文件：**
- 修改：`docker-compose.yml` — 删除 `anythingllm` 服务（容器定义 + volumes + 环境变量）、删除 `vllm` 服务、新增 `llama-cpp` 可选服务（`profiles: [ai-local]`，镜像 `ghcr.io/ggerganov/llama.cpp:server`，volume 挂载 `./models:/models`）、postgres 镜像已改为 `pgvector/pgvector:pg18`（Task 1.1）
- 修改：`.env.example` — 已更新（Task 1.1）
- 修改：`docs/v2/TECHv2.md` — 添加最终 Docker Compose 验证结果

**验证：** `docker compose up -d --build` 启动 4 个必须服务均 healthy；`docker compose ps` 不含 anythingllm；`docker compose --profile ai-local up -d` 额外启动 llama-cpp

**Commit：** `chore: Docker Compose 清理 — 移除 anythingllm+vllm，新增可选 llama-cpp`

---

### Task 7.3: 文档定稿

**文件：**
- 修改：`docs/v2/PRDv2.md` — 更新实施完成状态、版本号
- 修改：`docs/v2/TECHv2.md` — 更新实施完成状态、版本号
- 修改：`docs/API/README.md` — 确认所有 v2 API 端点已标注
- 新增：`docs/v2/CHANGELOG.md` — v1→v2 完整变更日志（文件变更清单 + API 变更清单 + 数据库变更清单）

**验证：** 所有 `.md` 文件在 VSCode 预览中无断链

**Commit：** `docs: v2 文档定稿 — PRDv2/TECHv2/API/CHANGELOG 最终版本`

---

### Task 7.4: 端到端验证 + 集成测试全量通过

**验证步骤（不提交代码）：**

```bash
# 1. 启动全部服务
docker compose up -d --build
docker compose ps  # 4 个必须服务 all healthy

# 2. 数据库迁移
make migrate

# 3. 加载种子数据
make seed

# 4. 全量集成测试
go test ./tests/... -v -tags=integration

# 5. 前端编译检查
cd web && npm run type-check && npm run build

# 6. 手动验证关键路径
# - 登录 → LLM 配置 → 测试连接
# - 创建知识库 → 上传 PDF → 查看处理状态 → 发布
# - 手动创建文章 → 审核 → 发布
# - 智能问答 → 验证 SSE 流式 + 管道步骤显示
# - 停用知识 → 验证不再被检索
```

**Commit：** `test: v2 端到端验证通过 — 全量集成测试 + 手动验证关键路径`

---

## 附录 A: 完整文件变更清单

### 新增文件 (25 个)

| 文件 | 里程碑 | 职责 |
|------|--------|------|
| `server/migrations/v2/001_pgvector_extension.sql` | M1 | pgvector 扩展 |
| `server/migrations/v2/002_alter_knowledge_bases.sql` | M1 | knowledge_bases 表变更 |
| `server/migrations/v2/003_alter_knowledge_articles.sql` | M1 | knowledge_articles 表变更 |
| `server/migrations/v2/004_alter_knowledge_chunks.sql` | M1 | knowledge_chunks 表变更 |
| `server/migrations/v2/005_create_llm_configs.sql` | M1 | llm_configs 表创建 |
| `server/migrations/v2/006_create_indexes.sql` | M1 | HNSW + 业务索引 |
| `server/migrations/v2/007_alter_chat_messages.sql` | M1 | chat_messages 增加 rag_pipeline |
| `server/internal/adapter/llm_client.go` | M2 | LLMClient 接口 + OpenAI 实现 |
| `server/internal/adapter/embedding_client.go` | M2 | EmbeddingClient 接口 + 实现 |
| `server/internal/adapter/vector_store.go` | M2 | VectorStore 接口 + pgvector 实现 |
| `server/internal/rag/types.go` | M3 | RAG 共享类型 |
| `server/internal/rag/chunker.go` | M3 | RecursiveCharacterTextSplitter |
| `server/internal/rag/bm25.go` | M3 | BM25 倒排索引 + 中文分词 |
| `server/internal/rag/embedder.go` | M3 | 批量 Embedding 生成 |
| `server/internal/rag/document_parser.go` | M3 | PDF/DOCX/MD/TXT 解析 |
| `server/internal/rag/hybrid.go` | M3 | RRF 混合融合 |
| `server/internal/rag/pipeline.go` | M3 | RAG 管道编排器 |
| `server/internal/rag/query_rewrite.go` | M3 | 查询改写 |
| `server/internal/rag/multi_route.go` | M3 | 多路检索 |
| `server/internal/rag/rerank.go` | M3 | 重排序 |
| `server/internal/rag/processor.go` | M3 | 文档异步处理 |
| `server/internal/service/llm_config_service.go` | M4 | LLM 配置管理 |
| `server/internal/handler/llm_config.go` | M5 | LLM 配置 API |
| `web/src/api/llm_config.ts` | M6 | LLM 配置前端 API |
| `web/src/views/admin/LLMConfig.vue` | M6 | LLM 配置页面 |
| `docs/v2/CHANGELOG.md` | M7 | v2 变更日志 |

### 新增测试文件 (10 个)

| 文件 | 里程碑 |
|------|--------|
| `server/tests/adapter/llm_client_test.go` | M2 |
| `server/tests/adapter/embedding_client_test.go` | M2 |
| `server/tests/adapter/vector_store_test.go` | M2 |
| `server/tests/rag/chunker_test.go` | M3 |
| `server/tests/rag/bm25_test.go` | M3 |
| `server/tests/rag/embedder_test.go` | M3 |
| `server/tests/rag/document_parser_test.go` | M3 |
| `server/tests/rag/hybrid_test.go` | M3 |
| `server/tests/rag/pipeline_test.go` | M3 |
| `server/tests/rag/pipeline_steps_test.go` | M3 |
| `server/tests/rag/processor_test.go` | M3 |
| `server/tests/service/llm_config_service_test.go` | M4 |

### 修改文件 (15 个)

| 文件 | 里程碑 | 变更内容 |
|------|--------|----------|
| `docker-compose.yml` | M1+M7 | pgvector镜像 + llama-cpp可选 + 移除anythinllm+vllm |
| `.env.example` | M1 | 移除 ANYTHINGLLM_*/VLLM_*，新增 LLM/Embedding 变量 |
| `server/migrations/seed.sql` | M1 | v2 文章模型 + LLM 配置默认值 |
| `server/internal/config/config.go` | M2 | LLM/Embedding 配置结构体 |
| `server/cmd/main.go` | M4 | 初始化 v2 RAG 引擎替代 RagClient |
| `server/internal/service/chat_service.go` | M4+M7 | 依赖 Pipeline+LLMClient，移除 RagClient |
| `server/internal/service/knowledge_service.go` | M4+M7 | 依赖 VectorStore+Processor，移除 RagClient |
| `server/internal/handler/chat.go` | M5 | SSE 流式升级 |
| `server/internal/handler/knowledge.go` | M5 | 文档上传/状态/重试 handler |
| `server/internal/dto/request/chat.go` | M5 | CreateChatRequest 增加 RAGOptions |
| `server/internal/dto/request/knowledge.go` | M5 | 文章请求体改为 title/content |
| `server/internal/router/admin.go` | M5+M7 | 新增 LLM配置+文档路由，删除 embedding-configs |
| `server/internal/router/router.go` | M5 | Handlers 新增 LLMConfig |
| `web/src/views/admin/KnowledgeEdit.vue` | M6 | 文档上传 + Markdown 编辑器 |
| `web/src/views/admin/KnowledgeList.vue` | M6 | v2 字段列 |
| `web/src/views/portal/Chat.vue` | M6 | 管道步骤 + rag_options |
| `web/src/views/admin/ModelConfig.vue` | M6 | 移除 AnythingLLM 项 |
| `web/src/api/chat.ts` | M6 | onStep 回调 + rag_options |
| `web/src/api/knowledge.ts` | M6 | 文档上传 API + title/content |
| `web/src/stores/chat.ts` | M6 | 管道步骤状态 |
| `web/src/router/index.ts` | M6 | llm-config 路由替代 embedding-config |

### 删除文件 (3 个)

| 文件 | 里程碑 |
|------|--------|
| `server/internal/adapter/rag_client.go` | M7 |
| `server/tests/adapter/rag_client_test.go` | M7 |
| `web/src/views/admin/EmbeddingConfig.vue` | M6 |

---

## 附录 B: 依赖关系图

```
M1 环境准备
  └─→ M2 适配层 (LLMClient / EmbeddingClient / VectorStore)
        └─→ M3 RAG 引擎 (Chunker / BM25 / DocParser / Pipeline / Processor)
              └─→ M4 Service 层 (LLMConfigService / ChatService / KnowledgeService)
                    └─→ M5 Handler+路由 (LLM Config API / 文档上传 API / SSE 升级)
                          └─→ M6 前端 (LLM 配置页 / 文档上传 / 知识库适配 / 问答升级)
                                └─→ M7 清理 (删除旧代码 / Docker 清理 / 文档定稿)
```

每个箭头表示"被依赖方必须先完成"。M2 和 M3 的 `rag/` 包可部分并行（`rag/types.go` 先合并后，`rag/chunker.go` 和 `rag/bm25.go` 可并行开发）。

---

## 附录 C: PRDv2 需求覆盖对照

| PRDv2 章节 | 覆盖任务 |
|------------|----------|
| US-V2-001 LLM 配置 | Task 4.1 (Service) + Task 5.1 (Handler) + Task 6.1 (前端) |
| US-V2-002 文档上传解析 | Task 3.5 (DocParser) + Task 3.9 (Processor) + Task 5.2 (Handler) + Task 6.2 (前端) |
| US-V2-003 统一文章模型 | Task 1.2 (迁移) + Task 1.3 (seed) + Task 4.3 (Service) + Task 5.2 (DTO) |
| US-V2-004 pgvector 向量存储 | Task 1.1 (镜像) + Task 1.2 (扩展) + Task 2.3 (VectorStore) |
| US-V2-005 RAG 检索管道 | Task 3.1-3.8 (rag/ 全部) + Task 4.2 (ChatService) |
| US-V2-006 SSE 流式 | Task 2.1 (LLMClient 流式) + Task 5.3 (Handler 升级) + Task 6.4 (前端) |
| US-V2-007 Docker 简化 | Task 1.1 (pgvector 镜像) + Task 7.2 (清理) |
| US-V2-008 前端适配 | Task 6.1-6.5 (前端全部) |
| FR-11~15 LLM 适配层 | Task 2.1 + Task 2.2 |
| FR-16~19 pgvector 适配层 | Task 2.3 + Task 1.2 |
| FR-20~24 文档处理管线 | Task 3.5 + Task 3.9 + Task 5.2 |
| FR-29~31 SSE 流式升级 | Task 5.3 + Task 2.1 |
| RM-1~9 移除 AnythingLLM | Task 7.1 + Task 7.2 + Task 5.4 + Task 6.5 |
