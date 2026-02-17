# 05. MemCP 技术深度剖析

---

## 5.1 核心原理：RLM 框架

MemCP 实现了 MIT CSAIL 的 **RLM（Recursive Language Model）框架**（arXiv:2512.24601）。核心思想：Claude 不再被动接收检索到的文本块，而是主动导航到所需内容——内容留在磁盘上作为命名变量，按需加载。

**Token 效率对比：**

```
传统 RAG：把整个文档塞进 prompt → 吃掉大量 token
MemCP：  内容存磁盘当"变量" → Claude 只加载需要的片段

例：分析一个 50K token 的文档
  传统方式：消耗 50,460 tokens
  MemCP：  消耗 231 tokens（节省 99.5%）
```

---

## 5.2 MAGMA 4-图架构

MemCP 的知识图谱基于 MAGMA 模型，维护 4 种类型的边：

| 边类型 | 含义 | 查询示例 |
|--------|------|---------|
| **语义边 (Semantic)** | 基于相似度的关联 | "和数据库设计相关的决策有哪些？" |
| **时间边 (Temporal)** | 30 分钟窗口内创建的洞察之间的链接 | "今天上午讨论了什么？" |
| **因果边 (Causal)** | 通过语言标记检测（"因为"/"所以"） | "为什么选了 SQLite？" |
| **实体边 (Entity)** | 提及相同命名实体的洞察之间的链接 | "所有关于 API 的讨论" |

所有图数据持久化在 SQLite（`graph.db`）中。

**对比 Mem0 图谱**：Mem0 使用 Neo4j，仅支持单一关系类型的边。MemCP 的 4 种边类型在关联推理上显著更强。

---

## 5.3 21 个 MCP 工具（6 大类）

### 核心记忆（5 个）

| 工具 | 功能 |
|------|------|
| `memcp_ping` | 健康检查 + 统计信息 |
| `memcp_remember` | 保存洞察（category / importance / tags） |
| `memcp_recall` | 按 query / category / importance 过滤检索 |
| `memcp_forget` | 按 ID 删除洞察 |
| `memcp_status` | 查看记忆统计 |

### 上下文管理（8 个）

| 工具 | 功能 |
|------|------|
| `memcp_load_context` | 将内容存为磁盘命名变量 |
| `memcp_inspect_context` | 预览内容元数据（不加载全文） |
| `memcp_get_context` | 读取存储的内容或指定行范围 |
| `memcp_chunk_context` | 按策略分块（6 种：auto / lines / paragraphs / headings / chars / regex） |
| `memcp_peek_chunk` | 读取特定分块 |
| `memcp_filter_context` | 按正则提取匹配行 |
| `memcp_list_contexts` | 列出所有存储变量 |
| `memcp_clear_context` | 删除存储的上下文 |

### 搜索（1 个）

| 工具 | 功能 |
|------|------|
| `memcp_search` | 统一搜索，5 级降级（hybrid → BM25 → fuzzy → semantic → keyword） |

### 图谱（2 个）

| 工具 | 功能 |
|------|------|
| `memcp_related` | 从某个洞察出发，按语义/时间/因果/实体边遍历图谱 |
| `memcp_graph_stats` | 查看节点数量和热门实体 |

### 生命周期（3 个）

| 工具 | 功能 |
|------|------|
| `memcp_retention_preview` | 干跑模式，预览哪些将被归档/清除 |
| `memcp_retention_run` | 执行 3-区生命周期（Active → Archive → Purge） |
| `memcp_restore` | 重新激活已归档项 |

### 多项目（2 个）

| 工具 | 功能 |
|------|------|
| `memcp_projects` | 列出所有项目及洞察/上下文/session 数量 |
| `memcp_sessions` | 查看 session，可按项目过滤 |

---

## 5.4 Context-as-Variable（RLM 模式）

MemCP 不将大文档加载到 prompt 中，而是存储在磁盘上。Claude 只看到元数据（类型、大小、token 数），按需加载特定分块。

**官方基准测试：**

| 场景 | 原生 Claude Code | MemCP | 改善 |
|------|-----------------|-------|------|
| `/compact` 后信息保留 | ~5% | **100%** | — |
| 3 次 compact 后 | ~2% | **100%** | — |
| 跨 session 召回 | 0% | **92%** | — |
| 加载 50 条洞察的 token 消耗 | 896 | 167 | **5.4x** |
| Reload 500 条洞察 | 9,380 | 462 | **20.3x** |
| 分析 50K 文档的 token 消耗 | 50,460 | 231 | **218x** |
| 交叉引用知识 | 1,861 | 172 | **10.8x** |
