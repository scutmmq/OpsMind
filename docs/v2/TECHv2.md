# 运维数字员工系统 — 技术架构文档 v2

| 项目 | 内容 |
| --- | --- |
| 文档版本 | v2.0 |
| 日期 | 2026-06-11 |
| 实施状态 | ✅ 已完成（M1-M7 全部交付，133 测试通过） |
| 变更说明 | v2 架构重构：移除 AnythingLLM，自建 Go RAG 引擎 + pgvector 向量存储 |
| 关联文档 | [CHANGELOG](CHANGELOG.md) · [PRDv2](PRDv2.md) · [PLANv2](PLANv2.md) · [API 文档](../API/README.md) |

---

## 1. 架构总览

### 1.1 系统架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           客户端（浏览器）                                     │
│  ┌─────────────────────────┐    ┌──────────────────────────────────────┐    │
│  │    门户端 (Portal)       │    │       后台管理端 (Admin)              │    │
│  │   - 智能问答 (SSE 流式)   │    │   - 申告处理  - 知识库管理            │    │
│  │   - 申告提交/进度查询      │    │   - 文档上传  - LLM 配置             │    │
│  │   - 站内消息              │    │   - 账号管理  - 数据看板              │    │
│  └───────────┬──────────────┘    └────────────────┬─────────────────────┘    │
└──────────────┼────────────────────────────────────┼──────────────────────────┘
               │ HTTP/REST + SSE                    │ HTTP/REST
               ▼                                    ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                       Go 后端服务 (Gin) — 端口 8080                           │
│                                                                              │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌───────────┐ │
│  │ Handler 层  │ │ Handler 层  │ │ Handler 层  │ │ Handler 层  │ │Handler层  │ │
│  │  Auth      │ │  Chat      │ │  Ticket    │ │  Knowledge │ │  LLM     │ │
│  └─────┬──────┘ └─────┬──────┘ └─────┬──────┘ └─────┬──────┘ └─────┬─────┘ │
│        │              │              │              │              │         │
│  ┌─────┴──────┐ ┌─────┴──────────────┴─────┐ ┌─────┴──────────────┴─────┐  │
│  │Service 层   │ │    Service 层             │ │    Service 层            │  │
│  │ Auth/User  │ │    ChatService            │ │    KnowledgeService      │  │
│  │ /Role      │ │    ┌──────────────────┐   │ │    ┌──────────────────┐  │  │
│  └────────────┘ │    │ 依赖:            │   │ │    │ 依赖:            │  │  │
│                 │    │ - rag.Pipeline   │   │ │    │ - rag.Chunker    │  │  │
│                 │    │ - adapter.LLM    │   │ │    │ - rag.Embedder   │  │  │
│                 │    │   Client         │   │ │    │ - adapter.Vector  │  │  │
│                 │    └──────────────────┘   │ │    │   Store          │  │  │
│                 └───────────────────────────┘ │    └──────────────────┘  │  │
│                                               └──────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                   RAG 引擎 (server/internal/rag/)                     │   │
│  │                                                                      │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │   │
│  │  │ Pipeline     │  │ Retriever    │  │ Processor    │               │   │
│  │  │ (编排器)      │  │ (检索器接口)  │  │ (文档处理器)  │               │   │
│  │  │              │  │              │  │              │               │   │
│  │  │ QueryRewrite │  │ VectorStore  │  │ DocParser    │               │   │
│  │  │ MultiRoute   │  │ BM25Index    │  │ Chunker      │               │   │
│  │  │ HybridFuse   │  │ RRF Fuse     │  │ Embedder     │               │   │
│  │  │ Rerank       │  │              │  │              │               │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘               │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                    适配层 (server/internal/adapter/)                   │   │
│  │  ┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐     │   │
│  │  │ LLMClient        │ │ EmbeddingClient  │ │ VectorStore      │     │   │
│  │  │ (OpenAI-compat)  │ │ (OpenAI-compat)  │ │ (pgvector)       │     │   │
│  │  │                  │ │                  │ │                  │     │   │
│  │  │ ChatCompletion   │ │ CreateEmbeddings │ │ BatchInsert      │     │   │
│  │  │ ChatCompletion   │ │                  │ │ CosineSearch     │     │   │
│  │  │   Stream         │ │                  │ │ DeleteByArticle  │     │   │
│  │  └────────┬─────────┘ └────────┬─────────┘ └────────┬─────────┘     │   │
│  │           │                   │                    │                │   │
│  │  ┌────────┴───────────────────┴────────────────────┴─────────┐      │   │
│  │  │ StorageClient (MinIO) — upload / presigned URL / delete   │      │   │
│  │  └───────────────────────────────────────────────────────────┘      │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                    LLM 配置管理 (atomic.Value 热替换)                   │   │
│  │  LLMConfigManager — GetConfig() / UpdateConfig()                      │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
└──────────────────────────────────┬───────────────────────────────────────────┘
                                   │
            ┌──────────────────────┼──────────────────────┐
            ▼                      ▼                      ▼
┌──────────────────┐  ┌──────────────────────┐  ┌──────────────────┐
│ pgvector/pg18    │  │ MinIO                │  │ llama.cpp        │
│ (PostgreSQL)     │  │ (对象存储)            │  │ (可选 profile)    │
│                  │  │                      │  │                  │
│ - 业务数据        │  │ Bucket:              │  │ OpenAI-compat    │
│ - 向量存储        │  │ opsmind-knowledge    │  │ API :8080/v1     │
│   (halfvec)      │  │ opsmind-attachments  │  │                  │
│ - HNSW 索引       │  │                      │  │ 模型 volume 挂载  │
└──────────────────┘  └──────────────────────┘  └──────────────────┘
```

### 1.2 架构风格

**单体分层架构（Modular Monolith）+ 领域模块化**

v2 延续 v1 的单体分层架构，但在 RAG 相关模块引入**领域模块化**——`rag/` 包是自包含的 RAG 引擎，不依赖 Handler/Service/Repository 层，仅依赖 `adapter/` 接口。这保证了：

- RAG 引擎可独立测试（mock adapter 接口即可）
- 后续如需将 RAG 拆为独立服务，`rag/` 包可直接复用
- 检索算法（BM25、RRF）与 HTTP 传输完全隔离

### 1.3 v1 → v2 架构变更对照

```
v1:  OpsMind Server → RagClient → AnythingLLM → vLLM
                       (HTTP)      (Node.js)     (HTTP)

v2:  OpsMind Server → rag.Pipeline → LLMClient → llama.cpp / OpenAI
                              ↓
                       VectorStore → pgvector (同一 PostgreSQL)
```

| 关注点 | v1 | v2 |
| --- | --- | --- |
| RAG 引擎位置 | AnythingLLM 容器内（Node.js） | OpsMind 进程内（Go `rag/` 包） |
| 向量存储 | AnythingLLM 内部 LanceDB（文件） | pgvector 扩展（PostgreSQL 同库） |
| LLM 调用路径 | OpsMind → AnythingLLM → vLLM | OpsMind → LLMClient → llama.cpp/OpenAI |
| Embedding 生成 | AnythingLLM 内部调用 vLLM | OpsMind → EmbeddingClient → llama.cpp/OpenAI |
| 文档处理 | AnythingLLM 内部 | OpsMind goroutine pool |
| 配置生效方式 | 环境变量（需重启） | atomic.Value 热替换（即时生效） |

---

## 2. 核心模块设计

### 2.1 RAG 引擎 (`server/internal/rag/`)

#### 2.1.1 包结构

```
server/internal/rag/
├── pipeline.go          # 管道编排器 — Execute(ctx, query, kbID, opts)
├── types.go             # 共享类型：RAGOptions, RAGResult, RetrievalResult, StreamEvent
├── query_rewrite.go     # 查询改写 — LLM 消除指代歧义
├── multi_route.go       # 多路检索 — LLM 生成子查询，并行执行
├── retriever.go         # Retriever 接口定义 + 多路结果去重合并
├── vector_retriever.go  # VectorRetriever — 调用 VectorStore.CosineSearch
├── bm25.go              # BM25 倒排索引 + BM25Retriever
├── hybrid.go            # RRF 融合算法
├── rerank.go            # 重排序 — LLM 对候选分块重新评分
├── document_parser.go   # 文档解析器 — PDF/DOCX/MD/TXT → 纯文本
├── chunker.go           # RecursiveCharacterTextSplitter
├── embedder.go          # EmbeddingClient 封装 + 批处理
└── processor.go         # 文档处理管线 — goroutine pool 异步执行
```

#### 2.1.2 核心接口

```go
// =============================================================================
// Retriever — 检索器统一接口
// =============================================================================
// 设计决策（ADR-V2-001）：
// 向量检索和 BM25 检索实现统一接口，以便在管道中以相同方式调用和融合。
// 查询改写和重排序不实现此接口——它们依赖 LLM，不适合抽象为"检索器"。
// 这是"混合模式"架构选择：检索阶段用统一接口，其他步骤用独立函数。

