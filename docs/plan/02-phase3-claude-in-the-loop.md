# Phase 3: Claude-in-the-Loop MAGMA Alignment

## Context

Phases 1-5 完成了 MAGMA 四图架构 + beam search + RRF + adaptive traversal + auto semantic edges + topological sort。79/79 E2E 测试通过。

**剩余 4 个 MAGMA 差异**需要 LLM 能力，我们用 **Claude-in-the-loop** 模式实现：mnemon 输出结构化候选 JSON，Claude 评估后回调 mnemon 命令。零 API key，零新依赖。

| 差异 | MAGMA 要求 | 当前状态 | 方案 |
|------|-----------|---------|------|
| D3 | LLM 推断因果子类型 (causes/enables/prevents) | keyword + token overlap | `remember` 输出 `causal_candidates`，Claude 评估后调 `link --type causal --meta '{"sub_type":"causes"}'` |
| D4 | LLM NER 实体抽取 | 5 条正则 | 新命令 `mnemon enrich <id> --entities "X,Y"` 让 Claude 补充实体 |
| D5 | ±k context neighbor edges (序列空间) | 仅时间窗口 proximity | 加 `sequence_index` 列，`CreateContextNeighborEdges()` (纯代码) |
| D7 | Narrative consolidation (事件分组) | 无 | 新命令 `mnemon consolidate`，新边类型 `narrative` |

---

## Phase 3A: Context Neighbor Edges (D5) — 纯代码

MAGMA §3.1: 除时间 proximity 外，按插入序列 ±k 位置建 context_neighbor 边。

### Schema 迁移：`internal/store/db.go`

```go
// Phase 3A migration: add sequence_index for context neighbor edges
db.conn.Exec(`ALTER TABLE insights ADD COLUMN sequence_index INTEGER`)
// Backfill: assign sequential index by created_at order
db.conn.Exec(`UPDATE insights SET sequence_index = (
    SELECT COUNT(*) FROM insights i2
    WHERE i2.created_at <= insights.created_at AND i2.id != insights.id
) WHERE sequence_index IS NULL`)
```

### 新增：`internal/store/node.go`

```go
// GetNextSequenceIndex returns the next sequence index for a new insight.
func (db *DB) GetNextSequenceIndex() (int, error)

// GetInsightsBySequenceRange returns active insights within [seqIdx-k, seqIdx+k].
func (db *DB) GetInsightsBySequenceRange(seqIdx, k int, excludeID string) ([]*model.Insight, error)

// UpdateSequenceIndex sets the sequence_index for an insight.
func (db *DB) UpdateSequenceIndex(id string, idx int) error
```

### 新增：`internal/graph/context_neighbor.go`

```go
const contextNeighborK = 3 // ±3 positions (MAGMA default)

// CreateContextNeighborEdges creates bidirectional temporal edges
// to insights within ±k sequence positions.
// Weight: 1.0 / (1.0 + distance * 0.5)  (MAGMA §3.1)
func CreateContextNeighborEdges(db *store.DB, insight *model.Insight, seqIdx int) int
```

- 边类型：`temporal`，metadata: `{"sub_type": "context_neighbor", "seq_distance": "2"}`
- 与现有 backbone/proximity 共存于 temporal 类型下，通过 metadata sub_type 区分

### 修改：`internal/graph/engine.go`

`OnInsightCreated` 增加步骤 2.5（在 temporal backbone 之后）：
```go
// 2.5. Context neighbor edges (MAGMA §3.1)
seqIdx, _ := db.GetNextSequenceIndex()
db.UpdateSequenceIndex(insight.ID, seqIdx)
stats.ContextNeighbor = CreateContextNeighborEdges(e.db, insight, seqIdx)
```

`EdgeStats` 加 `ContextNeighbor int` 字段。

### 修改：`cmd/remember.go`

输出 `edges_created` 增加 `context_neighbor` 字段。

---

