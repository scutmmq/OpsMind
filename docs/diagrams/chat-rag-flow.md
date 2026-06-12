# 智能问答 RAG 管道 v2 — 函数级调用链

> 代码基准：`handler/chat.go` → `service/chat_service.go` → `rag/pipeline.go` → `adapter/llm_client.go`
> 更新于 2026-06-12 — 反映 writeSSEEvent / SetWriteDeadline / TxManager 等重构

## 1. SSE 流式问答 — 完整函数调用链

```mermaid
sequenceDiagram
    actor U as 用户
    participant CH as ChatHandler.StreamChatSession<br/>handler/chat.go:139
    participant CS as ChatService.CreateChatSession<br/>service/chat_service.go:82
    participant KR as KnowledgeRepo.FindKBByID<br/>repository/knowledge_repo.go
    participant Pipe as Pipeline.Execute<br/>rag/pipeline.go:52
    participant QR as QueryRewrite<br/>rag/query_rewrite.go
    participant MR as MultiRoute<br/>rag/multi_route.go
    participant VR as VectorStore.CosineSearch<br/>adapter/vector_store.go
    participant B5 as BM25Retriever.Retrieve<br/>rag/bm25.go
    participant HF as HybridFuse<br/>rag/hybrid.go
    participant RR as Rerank<br/>rag/rerank.go
    participant LLM as OpenAIClient.ChatCompletionStream<br/>adapter/llm_client.go:191
    participant CR as ChatRepo.Create<br/>repository/chat_repo.go
    participant DB as PostgreSQL

    U->>CH: POST /api/v1/portal/chat-sessions/stream<br/>{question, kb_id}
    CH->>CH: c.ShouldBindJSON(&CreateChatRequest)
    CH->>CH: getCurrentUserID(c) → (userID, bool)
    CH->>CH: Set SSE headers + c.Status(200)

    CH->>CS: CreateChatSession(req, userID)

    Note over CS: === 1. 参数校验 ===
    CS->>CS: strings.TrimSpace(req.Question) — 非空校验
    CS->>KR: FindKBByID(req.KBID)
    KR->>DB: SELECT FROM knowledge_bases WHERE id=?
    DB-->>KR: *KnowledgeBase

    Note over CS,Pipe: === 2. RAG 管道 (Pipeline.Execute) ===
    CS->>Pipe: Execute(ctx, question, kbID, RAGOptions{<br/>TopK, QueryRewrite, MultiRoute, Hybrid, Rerank}, nil)

    alt QueryRewrite = true
        Pipe->>QR: rewrite(ctx, question, history)
        QR->>LLM: ChatCompletion(ctx, systemPrompt + question)
        LLM-->>QR: rewrittenQuery
    end

    alt MultiRoute = true
        Pipe->>MR: route(ctx, rewrittenQuery)
        MR->>LLM: ChatCompletion(ctx, routingPrompt)
        LLM-->>MR: []subQueries (2-4个)
    end

    par 向量检索
        Pipe->>VR: CosineSearch(ctx, kbID, embedding, topK)
        VR->>DB: SELECT * FROM knowledge_chunks<br/>ORDER BY embedding <=> $1 LIMIT $2
        DB-->>VR: []SearchResult{ChunkID, Score}
    and BM25 检索 (Hybrid=true)
        Pipe->>B5: Retrieve(ctx, kbID, query)
        B5->>B5: gse 分词 → 倒排索引 → Okapi BM25(k1=1.5,b=0.75)
        B5-->>Pipe: []bm25Result
    end

    alt Hybrid = true
        Pipe->>HF: fuse(vectorResults, bm25Results)
        Note over HF: RRF(k=60): score = Σ 1/(60+rank_i)
        HF-->>Pipe: []fusedResult
    end

    alt Rerank = true
        Pipe->>RR: Rerank(ctx, question, topCandidates)
        RR->>LLM: ChatCompletion(ctx, rerankPrompt)
        LLM-->>RR: rerankOrder
    end

    Pipe-->>CS: *RAGResult{Chunks []RetrievalResult, Metrics}

    Note over CS,LLM: === 3. LLM 生成 ===
    CS->>CS: 构造 SystemPrompt + ContextBuilder (全部 chunk)
    CS->>CS: 置信度 = max(pipelineChunks[].Score)

    alt LLM 可用
        CS->>LLM: ChatCompletion(ctx, ChatRequest{Model, Messages, MaxTokens, Temperature:0.3})
        LLM-->>CS: ChatResponse{Content, FinishReason}
    else LLM 不可用
        CS->>CS: fallbackAIUnavailable + canSubmit=true
    end

    Note over CS: === 4. 保存会话 ===
    CS->>CR: Create(&ChatSession{UserID, KBID, Question, Answer, Confidence, DurationMs})
    CR->>DB: INSERT INTO chat_sessions

    CS-->>CH: *ChatSessionResponse{SessionID, Answer, Sources, Confidence}

    Note over CH: === 5. SSE 流式输出 ===
    CH->>LLM: ChatCompletionStream(ctx, streamReq)
    Note over CH: http.NewResponseController(c.Writer)

    loop 逐 token
        LLM-->>CH: StreamChunk{Content, FinishReason}
        CH->>CH: writeSSEEvent(w, sseEvent{Type:"token", Content})
        CH->>CH: flusher.Flush()
        CH->>CH: rc.SetWriteDeadline(now + 30s)
    end

    CH->>CH: json.Marshal(resp) → metadataJSON
    CH->>CH: writeSSEEvent(w, sseEvent{Type:"done"})
    CH-->>U: SSE stream complete
```

