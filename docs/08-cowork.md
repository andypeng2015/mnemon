# 08. Cowork 双实例异步架构

---

## 8.1 单实例方案的问题

将"用户对话"和"记忆管理"放在同一个 Claude Code CLI 实例中，导致几个实际问题：

| 问题 | 影响 |
|------|------|
| **每次交互延迟增加** | MEMORY PROTOCOL 要求 3-5 次额外 MCP tool call（recall → 判断 → remember/forget） |
| **System Prompt 占用上下文** | MEMORY PROTOCOL 完整定义约 500 tokens，挤占对话可用空间 |
| **LLM 注意力分散** | 同时处理"回答用户"和"管理记忆"两个目标，两者都做不到最好 |
| **成本放大** | 每次对话的 token 消耗增加 30-50%（记忆操作的额外开销） |

---

## 8.2 Cowork 设计：双实例异步架构

核心思路：**启动一个独立的 Claude Code CLI 实例专门负责记忆管理**，主实例只专注用户对话。两个实例通过共享 MemCP 存储层通信。

```
┌─────────────────────────┐         ┌─────────────────────────┐
│  主实例（意识层）          │         │  记忆实例（潜意识层）      │
│  Conscious Agent         │         │  Subconscious Agent     │
│                         │         │                         │
│  ┌───────────────────┐  │         │  ┌───────────────────┐  │
│  │ System Prompt:     │  │         │  │ System Prompt:     │  │
│  │ "回答前先用        │  │         │  │ 完整 MEMORY        │  │
│  │  memcp_recall 搜索 │  │         │  │ PROTOCOL           │  │
│  │  相关记忆"         │  │         │  │ (5 步完整流程)      │  │
│  │                   │  │         │  │                   │  │
│  │ ~50 tokens        │  │         │  │ ~500 tokens        │  │
│  └───────────────────┘  │         │  └───────────────────┘  │
│                         │         │                         │
│  职责：                  │         │  职责：                  │
│  • 用户对话              │         │  • 事实提取              │
│  • 读取 MemCP（只读）    │         │  • 智能去重              │
│  • 写入 daily log       │         │  • 冲突解决              │
│  • 响应快、上下文干净    │         │  • 写入 MemCP            │
│                         │         │  • retention 管理        │
│  模型：Opus / Sonnet    │         │  模型：Haiku / Sonnet    │
│  （需要最强推理）         │         │  （大部分任务 Haiku 够用）  │
└────────────┬────────────┘         └────────────┬────────────┘
             │                                    │
             │          ┌──────────┐              │
             │          │  MemCP   │              │
             └─── 读 ──→│  (共享)  │←── 读+写 ────┘
                        │          │
                        │ SQLite + │
                        │ MAGMA 图 │
                        └──────────┘
```

这本质上就是 **Letta（MemGPT）的 "Subconscious Agent" 模式**——Letta 的 AI Memory SDK 用一个后台 agent 异步处理消息并更新 memory blocks，与我们的设计完全同构。区别是 Letta 需要完整的 Letta 服务端，我们用两个 Claude Code CLI 实例 + 共享 MemCP 即可。

---

## 8.3 两个实例如何协作

**主实例（意识层）**——纯记忆消费者，只读不写：

```
用户发消息
    │
    ▼
主实例收到消息
    │
    ├─ 调用 memcp_recall(query=<用户问题关键词>) 搜索相关记忆
    ├─ 调用 memcp_related(id=<相关洞察>) 查找因果/时间关联
    │
    ▼
结合记忆上下文 + 当前对话 → 生成回复
    │
    ▼
对话内容写入 daily log（已有机制）
```

**记忆实例（潜意识层）**——纯记忆生产者，不面向用户：

```
scheduler 定时触发（如每 30 分钟）
    │
    ▼
记忆实例启动，读取最近的 daily log / 对话记录
    │
    ▼
执行完整 MEMORY PROTOCOL：
  Step 1: 从对话记录中提取事实
  Step 2: 对每条事实 memcp_recall 搜索去重
  Step 3: 判断 ADD / UPDATE / DELETE / NOOP
  Step 4: 执行 memcp_remember / memcp_forget
  Step 5: 记录审计日志
    │
    ▼
实例退出，等待下次触发
```

