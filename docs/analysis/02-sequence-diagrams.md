# 2. 典型场景时序图

## 2.1 场景一：Remember（存储新记忆）

完整的写入管道：从 Claude 调用 `mnemon remember` 到图边自动生成、候选输出。

```mermaid
sequenceDiagram
    participant C as Claude (LLM)
    participant CMD as cmd/remember.go
    participant S as store/node.go
    participant E as embed/ollama.go
    participant GE as graph/engine.go
    participant GT as graph/temporal.go
    participant GN as graph/entity.go
    participant GC as graph/causal.go
    participant GS as graph/semantic.go
    participant SE as store/edge.go

    C->>CMD: mnemon remember "内容" --cat decision --imp 5 --entities "X,Y"
    CMD->>CMD: 解析参数, 创建 model.Insight (UUID, timestamps)
    CMD->>S: InsertInsight(insight)
    S-->>CMD: OK

    Note over CMD,E: Step 2: 可选向量嵌入
    CMD->>E: NewClient().Available()
    alt Ollama 可用
        E-->>CMD: true
        CMD->>E: Embed(content)
        E->>E: POST /api/embed (nomic-embed-text)
        E-->>CMD: []float64 (向量)
        CMD->>S: UpdateEmbedding(id, vector)
    else Ollama 不可用
        E-->>CMD: false
    end

    Note over CMD,GE: Step 3: 图引擎自动边生成
    CMD->>GE: OnInsightCreated(insight)

    GE->>GN: ExtractEntities(content) + merge --entities
    GN-->>GE: 合并后实体列表
    GE->>S: UpdateEntities(id, entities)

    GE->>GT: CreateTemporalEdge(insight)
    GT->>S: GetLatestInsightBySource()
    GT->>SE: InsertEdge(prev→new, PRECEDES)
    GT->>SE: InsertEdge(new→prev, SUCCEEDS)
    GT->>S: GetRecentInsightsInWindow(24h)
    loop 每个近邻 (最多10条)
        GT->>SE: InsertEdge(proximity, w=1/(1+hours))
    end
    GT-->>GE: temporal_count

    GE->>GN: CreateEntityEdges(insight)
    loop 每个实体
        GN->>S: FindInsightsWithEntity(entity)
        loop 每个共现 insight (最多5条)
            GN->>SE: InsertEdge(双向 entity 边)
        end
    end
    GN-->>GE: entity_count

    GE->>GC: CreateCausalEdges(insight)
    GC->>S: GetRecentInsightsBySource(10)
    loop 每条近期 insight
        GC->>GC: HasCausalSignal? + tokenOverlap >= 0.15?
        opt 满足条件
            GC->>GC: inferDirection + suggestSubType
            GC->>SE: InsertEdge(causal, 方向已确定)
        end
    end
    GC-->>GE: causal_count

    GE->>GS: CreateSemanticEdges(insight)
    alt embedding 可用
        GS->>S: GetAllEmbeddings()
        loop 每个已嵌入 insight
            GS->>GS: CosineSimilarity >= 0.50?
            opt 满足阈值 (最多3条)
                GS->>SE: InsertEdge(双向 semantic 边)
            end
        end
    end
    GS-->>GE: semantic_count

    GE-->>CMD: EdgeStats{temporal, entity, causal, semantic}

    Note over CMD,GS: Step 4: 候选输出（供 Claude 评估）
    CMD->>GS: FindSemanticCandidates(insight)
    GS-->>CMD: []SemanticCandidate (cosine >= 0.30)

    CMD->>GC: FindCausalCandidates(insight)
    GC->>GC: GetNeighborhood(id, maxHops=2, maxNodes=10)
    GC-->>CMD: []CausalCandidate (hop, via_edge, signal, sub_type)

    Note over CMD,S: Step 5: 生命周期更新
    CMD->>S: RefreshEffectiveImportance(id)
    CMD->>S: AutoPrune() [若 total > 1000]

    CMD->>S: LogOp("remember", id, detail)
    CMD-->>C: JSON{id, edges_created, semantic_candidates, causal_candidates, embedded, EI}

    Note over C: Claude 评估 candidates，决定是否调用 link
    opt semantic_candidates 有强候选
        C->>CMD: mnemon link <new_id> <candidate_id> --type semantic --weight 0.85
    end
    opt causal_candidates 有强候选
        C->>CMD: mnemon link <src> <tgt> --type causal --weight 0.8 --meta '{"sub_type":"causes"}'
    end
```

