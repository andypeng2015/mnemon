# 13. 架构演进：Binary + Skill 方案

---

## 13.1 从 MCP Server 到独立 Binary

早期架构基于 MemCP（Python MCP Server）+ Claude Code CLI 的组合。这个方案可以工作，但存在三个根本约束：

| 约束 | 原因 |
|------|------|
| **LLM 绑定** | MEMORY PROTOCOL 写在 Claude 的 System Prompt 里，换 LLM 就得重写 |
| **运行时重** | Python 解释器 + MCP JSON-RPC 协议 + 额外进程 |
| **Token 浪费** | MEMORY PROTOCOL ~500 tokens 常驻 System Prompt，无论是否需要 |

核心思路：**把记忆管理逻辑从"System Prompt 中的协议"变成"一个可执行的 Go binary"，通过 Skill 教任何 LLM CLI 怎么调用它**。

```
当前架构：
  Claude Code CLI（特定 LLM）
    → System Prompt 内嵌 MEMORY PROTOCOL（~500 tokens 常驻）
    → MCP JSON-RPC 协议（额外进程，Python 运行时开销）
    → MemCP（Python，需要 pip install）

演进架构：
  任意 LLM CLI
    → Skill: memory.md（~100 tokens，按需加载）
    → $ mnemon（单个 Go binary，零依赖）
    → 内嵌 SQLite + MAGMA 图（一切自包含）
```

---

## 13.2 Binary 命令接口设计

### 第一类：纯存储操作（Binary 自主完成，不需要 LLM）

```bash
# 记忆检索
$ mnemon recall "query keywords" --limit 10 --cat preference
# → 返回匹配的记忆列表（JSON）

# 存储新记忆
$ mnemon remember "User prefers Qdrant for vector DB" \
    --cat preference --imp 4 --tags "tool,database" --source owner
# → 返回新记忆 ID，自动建立 MAGMA 图连边

# 删除记忆
$ mnemon forget <memory-id>

# 图谱遍历
$ mnemon related <memory-id> --edge causal --depth 2
# → 返回因果链上的关联记忆

# 统计信息
$ mnemon status
# → 记忆总数、分类分布、图谱边数、存储大小

# 生命周期管理
$ mnemon retention --preview    # 预览将被归档/清除的记忆
$ mnemon retention --run        # 执行 Active → Archive → Purge

# 统一搜索（5 级降级）
$ mnemon search "query" --mode hybrid
# → hybrid → BM25 → fuzzy → semantic → keyword 自动降级
```

### 第二类：辅助 LLM 决策（Binary 提供上下文，LLM 做判断）

```bash
# 去重检查：返回已有相似记忆，供 LLM 判断 ADD/UPDATE/NOOP
$ mnemon diff "User now prefers Weaviate over Qdrant"
# → 返回：
# {
#   "new_fact": "User now prefers Weaviate over Qdrant",
#   "existing_similar": [
#     {"id": "m-42", "content": "User prefers Qdrant for vector DB", "similarity": 0.87}
#   ],
#   "suggestion": "CONFLICT: existing memory m-42 contradicts new fact"
# }
# LLM 根据返回结果决定：forget m-42 + remember new

# 批量预处理：从文本中提取候选 fact（轻量本地模型）
$ mnemon extract < conversation.txt
# → 返回候选 fact 列表，供 LLM 确认/修改/丢弃
```

---

## 13.3 Skill 模板

Skill 变得极其简洁——只教 LLM 怎么调命令，不教完整的 MEMORY PROTOCOL：

```markdown
---
name: memory
description: Persistent memory management via mnemon binary
user-invocable: false
metadata:
  requires:
    bins: ["mnemon"]
---
# Memory Management

## Retrieving Memories
Before answering questions that may benefit from past context:
  $ mnemon recall "<keywords>" --limit 10
  $ mnemon related <id> --edge causal

## Storing New Information
When you learn important new information from the user:
1. Check for duplicates:
   $ mnemon diff "<the new fact>"
2. If no conflict (suggestion: ADD):
   $ mnemon remember "<fact>" --cat <category> --imp <1-5>
3. If conflicts with existing (suggestion: CONFLICT):
   $ mnemon forget <old_id>
   $ mnemon remember "<updated fact>" --cat <category> --imp <1-5>
4. If identical (suggestion: DUPLICATE): skip

## Categories
preference | decision | fact | insight | context

## Importance: 1(ephemeral) to 5(core identity)
```