// Retriever 定义检索器接口。
// 向量检索、BM25 检索都实现此接口，便于在管道中统一编排和融合。
type Retriever interface {
    // Retrieve 执行检索，返回按分数降序排列的结果列表。
    Retrieve(ctx context.Context, query string, kbID int64, topK int) ([]RetrievalResult, error)
}

// RetrievalResult 单条检索结果。
type RetrievalResult struct {
    ChunkID   int64   `json:"chunk_id"`   // knowledge_chunks.id
    ArticleID int64   `json:"article_id"` // 所属文章 ID
    Content   string  `json:"content"`     // 分块文本内容
    Score     float64 `json:"score"`       // 检索分数（向量: cosine 相似度; BM25: Okapi 分数）
    Source    string  `json:"source"`      // 来源类型："vector" | "bm25"
}
```

```go
// =============================================================================
// Pipeline — RAG 管道编排器
// =============================================================================

// Pipeline 编排 RAG 检索管道的完整执行流程。
//
// 管道步骤（按序执行，每步可通过 RAGOptions 开关控制）：
//   1. QueryRewrite  — LLM 查询改写（可选）
//   2. MultiRoute    — LLM 多路子查询（可选）
//   3. HybridRetrieve — 向量 + BM25 混合检索 + RRF 融合
//   4. Rerank        — LLM 重排序（可选）
//
// 为什么采用混合架构模式而非统一 Pipeline Stage 接口：
// 检索阶段（向量/BM25）天然适合统一接口，查询改写和重排序各自有独特的
// LLM 交互模式，强求统一接口会增加不必要的抽象成本。
type Pipeline struct {
    vectorRetriever Retriever      // pgvector 向量检索
    bm25Retriever   Retriever      // BM25 稀疏检索（懒加载缓存）
    llmClient       LLMClient      // 用于查询改写、多路路由、重排序
    embedder        *Embedder      // 用于将查询文本转为向量
}

// Execute 执行完整的 RAG 检索管道。
//
// 参数：
//   - ctx: 上下文，用于超时控制和取消传播
//   - query: 用户原始问题
//   - history: 最近 N 轮对话历史（用于查询改写，可为空）
//   - kbID: 目标知识库 ID
//   - opts: 管道选项（控制各步骤的开关和参数）
//
// 返回值：
//   - RAGResult: 最终的 top_k 个相关分块 + 管道执行元数据
//   - error: 管道失败（注意：单步骤失败会降级而非整体失败）
func (p *Pipeline) Execute(
    ctx context.Context,
    query string,
    history []ChatMessage,
    kbID int64,
    opts RAGOptions,
) (*RAGResult, error)
```

```go
// =============================================================================
// RAGOptions — 管道配置
// =============================================================================

type RAGOptions struct {
    TopK         int  `json:"top_k"`          // 最终返回的分块数，默认 5，范围 1-20
    QueryRewrite bool `json:"query_rewrite"`  // 是否启用查询改写，默认 true
    MultiRoute   bool `json:"multi_route"`    // 是否启用多路检索，默认 true
    Hybrid       bool `json:"hybrid"`         // 是否启用 BM25 混合检索，默认 true
    Rerank       bool `json:"rerank"`         // 是否启用重排序，默认 true
    RouteCount   int  `json:"route_count"`    // 多路子查询数，默认 3，范围 2-4
    RerankCount  int  `json:"rerank_count"`   // 进入重排序的候选数，默认 10（top_k × 2）
}

// RAGResult 管道执行结果。
type RAGResult struct {
    Chunks  []RetrievalResult    `json:"chunks"`  // 最终检索到的分块（top_k 个）
    Metrics PipelineMetrics      `json:"metrics"` // 管道执行指标
}

// PipelineMetrics 管道各步骤耗时和状态。
type PipelineMetrics struct {
    Steps       []StepMetric `json:"steps"`
    TotalDurationMS int64    `json:"total_duration_ms"`
}

type StepMetric struct {
    StepID     string `json:"step_id"`     // query_rewrite | multi_route | vector_retrieval | bm25_retrieval | hybrid_fuse | rerank
    Label      string `json:"label"`       // 中文标签
    DurationMS int64  `json:"duration_ms"`
    Success    bool   `json:"success"`
    Error      string `json:"error,omitempty"`
}
```

#### 2.1.3 管道执行流程

```
Pipeline.Execute(ctx, query, history, kbID, opts)
│
├─ 1. QueryRewrite (opts.QueryRewrite == true)
│   └─ llmClient.ChatCompletion(ctx, rewritePrompt(query, history))
│       失败 → 降级使用原始 query，记录 warning 日志
│       SSE: {"type":"step","id":"query_rewrite","label":"查询改写"}
│
├─ 2. MultiRoute (opts.MultiRoute == true)
│   └─ llmClient.ChatCompletion(ctx, multiRoutePrompt(query))
│       生成 2-4 个子查询
│       失败 → 降级为单路 [query]
│       SSE: {"type":"step","id":"multi_route","label":"多路检索"}
│
├─ 3. HybridRetrieve (对每个子查询并行执行)
│   │
│   ├─ 3a. VectorRetrieve
│   │   └─ embedder.Embed(ctx, subQuery) → embedding
│   │       vectorRetriever.Retrieve(ctx, subQuery, kbID, topK*2)
│   │       → pgvector.CosineSearch(embedding, topK*2)
│   │   SSE: {"type":"step","id":"vector_retrieval","label":"向量检索"}
│   │
│   ├─ 3b. BM25Retrieve (opts.Hybrid == true)
│   │   └─ bm25Retriever.Retrieve(ctx, subQuery, kbID, topK*2)
│   │       → 懒加载 BM25 倒排索引 → Okapi 计分
│   │   SSE: {"type":"step","id":"bm25_retrieval","label":"BM25检索"}
│   │
│   └─ 3c. HybridFuse (opts.Hybrid == true && 两路都有结果)
│       └─ RRF(向量结果, BM25结果, k=60)
│       多路结果 → 去重合并（按 chunk_id）
│       SSE: {"type":"step","id":"hybrid_fuse","label":"结果融合"}
│
├─ 4. Rerank (opts.Rerank == true)
│   └─ llmClient.ChatCompletion(ctx, rerankPrompt(query, candidates))
│       对最多 rerankCount 个候选重新评分排序
│       失败 → 使用 RRF 排序结果
│       SSE: {"type":"step","id":"rerank","label":"重排序"}
│
└─ 5. 返回 top_k 个分块 + PipelineMetrics
```

#### 2.1.4 RRF 融合算法

```go
// HybridFuse 使用 Reciprocal Rank Fusion 融合两路检索结果。
//
// 算法：RRF_score(d) = Σ(1 / (k + rank_i(d)))
// 其中 k=60（来自论文推荐值，减少高排名项的权重优势）
//
// 为什么用 RRF 而非线性加权：
// 向量相似度和 BM25 分数的值域不同（cosine ∈ [0,1], BM25 无上界），
// 直接线性加权需要归一化，而 RRF 只依赖排名顺序，天然解决了值域差异。
func HybridFuse(
    vectorResults []RetrievalResult,
    bm25Results   []RetrievalResult,
    k             int,     // 默认 60
    topK          int,
) []RetrievalResult
```

### 2.2 适配层 (`server/internal/adapter/`)

#### 2.2.1 LLMClient

```go
// =============================================================================
// LLMClient — LLM 调用接口
// =============================================================================
// 设计决策（ADR-V2-002）：
// ChatCompletion 和 ChatCompletionStream 是两个独立方法，不通过参数切换。
// 调用方在编译时就知道自己需要流式还是非流式，分离方法比运行时判断更清晰。