## Phase 3B: Causal Candidates (D3) — Claude-in-the-loop

MAGMA §3.3: LLM 判断因果关系方向和子类型。

### 新增：`internal/graph/causal.go`

```go
// CausalCandidate represents a potential causal link for Claude to evaluate.
type CausalCandidate struct {
    ID               string  `json:"id"`
    Content          string  `json:"content"`
    Category         string  `json:"category"`
    CausalSignal     string  `json:"causal_signal"`     // 触发的关键词
    TokenOverlap     float64 `json:"token_overlap"`
    SuggestedSubType string  `json:"suggested_sub_type"` // "causes"|"enables"|"prevents"
}

// FindCausalCandidates returns insights that may have causal relationships
// with the given insight. Claude evaluates direction, sub_type, and weight.
func FindCausalCandidates(db *store.DB, insight *model.Insight) []CausalCandidate
```

逻辑：
1. 检查新 insight 是否有 causal signal（已有 `HasCausalSignal`）
2. 同时也检查最近 10 条 insight 中有 causal signal 的
3. 计算 token overlap，阈值 0.10（低召回优先）
4. 基于关键词猜测 `suggested_sub_type`：
   - "because/caused by/因为/由于" → "causes"
   - "so that/in order to/为了/以便" → "enables"
   - "despite/prevented/阻止" → "prevents"
   - 其他 → "causes"（默认）
5. 最多返回 5 个候选

### 修改：`cmd/remember.go`

输出增加 `causal_candidates` 字段：
```json
{
  "causal_candidates": [
    {
      "id": "abc-123",
      "content": "Chose Redis for caching",
      "causal_signal": "because",
      "token_overlap": 0.35,
      "suggested_sub_type": "causes"
    }
  ]
}
```

### 修改：`cmd/link.go`

`--meta` 已存在，Claude 通过 metadata 传递 sub_type：
```bash
mnemon link <src> <tgt> --type causal --weight 0.8 --meta '{"sub_type":"causes","reason":"Redis chosen because of latency requirements"}'
```

无需修改 link.go 代码 — 现有 `--meta` flag 已支持任意 JSON metadata。

---

## Phase 3C: Entity Enrichment (D4) — Claude-in-the-loop

MAGMA §3.2: LLM NER 提取结构化实体。

### 新命令：`cmd/enrich.go`

```
mnemon enrich <insight_id> --entities "React,TypeScript,Vercel" [--rebuild-edges]
```

功能：
1. 验证 insight 存在
2. 合并新 entities 到已有 entities（去重）
3. 更新数据库中的 entities JSON
4. 如果 `--rebuild-edges`：为新增实体创建 entity co-occurrence 边
5. 输出更新后的 insight

### 新增：`internal/store/node.go`

```go
// MergeEntities merges new entities into existing ones (deduplicates).
func (db *DB) MergeEntities(id string, newEntities []string) ([]string, error)
```

### 新增：`internal/graph/entity.go`

```go
// CreateEntityEdgesForNewEntities creates entity edges only for newly added entities.
func CreateEntityEdgesForNewEntities(db *store.DB, insight *model.Insight, newEntities []string) int
```

### 修改：`cmd/remember.go`

