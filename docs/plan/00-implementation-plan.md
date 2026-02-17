# Mnemon Implementation Plan

## Context

Mnemon 是一个面向 LLM agent 的独立记忆 daemon，基于 MAGMA 论文的四图架构。当前项目处于纯文档阶段（零代码），需要从零开始构建 Go CLI binary。

本计划综合了两个参考实现：
- **MAGMA 论文源码**（[FredJiang0324/MAMGA](https://github.com/FredJiang0324/MAMGA)，Python/NetworkX）：学术验证代码，忠实于论文但非生产级
- **MemCP**（[maydali28/memcp](https://github.com/maydali28/memcp)，Python/SQLite/MCP）：工程简化实现，面向 Claude Code 插件场景

我们的目标是在 Go 中构建一个**独立的 CLI binary**，取两者之长。

### 参考实现对比与取舍

| 组件 | MAGMA 源码做法 | MemCP 做法 | Mnemon 采用 | 理由 |
|------|-------------|-----------|------------|------|
| **存储** | NetworkX 内存图 + JSON 持久化 | SQLite WAL + 两张表 | SQLite（参考 MemCP） | 生产级持久化，并发安全 |
| **Temporal 边** | 严格骨干链 prev→next | 30 分钟窗口内互连 | 骨干链（参考 MAGMA） | 更简洁，边少，忠实论文 |
| **Entity 边** | LLM 提取 → 共现连边（max 5） | 5 条正则 → 共现连边 | 正则 + max 5（综合两者） | 零 LLM 依赖，加上限控制 |
| **Causal 边** | speaker 切换 → RESPONSE_TO | 英文因果关键词 → token 重叠 | 中英文关键词 + token 重叠（扩展 MemCP） | 更通用，不限于对话场景 |
| **Semantic 边** | cos > θ_sim (0.1-0.3) | cos > 0.3，fallback 关键词 Jaccard | 先跳过 | 需要 embedding，后续再加 |
| **检索** | RRF 融合 + Beam Search + 意图权重 | 5 级降级 + 简单意图检测 | 关键词搜索 → 意图感知遍历（渐进） | 先简单后复杂 |

---

## 总体架构

```
mnemon (Go binary)
├── cmd/              # cobra CLI 命令
│   ├── root.go       # 根命令 + 全局配置
│   ├── remember.go   # 写入 insight + 自动建边
│   ├── recall.go     # 关键词检索
│   ├── forget.go     # 软删除
│   ├── related.go    # 图遍历
│   ├── search.go     # 多策略搜索（M3）
│   ├── diff.go       # 去重辅助（M3）
│   └── status.go     # 统计信息
│
├── internal/
│   ├── store/         # SQLite 存储层
│   │   ├── db.go      # 连接管理、初始化、迁移
│   │   ├── node.go    # insights 表 CRUD
│   │   └── edge.go    # edges 表 CRUD
│   │
│   ├── graph/         # 图边生成引擎
│   │   ├── engine.go  # 统一入口：OnInsightCreated()
│   │   ├── temporal.go # 时间骨干链（MAGMA 方案）
│   │   ├── entity.go  # 实体共现（MemCP 正则方案）
│   │   └── causal.go  # 因果关键词（MemCP 方案，中英文扩展）
│   │
│   ├── search/        # 检索引擎（M3 起）
│   │   └── keyword.go # 关键词搜索
│   │
│   └── model/         # 数据模型
│       ├── node.go    # Insight 结构体
│       └── edge.go    # Edge 结构体 + 类型枚举
│
├── go.mod
└── main.go            # cobra 入口
```

---

## SQLite Schema 设计

综合 MemCP 的 SQLite 方案 + MAGMA 的节点模型：

```sql
-- insights 表（参考 MemCP nodes 表，字段精简）
CREATE TABLE insights (
    id          TEXT PRIMARY KEY,           -- UUID
    content     TEXT NOT NULL,              -- 记忆内容
    category    TEXT DEFAULT 'general',     -- preference|decision|fact|insight|context|general
    importance  INTEGER DEFAULT 3,          -- 1-5（MemCP 用字符串，我们用整数更适合排序）
    tags        TEXT DEFAULT '[]',          -- JSON 数组
    entities    TEXT DEFAULT '[]',          -- JSON 数组，自动提取的实体
    source      TEXT DEFAULT 'user',        -- user|agent|external
    access_count INTEGER DEFAULT 0,         -- 访问次数（用于 retention）
    created_at  TEXT NOT NULL,              -- RFC3339
    updated_at  TEXT NOT NULL,              -- RFC3339
    deleted_at  TEXT                        -- 软删除标记
);

-- edges 表（与 MemCP 一致）
CREATE TABLE edges (
    source_id   TEXT NOT NULL,
    target_id   TEXT NOT NULL,
    edge_type   TEXT NOT NULL CHECK(edge_type IN ('temporal','semantic','causal','entity')),
    weight      REAL DEFAULT 1.0,
    metadata    TEXT DEFAULT '{}',          -- JSON，存子类型、实体名等
    created_at  TEXT NOT NULL,
    PRIMARY KEY (source_id, target_id, edge_type),
    FOREIGN KEY (source_id) REFERENCES insights(id) ON DELETE CASCADE,
    FOREIGN KEY (target_id) REFERENCES insights(id) ON DELETE CASCADE
);

-- 索引
CREATE INDEX idx_insights_category ON insights(category);
CREATE INDEX idx_insights_importance ON insights(importance);
CREATE INDEX idx_insights_created ON insights(created_at);
CREATE INDEX idx_insights_deleted ON insights(deleted_at);
CREATE INDEX idx_edges_source ON edges(source_id);
CREATE INDEX idx_edges_target ON edges(target_id);
CREATE INDEX idx_edges_type ON edges(edge_type);
```

**设计决策**：
- `importance` 用整数 1-5 而非 MemCP 的字符串（low/medium/high/critical），更直观且排序友好
- `entities` 在写入时自动提取（正则），存 JSON 数组
- `deleted_at` 实现软删除（MemCP 直接硬删）
- 不建 `sessions`/`projects` 表，MVP 阶段不需要

---

## 分步实施计划

### Milestone 1：项目骨架 + 存储层 + 基础 CRUD

**目标**：`mnemon remember/recall/forget/status` 四个命令可用

**1.1 项目初始化**
- `go mod init github.com/Grivn/mnemon`
- 引入依赖：`modernc.org/sqlite`（纯 Go SQLite）、`spf13/cobra`（CLI）、`google/uuid`
- 创建 `main.go` + `cmd/root.go`

**1.2 数据模型（`internal/model/`）**
- `node.go`：Insight 结构体（对应 insights 表），JSON 序列化方法
- `edge.go`：Edge 结构体 + EdgeType 枚举（Temporal/Semantic/Causal/Entity）

**1.3 存储层（`internal/store/`）**
- `db.go`：打开 SQLite（WAL 模式）、建表（如不存在）、关闭
  - 数据文件路径默认 `~/.mnemon/mnemon.db`，可通过 `--data-dir` 覆盖
- `node.go`：InsertInsight / QueryInsights / SoftDeleteInsight / GetInsightByID / GetStats
  - QueryInsights 支持按 content LIKE、category、importance 过滤
- `edge.go`：InsertEdge / GetEdgesByNode / DeleteEdgesByNode

**1.4 CLI 命令（`cmd/`）**
- `remember.go`：
  ```
  mnemon remember "User prefers Qdrant" --cat preference --imp 4 --tags "tool,db"
  ```
  写入 insights 表，输出 JSON（id + created_at）

- `recall.go`：
  ```
  mnemon recall "vector database" --cat preference --limit 10
  ```
  关键词搜索（SQL LIKE），按 importance DESC + created_at DESC 排序，输出 JSON 数组

- `forget.go`：
  ```
  mnemon forget <id>
  ```
  软删除（设置 deleted_at），输出确认信息

- `status.go`：
  ```
  mnemon status
  ```
  输出统计：总 insights 数、分类分布、边数、存储文件大小

**验证**：手动测试 remember → recall → forget 闭环

---

### Milestone 2：图边自动生成

**目标**：`remember` 写入时自动创建 Temporal + Entity + Causal 边，`related` 命令可遍历

**2.1 图边引擎（`internal/graph/engine.go`）**
- `OnInsightCreated(insight)` — 统一入口，依次调用各边生成器
- 返回 `EdgeStats{temporal: N, entity: N, causal: N}`

**2.2 Temporal 边（`internal/graph/temporal.go`）**

采用 **MAGMA 论文的骨干链方案**（不是 MemCP 的时间窗口）：
- 每次写入新 insight，查找最近一条 insight（按 created_at DESC LIMIT 1，排除自身）
- 创建双向边：prev → new（PRECEDES）、new → prev（SUCCEEDS）
- weight = 1.0，metadata 存 `{"sub_type": "backbone"}`

理由：骨干链更简洁（只建一对边），且与 MAGMA 论文一致。MemCP 的窗口方案会在 30 分钟内创建大量边。

**2.3 Entity 边（`internal/graph/entity.go`）**

采用 **MemCP 的正则方案**，适配中英文：
```go
// 实体提取正则模式
var entityPatterns = []*regexp.Regexp{
    regexp.MustCompile(`\b([A-Z][a-z]+(?:[A-Z][a-z]+)+)\b`),         // CamelCase
    regexp.MustCompile(`(?:^|[\s"'(])([.\w/-]+\.\w{1,10})(?:[\s"'),]|$)`), // 文件路径
    regexp.MustCompile(`https?://[^\s"'<>)]+`),                       // URL
    regexp.MustCompile(`@([a-zA-Z_]\w+)`),                            // @mention
    regexp.MustCompile(`[《「]([^》」]+)[》」]`),                      // 中文书名号/引号
}
```

提取实体后：
1. 存入 insight 的 `entities` JSON 字段
2. 查找已有 insights 中包含相同实体的节点（SQL: `entities LIKE '%"实体名"%'`）
3. 每对节点每个共享实体创建一条 entity 边，metadata 存 `{"entity": "Qdrant"}`
4. 参考 MAGMA 代码：每个实体最多连 5 个已有节点（避免热门实体产生过多边）

**2.4 Causal 边（`internal/graph/causal.go`）**

采用 **MemCP 的关键词检测方案**，中英文扩展：
```go
var causalPattern = regexp.MustCompile(
    `(?i)\b(because|therefore|due to|caused by|as a result|decided to|` +
    `chosen because|so that|in order to|leads to|results in)\b|` +
    `(因为|所以|由于|导致|因此|决定|为了|以便)`
)
```

如果新 insight 匹配因果模式：
1. 取最近 10 条 insights（排除已删除）
2. 计算 token 重叠度（分词后 |交集| / max(|A|, |B|)）
3. 重叠 ≥ 0.15 → 创建 causal 边，weight 为重叠度

**2.5 related 命令（`cmd/related.go`）**
```
mnemon related <id> --edge entity --depth 2
```
从指定节点出发，按边类型做 BFS 遍历（参考 MAGMA graph_db.py 的 traverse 方法），返回关联节点列表（JSON）。

**2.6 remember 命令增强**
- 写入 insight 后调用 `graph.OnInsightCreated()`
- 输出中增加 `edges_created` 统计

**验证**：
- remember 3 条提到 "Qdrant" 的 insight → 检查自动创建了 entity 边
- remember 连续 2 条 → 检查 temporal 骨干链
- remember 1 条含 "because" → 检查 causal 边
- related 遍历验证

---

### Milestone 3：搜索增强 + diff 去重

**目标**：`search` 多策略搜索，`diff` 辅助 LLM 判断去重

**3.1 关键词搜索增强（`internal/search/keyword.go`）**
- token 化查询和内容（英文按空格+标点分词，中文按字符 bigram）
- 计算 `|intersection| / |query_tokens|` 得分
- 按得分排序返回

**3.2 search 命令（`cmd/search.go`）**
```
mnemon search "vector database preference" --limit 10
```
先用增强关键词搜索，后续可叠加 BM25。

**3.3 diff 命令（`cmd/diff.go`）**
```
mnemon diff "User now prefers Weaviate over Qdrant"
```
1. 对输入文本做关键词搜索，取 top 5 相似 insights
2. 计算相似度分数
3. 输出 JSON：
   ```json
   {
     "new_fact": "...",
     "similar": [
       {"id": "...", "content": "...", "similarity": 0.87, "suggestion": "CONFLICT"}
     ]
   }
   ```
4. suggestion 逻辑：
   - similarity > 0.9 → `DUPLICATE`
   - similarity > 0.5 且内容有矛盾信号（含否定词等）→ `CONFLICT`
   - similarity > 0.5 → `UPDATE`
   - 无相似结果 → `ADD`

**验证**：
- search 能找到相关记忆
- diff 能正确识别重复和冲突

---

### Milestone 4：Skill 接入 + 意图感知检索

**目标**：编写 memory.md skill，接入 Claude Code 实际使用；recall 支持意图感知

**4.1 recall 增强：意图感知**

借鉴 MAGMA 的 Adaptive Traversal，在 recall 时：
1. 检测查询意图（WHY / WHEN / ENTITY / GENERAL）— 关键词匹配
2. 先做关键词搜索找锚点
3. 从锚点做图遍历，按意图加权不同边类型
4. 合并排序返回

意图权重（参考 MAGMA 源码 `_probabilistic_beam_search` 中的 `attention_weights`）：
```
WHY:     causal=0.7, temporal=0.2, entity=0.05, semantic=0.05
WHEN:    temporal=0.7, causal=0.1, entity=0.1, semantic=0.1
ENTITY:  entity=0.6, semantic=0.3, temporal=0.05, causal=0.05
GENERAL: 均匀 0.25
```

**4.2 memory.md Skill**

编写 skill 文件，教 LLM 使用 mnemon CLI：
- recall 检索 → remember 存储 → diff 去重 → forget 清理
- ~100 tokens，对任何 LLM CLI 通用

**4.3 实际接入 Claude Code**
- 将 skill 放入 Claude Code 可加载的位置
- 真实对话中测试记忆的存取效果

**验证**：
- "为什么选了 Qdrant" → 优先返回 causal 关联的记忆
- Claude Code 通过 skill 成功调用 mnemon 命令

---

## 关键依赖

| 库 | 用途 | 版本 |
|----|------|------|
| `modernc.org/sqlite` | 纯 Go SQLite，零 CGO | latest |
| `github.com/spf13/cobra` | CLI 框架 | v1.8+ |
| `github.com/google/uuid` | UUID 生成 | v1.6+ |

**不引入**（MVP 阶段）：bleve（BM25）、embedding 模型、ONNX runtime

---

## 端到端验证

每个 Milestone 完成后的测试命令：

```bash
# M1: 基础 CRUD
mnemon remember "User prefers Qdrant for vector DB" --cat preference --imp 4
mnemon recall "vector database"
mnemon status
mnemon forget <id>

# M2: 图边
mnemon remember "Chose Qdrant because of Rust performance" --cat decision --imp 5
mnemon remember "Qdrant benchmark shows 10ms p99" --cat fact --imp 3
mnemon related <id> --edge entity
mnemon related <id> --edge causal

# M3: 搜索 + 去重
mnemon search "database performance"
mnemon diff "Switched from Qdrant to Weaviate"

# M4: Skill 接入
# 在 Claude Code 中通过 skill 自动调用以上命令
```

---

## 不在本计划范围内（后续迭代）

- Semantic 边（需要 embedding 模型）
- Daemon 模式（后台自动采集）
- Retention 生命周期（Archive → Purge）
- REST API / Web UI
- 多 agent 并发写入控制
- 记忆安全（来源标记、输入验证）
- RRF 融合检索（MAGMA 的多路合并）
- Beam Search 遍历（MAGMA 的高级遍历策略）