// LLMClient 定义 LLM 调用接口（OpenAI-compatible 协议）。
type LLMClient interface {
    // ChatCompletion 同步对话 — 用于查询改写、多路路由、重排序等非流式场景。
    ChatCompletion(ctx context.Context, req ChatRequest) (*ChatResponse, error)

    // ChatCompletionStream 流式对话 — 用于对用户的 SSE 实时回答。
    // 返回 channel 逐 token 输出，调用方通过 range channel 消费。
    ChatCompletionStream(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error)
}

// ChatRequest 对话请求。
type ChatRequest struct {
    Model       string        `json:"model"`
    Messages    []ChatMessage `json:"messages"`
    MaxTokens   int           `json:"max_tokens,omitempty"`
    Temperature float64       `json:"temperature,omitempty"` // 检索类任务 0.0，生成类 0.7
}

// ChatResponse 同步对话响应。
type ChatResponse struct {
    Content     string `json:"content"`      // 完整回复文本
    TokensUsed  int    `json:"tokens_used"`
    FinishReason string `json:"finish_reason"`
}

// StreamChunk SSE 流式的单个 token 块。
type StreamChunk struct {
    Content      string `json:"content"`       // token 文本
    FinishReason string `json:"finish_reason"` // "stop" | "length" | ""（空表示未结束）
    Error        error  `json:"-"`             // 流式传输错误（channel 关闭前发送）
}

// ChatMessage 对话消息。
type ChatMessage struct {
    Role    string `json:"role"`    // "system" | "user" | "assistant"
    Content string `json:"content"`
}

// =============================================================================
// OpenAIClient — LLMClient 的通用实现
// =============================================================================
// 通过 BaseURL + API Key 调用任意 OpenAI-compatible API：
//   - llama.cpp server    → http://llama-cpp:8080/v1
//   - OpenAI              → https://api.openai.com/v1
//   - DeepSeek / Moonshot → 各服务商地址

// OpenAIClient 实现 LLMClient，对接 OpenAI-compatible API。
type OpenAIClient struct {
    baseURL    string
    apiKey     string
    httpClient *http.Client
}

func NewOpenAIClient(baseURL, apiKey string, timeout time.Duration) *OpenAIClient

// ChatCompletionStream 流式解析实现：
//   1. POST {baseURL}/chat/completions，body 中 stream: true
//   2. 读取 resp.Body 的 SSE 流（bufio.Scanner）
//   3. 解析 "data: {...}" 行，提取 choices[0].delta.content
//   4. 通过 channel 逐 token 发送
//   5. 检测 ctx.Done() → 取消请求
//   6. 收到 "[DONE]" → close channel
```

#### 2.2.2 EmbeddingClient

```go
// EmbeddingClient 定义 Embedding 生成接口（OpenAI-compatible 协议）。
type EmbeddingClient interface {
    // CreateEmbeddings 生成文本向量。
    // 支持批量输入，一次调用传入多个文本，减少 API 往返。
    CreateEmbeddings(ctx context.Context, req EmbeddingRequest) (*EmbeddingResponse, error)
}

type EmbeddingRequest struct {
    Model string   `json:"model"`  // embedding 模型名称，如 "bge-m3"
    Input []string `json:"input"`  // 待向量化的文本列表（批量）
}

type EmbeddingResponse struct {
    Embeddings [][]float32 `json:"embeddings"` // 每个 input 对应一个向量
    Dimension  int         `json:"dimension"`  // 向量维度
    TokensUsed int         `json:"tokens_used"`
}
```

#### 2.2.3 VectorStore

```go
// =============================================================================
// VectorStore — pgvector 向量存储接口
// =============================================================================
// 设计决策（ADR-V2-003）：
// 接口明确暴露 pgvector 概念（halfvec、cosine），不追求数据库无关的抽象。
// 原因：pgvector 的 halfvec 类型、HNSW 索引、<=> 算子都是不可替代的能力，
// 过度抽象反而限制了性能优化空间。如果未来更换向量库，直接重写实现即可。

// VectorStore 定义 pgvector 向量存储接口。
type VectorStore interface {
    // BatchInsert 批量写入分块向量。
    // 使用 pgvector halfvec 类型存储 embedding（float32 → float16 精度），
    // 存储空间减半，精度损失对 cosine 排序影响 < 0.1%。
    //
    // SQL: INSERT INTO knowledge_chunks (article_id, kb_id, content,
    //      chunk_index, embedding, embedding_model, vector_dimension)
    //      VALUES ($1, $2, $3, $4, $5::halfvec, $6, $7)
    BatchInsert(ctx context.Context, chunks []VectorChunk) error

    // CosineSearch 余弦相似度检索。
    // 使用 pgvector <=> 算子计算余弦距离，转换为相似度分数。
    //
    // SQL: SELECT id, article_id, content, chunk_index,
    //           1 - (embedding <=> $1::halfvec) AS score
    //      FROM knowledge_chunks
    //      WHERE kb_id = $2
    //      ORDER BY embedding <=> $1::halfvec
    //      LIMIT $3
    CosineSearch(ctx context.Context, kbID int64, embedding []float32, topK int) ([]SearchResult, error)

    // DeleteByArticle 删除指定文章的所有向量分块。
    DeleteByArticle(ctx context.Context, articleID int64) error

    // DeleteByKB 删除指定知识库的所有向量分块。
    DeleteByKB(ctx context.Context, kbID int64) error

    // CountByKB 统计知识库的分块总数（用于 BM25 索引规模判断）。
    CountByKB(ctx context.Context, kbID int64) (int64, error)

    // GetChunksByArticle 获取指定文章的所有分块内容（不含向量，用于重索引）。
    GetChunksByArticle(ctx context.Context, articleID int64) ([]ChunkContent, error)
}

// VectorChunk 待写入的向量分块。
type VectorChunk struct {
    ArticleID       int64     `json:"article_id"`
    KBID            int64     `json:"kb_id"`
    Content         string    `json:"content"`
    ChunkIndex      int       `json:"chunk_index"`
    Embedding       []float32 `json:"embedding"`        // float32 切片，写入时转为 halfvec
    EmbeddingModel  string    `json:"embedding_model"`
    VectorDimension int       `json:"vector_dimension"`
}

// SearchResult 向量检索结果。
type SearchResult struct {
    ChunkID    int64   `json:"chunk_id"`
    ArticleID  int64   `json:"article_id"`
    Content    string  `json:"content"`
    ChunkIndex int     `json:"chunk_index"`
    Score      float64 `json:"score"` // 余弦相似度 [0, 1]
}

// ChunkContent 分块内容（不含向量）。
type ChunkContent struct {
    ID         int64  `json:"id"`
    Content    string `json:"content"`
    ChunkIndex int    `json:"chunk_index"`
}
```

### 2.3 BM25 倒排索引

```go
// =============================================================================
// BM25Index — Go 原生 BM25 倒排索引
// =============================================================================
// 设计决策（ADR-V2-004）：
// 懒加载 + TTL 过期策略。
// - 首次 BM25 查询时从 knowledge_chunks 表加载所有分块并构建倒排索引
// - 索引缓存到内存，t.tll（默认 30 分钟）无查询后自动释放
// - 知识库有新文章发布/停用时，主动失效该 kb 的索引缓存
// 原因：避免启动时全量加载所有知识库的索引（大知识库启动慢），
// 同时避免每次查询都重新构建（查询延迟不可接受）。

