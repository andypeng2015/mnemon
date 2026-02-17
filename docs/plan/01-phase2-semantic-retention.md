# Mnemon Phase 2: Semantic Edges + Retention Lifecycle

## Context

Phase 1 已完成 MAGMA 四图中的三种边（temporal/entity/causal）+ CLI 全套命令 + Claude Code 集成（skill + hook）。44 条 E2E 测试全通过。

Phase 2 要完成：
1. **Semantic 边** — 四图架构最后一块拼图
2. **Retention 生命周期** — 记忆的自动衰减和清理

核心设计决策：**Claude-in-the-loop** — 不引入任何 LLM/embedding 依赖，而是让 Claude Code（已经在运行的 LLM）充当语义判断引擎。mnemon 输出结构化候选数据，Claude 评估后回调 mnemon 执行。

---

## 架构：Claude-in-the-loop

```
mnemon remember "..."
  → 输出 JSON 含 semantic_candidates[]
  → Claude 评估每个候选的语义相关性
  → Claude 调用: mnemon link <src> <tgt> --type semantic --weight 0.85

mnemon gc
  → 输出 JSON 含低分 candidates[]
  → Claude 审查每条，判断保留/清理
  → Claude 调用: mnemon forget <id> 或 mnemon gc --keep <id>
```

零新依赖，go.mod 不变。

---

## Feature 1: Semantic Edges

### 新文件：`internal/graph/semantic.go`

用现有 `search.ContentSimilarity()` (token overlap) 找语义候选，不创建边 — 只输出候选列表供 Claude 评估。

```go
func FindSemanticCandidates(db *store.DB, insight *model.Insight) []SemanticCandidate
```

- 阈值 `minSemanticSimilarity = 0.10`（故意设低，召回优先，Claude 做精排）
- 最多返回 `maxSemanticCandidates = 5`
- 排除自身，按 token_similarity 降序

### 新文件：`cmd/link.go`

Claude 回调命令，手动创建/更新边：

```
mnemon link <source_id> <target_id> --type semantic --weight 0.85 [--meta '{"reason":"..."}']
```

- 创建**双向**边（与 temporal/entity 一致）
- weight 范围 0.0-1.0，验证
- 验证两端 insight 存在
- metadata 标记 `created_by: claude`
- 利用现有 `InsertEdge()`（INSERT OR REPLACE），重复调用更新 weight

### 修改：`cmd/remember.go`

输出增加 `semantic_candidates` 字段：

```json
{
  "id": "abc-123",
  "edges_created": {"temporal": 2, "entity": 0, "causal": 0, "semantic": 0},
  "semantic_candidates": [
    {"id": "def-456", "content": "...", "category": "decision", "token_similarity": 0.42}
  ]
}
```

### 修改：`internal/graph/engine.go`

`EdgeStats` 增加 `Semantic int` 字段（自动生成始终为 0，由 `link` 命令填充）。

---

## Feature 2: Retention Lifecycle

### Schema 迁移：`internal/store/db.go`

```go
db.conn.Exec(`ALTER TABLE insights ADD COLUMN last_accessed_at TEXT`)
```

向后兼容：ALTER TABLE ADD COLUMN 对已有数据库安全（列已存在时忽略错误）。

### 修改：`internal/store/node.go`

**IncrementAccessCount** — 同时更新 `last_accessed_at`：

```go
UPDATE insights SET access_count = access_count + 1, last_accessed_at = ? WHERE id = ?
```

**新方法 GetRetentionCandidates** — 计算保留分数：

```
retention_score = 0.35 * (importance/5)
               + 0.20 * min(access_count/10, 1)
               + 0.30 * max(0, 1 - days_since_access/90)
               + 0.15 * min(edge_count/5, 1)
```

返回低于阈值的 insights，按分数升序，附带各项分数明细。

**新方法 BoostRetention** — `gc --keep` 调用：access_count +3，刷新 last_accessed_at。

### 新文件：`cmd/gc.go`