输出增加 `entity_hints` 字段 — 提示 Claude 当前正则提取的实体，让 Claude 决定是否需要补充：
```json
{
  "entities": ["HttpServer", "config.yml"],
  "entity_hints": "Auto-extracted by regex. Consider running `mnemon enrich <id> --entities \"X,Y\" --rebuild-edges` if important entities were missed."
}
```

---

## Phase 3D: Narrative Consolidation (D7) — Claude-in-the-loop

MAGMA §4.1: 将相关事件分组为叙事节点，用 PART_OF 边链接。

### 新边类型：`narrative`

**修改：`internal/model/edge.go`**
```go
EdgeNarrative EdgeType = "narrative" // PART_OF relationship
```
加入 `ValidEdgeTypes` map。

**修改：`internal/store/db.go`**
```sql
-- Phase 3D migration: extend CHECK constraint for narrative edge type
-- SQLite 不支持 ALTER CHECK，但新 DB 使用新约束，旧 DB 通过 INSERT OR REPLACE 绕过
```

策略：在 `migrate()` 中用 `CREATE TABLE IF NOT EXISTS` 处理新 DB。对旧 DB，narrative 边通过 `INSERT OR REPLACE` 绕过 CHECK（因为 CHECK 只在 CREATE TABLE 时定义，已有表的 CHECK 不变）。

**实际方案**：SQLite CHECK 约束在 CREATE TABLE IF NOT EXISTS 时如果表已存在不会更新。因此：
1. 新 DB：直接用含 `narrative` 的 CHECK
2. 旧 DB：重建边表（复制数据 → 删旧表 → 建新表 → 恢复数据）

```go
// Phase 3D migration: add narrative to edge_type CHECK constraint
// Check if constraint needs updating by trying an insert
_, err := db.conn.Exec(`INSERT INTO edges VALUES ('__test','__test','narrative',0,'{}',datetime('now'))`)
if err != nil {
    // CHECK constraint doesn't allow 'narrative', need to recreate table
    db.conn.Exec(`ALTER TABLE edges RENAME TO edges_old`)
    db.conn.Exec(`CREATE TABLE edges (... CHECK(edge_type IN ('temporal','semantic','causal','entity','narrative')) ...)`)
    db.conn.Exec(`INSERT INTO edges SELECT * FROM edges_old`)
    db.conn.Exec(`DROP TABLE edges_old`)
} else {
    // Clean up test row
    db.conn.Exec(`DELETE FROM edges WHERE source_id = '__test'`)
}
```

### 新增 `narrative` 分类到 model

**修改：`internal/model/node.go`** — 在 `ValidCategories` 中加入 `"narrative"`。

### 新命令：`cmd/consolidate.go`

两种模式：

**建议模式**（默认）：
```bash
mnemon consolidate [--window 72h] [--min-cluster 3]
```

输出：
```json
{
  "clusters": [
    {
      "cluster_id": 1,
      "time_range": {"start": "2026-02-15T10:00:00Z", "end": "2026-02-15T14:00:00Z"},
      "insights": [
        {"id": "abc", "content": "Set up React project", "category": "context"},
        {"id": "def", "content": "Chose TypeScript for type safety", "category": "decision"},
        {"id": "ghi", "content": "Added ESLint config", "category": "context"}
      ],
      "suggested_title": "Project setup and tooling decisions",
      "shared_entities": ["React", "TypeScript"]
    }
  ],
  "actions": {
    "create": "mnemon consolidate --create --title \"<title>\" --members \"<id1>,<id2>,<id3>\""
  }
}
```

**创建模式**：
```bash
mnemon consolidate --create --title "Project setup" --members "abc,def,ghi"
```

功能：
1. 创建 narrative insight（category=narrative, content=title）
2. 为每个 member 创建 `narrative` 边（member → narrative, sub_type=part_of）
3. 输出创建结果

### 新增：`internal/graph/narrative.go`

```go
// NarrativeCluster represents a group of temporally close, entity-overlapping insights.
type NarrativeCluster struct {
    Insights       []*model.Insight `json:"insights"`
    TimeRange      TimeRange        `json:"time_range"`
    SharedEntities []string         `json:"shared_entities"`
    SuggestedTitle string           `json:"suggested_title"`
}