// BM25Index 单个知识库的 BM25 倒排索引。
// 非线程安全，由 BM25Retriever 的 sync.RWMutex 保护。
type BM25Index struct {
    kbID          int64
    chunks        []bm25Document              // 所有分块
    avgDL         float64                     // 平均文档长度（token 数）
    invertedIndex map[string][]bm25Posting    // token → 倒排列表
    k1            float64                     // 词频饱和参数，默认 1.5
    b             float64                     // 长度归一化参数，默认 0.75
    lastUsed      time.Time                   // 最后使用时间
    ttl           time.Duration               // 空闲过期时间，默认 30 分钟
}

type bm25Document struct {
    chunkID int64
    tokens  []string
    length  int
}

type bm25Posting struct {
    chunkID    int64
    termFreq   int     // 词在该文档中的出现次数
}

// BM25Retriever 实现 Retriever 接口，管理多个知识库的 BM25Index。
type BM25Retriever struct {
    mu       sync.RWMutex
    indexes  map[int64]*BM25Index  // kbID → 索引
    db       *gorm.DB              // 用于懒加载分块
    seg      Segmenter             // 中文分词器（gse）
    ttl      time.Duration
    stopChan chan struct{}         // 停止后台清理 goroutine
}

// 中文分词器接口（适配 gse 或其他分词库）。
type Segmenter interface {
    Segment(text string) []string
}

// Retrieve 实现 Retriever 接口。
func (r *BM25Retriever) Retrieve(ctx context.Context, query string, kbID int64, topK int) ([]RetrievalResult, error) {
    // 1. 获取或懒加载索引
    // 2. 中文分词 → tokens
    // 3. 对每个分块计算 BM25 分数
    // 4. 按分数降序排列，截取 topK
}

// Invalidate 失效指定知识库的索引缓存（发布/停用文章时调用）。
func (r *BM25Retriever) Invalidate(kbID int64) {
    r.mu.Lock()
    defer r.mu.Unlock()
    delete(r.indexes, kbID)
}

// startCleanup 后台 goroutine，每 5 分钟检查并释放过期索引。
func (r *BM25Retriever) startCleanup() {
    ticker := time.NewTicker(5 * time.Minute)
    go func() {
        for {
            select {
            case <-ticker.C:
                r.cleanupExpired()
            case <-r.stopChan:
                ticker.Stop()
                return
            }
        }
    }()
}
```

### 2.4 文档处理管线

```go
// =============================================================================
// Processor — 文档异步处理管线
// =============================================================================
// 并发模型：
// 使用 buffered channel 作为任务队列 + goroutine pool（大小=CPU 核数）并行处理。
// 每个文档的处理步骤：下载 MinIO → 解析文本 → 分块 → embedding → 写入 pgvector。
// 处理进度通过 knowledge_articles.process_status 持久化，前端轮询查询。

// Processor 文档处理器。
type Processor struct {
    poolSize int              // goroutine 数 = CPU 核数
    taskCh   chan processTask // 任务队列（buffered，容量 100）
    parser   *DocParser
    chunker  *Chunker
    embedder *Embedder
    store    VectorStore
    repo     *repository.KnowledgeRepo
}

type processTask struct {
    ArticleID int64
    KBID      int64
    MinioPath string
    FileType  string // pdf / docx / md / txt
}

// Submit 提交文档处理任务。
// 不阻塞——任务入队后立即返回，由 goroutine pool 异步执行。
func (p *Processor) Submit(task processTask)

// Stop 优雅关闭：等待当前处理中的任务完成，不再接受新任务。
func (p *Processor) Stop()
```

```go
// =============================================================================
// DocParser — 多格式文档解析
// =============================================================================

// DocParser 文档解析器。
// 支持 PDF（仅文本提取）、DOCX、Markdown、纯文本四种格式。
type DocParser struct{}

// Parse 根据文件扩展名选择解析器，返回提取的纯文本。
//
// PDF 解析：使用 ledongthuc/pdf 库，逐页提取文本。
// 为什么不用 pdfcpu（功能更全）：pdfcpu 侧重 PDF 创建和修改，
// ledongthuc/pdf 专注于文本提取，API 更简洁，适合当前仅文本提取的需求。
//
// DOCX 解析：基于 archive/zip + encoding/xml 标准库实现。
// DOCX 本质是 ZIP 中的 Office Open XML，标准库即可完整处理。
// 为什么不引入第三方 DOCX 库：减少依赖，DOCX 文本提取逻辑简单。
func (p *DocParser) Parse(reader io.Reader, fileType string) (string, error)
```

```go
// =============================================================================
// Chunker — RecursiveCharacterTextSplitter
// =============================================================================

// Chunker 文本分块器。
//
// 算法：RecursiveCharacterTextSplitter（递归字符分割）。
// 优先级分隔符：\n\n → \n → 。 → . → 空格 → 字符级
// 按优先级尝试分割，若分割后仍超过 chunkSize，降级到下一级分隔符。
//
// chunkSize 默认 1000 字符，chunkOverlap 默认 200 字符。
// 这两个参数对应 rag-engine 的 langchain RecursiveCharacterTextSplitter 行为。
type Chunker struct {
    ChunkSize    int // 每块最大字符数，默认 1000
    ChunkOverlap int // 块间重叠字符数，默认 200
}

// Split 将文本分割为多个重叠的块。
func (c *Chunker) Split(text string) []string
```

```go
// =============================================================================
// Embedder — Embedding 生成器
// =============================================================================

// Embedder 封装 EmbeddingClient，提供批处理能力。
//
// 为什么需要 Embedder 而非直接使用 EmbeddingClient：
// 分块数可能很大（长文档数百块），需要分批调用 API（每批 20 条），
// 同时处理速率限制和重试逻辑。这些关注点不应该由调用方处理。
type Embedder struct {
    client    EmbeddingClient
    batchSize int // 每批发送的文本数，默认 20
    model     string
}

// Embed 批量生成分块的向量。
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error)
```

### 2.5 LLM 配置管理

```go
// =============================================================================
// LLMConfigManager — 配置热替换
// =============================================================================
// 设计决策（ADR-V2-005）：
// 使用 atomic.Value 实现配置热替换。
// - GET 操作无锁（atomic.Load），高频调用零开销
// - PUT 操作更新 DB + atomic.Store，瞬时生效
// - 不每次查询 DB：LLM 调用是高频操作，每次查询 DB 会引入 1-5ms 延迟

// LLMConfigManager 管理 LLM/Embedding 运行时配置。
type LLMConfigManager struct {
    current atomic.Value // *LLMConfig，零锁读取
    mu      sync.Mutex   // 写操作互斥锁
    db      *gorm.DB
}

// LLMConfig 运行时 LLM 配置。
type LLMConfig struct {
    LLMBaseURL       string `json:"llm_base_url"`
    LLMAPIKey        string `json:"llm_api_key"`
    LLMModel         string `json:"llm_model"`
    LLMMaxTokens     int    `json:"llm_max_tokens"`
    EmbeddingModel   string `json:"embedding_model"`
    EmbeddingDimension int  `json:"embedding_dimension"`
    TopK             int    `json:"top_k"`              // 默认 5
    ConfidenceThreshold float64 `json:"confidence_threshold"` // 默认 0.6
}

// GetConfig 获取当前配置（零锁，高频安全）。
func (m *LLMConfigManager) GetConfig() *LLMConfig {
    return m.current.Load().(*LLMConfig)
}

// UpdateConfig 更新配置（写 DB + 替换内存值）。
// 仅在配置实际变更时更新，避免无效的 DB 写入。
func (m *LLMConfigManager) UpdateConfig(cfg *LLMConfig) error
```

---

## 3. 数据库设计

### 3.1 变更总览

v2 数据库变更涉及 3 张修改表 + 1 张新表：

| 表 | 变更类型 | 说明 |
| --- | --- | --- |
| `knowledge_bases` | 修改 | 删除 `rag_workspace_slug`，新增 `llm_config_id` |
| `knowledge_articles` | 修改 | 统一文章模型 — 删除 `question`/`rag_document_location`，`answer`→`content`，新增 7 个字段 |
| `knowledge_chunks` | 修改 | 新增 `embedding halfvec`、`kb_id`、`chunk_index`，删除 `sync_*` 三个字段 |
| `llm_configs` | **新增** | LLM/Embedding 提供商配置表 |

### 3.2 DDL

```sql
-- =============================================================================
-- pgvector 扩展
-- =============================================================================
CREATE EXTENSION IF NOT EXISTS vector;

