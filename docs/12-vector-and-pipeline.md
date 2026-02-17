# 12. 向量检索层与学习 Pipeline

---

## 12.1 为什么需要向量层

MemCP 的 5 级降级搜索（hybrid → BM25 → fuzzy → semantic → keyword）在千~万级洞察规模下表现优秀。但当原始素材达到十万~百万级时，需要专门的向量数据库：

| 场景 | MemCP 能否覆盖 | 向量 DB 的价值 |
|------|--------------|---------------|
| 精炼后的洞察检索 | 完全覆盖 | 无需 |
| 大量推文原文检索 | 性能不足 | 高效语义搜索 |
| 长文档/文章库检索 | Context-as-Variable 可用 | 跨文档关联搜索 |
| "模糊"语义匹配 | BM25 + fuzzy 基本够 | embedding 更精准 |

---

## 12.2 Qdrant 作为可选增强

**为什么选 Qdrant：**

| 方案 | 优势 | 劣势 | 推荐度 |
|------|------|------|--------|
| **Qdrant** | Rust 编写，性能最佳，官方 MCP Server，Docker 一行启动 | 需独立进程 | ★★★★★ |
| **ChromaDB** | 最简单，嵌入式，零配置 | Python 生态，规模上限 <10M | ★★★ |
| **sqlite-vec** | 与 MemCP 共享 SQLite，最轻量 | 性能一般，大规模不佳 | ★★★ |

---

## 12.3 双 MCP 协作模式

```
Claude Code CLI
    │
    ├── MCP Server 1: MemCP（知识图谱层）
    │   ├─ remember / recall / forget     ← 结构化洞察
    │   ├─ MAGMA 4-图遍历                 ← 因果/时间/语义/实体关联
    │   ├─ context 管理                   ← 大文档按需加载
    │   └─ retention lifecycle            ← 记忆生命周期
    │
    ├── MCP Server 2: Qdrant（语义检索层）
    │   ├─ store_document                 ← 批量文档摄入
    │   ├─ semantic_search                ← 大规模语义搜索
    │   └─ hybrid_search (BM25+向量)      ← 精确匹配+语义
    │
    └── Claude 自主决策调用哪个工具
        "这个问题需要因果推理" → memcp_related (因果边)
        "找一下相关文档"       → semantic_search (向量)
        "记住这个发现"         → memcp_remember (图谱)
```

---

## 12.4 何时引入向量层

向量层是**可选增强**，不是必需组件。引入时机：

- MemCP 洞察数量超过 5,000 且搜索响应变慢时
- 需要大量原始文档的语义检索时
- 需要跨文档/跨渠道的关联搜索时

---

## 12.5 Scheduler 驱动的多渠道内容摄入

```
  各渠道内容（Twitter / WeChat / RSS / 文档）
       │
       ▼
  scheduler（定时拉取）
       │
       ▼
  ┌─────────────────────────────────┐
  │  Claude Code 处理               │
  │  1. 原始内容 → Qdrant           │  存原文 + embedding
  │  2. LLM 提炼关键洞察            │
  │  3. 洞察 → MemCP remember       │  存入知识图谱
  │  4. 自动建立关联（因果/实体）     │  MAGMA 图自动连边
  └─────────────────────────────────┘
       │
       ▼
  后续使用时：
  Claude: "找相关文档"           → Qdrant semantic_search
  Claude: "为什么当时这样决定"   → MemCP memcp_related (因果边)
  Claude: "最近学到了什么"       → MemCP memcp_recall (时间边)
```

---

## 12.6 批量文档摄入流程

```
流程:
1. scheduler 定时触发（如每 4 小时）
2. 从 Twitter / WeChat / RSS 拉取新内容
3. 调用 Claude Code CLI:
   chat -m "以下是今天从 Twitter 拉取的新内容，
   请按照 MEMORY PROTOCOL 处理：提取事实 → 去重 → 存储。
   [内容]"
4. Claude 自动调用 memcp_remember / memcp_recall / memcp_forget
5. 高价值内容同步写入 Qdrant（如已启用）
6. 完成
```

---

## 12.7 Cowork 模式下的学习 Pipeline

在 Cowork 架构下，学习 Pipeline 天然由记忆实例承担：

```
  各渠道内容（Twitter / WeChat / RSS / 文档）
       │
       ▼
  scheduler（定时拉取）
       │
       ├─ 原始内容 → Qdrant（存原文 + embedding）
       │
       ▼
  记忆实例（潜意识层）处理：
  ┌─────────────────────────────────┐
  │  1. 读取新拉取的内容             │
  │  2. 执行 MEMORY PROTOCOL        │
  │     提取 → 去重 → 冲突解决       │
  │  3. 洞察 → MemCP remember       │  存入知识图谱
  │  4. 自动建立关联（因果/实体）     │  MAGMA 图自动连边
  └─────────────────────────────────┘
       │
       ▼
  主实例后续使用时：
  Claude: "找相关文档"           → Qdrant semantic_search
  Claude: "为什么当时这样决定"   → MemCP memcp_related (因果边)
  Claude: "最近学到了什么"       → MemCP memcp_recall (时间边)
```

记忆实例同时承担两个 scheduler 任务：
- **memory-processing**：处理用户对话记录 → 提取记忆（每 30 分钟）
- **learning**：处理外部渠道内容 → 提取记忆（每 4 小时）

两个任务共享同一套 MEMORY PROTOCOL，只是输入源不同。

---

## 12.8 与现有 Scheduler 的集成

| 现有任务 | 说明 | 记忆增强 |
|---------|------|---------|
| memory-consolidation | 4 小时整理 MEMORY.md | 同步执行 MemCP 记忆协议 |
| evening-summary | 晚间总结 | 总结的洞察写入 MemCP |
| daily-briefing | 早间简报 | 从 MemCP recall 获取上下文 |
| **新增: memory-processing** | 每 30 分钟处理对话记录 | 记忆实例执行 MEMORY PROTOCOL |
| **新增: learning** | 每 4 小时拉取渠道内容 | 记忆实例执行 MEMORY PROTOCOL |