### 数据库状态变化

| 步骤 | insights 表 | edges 表 | oplog 表 |
|------|-------------|----------|----------|
| InsertInsight | +1 行 | — | — |
| UpdateEmbedding | embedding 列更新 | — | — |
| UpdateEntities | entities 列更新 | — | — |
| CreateTemporalEdge | — | +2 (backbone) + N (proximity) | — |
| CreateEntityEdges | — | +2×M (双向共现) | — |
| CreateCausalEdges | — | +K (因果) | — |
| CreateSemanticEdges | — | +2×L (双向语义) | — |
| LogOp | — | — | +1 |
| link (Claude 手动) | — | +2 (双向) | +1 |

## 2.2 场景二：Smart Recall（智能召回）

完整的读取管道：从查询到多信号锚点选择、beam search、多因子 reranking。

```mermaid
sequenceDiagram
    participant C as Claude (LLM)
    participant CMD as cmd/recall.go
    participant E as embed/ollama.go
    participant GN as graph/entity.go
    participant INT as search/intent.go
    participant REC as search/recall.go
    participant KW as search/keyword.go
    participant S as store/node.go
    participant SE as store/edge.go

    C->>CMD: mnemon recall "why Alpha service routing" --smart --intent WHY
    CMD->>CMD: keyword = "why Alpha service routing"

    Note over CMD,INT: Step 1: Intent 解析
    alt --intent 提供
        CMD->>INT: IntentFromString("WHY")
        INT-->>CMD: IntentWhy, intentSource="override"
    else 自动检测
        CMD->>INT: DetectIntent(keyword)
        INT->>INT: regex 匹配 whyPatterns/whenPatterns/entityPatterns
        INT-->>CMD: IntentWhy, intentSource="auto"
    end

    Note over CMD,E: Step 2: Query 向量化
    CMD->>E: Available() → Embed(keyword)
    E-->>CMD: queryVec (或 nil)

    CMD->>GN: ExtractEntities(keyword)
    GN-->>CMD: queryEntities = ["Alpha"]

    Note over CMD,REC: Step 3: IntentAwareRecall 核心管道
    CMD->>REC: IntentAwareRecall(db, query, queryVec, queryEntities, limit=10, &WHY)

    REC->>S: GetAllActiveInsights()
    S-->>REC: allInsights[]

    Note over REC,KW: Step 3a: 多信号锚点选择 (RRF)
    REC->>KW: KeywordSearch(allInsights, query, top=20)
    KW-->>REC: keywordResults[] (token overlap 评分)

    alt queryVec 不为 nil
        REC->>REC: vectorSearch(allInsights, queryVec, top=20)
        Note right of REC: 遍历所有 embedding,<br/>cosine >= 0.1 保留
    end

    REC->>REC: timeResults[] (按 created_at 降序, top=20)

    REC->>REC: RRF 融合: score = Σ 1/(k + rank + 1), k=60
    REC->>REC: 取 top-20 anchors

    Note over REC,SE: Step 3b: Beam Search 图遍历
    REC->>INT: GetWeights(WHY)
    INT-->>REC: {causal:0.70, temporal:0.20, entity:0.05, semantic:0.05}

    loop 每个 anchor
        REC->>REC: beamSearchFromAnchor(anchor, params)
        Note right of REC: WHY params:<br/>beamWidth=15, maxDepth=5,<br/>maxVisited=500

        loop depth = 1..maxDepth
            REC->>SE: GetEdgesByNode(currentNode)
            SE-->>REC: edges[]
            loop 每条边
                REC->>REC: transition = score_u + λ₁·φ(edge_type)·weight + λ₂·cosine
                Note right of REC: λ₁=1.0, λ₂=0.4<br/>φ = intent 权重映射
            end
            REC->>REC: 保留 top-beamWidth 节点（优先队列）
        end
    end

    Note over REC: Step 4: 多因子 Reranking
    REC->>REC: 对每个候选节点计算:
    Note right of REC: keyword_score = |queryTokens ∩ nodeTokens| / |queryTokens|<br/>entity_score = |queryEntities ∩ nodeEntities| / max(1, |queryEntities|)<br/>similarity = cosine(queryVec, nodeVec)<br/>graph_score = min-max 归一化 beam 分数

    alt hasEmbeddings
        REC->>REC: final = 0.30·kw + 0.15·ent + 0.35·sim + 0.20·graph
    else noEmbeddings
        REC->>REC: final = 0.45·kw + 0.25·ent + 0.30·graph
    end

    Note over REC: Step 5: WHY 后处理 — 因果拓扑排序
    REC->>SE: GetEdgesBySourceAndType(*, causal)
    SE-->>REC: causalEdges[]
    REC->>REC: Kahn's 拓扑排序 (原因在前, 结果在后)

    Note over REC: Step 6: 稀疏提示
    REC->>REC: len(results) < limit/2 → hint="sparse_results"

    REC-->>CMD: RecallResponse{results[], meta}

    loop 每条结果
        CMD->>S: IncrementAccessCount(id)
    end
    CMD->>S: LogOp("recall:smart", "", "q=... hits=N")
    CMD-->>C: JSON{results, meta}

    Note over C: Claude 根据 signals 字段<br/>复判结果排序与相关性
```