// FindNarrativeClusters groups insights by time window and entity overlap.
// Returns clusters with >= minCluster members.
func FindNarrativeClusters(db *store.DB, window time.Duration, minCluster int) ([]NarrativeCluster, error)
```

聚类逻辑：
1. 获取所有 active insights，按 created_at 排序
2. 滑动窗口：如果 insight[i+1].created_at - insight[i].created_at < window，归入同一组
3. 同组内进一步按 entity 重叠过滤：需共享 ≥ 1 个 entity 或 token_similarity > 0.15
4. 过滤掉已有 narrative 边的 insight（避免重复）
5. 过滤掉 < minCluster 的组
6. 生成 suggested_title：取组内共享 entities + 第一条 insight 的 category

### 修改：`internal/search/intent.go`

在所有 intent weight maps 中加入 `narrative` 边类型权重：
```go
IntentWhy:     { ..., model.EdgeNarrative: 0.0 },
IntentWhen:    { ..., model.EdgeNarrative: 0.1 },   // narrative helps timeline
IntentEntity:  { ..., model.EdgeNarrative: 0.05 },
IntentGeneral: { ..., model.EdgeNarrative: 0.20 },
```

调整：WHEN 和 GENERAL 给 narrative 一定权重，WHY 不变。需要重新分配使总和 = 1.0。

---

## Phase 3E: Skill 更新

### `memory.md` 新增工作流

**Causal linking（remember 之后）：**
```
When `mnemon remember` outputs `causal_candidates`:
1. Evaluate each candidate — does a real causal relationship exist?
2. Determine sub_type: causes, enables, or prevents
3. For real causal links:
   mnemon link <new_id> <candidate_id> --type causal --weight 0.8 \
     --meta '{"sub_type":"causes","reason":"..."}'
4. Skip candidates where overlap is coincidental
```

**Entity enrichment（remember 之后）：**
```
When `mnemon remember` shows `entities` in output:
1. Review if important entities were missed by regex extraction
2. For domain concepts, project names, people, technologies not captured:
   mnemon enrich <id> --entities "Entity1,Entity2" --rebuild-edges
3. Skip if regex already captured all meaningful entities
```

**Narrative consolidation（定期）：**
```
Trigger when: 20+ insights without review, or user asks to organize memories
1. mnemon consolidate --window 72h --min-cluster 3
2. Review each cluster — does it represent a coherent narrative?
3. For valid narratives:
   mnemon consolidate --create --title "..." --members "id1,id2,id3"