-- =============================================================================
-- 1. knowledge_bases 表变更
-- =============================================================================
ALTER TABLE knowledge_bases DROP COLUMN IF EXISTS rag_workspace_slug;
ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS llm_config_id bigint;

-- =============================================================================
-- 2. knowledge_articles 表变更（统一文章模型）
-- =============================================================================
-- 重命名 answer → content（保留已有数据）
ALTER TABLE knowledge_articles RENAME COLUMN answer TO content;

-- 删除 AnythingLLM 相关字段
ALTER TABLE knowledge_articles DROP COLUMN IF EXISTS question;
ALTER TABLE knowledge_articles DROP COLUMN IF EXISTS rag_document_location;

-- 新增字段
ALTER TABLE knowledge_articles ADD COLUMN IF NOT EXISTS title varchar(255) NOT NULL DEFAULT '';
ALTER TABLE knowledge_articles ADD COLUMN IF NOT EXISTS source_type smallint NOT NULL DEFAULT 1;
ALTER TABLE knowledge_articles ADD COLUMN IF NOT EXISTS word_count integer NOT NULL DEFAULT 0;
ALTER TABLE knowledge_articles ADD COLUMN IF NOT EXISTS chunk_count integer NOT NULL DEFAULT 0;
ALTER TABLE knowledge_articles ADD COLUMN IF NOT EXISTS file_type varchar(16);
ALTER TABLE knowledge_articles ADD COLUMN IF NOT EXISTS minio_path varchar(512);
ALTER TABLE knowledge_articles ADD COLUMN IF NOT EXISTS process_status varchar(16) NOT NULL DEFAULT 'completed';
ALTER TABLE knowledge_articles ADD COLUMN IF NOT EXISTS process_error text;

-- 为已有数据填充 title（用 content 的前 50 字符）
UPDATE knowledge_articles SET title = LEFT(COALESCE(content, ''), 50) WHERE title = '';
-- 已有数据为手动创建
UPDATE knowledge_articles SET source_type = 1 WHERE source_type = 1;
-- 已有数据视为处理完成
UPDATE knowledge_articles SET process_status = 'completed' WHERE process_status = 'pending';

COMMENT ON COLUMN knowledge_articles.source_type IS '1=manual(手动输入), 2=upload(文档上传)';
COMMENT ON COLUMN knowledge_articles.process_status IS 'pending/parsing/chunking/embedding/completed/failed';
COMMENT ON COLUMN knowledge_articles.file_type IS 'pdf/docx/md/txt，仅 source_type=upload 时有值';

-- =============================================================================
-- 3. knowledge_chunks 表变更（pgvector 向量存储）
-- =============================================================================
-- 删除 AnythingLLM 同步状态字段
ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS sync_status;
ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS sync_error;
ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS synced_at;

-- 新增向量和索引字段
ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS kb_id bigint NOT NULL DEFAULT 0;
ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS chunk_index integer NOT NULL DEFAULT 0;

-- 向量列：使用 halfvec（半精度 float16），维度由知识库配置决定
-- 注意：pgvector 不支持 ALTER TABLE ADD COLUMN 时指定 halfvec 维度，
-- 需通过 application 层迁移脚本动态构造 SQL（维度从 knowledge_bases.vector_dimension 读取）
-- 示例（维度 1024）：
-- ALTER TABLE knowledge_chunks ADD COLUMN embedding halfvec(1024);

-- HNSW 向量索引（硬编码，用户不可见）
-- 为什么用 HNSW 而非 IVFFlat：
-- HNSW 查询速度更快（10000 规模 < 50ms vs IVFFlat ~100ms），
-- 且无需定期 REINDEX（IVFFlat 在大量写入后需要重建）。
-- 为什么用 halfvec_cosine_ops：cosine 距离是语义相似度的标准度量。
-- CREATE INDEX idx_chunks_embedding ON knowledge_chunks
--     USING hnsw (embedding halfvec_cosine_ops)
--     WITH (m = 16, ef_construction = 200);

COMMENT ON COLUMN knowledge_chunks.embedding IS 'halfvec 类型，半精度向量（float16），维度由知识库配置决定';
COMMENT ON COLUMN knowledge_chunks.kb_id IS '冗余字段：加速按知识库的检索过滤，避免 JOIN knowledge_articles';

-- =============================================================================
-- 4. llm_configs 表（新增）
-- =============================================================================
CREATE TABLE IF NOT EXISTS llm_configs (
    id                bigint PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    name              varchar(128) NOT NULL,
    provider_type     smallint NOT NULL DEFAULT 1,
    base_url          varchar(512) NOT NULL,
    api_key           varchar(512),
    llm_model         varchar(128) NOT NULL,
    embedding_model   varchar(128) NOT NULL,
    max_tokens        integer NOT NULL DEFAULT 8192,
    vector_dimension  integer NOT NULL DEFAULT 1024,
    is_default        boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

COMMENT ON COLUMN llm_configs.provider_type IS '1=llama.cpp, 2=OpenAI-compatible';
COMMENT ON COLUMN llm_configs.api_key IS 'API 密钥（AES-256 加密存储）；llama.cpp 本地部署可为空';
COMMENT ON COLUMN llm_configs.vector_dimension IS 'embedding 向量维度（如 bge-m3=1024, text-embedding-3-small=1536）';

-- 最多一个默认配置
CREATE UNIQUE INDEX idx_llm_configs_default ON llm_configs (is_default) WHERE is_default = true;
```

### 3.3 索引策略

```sql
-- knowledge_chunks 表索引（向量检索 + 业务查询）
-- HNSW 向量索引（application 层动态创建，DDL 维度不可硬编码）
-- CREATE INDEX idx_chunks_embedding ON knowledge_chunks
--     USING hnsw (embedding halfvec_cosine_ops)
--     WITH (m = 16, ef_construction = 200);

-- 按知识库过滤检索（配合向量索引使用）
CREATE INDEX IF NOT EXISTS idx_chunks_kb_id ON knowledge_chunks (kb_id);

-- 按文章删除向量
CREATE INDEX IF NOT EXISTS idx_chunks_article_id ON knowledge_chunks (article_id);

-- knowledge_articles 表索引
CREATE INDEX IF NOT EXISTS idx_articles_kb_id ON knowledge_articles (kb_id);
CREATE INDEX IF NOT EXISTS idx_articles_status ON knowledge_articles (status);
CREATE INDEX IF NOT EXISTS idx_articles_process_status ON knowledge_articles (process_status);
```

### 3.4 v1 → v2 数据迁移

```sql
-- =============================================================================
-- 迁移脚本：将 v1 FAQ 数据转换为 v2 统一文章模型
-- =============================================================================

-- 1. 为已有 knowledge_articles 填充 title（取原有 question 字段值，若已被删除则取 content 前 50 字）
-- （此步骤在字段变更前执行，由 application 迁移脚本处理）

-- 2. 填充 knowledge_chunks.kb_id（从未直接存储，需通过 article → kb 关联）
UPDATE knowledge_chunks c
SET kb_id = a.kb_id
FROM knowledge_articles a
WHERE c.article_id = a.id AND c.kb_id = 0;

-- 3. 为已有 chunks 填充 chunk_index（按 article_id 分组递增）
-- （此步骤在 application 层执行，纯 SQL 的窗口函数也可实现）
UPDATE knowledge_chunks c
SET chunk_index = sub.rn - 1
FROM (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY article_id ORDER BY id) AS rn
    FROM knowledge_chunks
) sub
WHERE c.id = sub.id AND c.chunk_index = 0;
```

---

## 4. API 设计

### 4.1 通用约定

与 v1 保持一致：
- Content-Type: `application/json`
- 认证: `Authorization: Bearer <access_token>`
- 分页: `?page=1&page_size=20`
- 响应格式: `{"code": 0, "message": "success", "data": {}}`

### 4.2 新增 API

#### POST `/api/v1/admin/knowledge-bases/:kb_id/documents/upload`

文档上传 API。接收 multipart/form-data，异步处理。

**请求：**
```
Content-Type: multipart/form-data
files: [file1.pdf, file2.docx, ...]  (多文件，单文件 ≤ 20MB)
```

**响应（HTTP 200）：**
```json
{
  "code": 0,
  "data": {
    "documents": [
      {
        "article_id": 101,
        "file_name": "账号管理FAQ.pdf",
        "file_size": 524288,
        "file_type": "pdf",
        "process_status": "pending"
      },
      {
        "article_id": 102,
        "file_name": "网络故障排查.docx",
        "file_size": 131072,
        "file_type": "docx",
        "process_status": "pending"
      }
    ]
  }
}
```

**错误响应：**
```json
{
  "code": 10003,
  "message": "不支持的文件格式: exe（仅支持 PDF、DOCX、MD、TXT）"
}
```

#### GET `/api/v1/admin/knowledge-bases/:kb_id/documents/:id/status`

查询文档处理状态。

**响应：**
```json
{
  "code": 0,
  "data": {
    "article_id": 101,
    "process_status": "embedding",
    "process_error": null,
    "progress": {
      "stage": "embedding",
      "current": 15,
      "total": 20
    }
  }
}
```

#### POST `/api/v1/admin/knowledge-bases/:kb_id/documents/:id/retry`

重试失败的文档处理。

**响应：**
```json
{
  "code": 0,
  "message": "已重新加入处理队列"
}
```

### 4.3 修改 API

#### POST `/api/v1/portal/chat-sessions/stream` — SSE 流式问答（升级）

**请求（不变）：**
```json
{
  "question": "账号冻结怎么处理",
  "kb_id": 1,
  "rag_options": {
    "top_k": 5,
    "query_rewrite": true,
    "multi_route": true,
    "hybrid": true,
    "rerank": true
  }
}
```

**响应（SSE 事件流，格式与 v1 前端兼容）：**
```
data: {"type":"step","id":"query_rewrite","label":"查询改写"}