---

## 8.4 触发模式选择

记忆实例的触发有三种模式：

```
模式 A：文件监听（近实时）
  主实例 → 对话写入 daily log
  记忆实例 → watch daily log 变化 → 立即处理新内容
  延迟：秒级
  复杂度：需要常驻进程 + 文件监听

模式 B：Scheduler 定时批处理（推荐）
  主实例 → 正常对话
  scheduler → 每 30 分钟触发记忆实例 → 处理最近的对话记录
  延迟：分钟级
  复杂度：最低，复用现有 scheduler

模式 C：事件管道（最强）
  主实例 → 对话结束 → 写入事件队列
  记忆实例 → 消费队列 → 近实时处理
  延迟：秒级
  复杂度：需要消息队列基础设施
```

**推荐模式 B**，理由：
- 直接复用现有 scheduler，零额外基础设施
- 批处理比实时处理更适合记忆管理（有更完整的上下文做判断）
- 30 分钟延迟对记忆场景完全可接受——刚聊完的内容在主实例上下文窗口里，不需要从 MemCP 召回
- 记忆的核心价值在**跨 session** 和 **compact 之后**，不需要秒级同步

---

## 8.5 模型选择策略

Cowork 架构的最大成本优势：**记忆实例可以用更便宜的模型**。

| 记忆任务 | 需要的能力 | 推荐模型 | 成本对比 Opus |
|---------|-----------|---------|-------------|
| 事实提取 | 阅读理解 + 结构化输出 | **Haiku** | ~1/60 |
| 去重判断 | 语义比较 | **Haiku** | ~1/60 |
| 冲突解决 | 推理 + 判断新旧信息可信度 | **Sonnet** | ~1/5 |
| 用户对话 | 全能力推理 | **Opus / Sonnet** | 基准 |

记忆实例 80% 的工作（提取 + 去重）用 Haiku 即可胜任，只有冲突解决需要 Sonnet。相比单实例方案（所有记忆操作都用 Opus），**成本降低 10-20 倍**。

---

## 8.6 时序问题：主实例查询时记忆还没更新怎么办

这是异步方案的固有问题。但实际影响有限：

```
时间线：
  T+0min    用户说"我决定用 Qdrant"
  T+0min    主实例回复（上下文窗口里有这条信息）
  T+0~30min 这条信息还没写入 MemCP
  T+30min   scheduler 触发记忆实例 → 处理 → 写入 MemCP
  T+30min+  后续 session / compact 后 → 主实例能从 MemCP recall 到
```

**为什么不是问题：**
- 对话中刚说的内容在主实例的上下文窗口里，不需要从 MemCP 查
- compact 至少要几十分钟后才会发生，到那时记忆实例早已处理完
- 跨 session 场景更不用担心——用户下次开新会话时，30 分钟的记忆早已入库

**唯一的边缘情况**：用户在 30 分钟内 compact，且 compact 丢弃了关键信息，且记忆实例还没处理。这个概率极低，且可通过缩短 scheduler 间隔（如 10 分钟）进一步降低。

---

## 8.7 并发安全

两个实例共享同一个 MemCP SQLite 文件：

| 操作 | 主实例 | 记忆实例 | 冲突风险 |
|------|--------|---------|---------|
| `memcp_recall` | ✅ 读 | ✅ 读 | 无（多读安全） |
| `memcp_remember` | ❌ 不写 | ✅ 写 | 无（单写者） |
| `memcp_forget` | ❌ 不写 | ✅ 写 | 无（单写者） |
| `memcp_related` | ✅ 读 | ✅ 读 | 无（多读安全） |

由于主实例是纯读、记忆实例是读+写，且 scheduler 模式下只有一个记忆实例在运行，**不存在写冲突**。SQLite 的 WAL 模式天然支持"一写多读"。

---

## 8.8 Cowork 方案对比总结