4. Skip clusters that are just temporal coincidence
```

### `CLAUDE.md` 新增

```
mnemon enrich <id> --entities "X,Y" --rebuild-edges  # supplement entities
mnemon consolidate [--window 72h] [--min-cluster 3]  # find narrative clusters
mnemon consolidate --create --title "..." --members "id1,id2,id3"  # create narrative
```

---

## 实施顺序

### Phase 3A: Context Neighbor (纯代码)
| 步骤 | 文件 | 操作 |
|------|------|------|
| 1 | `internal/store/db.go` | 加 sequence_index 迁移 + backfill |
| 2 | `internal/store/node.go` | 加 GetNextSequenceIndex, GetInsightsBySequenceRange, UpdateSequenceIndex |
| 3 | `internal/graph/context_neighbor.go` | 新建 |
| 4 | `internal/graph/engine.go` | OnInsightCreated 加 context neighbor 步骤 + EdgeStats 加字段 |
| 5 | `cmd/remember.go` | 输出加 context_neighbor |
| 6 | `scripts/e2e_test.sh` | 加 Milestone 8 测试 |

### Phase 3B: Causal Candidates (Claude-in-the-loop)
| 步骤 | 文件 | 操作 |
|------|------|------|
| 7 | `internal/graph/causal.go` | 加 CausalCandidate + FindCausalCandidates |
| 8 | `cmd/remember.go` | 输出加 causal_candidates |
| 9 | `scripts/e2e_test.sh` | 加 Milestone 9 测试 |

### Phase 3C: Entity Enrichment (Claude-in-the-loop)
| 步骤 | 文件 | 操作 |
|------|------|------|
| 10 | `internal/store/node.go` | 加 MergeEntities |
| 11 | `internal/graph/entity.go` | 加 CreateEntityEdgesForNewEntities |
| 12 | `cmd/enrich.go` | 新建 |
| 13 | `cmd/remember.go` | 输出加 entity_hints |
| 14 | `scripts/e2e_test.sh` | 加 Milestone 10 测试 |

### Phase 3D: Narrative Consolidation (Claude-in-the-loop)
| 步骤 | 文件 | 操作 |
|------|------|------|
| 15 | `internal/model/edge.go` | 加 EdgeNarrative |
| 16 | `internal/model/node.go` | ValidCategories 加 narrative |
| 17 | `internal/store/db.go` | CHECK 约束迁移 |
| 18 | `internal/graph/narrative.go` | 新建 |
| 19 | `cmd/consolidate.go` | 新建 |
| 20 | `internal/search/intent.go` | intent weights 加 narrative |
| 21 | `scripts/e2e_test.sh` | 加 Milestone 11 测试 |

### Phase 3E: Skill Integration
| 步骤 | 文件 | 操作 |
|------|------|------|
| 22 | `memory.md` | 加 causal/enrich/consolidate 工作流 |
| 23 | `CLAUDE.md` | 加命令快速参考 |

---

## 变更汇总

| 类型 | 文件 | 预估行数 |
|------|------|---------|
| 新建 | `internal/graph/context_neighbor.go` | ~65 |
| 新建 | `internal/graph/narrative.go` | ~120 |
| 新建 | `cmd/enrich.go` | ~90 |
| 新建 | `cmd/consolidate.go` | ~150 |
| 修改 | `internal/model/edge.go` | +3 |
| 修改 | `internal/model/node.go` | +1 |
| 修改 | `internal/store/db.go` | +20 |
| 修改 | `internal/store/node.go` | +40 |
| 修改 | `internal/graph/causal.go` | +70 |
| 修改 | `internal/graph/entity.go` | +25 |
| 修改 | `internal/graph/engine.go` | +8 |
| 修改 | `internal/search/intent.go` | +8 |
| 修改 | `cmd/remember.go` | +12 |
| 修改 | `memory.md` | +40 |
| 修改 | `CLAUDE.md` | +5 |
| 修改 | `scripts/e2e_test.sh` | +120 |

**零新依赖**，go.mod 不变。

---

## 关键设计决策

1. **Context neighbor 用 temporal 边类型** — 不新增 edge type，通过 metadata `sub_type: context_neighbor` 区分。避免 CHECK 约束迁移。beam search 已经遍历 temporal 边，自动受益。

2. **Causal candidates 低阈值 (0.10)** — 召回优先，Claude 做精排。自动创建的 causal 边（keyword+overlap≥0.15）保留，candidates 是额外的 Claude 评估层。

3. **Narrative 边需要新 edge type** — 因为 PART_OF 语义与四图中任何现有类型都不同。需要 CHECK 约束迁移。

4. **enrich 支持 --rebuild-edges** — 默认只更新 entities JSON，加 flag 才重建 entity edges，避免每次都扫描全库。

---

## 验证

```bash
# 1. 构建 + 全量 E2E（含新 M8-M11）
make test

# 2. 向后兼容：旧 DB 自动迁移
./mnemon --data-dir .testdata/m2 status

# 3. Context neighbor 验证
mnemon remember "fact 1" && mnemon remember "fact 2" && mnemon remember "fact 3"
mnemon related <id2> --edge temporal  # 应看到 context_neighbor 边

# 4. Causal candidates 验证
mnemon remember "Chose Redis because of latency" --cat decision
# 输出应含 causal_candidates（如果有相关 insight）

# 5. Entity enrich 验证
mnemon enrich <id> --entities "Redis,Memcached" --rebuild-edges
mnemon related <id> --edge entity  # 应看到新 entity 边

# 6. Narrative consolidation 验证
mnemon consolidate --window 72h --min-cluster 2
mnemon consolidate --create --title "Setup phase" --members "id1,id2"
mnemon related <narrative_id> --edge narrative  # PART_OF 边
```