data: {"type":"step","id":"multi_route","label":"多路检索"}

data: {"type":"step","id":"vector_retrieval","label":"向量检索"}

data: {"type":"step","id":"bm25_retrieval","label":"BM25检索"}

data: {"type":"step","id":"hybrid_fuse","label":"结果融合"}

data: {"type":"step","id":"rerank","label":"重排序"}

data: {"type":"step","id":"llm_generate","label":"LLM 生成"}

data: {"type":"token","content":"账号冻结"}

data: {"type":"token","content":"的处理步骤"}

data: {"type":"token","content":"如下：\n\n1."}

...

data: {"type":"done","metadata":{"session_id":12345,"answer":"账号冻结的处理步骤...","sources":[...],"confidence":0.85,"pipeline":{"steps":[...],"total_duration_ms":3200}}}
```

**v1 → v2 变更点：**
- `token` 事件从「模拟分块（每次 5 个 rune）」变为「真正的 LLM token 级流式」（LLM 输出什么就转发什么）
- 新增 `step` 事件类型，前端可选展示管道进度
- `done` 事件的 `metadata` 中新增 `pipeline` 字段（管道耗时明细）
- v1 前端 `web/src/api/chat.ts` 的 `streamChatSession` 无需改动即可兼容

### 4.4 LLM 配置 API

#### GET `/api/v1/admin/llm-configs`

获取 LLM 配置列表。

#### POST `/api/v1/admin/llm-configs`

创建 LLM 配置。

**请求：**
```json
{
  "name": "本地 llama.cpp",
  "provider_type": 1,
  "base_url": "http://llama-cpp:8080/v1",
  "api_key": "",
  "llm_model": "qwen3-4b",
  "embedding_model": "bge-m3",
  "max_tokens": 8192,
  "vector_dimension": 1024,
  "is_default": true
}
```

#### PUT `/api/v1/admin/llm-configs/:id`

更新 LLM 配置。更新 `is_default=true` 的配置时，自动将旧默认配置设为 `false`。

#### DELETE `/api/v1/admin/llm-configs/:id`

删除 LLM 配置。不允许删除当前正在使用的默认配置。

#### POST `/api/v1/admin/llm-configs/:id/test`

测试 LLM 连接。发送一个简短的 ChatCompletion 请求验证 Base URL 可达性和模型可用性。

**响应：**
```json
{
  "code": 0,
  "data": {
    "success": true,
    "latency_ms": 120,
    "model": "qwen3-4b"
  }
}
```

### 4.5 不变 API

以下 v1 API 完全不变（仅内部实现切换到新 RAG 引擎）：

| 路径 | 说明 |
| --- | --- |
| `POST /api/v1/portal/chat-sessions` | 同步问答（非流式） |
| `POST /api/v1/portal/chat-sessions/:id/feedback` | 问答反馈 |
| `GET /api/v1/portal/chat-sessions/:id` | 问答详情 |
| `GET/POST /api/v1/admin/knowledge-bases` | 知识库 CRUD |
| `POST /api/v1/admin/knowledge-bases/:kb_id/articles` | 创建文章（请求体变，路径不变） |
| `PUT /api/v1/admin/articles/:id` | 更新文章 |
| `POST /api/v1/admin/articles/:id/submit-review` | 提交审核 |
| `POST /api/v1/admin/articles/:id/review` | 审核知识 |
| `POST /api/v1/admin/articles/:id/publish` | 发布（内部逻辑变） |
| `POST /api/v1/admin/articles/:id/disable` | 停用（内部逻辑变） |

---

## 5. 架构决策记录 (ADR)

### ADR-V2-001: RAG 管道采用混合架构模式

**状态：** 已接受

**背景：** PRDv2 定义的管道包含 5 种步骤 — 查询改写、多路检索、向量检索、BM25 检索、RRF 融合、重排序。其中向量检索和 BM25 检索有相同的调用模式（输入查询 + kbID → 返回排序结果），适合抽象为统一接口；查询改写和重排序各自有独特的 LLM 交互模式，不适合统一抽象。

**决策：** 检索阶段（向量检索、BM25 检索）实现统一的 `Retriever` 接口，通过 RRF 融合统一编排。查询改写、多路检索、重排序各自为独立函数，由 `Pipeline.Execute` 按固定顺序调用。

**备选方案：**
- **统一 Stage 接口（可插拔管道）：** 所有步骤实现 `Stage(ctx, input) (output, error)`，管道通过配置串联。优势是扩展性强；劣势是类型不安全（输入输出需要 `interface{}` 断言）、调试困难。
- **全硬编码编排：** 所有步骤都是 Pipeline 的私有方法。优势是简单；劣势是检索器无法复用、测试需要 mock 整个 Pipeline。

**后果：**
- 正面：Retriever 接口使向量检索和 BM25 检索可独立测试和替换；非检索步骤保持简单直观。
- 负面：新增检索类型（如混合检索中加入第 3 路）需要实现 Retriever 接口并改编排器，但这是合理的修改范围。

---

### ADR-V2-002: LLMClient 使用分离的流式/非流式方法

**状态：** 已接受

**背景：** LLM 调用有两种场景 — 同步场景（查询改写、多路路由、重排序）需要完整结果，流式场景（用户问答）需要逐 token 输出。需要决定接口的组织方式。

**决策：** 提供两个独立方法 `ChatCompletion` 和 `ChatCompletionStream`，不使用 `stream bool` 参数切换。

**备选方案：**
- **单一方法 + stream 参数：** 接口更简洁但调用方需要运行时判断返回类型（`if stream { use channel } else { use response }`），类型系统无法帮助校验。
- **仅流式接口：** 同步场景需要手动收集 channel 数据，增加不必要的代码。

**后果：**
- 正面：编译时类型安全 — 同步场景不接触 channel，流式场景不接触完整响应。
- 负面：实现层（OpenAIClient）有两套逻辑，但核心的 HTTP 请求和 SSE 解析可共享。

---

### ADR-V2-003: VectorStore 接口明确耦合 pgvector

**状态：** 已接受

**背景：** pgvector 的 `halfvec` 类型、`<=>` 余弦距离算子、HNSW 索引是核心性能优化手段。需要决定 VectorStore 接口的抽象层级。

**决策：** 接口明确暴露 pgvector 概念（`BatchInsert` 使用 halfvec，`CosineSearch` 使用 cosine 距离），不追求数据库无关抽象。

**备选方案：**
- **数据库无关接口：** `Search(ctx, query, topK)` 不指定距离度量，由实现决定。优势是可替换其他向量库；劣势是 pgvector 的 cosine/halfvec/HNSW 优化无法在接口中表达。
- **分层接口：** 高层通用 + 底层 pgvector 特定。优势是分层清晰；劣势是增加抽象层级，当前只有一个实现，过度设计。

**后果：**
- 正面：充分利用 pgvector 性能特性，接口语义明确。
- 负面：未来替换向量库（如 Qdrant）需要重写实现和接口。但考虑到 pgvector 是本项目唯一规划中的向量库，且替换成本远低于维护抽象层的成本，此权衡可接受。

---

### ADR-V2-004: BM25 索引采用懒加载 + TTL 过期

**状态：** 已接受

**背景：** BM25 需要为每个知识库构建倒排索引（map[token][]posting），需要决定索引的构建时机和生命周期。

**决策：** 首次 BM25 查询时懒加载构建索引，30 分钟无查询后自动释放；知识库有新文章发布/停用时主动失效缓存。

**备选方案：**
- **启动时全量构建：** 每次查询零延迟，但大知识库启动慢且占用常驻内存。
- **每次查询重新构建：** 零内存占用，但查询延迟不可接受（10000 分块需要 ~500ms 构建索引）。
- **BoltDB 磁盘存储：** 内存友好但有 IO 开销，MVP 阶段过度设计。

**后果：**
- 正面：平衡了查询性能（热知识库索引常驻）和内存占用（冷知识库自动释放）。
- 负面：首次 BM25 查询有 100-500ms 的索引构建延迟（取决于分块数）— 在 SSE 流式场景下，此延迟在 step 事件阶段，用户感知为「BM25 检索」步骤耗时略长。

---

### ADR-V2-005: LLM 配置通过 atomic.Value 热替换

**状态：** 已接受

**背景：** PRDv2 要求 LLM/Embedding 配置修改后即时生效，无需重启服务。需要选择配置热加载方案。

**决策：** 使用 `sync/atomic.Value` 存储运行时配置，`PUT /llm-configs` 更新后 `atomic.Store` 替换。

**备选方案：**
- **每次查询 DB：** 配置始终一致但每次 LLM 调用额外 1-5ms DB 延迟。
- **定期刷新（如每 30 秒）：** 平衡方案，但配置变更后最多 30 秒延迟生效。

**后果：**
- 正面：GET 操作零锁开销（适合 LLM 调用的高频场景），配置修改瞬时生效。
- 负面：多实例部署时各实例独立更新，可能出现短暂不一致（单实例部署无此问题）。

---

### ADR-V2-006: 迁移策略 — Big Bang 一次性切换

**状态：** 已接受

**背景：** v2 涉及 `RagClient` 接口删除、KnowledgeArticle 模型变更、knowledge_chunks 表结构变更。新旧模型差异大（FAQ vs 统一文章），渐进迁移的兼容成本高。

**决策：** 新 RAG 模块开发完成后，一次性删除所有 v1 RAG 相关代码，编写数据迁移脚本将 v1 FAQ 数据转为 v2 文章模型。

**备选方案：**
- **双写 + 灰度切换：** 新老系统并行运行。优势是风险可控；劣势是两个 RAG 引擎共存时的数据一致性极难保证，且代码库复杂度激增。
- **特性开关：** 在代码中通过 `if v2Enabled` 分支。优势是可运行时回滚；劣势是代码充满 if-else，测试矩阵翻倍。

**后果：**
- 正面：代码库干净、无遗留兼容逻辑、测试范围明确。
- 负面：切换失败只能回滚代码版本（而非运行时关闭特性开关）。缓解措施：充分的集成测试 + 数据库备份 + 迁移脚本可逆。

---

## 6. 部署架构

### 6.1 Docker Compose 服务

```yaml
services:
  # ===== 必须服务 =====
  opsmind-server:
    build: ./server
    ports: ["8080:8080"]
    environment:
      DB_HOST: postgres
      MINIO_ENDPOINT: minio:9000
      LLM_BASE_URL: ${LLM_BASE_URL:-http://llama-cpp:8080/v1}
      LLM_API_KEY: ${LLM_API_KEY:-}
      LLM_MODEL: ${LLM_MODEL:-qwen3-4b}
      LLM_MAX_TOKENS: ${LLM_MAX_TOKENS:-8192}
      EMBEDDING_MODEL: ${EMBEDDING_MODEL:-bge-m3}
      EMBEDDING_DIMENSION: ${EMBEDDING_DIMENSION:-1024}
    depends_on: [postgres, minio]
    networks: [opsmind]

  opsmind-web:
    build: ./web
    ports: ["5173:80"]
    depends_on: [opsmind-server]
    networks: [opsmind]

  postgres:
    image: pgvector/pgvector:pg18
    environment:
      POSTGRES_DB: opsmind
      POSTGRES_USER: opsmind
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-opsmind_dev}
    ports: ["5432:5432"]
    volumes: [postgres_data:/var/lib/postgresql/data]
    networks: [opsmind]

  minio:
    image: minio/minio:latest
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: ${MINIO_ROOT_USER:-minioadmin}
      MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD:-minioadmin}
    ports: ["9000:9000", "9001:9001"]
    volumes: [minio_data:/data]
    networks: [opsmind]

  # ===== 可选服务（profile: ai-local）=====
  llama-cpp:
    image: ghcr.io/ggerganov/llama.cpp:server
    container_name: opsmind-llama-cpp
    ports: ["8080:8080"]
    volumes:
      - ${LLAMA_MODELS_DIR:-./models}:/models:ro
    command:
      - "--model"
      - "/models/${LLM_MODEL:-qwen3-4b}.gguf"
      - "--host"
      - "0.0.0.0"
      - "--port"
      - "8080"
    networks: [opsmind]
    profiles: [ai-local]