| 维度 | 单实例方案 | Cowork 方案 |
|------|---------------------|------------|
| **主实例延迟** | +3-5 次 tool call / 交互 | 零额外开销 |
| **System Prompt 开销** | ~500 tokens（MEMORY PROTOCOL） | ~50 tokens（只读指令） |
| **LLM 注意力** | 分散（对话 + 记忆） | 专注（各司其职） |
| **记忆处理模型** | Opus（与对话同模型） | Haiku/Sonnet（便宜 10-20x） |
| **记忆时效性** | 实时 | 延迟 ≤30 分钟 |
| **运维复杂度** | 1 个进程 | 2 个进程（scheduler 管理） |
| **Letta 等价性** | 无直接对应 | = Subconscious Agent 模式 |

**结论：Cowork 方案严格优于单实例方案**——它以可接受的 30 分钟延迟为代价，换来了响应速度提升、成本降低 10-20 倍、主实例上下文更干净三大收益。

---

## 8.9 System Prompt 记忆协议

### 记忆实例：MEMORY PROTOCOL 完整定义

以下协议配置在**记忆实例（潜意识层）**的 System Prompt 中：

```
## MEMORY PROTOCOL

When processing ANY new information (conversations, documents, channel data),
execute the following memory management workflow:

### Step 1: Fact Extraction
Identify key facts from the input:
- Personal preferences and decisions
- Important events and milestones
- Technical choices and rationale
- People, organizations, and relationships
- Opinions, judgments, and frameworks

### Step 2: Deduplication Check
For each extracted fact:
1. Call memcp_recall(query=<fact summary>) to search existing memories
2. Review the returned results for semantic overlap

### Step 3: Decision
For each fact, decide one of:
- ADD: No similar memory exists → call memcp_remember with appropriate
  category, importance (1-5), and tags
- UPDATE: Similar memory exists but information has changed →
  call memcp_forget(old_id) then memcp_remember(new_fact)
- DELETE: Existing memory is contradicted or invalidated →
  call memcp_forget(old_id), optionally memcp_remember(correction)
- NOOP: Existing memory already captures this fact → skip

### Step 4: Execute
Execute the decided operations via MemCP tools.

### Step 5: Audit
For non-NOOP decisions, briefly note what was done and why
in the workspace daily log.

### Category Guidelines
- "preference": User likes/dislikes, tool choices, workflow habits
- "decision": Technical decisions with rationale
- "fact": Objective information, dates, relationships
- "insight": Analysis conclusions, patterns discovered
- "context": Project state, ongoing work, blockers

### Importance Scale
- 5: Core identity/values, critical decisions
- 4: Important preferences, key technical choices
- 3: Useful context, moderate relevance
- 2: Minor details, low-frequency relevance
- 1: Ephemeral, may be pruned in retention cycle
```

### 主实例：精简记忆使用指令

以下指令配置在**主实例（意识层）**的 System Prompt 中——仅 ~50 tokens：

```
## MEMORY USAGE

Before answering questions, search for relevant context:
1. Use memcp_recall(query=<keywords>) to find related memories
2. Use memcp_related(id=<insight_id>) to explore causal/temporal links
3. Incorporate retrieved context into your response

You do NOT manage memories. A background process handles extraction,
deduplication, and conflict resolution automatically.
```

### 协议如何等价 Mem0

| Mem0 组件 | Cowork 等价物 | 执行者 |
|-----------|-------------|--------|
| `FACT_RETRIEVAL_PROMPT` | MEMORY PROTOCOL Step 1 | 记忆实例 |
| `get_update_memory_messages()` | Step 2: `memcp_recall` 搜索 | 记忆实例 |
| LLM 判断 ADD/UPDATE/DELETE/NOOP | Step 3: Decision | 记忆实例（Haiku/Sonnet） |
| `_create_memory` / `_update_memory` / `_delete_memory` | Step 4: `memcp_remember` / `memcp_forget` | 记忆实例 |
| History/Audit Log | Step 5: workspace 日志 | 记忆实例 |
| Memory retrieval at query time | MEMORY USAGE: `memcp_recall` / `memcp_related` | 主实例 |