### 数据库状态变化

| 步骤 | insights 表 | edges 表 | oplog 表 |
|------|-------------|----------|----------|
| GetAllActiveInsights | 读（全表扫描，排除 deleted_at） | — | — |
| GetEdgesByNode | — | 读（按 source_id/target_id 索引） | — |
| IncrementAccessCount | access_count +1, last_accessed_at 更新 | — | — |
| LogOp | — | — | +1 |

## 2.3 场景三：Diff + Remember 组合（去重后存储）

Claude 的标准记忆存储流程：先 diff 检查重复，再决定存储。

```mermaid
sequenceDiagram
    participant C as Claude (LLM)
    participant DIFF as cmd/diff.go
    participant REM as cmd/remember.go
    participant KW as search/keyword.go
    participant S as store/node.go

    C->>DIFF: mnemon diff "新发现的事实"
    DIFF->>S: GetAllActiveInsights()
    DIFF->>KW: KeywordSearch(insights, "新发现的事实", limit=5)
    KW-->>DIFF: matches[]

    loop 每个匹配
        DIFF->>KW: ContentSimilarity(new_fact, match.content)
        KW-->>DIFF: similarity float64
    end

    DIFF->>DIFF: 分类判定:
    Note right of DIFF: similarity >= 0.85 → DUPLICATE<br/>similarity >= 0.50 + 否定词 → CONFLICT<br/>similarity >= 0.50 → UPDATE<br/>otherwise → ADD

    DIFF-->>C: JSON{suggestion: "ADD", similar: [...]}

    alt suggestion == "ADD"
        C->>REM: mnemon remember "新发现的事实" --cat fact --imp 3
    else suggestion == "CONFLICT"
        C->>C: mnemon forget <old_id>
        C->>REM: mnemon remember "更新后的事实" --cat fact --imp 3
    else suggestion == "DUPLICATE"
        C->>C: 跳过存储
    end
```

## 2.4 场景四：GC 生命周期管理

```mermaid
sequenceDiagram
    participant C as Claude (LLM)
    participant GC as cmd/gc.go
    participant S as store/node.go

    C->>GC: mnemon gc --threshold 0.5
    GC->>S: GetRetentionCandidates(threshold=0.5)

    S->>S: 查询所有非删除 insight
    loop 每个 insight
        S->>S: ComputeEffectiveImportance()
        Note right of S: EI = base × log(1+access) × 0.5^(days/30) × edge_factor
        S->>S: IsImmune?
        Note right of S: importance >= 4 OR access_count >= 3
        opt EI < threshold AND NOT immune
            S->>S: 加入候选列表
        end
    end
    S-->>GC: candidates[]

    GC-->>C: JSON{candidates, total_insights, candidates_found, actions}

    Note over C: Claude 评估每个候选

    opt 决定保留某条
        C->>GC: mnemon gc --keep <id>
        GC->>S: BoostRetention(id)
        Note right of S: access_count += 3<br/>last_accessed_at = now<br/>→ 变为 immune
        S-->>GC: OK
        GC-->>C: JSON{status: "retained", immune: true}
    end

    opt 决定删除某条
        C->>GC: mnemon forget <id>
    end
```