volumes:
  postgres_data:
  minio_data:

networks:
  opsmind:
    driver: bridge
```

### 6.2 MinIO Bucket 规划

| Bucket | 用途 | 路径模式 | 生命周期 |
| --- | --- | --- | --- |
| `opsmind-knowledge` | 知识文档原件 | `kb_{kb_id}/{article_id}/{filename}` | 与文章同生命周期 |
| `opsmind-attachments` | 申告附件 | `ticket_{ticket_id}/{filename}` | 与申告同生命周期 |

### 6.3 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `POSTGRES_PASSWORD` | opsmind_dev | PostgreSQL 密码 |
| `MINIO_ROOT_USER` | minioadmin | MinIO 管理员 |
| `MINIO_ROOT_PASSWORD` | minioadmin | MinIO 密码 |
| `JWT_SECRET` | — | JWT 签名密钥（必须设置） |
| `LLM_BASE_URL` | http://llama-cpp:8080/v1 | LLM API 地址 |
| `LLM_API_KEY` | — | API 密钥（OpenAI 需要；llama.cpp 不需要） |
| `LLM_MODEL` | qwen3-4b | LLM 模型名称 |
| `LLM_MAX_TOKENS` | 8192 | 最大生成 Token |
| `EMBEDDING_MODEL` | bge-m3 | Embedding 模型名称 |
| `EMBEDDING_DIMENSION` | 1024 | 向量维度 |
| `AI_CONFIDENCE_THRESHOLD` | 0.6 | 置信度阈值 |
| `AI_DEFAULT_TOP_K` | 5 | 默认检索数量 |
| `LLAMA_MODELS_DIR` | ./models | llama.cpp 模型目录 |

---

## 7. 错误处理与降级

### 7.1 RAG 管道降级策略

RAG 管道各步骤的失败不阻塞后续步骤，降级策略如下：

| 步骤 | 失败行为 | 降级结果 | 日志级别 |
| --- | --- | --- | --- |
| QueryRewrite | 降级 | 使用原始 query | WARN |
| MultiRoute | 降级 | 使用单路 `[query]` | WARN |
| VectorRetrieve | **阻塞** | 返回错误（向量检索是核心路径，不可降级） | ERROR |
| BM25Retrieve | 降级 | BM25 结果为空，仅用向量结果 | WARN |
| HybridFuse | 降级 | 仅用向量结果（若 BM25 失败）或仅用 BM25 结果（若向量失败） | INFO |
| Rerank | 降级 | 使用 RRF 融合排序结果 | WARN |

### 7.2 LLM 调用错误处理

```
LLM 调用
  ├─ 网络超时（context.DeadlineExceeded）
  │   └─ 重试 1 次（间隔 1s）→ 仍失败 → 返回错误
  ├─ HTTP 4xx（401/403/429）
  │   └─ 不重试 → 返回错误 + 记录详细日志
  ├─ HTTP 5xx（500/502/503）
  │   └─ 重试 1 次（间隔 2s）→ 仍失败 → 返回错误
  └─ SSE 流式中途断开
      └─ 结束流式，发送 error 事件，前端展示已生成的内容 + 错误提示
