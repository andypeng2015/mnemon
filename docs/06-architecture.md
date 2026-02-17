# 06. Mnemon 核心架构设计

---

## 6.1 设计原语

Mnemon 是一个**独立运行的 memory daemon**，而非嵌入某个 agent 框架的库或插件：

- 24/7 持久化运行的 source of truth
- 多接口消费：LLM-CLI（via skills）、REST API（web/apps）、CLI（开发者）
- Graph-native 数据结构（knowledge graph），非 vector store
- LLM-agnostic，不绑定特定 LLM-CLI 实现

---

## 6.2 通过 Skill 接入 LLM-CLI

```
[Claude Code] ──skill──→ [Mnemon Daemon] ←──REST API──── [Web UI]
[Cursor]      ──skill──→ [Mnemon Daemon] ←──CLI────────── [Developer]
[其他 Agent]  ──skill──→ [Mnemon Daemon] ←──其他 Agent── [维护 Agent]
```

Skill 是 LLM-CLI 连接外部能力的标准方式之一，不改变 Mnemon 作为独立服务的本质。类似 MMO World Server 可通过 TCP/HTTP/WebSocket 等多种协议连接。

---

## 6.3 关键设计突破：LLM 主动记忆管理

传统方案（Mem0/Letta）中，记忆写入/遗忘由系统规则决定。Mnemon 通过 skill 让 LLM 在推理过程中**主动**决定：

- 什么值得写入图谱
- 如何关联已有实体
- 何时召回哪些记忆
- 旧记忆是否被新信息推翻

遗忘不再需要预设规则（如"30天未访问就衰减"）——LLM 在检索到旧记忆时自己判断是否过时并主动更新/删除。记忆维护和推理决策用同一智能体、同一上下文。

---

## 6.4 整体架构：MemCP + Claude Code CLI

用 Claude Code CLI 的 LLM 推理能力，补齐 MemCP 缺失的 3 个核心能力：

| Mem0 能力 | MemCP 现状 | Claude Code CLI 怎么补 |
|-----------|-----------|----------------------|
| **自动事实提取** | 需手动 `remember` | Claude 读内容 → 提取 facts → 调 `memcp_remember` |
| **智能去重** | 无 | Claude 先 `memcp_recall` 搜索 → 判断是否重复 → 决定 ADD/UPDATE/NOOP |
| **冲突解决** | 无 | Claude 对比新旧 → 决定 UPDATE 旧记忆还是 DELETE |

### 整体架构图

```
┌─────────────────────────────────────────────────────────┐
│                    Claude Code CLI                       │
│                                                         │
│  System Prompt 内置"记忆管理协议"：                       │
│  ┌─────────────────────────────────────────────────┐    │
│  │ MEMORY PROTOCOL:                                 │    │
│  │ 1. Extract facts from any new information        │    │
│  │ 2. For each fact: search existing memories       │    │
│  │ 3. Decide: ADD / UPDATE / DELETE / NOOP          │    │
│  │ 4. Execute via MemCP tools                       │    │
│  │ 5. Log decisions for audit                       │    │
│  └─────────────────────────────────────────────────┘    │
│         │                              │                 │
│         ▼                              ▼                 │
│  ┌─────────────┐              ┌──────────────┐          │
│  │ MCP: MemCP   │              │ MCP: Qdrant   │         │
│  │ (知识图谱层)  │              │ (向量检索层)   │         │
│  │              │              │               │         │
│  │ remember     │   ◄─ 精炼 ─► │ store_doc     │         │
│  │ recall       │              │ search        │         │
│  │ forget       │              │ hybrid_search │         │
│  │ related      │              │               │         │
│  │ retention    │              │               │         │
│  └──────┬───────┘              └──────┬────────┘         │
│         │                              │                 │
│         ▼                              ▼                 │
│   SQLite + MAGMA 图                Qdrant (Docker)       │
│   (高价值洞察+关联)              (原始文档+embedding)     │
└─────────────────────────────────────────────────────────┘
          │
          │ scheduler 定时投喂
          │
    ┌─────┴──────────────────┐
    │  Twitter  │  WeChat     │
    │  RSS      │  文档/网页   │
    └────────────────────────┘
```

---

## 6.5 双层分工

| 维度 | MemCP（图谱层） | 向量 DB（检索层） |
|------|----------------|------------------|
| **存什么** | 高价值洞察、决策、因果链 | 原始文档、文章、聊天记录 |
| **规模** | 千~万级（精炼后的知识） | 十万~百万级（原始素材） |
| **搜索方式** | 图遍历 + 5 级降级搜索 | 向量相似度 + BM25 混合 |
| **关联能力** | 强（4 种边类型） | 弱（仅相似度） |
| **角色** | "大脑皮层"——推理和关联 | "海马体"——海量存取 |

---

## 6.6 自动事实提取