**~100 tokens**，对比当前 MEMORY PROTOCOL 的 ~500 tokens，省 80%。并且这个 Skill 对任何 LLM 都通用——它只是在教"怎么调 CLI 命令"。

---

## 13.4 LLM 可移植性

Binary + Skill 的组合可以适配任何 LLM CLI：

```
┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐
│  Claude Code CLI  │    │  OpenClaw/Nanobot │    │  Cursor / Aider  │
│                  │    │                  │    │                  │
│  skill:          │    │  skill:          │    │  rules /         │
│   memory.md      │    │   memory.md      │    │   convention     │
│  (Markdown)      │    │  (SKILL.md+YAML) │    │  (各自格式)       │
└────────┬─────────┘    └────────┬─────────┘    └────────┬─────────┘
         │                       │                       │
         └───────────┬───────────┘───────────────────────┘
                     │
                     ▼
              $ mnemon
              (同一个 binary，同一套命令)
                     │
                     ▼
              SQLite + MAGMA 图
              (同一份数据)
```

**关键优势**：不同 LLM CLI 共享同一份记忆数据。用 Claude Code 积累的知识，在 Cursor 中也能检索到。这在 MCP 方案中也能做到（多个 CLI 连同一个 MCP Server），但 binary 方案更简单——不需要运行额外的服务进程。

---

## 13.5 Cowork 模式的进化：Daemon 模式

Binary 可以同时扮演"潜意识层"角色，替代"另一个 Claude CLI 实例"的设计：

```
当前 Cowork：
  主 CLI（意识层）  +  记忆 CLI（潜意识层，另一个 LLM 实例）
  成本：需要额外的 LLM API 调用（即使用 Haiku，仍有成本）

Binary Cowork：
  主 CLI（意识层）  +  mnemon daemon（潜意识层，本地进程）
  成本：接近零（本地 binary，可选嵌入小模型做基础提取）
```

```bash
# 启动 daemon 模式
$ mnemon daemon \
    --watch workspace/memory/ \
    --interval 30m \
    --auto-extract \
    --source daily-log

# daemon 自动执行：
# 1. 监听 daily log 文件变化
# 2. 用内嵌轻量模型做基础 entity/fact 提取
# 3. 调 mnemon diff 检查去重
# 4. 基础情况自动处理（明显的 ADD/DUPLICATE）
# 5. 复杂冲突标记为 pending，等主 CLI 的 LLM 下次交互时确认
```

**分层处理策略：**

| 场景 | 处理者 | 示例 |
|------|--------|------|
| 明显新事实 | daemon 自动 ADD | "今天部署了 v2.0" |
| 完全重复 | daemon 自动 NOOP | 与已有记忆内容相同 |
| 轻微更新 | daemon 自动 UPDATE | "API 延迟从 200ms 降到 150ms" |
| 语义冲突 | 标记 pending → LLM 确认 | "不再使用 Qdrant" vs 已有 "偏好 Qdrant" |
| 复杂判断 | 标记 pending → LLM 确认 | 涉及偏好/决策变更的模糊情况 |

```bash
# 主 CLI 启动时自动检查 pending 项
$ mnemon pending
# → 列出需要 LLM 确认的记忆操作
# → Skill 教 LLM 在会话开始时处理这些 pending 项
```

---

## 13.6 Binary 内部架构

```
mnemon binary
├── cmd/                        # CLI 命令入口
│   ├── recall.go               # 检索
│   ├── remember.go             # 存储
│   ├── forget.go               # 删除
│   ├── related.go              # 图遍历
│   ├── diff.go                 # 去重辅助
│   ├── extract.go              # 文本预处理
│   ├── search.go               # 5 级降级搜索
│   ├── retention.go            # 生命周期
│   ├── daemon.go               # 后台监听模式
│   └── status.go               # 统计
│
├── internal/
│   ├── graph/                  # MAGMA 4-图实现
│   │   ├── semantic.go         # 语义边（embedding 相似度）
│   │   ├── temporal.go         # 时间边（30 分钟窗口）
│   │   ├── causal.go           # 因果边（语言标记检测）
│   │   └── entity.go           # 实体边（命名实体共现）
│   │
│   ├── search/                 # 搜索引擎
│   │   ├── bm25.go             # BM25 全文搜索
│   │   ├── fuzzy.go            # 模糊匹配
│   │   ├── semantic.go         # 语义搜索（本地 embedding）
│   │   └── hybrid.go           # 混合搜索 + 5 级降级
│   │
│   ├── storage/                # 存储层
│   │   └── sqlite.go           # SQLite WAL 模式
│   │
│   ├── retention/              # 生命周期
│   │   └── lifecycle.go        # Active → Archive → Purge
│   │
│   ├── extract/                # 轻量提取（daemon 用）
│   │   ├── entity.go           # 正则 + 规则 entity 提取
│   │   └── local_model.go      # 可选：ONNX 小模型推理
│   │
│   └── security/               # 记忆安全
│       ├── source.go           # 来源标记（owner/external/visitor）
│       ├── validation.go       # 输入验证
│       └── audit.go            # 审计日志
│
└── go.mod                      # 最小依赖
    ├── modernc.org/sqlite      # 纯 Go SQLite（零 CGO）
    ├── github.com/blevesearch/bleve  # BM25 搜索
    └── github.com/agnivade/levenshtein  # fuzzy 匹配
```