## 2.5 场景五：Link（手动边创建）

Claude 评估 candidates 后手动建边的流程。

```mermaid
sequenceDiagram
    participant C as Claude (LLM)
    participant LINK as cmd/link.go
    participant S as store/node.go
    participant SE as store/edge.go

    Note over C: remember 输出中<br/>causal_candidates[0].suggested_sub_type = "causes"

    C->>LINK: mnemon link <src> <tgt> --type causal --weight 0.8 --meta '{"sub_type":"causes","reason":"..."}'

    LINK->>LINK: 验证 edge_type ∈ {temporal, semantic, causal, entity}
    LINK->>LINK: 验证 weight ∈ [0.0, 1.0]

    LINK->>S: GetInsightByID(src)
    S-->>LINK: srcInsight (或 error)
    LINK->>S: GetInsightByID(tgt)
    S-->>LINK: tgtInsight (或 error)

    LINK->>LINK: metadata.created_by = "claude"

    LINK->>SE: InsertEdge(src→tgt, causal, 0.8, metadata)
    Note right of SE: INSERT OR REPLACE<br/>(允许更新已有边的权重)
    LINK->>SE: InsertEdge(tgt→src, causal, 0.8, metadata)

    LINK->>S: LogOp("link", src, "src→tgt causal w=0.8")

    LINK-->>C: JSON{status: "linked", source_id, target_id, edge_type, weight}
```

## 2.6 场景六：Embed（向量嵌入管理）

```mermaid
sequenceDiagram
    participant C as Claude (LLM)
    participant EMB as cmd/embed.go
    participant E as embed/ollama.go
    participant S as store/node.go

    alt --status 模式
        C->>EMB: mnemon embed --status
        EMB->>S: EmbeddingStats()
        S-->>EMB: {total, embedded, coverage}
        EMB->>E: Available()
        E-->>EMB: true/false
        EMB-->>C: JSON{total, embedded, coverage, model, ollama_available}

    else --all 模式 (backfill)
        C->>EMB: mnemon embed --all
        EMB->>E: Available()
        E-->>EMB: true
        EMB->>S: GetInsightsWithoutEmbedding()
        S-->>EMB: unembedded[]

        loop 每个未嵌入 insight
            EMB->>E: Embed(content)
            E->>E: POST /api/embed
            E-->>EMB: []float64
            EMB->>S: UpdateEmbedding(id, vector)
        end
        EMB-->>C: JSON{status: "backfill_complete", succeeded, failed}

    else 单条模式
        C->>EMB: mnemon embed <id>
        EMB->>S: GetInsightByID(id)
        EMB->>E: Embed(content)
        EMB->>S: UpdateEmbedding(id, vector)
        EMB-->>C: JSON{status, id, model, dimensions}
    end
```

## 2.7 端到端典型会话流程

一个完整的 Claude 使用 mnemon 的会话流程：

```mermaid
sequenceDiagram
    participant U as 用户
    participant C as Claude
    participant M as mnemon CLI

    Note over U,M: 会话开始

    C->>M: mnemon recall "<当前话题>" --smart --limit 5
    M-->>C: RecallResponse{results, meta}
    Note right of C: 加载历史上下文

    U->>C: "帮我实现 X 功能"
    C->>C: 结合 recall 结果 + 用户需求<br/>制定方案并实现

    Note over C,M: 实现过程中发现值得记忆的信息

    C->>M: mnemon diff "选择 Redis 作为缓存方案因为延迟要求"
    M-->>C: {suggestion: "ADD"}

    C->>M: mnemon remember "选择 Redis 作为缓存方案因为延迟要求" --cat decision --imp 5 --entities "Redis,caching"
    M-->>C: {id, edges_created, causal_candidates: [...], semantic_candidates: [...]}

    Note over C: 评估 candidates
    opt causal_candidate 有效
        C->>M: mnemon link <new> <old> --type causal --weight 0.8 --meta '{"sub_type":"causes"}'
    end
    opt semantic_candidate 有效
        C->>M: mnemon link <new> <old> --type semantic --weight 0.85
    end

    C-->>U: "已完成 X 功能的实现，方案是..."

    Note over U,M: 多次对话后

    C->>M: mnemon gc --threshold 0.5
    M-->>C: {candidates: [...]}
    Note right of C: 定期清理低价值记忆
```