```
触发时机：每次收到新内容（对话/文档/渠道数据）
                │
                ▼
  Claude Code System Prompt 中内置指令：
  ┌─────────────────────────────────────────────┐
  │ "当你处理任何新信息时，自动执行以下步骤：     │
  │  1. 提取关键事实（人物/偏好/决策/事件）       │
  │  2. 对每条事实调用 memcp_recall 搜索已有记忆  │
  │  3. 如果是新事实 → memcp_remember            │
  │  4. 如果与旧记忆冲突 → 更新旧记忆            │
  │  5. 如果重复 → 跳过"                         │
  └─────────────────────────────────────────────┘
```

---

## 6.7 智能去重 + 冲突解决

```
Claude 提取出 fact: "用户偏好用 Qdrant 做向量数据库"
        │
        ▼
调用 memcp_recall(query="用户 向量数据库 偏好")
        │
        ▼
  ┌─ 无相似结果 ─→ memcp_remember(新 fact)         [ADD]
  │
  ├─ 找到: "用户在考虑 ChromaDB"
  │   → 判断: 信息已更新，旧的过时
  │   → memcp_forget(旧 ID) + memcp_remember(新)    [UPDATE]
  │
  ├─ 找到: "用户偏好 Qdrant"
  │   → 判断: 完全重复
  │   → 跳过                                        [NOOP]
  │
  └─ 找到: "用户不喜欢 Qdrant"
      → 判断: 直接矛盾，新信息更可信
      → memcp_forget(旧) + memcp_remember(新)       [DELETE+ADD]
```

---

## 6.8 多 MCP 配置

```json
{
  "mcpServers": {
    "memcp": {
      "command": "python",
      "args": ["-m", "memcp"],
      "env": {
        "MEMCP_PROJECT": "mygoals"
      }
    },
    "qdrant-memory": {
      "command": "npx",
      "args": ["-y", "@qdrant/mcp-server-qdrant"],
      "env": {
        "QDRANT_URL": "http://localhost:6333",
        "COLLECTION_NAME": "mygoals_knowledge"
      }
    }
  }
}
```

Claude Code 原生支持多 MCP Server 并行运行，自主根据工具描述决定调用哪个。

---

## 6.9 与 Mem0 的组件级等价

| Mem0 组件 | 我们的等价实现 | 备注 |
|-----------|--------------|------|
| `FACT_RETRIEVAL_PROMPT` | Claude System Prompt 中的 MEMORY PROTOCOL Step 1 | Claude 推理能力更强 |
| `get_update_memory_messages()` | Claude 自主对比 `memcp_recall` 结果和新 fact | Step 2-3 |
| LLM 判断 ADD/UPDATE/DELETE/NOOP | Claude 的推理（同为 LLM，但 Opus/Sonnet 强于 GPT-4o-mini） | Step 3 |
| Vector Store (Qdrant/Chroma) | Qdrant MCP Server（可选） | 详见 [12-vector-and-pipeline.md](12-vector-and-pipeline.md) |
| Graph Store (Neo4j) | MemCP MAGMA 4-图 | **更强**：4 种边 vs 单一关系边 |
| History/Audit Log | MemCP session tracking + workspace 日志 | Step 5 |
| Embedding Model | MemCP 内置 model2vec（本地）或 Qdrant embedding | 零外部依赖 |
| `ThreadPoolExecutor` 并行 | Claude Code 自主决定工具调用顺序 | 原生并行工具调用 |

---

## 6.10 能力级对比

| 能力 | Mem0 | MemCP + Claude Code CLI |
|------|------|------------------------|
| 自动事实提取 | ✅ 内置 | ✅ System Prompt 协议 |
| 智能去重 | ✅ 内置 | ✅ recall + LLM 判断 |
| 冲突解决 | ✅ 内置 | ✅ 对比 + LLM 裁决 |
| 图谱记忆 | ✅ Neo4j（单关系） | ✅ MAGMA 4-图（**更强**） |
| 向量搜索 | ✅ 内置 | ✅ Qdrant MCP（可选） |
| 大文档处理 | ❌ 无 | ✅ Context-as-Variable |
| 记忆生命周期 | ❌ 无 | ✅ 3-区 retention |
| 5 级降级搜索 | ❌ 仅向量 | ✅ hybrid→BM25→fuzzy→semantic→keyword |
| 零外部依赖 | ❌ 需 LLM Key + DB | ✅ 完全本地 |
| 多项目隔离 | ✅ user_id 过滤 | ✅ project 隔离 |

---

## 6.11 我们方案的独特优势

1. **图谱能力更强**——MAGMA 4-图（语义/时间/因果/实体）远超 Mem0 的单一关系边
2. **LLM 能力更强**——Claude Opus/Sonnet 的推理通常优于 Mem0 默认的 GPT-4o-mini
3. **零额外依赖**——不需要 Mem0 账号，不需要 OpenAI Key
4. **完全可控**——所有逻辑在 System Prompt 中，随时可调
5. **Context-as-Variable**——MemCP 独有的大文档按需加载，Mem0 不具备
6. **渐进式增强**——从零依赖起步，按需引入 Qdrant，不做一次性大投入
7. **Cowork 成本优化**——记忆管理用 Haiku（成本 1/60），主对话用 Opus/Sonnet，混合模型策略降低总成本