## 2. 非流式问答

```mermaid
sequenceDiagram
    actor U as 用户
    participant CH as ChatHandler.CreateChatSession<br/>handler/chat.go:49
    participant CS as ChatService.CreateChatSession<br/>service/chat_service.go:82
    participant Pipe as Pipeline.Execute
    participant LLM as OpenAIClient.ChatCompletion
    participant CR as ChatRepo.Create

    U->>CH: POST /api/v1/portal/chat-sessions
    CH->>CS: CreateChatSession(req, userID)
    CS->>Pipe: Execute(ctx, question, kbID, opts, nil)
    Pipe-->>CS: *RAGResult
    CS->>LLM: ChatCompletion(ctx, ChatRequest)
    LLM-->>CS: ChatResponse
    CS->>CR: Create(session)
    CS-->>CH: *ChatSessionResponse
    CH-->>U: 200 {code:0, data:{session_id, answer, sources, confidence}}
```

## 3. 降级矩阵

```mermaid
flowchart TD
    Start([Pipeline.Execute]) --> QR{QueryRewrite?}
    QR -->|true| QR_LLM[QueryRewrite → LLMClient.ChatCompletion]
    QR -->|false| MR
    QR_LLM -->|OK| MR{MultiRoute?}
    QR_LLM -->|fail| QR_DG[降级：使用原始 question]
    QR_DG --> MR

    MR -->|true| MR_LLM[MultiRoute → LLMClient.ChatCompletion]
    MR -->|false| VR
    MR_LLM -->|OK| VR[VectorStore.CosineSearch]
    MR_LLM -->|fail| VR_DG[降级：单路检索]
    VR_DG --> VR

    VR -->|OK| BM{Hybrid?}
    VR -->|fail ❌| VRFail[返回 code=20002 ErrRAGUnavailable]

    BM -->|true| BM25[BM25Retriever.Retrieve]
    BM -->|false| Rerank
    BM25 -->|OK| Fuse[HybridFuse: RRF k=60]
    BM25 -->|fail| BM_DG[降级：仅向量结果]
    BM_DG --> Rerank
    Fuse --> Rerank

    Rerank{Rerank?} -->|true| Rerank_LLM[Rerank → LLMClient.ChatCompletion]
    Rerank -->|false| LLMGen
    Rerank_LLM -->|OK| LLMGen[LLMClient.ChatCompletion → 生成答案]
    Rerank_LLM -->|fail| Rerank_DG[降级：RRF 排序结果]
    Rerank_DG --> LLMGen

    LLMGen -->|OK| Done([返回 ChatSessionResponse])
    LLMGen -->|fail ❌| LLMFail[返回 code=20001 ErrAIUnavailable]

    style VRFail fill:#ef444420,stroke:#ef4444
    style LLMFail fill:#ef444420,stroke:#ef4444
    style QR_DG fill:#f59e0b20,stroke:#f59e0b
    style VR_DG fill:#f59e0b20,stroke:#f59e0b
    style BM_DG fill:#f59e0b20,stroke:#f59e0b
    style Rerank_DG fill:#f59e0b20,stroke:#f59e0b
```