两种模式：

**建议模式**（默认）：
```
mnemon gc [--threshold 0.4] [--limit 20]
```
输出：
```json
{
  "total_insights": 47,
  "threshold": 0.4,
  "candidates_found": 3,
  "candidates": [
    {
      "insight": {"id": "...", "content": "...", "importance": 2, "access_count": 0},
      "retention_score": 0.18,
      "days_since_access": 94.5,
      "edge_count": 1,
      "components": {"importance": 0.4, "access": 0.0, "recency": 0.0, "edge": 0.2}
    }
  ],
  "actions": {"purge": "mnemon forget <id>", "keep": "mnemon gc --keep <id>"}
}
```

**保留模式**：
```
mnemon gc --keep <id>
```

---

## Feature 3: Skill 更新

### `memory.md` 新增

**Semantic linking 工作流**（remember 之后）：
- 检查 `semantic_candidates`，对每个真正语义相关的候选调用 `mnemon link`
- weight 范围：0.3（弱关联）到 0.95（强关联）
- 跳过仅词汇重叠但语义无关的候选

**Retention review 工作流**（定期）：
- 触发条件：50+ insights、或 10+ 对话未审查、或用户提及记忆混乱
- `mnemon gc --threshold 0.4` → 审查 → `forget` 或 `gc --keep`
- 判断规则：importance >= 4 的决策/偏好通常保留；过期 context 通常清理

### `CLAUDE.md` 新增

简要引用 `link` 和 `gc` 命令。

---

## 实施顺序

### Phase 2A: Semantic Edges

| 步骤 | 文件 | 操作 |
|------|------|------|
| 1 | `internal/graph/semantic.go` | 新建 |
| 2 | `internal/graph/engine.go` | EdgeStats 加 Semantic 字段 |
| 3 | `cmd/remember.go` | 输出加 semantic_candidates |
| 4 | `cmd/link.go` | 新建 |
| 5 | `scripts/e2e_test.sh` | 加 Milestone 5 测试 |

### Phase 2B: Retention Lifecycle

| 步骤 | 文件 | 操作 |
|------|------|------|
| 6 | `internal/store/db.go` | 加 last_accessed_at 迁移 |
| 7 | `internal/store/node.go` | 改 IncrementAccessCount，加 GetRetentionCandidates/BoostRetention |
| 8 | `cmd/gc.go` | 新建 |
| 9 | `scripts/e2e_test.sh` | 加 Milestone 6 测试 |

### Phase 2C: Skill 集成

| 步骤 | 文件 | 操作 |
|------|------|------|
| 10 | `memory.md` | 加 semantic linking + retention review |
| 11 | `CLAUDE.md` | 加 link + gc 快速参考 |

---

## 变更汇总

| 类型 | 文件 | 预估行数 |
|------|------|---------|
| 新建 | `internal/graph/semantic.go` | ~55 |
| 新建 | `cmd/link.go` | ~85 |
| 新建 | `cmd/gc.go` | ~75 |
| 修改 | `internal/graph/engine.go` | +1 |
| 修改 | `cmd/remember.go` | +5 |
| 修改 | `internal/store/db.go` | +2 |
| 修改 | `internal/store/node.go` | +95 |
| 修改 | `memory.md` | +45 |
| 修改 | `CLAUDE.md` | +10 |
| 修改 | `scripts/e2e_test.sh` | +80 |

**零新依赖**，go.mod 不变。

---

## 验证

```bash
# 1. 构建 + 全量 E2E 测试（含新 M5/M6）
make test

# 2. 向后兼容：用已有 .testdata 跑
./mnemon --data-dir .testdata/m2 status

# 3. 集成验证：新 Claude Code 会话中
#    - remember 几条相关事实 → 检查 semantic_candidates 输出
#    - Claude 创建 semantic link → mnemon status 看 edge_count 增长
#    - recall --smart → 验证 semantic 边参与检索
#    - gc --threshold 0.5 → Claude 审查并操作
```
