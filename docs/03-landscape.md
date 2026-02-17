# 03. 现有方案概览与对比

---

## 3.1 架构拓扑

```
Mem0:    [Agent + 记忆读写] → 同步管线（库/SDK 嵌入调用链）
Letta:   [主 Agent] + [Sleep-time Agent] → 同一框架内
MemCP:   [MCP Plugin] → 挂载在 Claude Code 上，单消费者
Mnemon:  [独立 Daemon] ←→ [多维护 Agent] ←→ [多种消费者]
```

架构拓扑从**星型**（agent 中心 + 内置记忆）变成**网状**（多独立 agent + 共享记忆层）。

---

## 3.2 Mem0 的局限

- 维护者和使用者是同一 agent
- 库/SDK 嵌入在 agent 调用链里，同步管线
- LLM 只作为"提取函数"被调用一次
- "Dynamic forgetting"和"priority scoring"是规则引擎，非智能体判断

---

## 3.3 Letta/MemGPT 的局限

- 原版：self-editing memory，agent 通过 tool calling 管理自己记忆
- 新版：引入 sleep-time agents，异步整理记忆（维护者已分离）
- 但 sleep-time agent 仍在 Letta 框架内部，操作同一 agent 的 memory blocks
- 更像"意识"和"潜意识"两个线程，不是外部独立服务

### Letta 的三层记忆设计（参考）

```
┌─────────────────────────────────────┐
│  上下文内记忆（Context Window 内）      │
│  ├─ System Prompt（系统指令）          │
│  ├─ Core Memory（可读写记忆块）        │  ← Agent 可自主编辑
│  └─ Working Context（当前对话）        │
├─────────────────────────────────────┤
│  上下文外记忆（持久化存储）              │
│  ├─ Recall Memory（全部对话历史）      │  ← 无限长度，语义搜索
│  └─ Archival Memory（归档知识）        │  ← 无限长度，重要信息
└─────────────────────────────────────┘
```

关键启发：**Agent 自己决定何时读写记忆**（通过工具调用），而非开发者硬编码。这正是 MEMORY PROTOCOL 的设计哲学。

---

## 3.4 MemCP（maydali28/memcp）

近期发布的 MAGMA MCP 实现。实现了四图结构（Semantic/Temporal/Causal/Entity），用 SQLite 存储，21 个 MCP tools，支持 sub-agent 编排。但：

- **是 MCP plugin，不是 daemon**——生命周期绑定 Claude Code 会话
- **单消费者**——服务于一个 Claude Code 实例
- **同步维护**——sub-agent 在同一 MCP 调用链里运行，非独立后台进程
- **绑定 Claude Code**——非 LLM-agnostic

MemCP 验证了 MAGMA 四图架构在工程上 feasible，Mnemon 要做的是把它从"某个 IDE 的插件"提升到"独立基础设施"。

---

## 3.5 Mnemon 的结构性优势

- 独立 daemon 天然解决多 agent 共享（架构起点而非后加功能）
- Graph 原生支持关系查询、演化查询、矛盾检测
- LLM 交互式维护提供主动记忆管理空间
- 优势主要体现在**上限更高、扩展空间更大**，真正差异在时间维度显现

---

## 3.6 MemCP 与 Mem0 的逐项对比

| 能力维度 | MemCP | Mem0 |
|---------|-------|------|
| **自动事实提取** | 需手动 `remember` | LLM 自动提取 |
| **智能去重** | 无 | LLM 对比判断 |
| **冲突解决** | 无 | LLM 裁决 ADD/UPDATE/DELETE |
| **图谱存储** | MAGMA 4-图（4 种边） | Neo4j（单一关系边） |
| **向量搜索** | 可选 model2vec（本地） | 内置 embedding |
| **大文档处理** | Context-as-Variable（极致省 token） | 无 |
| **记忆生命周期** | 3-区（Active/Archive/Purge） | 无 |
| **5 级降级搜索** | hybrid → BM25 → fuzzy → semantic → keyword | 仅向量搜索 |
| **外部依赖** | 零 API Key | 需 LLM Key + DB |

**结论**：MemCP 在图谱、大文档处理、生命周期管理上领先；Mem0 在自动提取、去重、冲突解决上领先。两者互补性极强。

---

## 3.7 需要诚实的地方

- 图谱构建质量仍依赖 LLM 的 entity/relation extraction 准确率
- 向量检索在模糊语义匹配上有不可替代优势——可能需要 graph + vector 混合
- 遗忘和衰减策略是未解问题