```

### 7.3 pgvector 错误处理

```go
// pgvector 连接错误 → 返回 code=20002（RAG 服务不可用）
// 向量写入失败 → article.process_status = "failed"，记录 process_error
// 向量检索超时 → 返回错误，不降级（向量检索是不可或缺的核心路径）
```

### 7.4 Embedding 批处理重试

```go
// 每批 20 条文本调用 Embedding API
// 单批失败 → 重试 1 次 → 仍失败 → 将这批标记为失败
// 部分批次失败的文档 → process_status = "failed"，process_error 记录失败范围
// 已成功的批次不回滚（已写入 pgvector 的向量保留，由重试流程清理）
```

---

## 8. 测试策略

### 8.1 单元测试

| 模块 | 测试文件 | 覆盖内容 |
| --- | --- | --- |
| `rag/chunker` | `chunker_test.go` | 递归分割逻辑、中英文分块、overlap 正确性、边界情况（空文本、单字符、恰好等于 chunkSize） |
| `rag/bm25` | `bm25_test.go` | Okapi 分数计算、倒排索引构建、中文分词、多知识库隔离、索引失效 |
| `rag/hybrid` | `hybrid_test.go` | RRF 融合排序、相同文档去重、不同 k 值影响、空结果处理 |
| `rag/document_parser` | `document_parser_test.go` | PDF/DOCX/MD/TXT 解析正确性、空文件、大文件、编码处理 |
| `adapter/llm_client` | `llm_client_test.go` | HTTP mock 测试、流式解析、错误处理、超时 |
| `adapter/vector_store` | `vector_store_test.go` | 需要 pgvector 实例（integration tag） |
| `rag/pipeline` | `pipeline_test.go` | mock Retriever + mock LLMClient，验证编排逻辑和降级行为 |
| `service/chat_service` | `chat_service_test.go` | mock Pipeline，验证会话创建、SSE 流式、降级 |
| `service/knowledge_service` | `knowledge_service_test.go` | mock Processor，验证发布/停用流程 |

### 8.2 集成测试（`-tags=integration`）

| 测试 | 内容 |
| --- | --- |
| `vector_store_test.go` | 真实 pgvector：写入向量 → cosine 检索验证 → 按文章删除验证 |
| `pipeline_integration_test.go` | 真实 pgvector + mock LLMClient：完整管道端到端测试 |
| `processor_integration_test.go` | 真实 MinIO + pgvector：上传→解析→分块→embedding→检索 |

### 8.3 测试数据

- 准备 5 个测试文档（1 个 PDF、1 个 DOCX、1 个 MD、2 个 TXT），覆盖四种格式
- 准备 20 条测试 FAQ（中文），用于验证 BM25 + 向量混合检索效果
- Mock LLM 响应用于管道测试（固定返回文本，消除外部依赖）

---

## 9. 性能基准

| 指标 | 目标 | 测量条件 |
| --- | --- | --- |
| 向量检索（pgvector CosineSearch） | < 50ms | 10000 条 halfvec(1024)，HNSW 索引 |
| BM25 索引构建 | < 500ms | 10000 个分块，平均 500 字/块 |
| BM25 检索 | < 50ms | 索引已构建 |
| RRF 融合 | < 1ms | 两路各 10 个结果 |
| 文档解析 + 分块 + embedding | < 30s | 1MB PDF 文件，chunk_size=1000 |
| SSE 首 token 延迟 | < 2s | 含完整检索管道 |

---

## 10. 安全设计

(v1 的安全设计全部保留，以下为 v2 新增)

| 关注点 | 措施 |
| --- | --- |
| LLM API Key | 数据库 AES-256 加密存储（使用 `crypto/aes`），API 响应中掩码显示（`sk-****xxxx`） |
| 文档上传 | 文件大小 ≤ 20MB，格式白名单（pdf/docx/md/txt），MinIO bucket 不公开访问（预签名 URL 访问） |
| pgvector SQL 注入 | 使用参数化查询（`$1::halfvec`），不拼接 SQL |
| SSE 流式 | 检测客户端断连后立即取消 LLM 请求，防止资源泄漏 |

---

## 11. 风险与缓解

| 风险 | 影响 | 概率 | 缓解措施 |
| --- | --- | --- | --- |
| **pgvector HNSW 索引构建慢** | 首次查询延迟 > 1s | 低 | HNSW 构建在数据写入时完成，查询时不触发构建 |
| **BM25 懒加载导致首次查询延迟** | 用户感知 BM25 步骤慢 | 中 | 在 step 事件中展示进度；后续可改为发布时预构建 |
| **中文分词词典加载慢** | 首次 BM25 查询延迟 +200ms | 中 | 选用 gse（纯 Go），词典在 BM25Retriever 初始化时一次性加载 |
| **llama.cpp server 并发能力有限** | 多用户同时问答时排队 | 中 | 支持外部 OpenAI API 作为高性能降级方案 |
| **halfvec 精度损失影响检索质量** | 相似度排序微变 | 极低 | halfvec 对 cosine 排序影响 < 0.1%，学术界已验证 |
| **文档解析大文件 OOM** | 服务崩溃 | 低 | 单文件 ≤ 20MB；解析超时 60s；并发池大小 = CPU 核数 |

---

## 12. 附录：关键 Go 依赖

| 用途 | 库 | 版本 | 选择理由 |
| --- | --- | --- | --- |
| Web 框架 | `github.com/gin-gonic/gin` | v1.9+ | v1 已有，不变 |
| ORM | `gorm.io/gorm` | v1.25+ | v1 已有，不变 |
| pgvector | `github.com/pgvector/pgvector-go` | latest | pgvector 官方 Go SDK，支持 halfvec/hnsw |
| PDF 解析 | `github.com/ledongthuc/pdf` | latest | 纯 Go，专注文本提取 |
| 中文分词 | `github.com/go-ego/gse` | latest | 纯 Go 无 CGO，Docker 构建简单 |
| JWT | `github.com/golang-jwt/jwt/v5` | v5 | v1 已有，不变 |
| MinIO SDK | `github.com/minio/minio-go/v7` | v7 | v1 已有，不变 |
| 配置管理 | `github.com/spf13/viper` | v1.18+ | v1 已有，不变 |
| 并发控制 | `golang.org/x/sync` | latest | errgroup 官方扩展 |

---

## 附录 B: v1 → v2 完整变更文件清单

见 [PRDv2.md §12](PRDv2.md#12-附录v1--v2-文件变更清单)。
