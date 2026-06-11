# OpsMind v2 变更日志

> **发布版本：** v2.0.0  
> **发布日期：** 2026-06-11  
> **变更范围：** 移除 AnythingLLM → 自建 Go RAG 引擎 + pgvector + SSE 流式升级 + 前端适配

---

## 架构变更

| v1 (已废弃) | v2 (当前) |
|------------|----------|
| AnythingLLM 作为 RAG 服务 | 自建 Go RAG 引擎 (`server/internal/rag/`) |
| AnythingLLM LanceDB 向量存储 | pgvector (HNSW 索引 + halfvec 半精度) |
| VLLM 推理引擎 | llama.cpp server 或 OpenAI-compatible API |
| `embedding-configs` API | `llm-configs` API（统一管理 LLM + Embedding） |
| 问题-答案双字段文章模型 | 标题-内容统一文章模型 |
| 5 字符 + 30ms 间隔模拟流式 | 真正 token 级 SSE 流式（LLMClient.ChatCompletionStream） |
| 3 Docker 服务 (anythingllm+vllm+app) | 4 必须 + 1 可选 (app+web+pg+minio+llama-cpp) |

---

## 文件变更清单

### 新增文件

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
| `server/internal/service/chat_service_v2.go` | M4 | ChatServiceV2 |
| `server/internal/service/knowledge_service_v2.go` | M4 | KnowledgeServiceV2 |
| `server/internal/handler/llm_config.go` | M5 | LLM 配置 API |
| `web/src/api/llm_config.ts` | M6 | LLM 配置前端 API |
| `web/src/views/admin/LLMConfig.vue` | M6 | LLM 配置页面 |

### 删除文件

| 文件 | 里程碑 | 原因 |
|------|--------|------|
| `server/internal/adapter/rag_client.go` | M7 | 移除 AnythingLLM RagClient 适配器 |
| `server/tests/adapter/rag_client_test.go` | M7 | 移除 RagClient 测试 |
| `web/src/views/admin/EmbeddingConfig.vue` | M6 | 已被 LLMConfig.vue 替代 |

### 修改文件

| 文件 | 里程碑 | 变更内容 |
|------|--------|----------|
| `docker-compose.yml` | M1 | pgvector 镜像 + llama-cpp 可选服务 |
| `.env.example` | M1 | LLM/Embedding 环境变量 |
| `server/migrations/seed.sql` | M1 | v2 文章模型 + LLM 配置默认值 |
| `server/internal/config/config.go` | M2 | LLM/Embedding 配置结构体 |
| `server/cmd/main.go` | M4+M7 | 初始化 v2 RAG 引擎，移除 RagClient |
| `server/internal/service/chat_service.go` | M4+M7 | 移除 RagClient 依赖 |
| `server/internal/service/knowledge_service.go` | M4+M7 | 移除 RagClient 依赖 |
| `server/internal/handler/chat.go` | M5 | SSE 升级为真实 token 流式 |
| `server/internal/handler/knowledge.go` | M5 | 文档上传/状态/重试 |
| `server/internal/dto/request/chat.go` | M5 | RAGOptions |
| `server/internal/dto/request/knowledge.go` | M5 | title/content |
| `server/internal/router/admin.go` | M5 | LLM 配置路由 + 文档路由 |
| `server/internal/router/router.go` | M5 | Handlers 新增 LLMConfig |
| `web/src/views/admin/KnowledgeEdit.vue` | M6 | 文档上传 + v2 字段 |
| `web/src/views/admin/KnowledgeList.vue` | M6 | v2 列 + 来源筛选 |
| `web/src/views/portal/Chat.vue` | M6 | 管道步骤 + rag_options |
| `web/src/views/admin/ModelConfig.vue` | M6 | LLM 配置入口 |
| `web/src/api/chat.ts` | M6 | onStep 回调 + rag_options |
| `web/src/api/knowledge.ts` | M6 | 文档上传 API + v2 字段 |
| `web/src/stores/chat.ts` | M6 | 管道步骤状态 |
| `web/src/router/index.ts` | M6 | llm-config 路由 |

---

## API 变更

### 新增端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/admin/llm-configs` | 列出 LLM 配置 |
| POST | `/api/v1/admin/llm-configs` | 创建 LLM 配置 |
| GET | `/api/v1/admin/llm-configs/:id` | 获取 LLM 配置 |
| PUT | `/api/v1/admin/llm-configs/:id` | 更新 LLM 配置 |
| DELETE | `/api/v1/admin/llm-configs/:id` | 删除 LLM 配置 |
| POST | `/api/v1/admin/llm-configs/:id/test` | 测试 LLM 连接 |
| POST | `/api/v1/admin/knowledge-bases/:id/documents/upload` | 上传文档 |
| GET | `/api/v1/admin/knowledge-bases/:id/documents/:id/status` | 文档处理状态 |
| POST | `/api/v1/admin/knowledge-bases/:id/documents/:id/retry` | 重试文档处理 |

### 删除端点

| 方法 | 路径 | 替代 |
|------|------|------|
| GET/POST/PUT/DELETE | `/api/v1/admin/embedding-configs` | `/api/v1/admin/llm-configs` |

### 请求体变更

| 端点 | v1 字段 | v2 字段 |
|------|---------|---------|
| `POST /articles` | `question`, `answer` | `title`, `content`, `source_type` |
| `PUT /articles/:id` | `question`, `answer` | `title`, `content` |
| `POST /chat-sessions/stream` | — | 新增 `rag_options` |

---

## 数据库变更

| 表 | 变更 |
|----|------|
| `knowledge_bases` | 删除 `rag_workspace_slug`，新增 `llm_config_id` |
| `knowledge_articles` | 删除 `question`/`rag_document_location`，`answer`→`content`，新增 `title`/`source_type`/`word_count`/`chunk_count`/`file_type`/`minio_path`/`process_status` |
| `knowledge_chunks` | 删除 `sync_status`/`sync_error`/`synced_at`，新增 `kb_id`/`chunk_index`/`embedding halfvec(N)` |
| `chat_messages` | 新增 `rag_pipeline` jsonb |
| `llm_configs` | **新表** — id/name/provider_type/base_url/api_key/llm_model/embedding_model/max_tokens/vector_dimension/is_default |
| 索引 | HNSW 向量索引 + kb_id 索引 + article_id 索引 |

---

## 测试覆盖

| 测试套件 | 测试文件数 | 测试数 | 类型 |
|----------|-----------|--------|------|
| Go adapter | 2 | 9 | 单元 |
| Go config | 1 | 5 | 单元 |
| Go rag | 7 | 37 | 单元 |
| Go service | 3 | 19 | 单元 |
| Go handler | 3 | 13 | 单元 |
| Go integration | 4 | 8 | 集成 |
| **Go 小计** | **20** | **91** | |
| Vue API | 5 | 29 | 单元 |
| Vue Store | 3 | 13 | 单元 |
| **前端小计** | **8** | **42** | |
| **总计** | **28** | **133** | |