**关键依赖选择**：

| 功能 | 库 | 特点 |
|------|-----|------|
| SQLite | `modernc.org/sqlite` | 纯 Go 实现，零 CGO，交叉编译友好 |
| BM25 | `blevesearch/bleve` | 成熟的 Go 全文搜索引擎 |
| Fuzzy | `agnivade/levenshtein` | 轻量编辑距离 |
| Semantic | 可选 ONNX Runtime | 本地 embedding 模型（如 all-MiniLM-L6-v2） |
| CLI | `spf13/cobra` | Go 标准 CLI 框架 |

---

## 13.7 效率对比

| 维度 | MemCP + MCP + System Prompt | Binary + Skill |
|------|----------------------------|----------------|
| **主实例 token 开销** | ~500 tokens（MEMORY PROTOCOL 常驻） | ~100 tokens（Skill 按需加载） |
| **记忆操作延迟** | MCP JSON-RPC + Python 解析 | 直接 shell 调用 Go binary |
| **潜意识层成本** | Haiku API 调用（~Opus 的 1/60） | 本地 binary daemon，接近零 |
| **部署依赖** | Python + pip + MemCP + MCP 协议 | 单个 binary（`go build`） |
| **LLM 可移植性** | 绑定 Claude System Prompt 格式 | 任何能运行 shell 命令的 CLI |
| **运行时进程** | 2-3 个（主 CLI + 记忆 CLI + MCP Server） | 2 个（主 CLI + binary daemon） |
| **启动时间** | Python 解释器 + 模块加载（~1-2s） | Go binary 即启即用（~10ms） |
| **存储格式** | MemCP 的 SQLite schema（外部控制） | 自定义 schema（完全可控） |
| **跨 CLI 共享记忆** | 需多 CLI 连同一 MCP Server | 天然共享（同一 SQLite 文件） |

---

## 13.8 与 MemCP 方案的关系

Binary 方案不是推翻 MemCP 方案，而是**渐进演进**：

```
Phase 1（当前计划）：
  使用 MemCP 做 MVP 验证
  → 验证 MAGMA 4-图、搜索、retention 的设计是否有效
  → 验证 Skill 教 LLM 做记忆管理是否可行
  → 低成本快速迭代

Phase 2-3（演进方向）：
  将验证有效的设计用 Go 重新实现为 mnemon binary
  → 参考 MemCP 的 MAGMA 图算法和搜索策略
  → 加入 daemon 模式替代"第二个 CLI 实例"
  → 加入记忆安全层（来源标记、输入验证）
  → 发布为独立可分发工具
```

**不需要从零开始**——MemCP 是我们的设计验证工具，binary 是最终的生产形态。MemCP 的 Python 代码是 Go 实现的参考蓝本。

---

## 13.9 这意味着什么

```
┌──────────────────────────────────────────────────────────────┐
│  OpenClaw 哲学: "最好的 agent 框架就是没有框架"                  │
│       ↓                                                      │
│  框架极简 → while loop + tools + skills                       │
│       ↓                                                      │
│  差异化聚焦 → Memory 是核心投资                                 │
│       ↓                                                      │
│  Memory 的最佳载体 → 不是 MCP Server，不是 System Prompt        │
│                      而是一个独立的、自包含的、可分发的 binary    │
│       ↓                                                      │
│  最终形态:                                                    │
│    mnemon（Go binary）= 记忆能力的"器官"                       │
│    memory.md（Skill）= 教任何 LLM 使用这个器官的"教材"          │
│    mnemon daemon = 自主运行的"潜意识"                           │
│                                                              │
│  一个 binary，一份 skill，适配所有 LLM，共享所有记忆             │
└──────────────────────────────────────────────────────────────┘
```
