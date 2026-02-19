# OpenClaw 生态竞争分析

> 调研日期: 2026-02-20

## 一、OpenClaw 内置记忆系统

### 架构

- 两层 Markdown 文件: `MEMORY.md`（长期）+ `memory/YYYY-MM-DD.md`（每日日志）
- Embedding: OpenAI / Gemini / Voyage / 本地 GGUF 自动选择
- 分块: 400 tokens，80 overlap
- 存储: SQLite + sqlite-vec（per-agent）
- 搜索: 向量(70%) + BM25(30%) 混合
- MMR 去重 + 时间衰减（指数，30天半衰期）
- 压缩前记忆 flush hook
- 工具: `memory_search` + `memory_get`

### 结构性缺陷

**时间窗口陷阱**: 仅加载今天+昨天的日志，3天前的记忆依赖 `memory_search` 命中——前提是模型当天写了下来。遗漏即永久丢失。

**信息密度低**: 日志全量注入 system prompt（2000-5000 tokens），不管是否与当前对话相关。push 模型 vs mnemon 的 pull 模型。

**LLM 抽取信息损失**: 依赖模型自主判断"什么值得记"，但：
- 关联性丢失: 写"决定用 Redis"但不写"与 latency 问题的因果关系"
- 实体丢失: 自然语言叙述，不结构化提取 entities
- 粒度不可控: 有时流水账，有时关键决策一笔带过

**检索天花板**: 向量+BM25 flat search，无法做因果链回溯、时序事件重建、跨主题实体关联。

### 社区评价

社区博文 "OpenClaw's Memory Is Broken" 直接批评。Issue #2910、#2542 请求知识图谱层，均被官方关闭：

> "This issue is unlikely to be considered in the near term, so we're closing it to keep the pipeline moving while we focus on stability."

官方当前优先级是稳定性，不做记忆架构升级。

### 已知 Bug（开放中）

| Issue | 问题 |
|-------|------|
| #17576 | 内存索引器跳过 memory/*.md，仅索引 session |
| #20160 | 重建后 memory 文件未被索引 |
| #19913 | session 转录淹没 memory_search 结果（BM25 污染） |
| #14143 | 压缩永远不触发（默认 safeguard 模式） |
| #17854 | QMD 后端 OOM（292KB markdown 就崩） |
| #9888 | memory-core 更新后完全停止持久化 |

## 二、OpenClaw 记忆插件生态

### 已有插件

| 插件 | 类型 | 额外依赖 | 图能力 |
|------|------|---------|--------|
| `memory-core` (内置) | 默认 | 需 embedding API key | 无 |
| `memory-lancedb` (内置) | 向量存储 | `OPENAI_API_KEY` | 无 |
| `@mem0/openclaw-mem0` | 云端 | Mem0 API key | 无图 |
| `@cognee/cognee-openclaw` | 图谱增强 | Cognee Docker + OPENAI key | 单层语义图 |
| `openclaw-graphiti-memory` (社区) | 时序图谱 | Neo4j + Graphiti Docker + OPENAI key | 时序图 |
| `openclaw-penfield` | 知识图谱 | Penfield 闭源服务 | 18+ 关系类型 |
| `MemOS Cloud` | 多 agent 共享 | MemOS 云端 | 无 |
| `Supermemory` | 用户画像 | Supermemory Pro | 无 |
| `Maximem` | 跨渠道 | 独立部署 | 无 |

### Cognee 现状

- 版本: `@cognee/cognee-openclaw` v2026.2.4（2/5 发布）
- GitHub 6 星，npm 零依赖项，基本无社区采用
- 增强层，不替代 MEMORY.md
- 搜索模式: `GRAPH_COMPLETION`（图遍历+向量）、`CHUNKS`、`SUMMARIES`
- 依赖: Cognee Docker + `OPENAI_API_KEY`
- 图模型: 单层语义关系图（entity→relationship→entity），无时序/因果/语义边区分
- 无基准测试

### Graphiti 现状

- Zep 未发布官方 OpenClaw 插件
- 仅有社区 shell 脚本封装（curl 调 REST API）
- 最完整社区方案: `clawdbrunner/openclaw-graphiti-memory`（49 星，v2.0.0）
- 依赖: Neo4j + Qdrant + Graphiti Docker + `OPENAI_API_KEY`
- 基准测试强: DMR 94.8%，LongMemEval +18.5%，但仅限 OpenAI 模型
- 双时态模型（t_valid / t_invalid），支持"某时间点事实"查询

## 三、API Key / 认证摩擦（社区痛点）

### 核心问题

用户通过 OAuth 认证了 Claude/OpenAI，但记忆功能仍要求独立的 API key。12+ 个 issue 记录了这一摩擦。

### 关键 Issue

| Issue | 核心问题 |
|-------|---------|
| Discussion #3320 | Google OAuth 用户需要额外 Gemini key 才能用 embedding |
| #8131 | 配置本地 provider，memory_search 仍要求 OpenAI/Google key |
| #19813 | QMD 纯 BM25 模式仍验证 embedding API key（2/18 提交） |
| #8181 | 请求零配置本地 embedding |
| #16670 | 记忆无声失败——用户不知道需要配 embedding key |
| #6794 / #14272 | session-memory hook 硬编码 Anthropic，非 Anthropic 用户报错 |
| #5175 | 配置本地模型后仍发网络请求 |
| #17919 | 提议用 LLM 推理替代 embedding，彻底消除 API key |
| #20571 | memory-lancedb 不支持自定义 baseURL |
| #3309 | "本地" memory-lancedb 实际仍调 OpenAI embedding API |
| #17708 | RFC: 内置向量记忆，消除各插件各自带 ChromaDB/Pinecone |

### Hacker News 社区声音

> "The setup friction is real though. Docker, API keys, channel auth, gateway config. That's the actual barrier to adoption, not the underlying tech."

### 第三方插件的额外依赖汇总

| 插件 | 用户需要额外准备 |
|------|----------------|
| Graphiti | Neo4j Docker + Graphiti Docker + `OPENAI_API_KEY` + Colima |
| Cognee | Cognee Docker + `OPENAI_API_KEY` + Cognee API key |
| Mem0 | Mem0 API key（云端）或 `OPENAI_API_KEY`（自托管） |
| memory-lancedb | `OPENAI_API_KEY`（embedding 必须） |

## 四、mnemon vs OpenClaw 生态：差异化分析

### 多图 Agentic Memory 的价值（MAGMA 视角）

平面记忆（OpenClaw）是检索系统，图记忆（mnemon）是推理系统。记忆之间的"边"承载的信息量不亚于记忆本身。

四种边各自解决不可替代的问题：

| 边类型 | 回答的问题 | OpenClaw 能否替代 |
|--------|-----------|-----------------|
| Temporal | "X 之前/之后发生了什么" | 部分（日期文件名隐含时序，无法跨日遍历） |
| Entity | "关于 X 的所有记忆" | 部分（关键词搜索，无法发现间接关联） |
| Causal | "为什么 X" / "X 导致了什么" | 不能（因果关系无法从文本相似度推断） |
| Semantic | "与 X 概念相似但用词不同的记忆" | 部分（向量搜索，但不是可遍历的关联网络） |

### 意图感知检索 vs 平面搜索

MAGMA 的 intent-aware adaptive traversal：不同问题类型使用不同图遍历策略。

- WHY: beam_width=15, max_depth=5, 因果权重 0.7 → 沿因果链回溯
- WHEN: beam_width=10, max_depth=5, 时序权重 0.65 → 沿时间线展开
- ENTITY: beam_width=10, max_depth=4, 实体权重 0.55 → 从实体辐射
- GENERAL: 四信号均衡，RRF 融合

OpenClaw 的 `memory_search` 只有一种模式: 向量+BM25→排序→返回 top-K，无论问题类型。

### mnemon 独有能力（OpenClaw 生态不存在）

| 能力 | mnemon | OpenClaw 最接近的 |
|------|--------|-----------------|
| 意图感知 beam search | 核心特性 | 无 |
| 因果图 + 方向推断 | 核心特性 | Cognee 有图但无因果推理 |
| LLM 监督式链接（candidates） | 核心特性 | 所有插件自动链接或不链接 |
| Diff 冲突检测（dup/conflict/update） | 核心特性 | OpenClaw 只做追加 |
| GC + 免疫保护 | 核心特性 | 仅简单时间衰减 |
| 图谱可视化（DOT + vis.js） | 核心特性 | 无 |
| Go 零依赖二进制 | 架构优势 | 全部 TypeScript/Python |
| 审计日志（oplog） | 核心特性 | 无 |

### OpenClaw 已覆盖的能力

- 基础记忆 CRUD: remember/recall/forget
- 语义搜索: 向量 + 关键词混合
- 自动回忆注入: conversation start 加载记忆
- 自动捕获: conversation end 保存
- 时间衰减: 指数衰减

## 五、Cognee / Graphiti 不能替代 mnemon

### 图谱表达力对比

| 维度 | mnemon | Cognee | Graphiti |
|------|--------|--------|---------|
| 边类型 | 4 种（temporal/entity/causal/semantic） | 1 种（语义关系） | 2 种（episodic + semantic） |
| 因果推理 | 有（方向推断 + LLM 过滤） | 无 | 无 |
| 意图感知检索 | 有（WHY/WHEN/ENTITY 不同策略） | 无（统一遍历） | 无（统一搜索） |
| 时序建模 | 时序边 + 时间窗口 | 无 | 双时态模型（更强） |
| 生命周期管理 | diff/GC/decay/immunity | 无 | 无 |

### 实体提取：mnemon 不弱于 Cognee

mnemon 的实体提取有两条路径：
1. **主路径**: 宿主 LLM（Claude）在调用 remember 时提取 —— 带完整对话上下文
2. **兜底**: 二进制内部正则 + 200 词词典

Cognee 用自己的 LLM 从 Markdown 提取 —— 只有文本片段上下文，缺少对话上下文。

差异不在提取质量，在自动化程度。Cognee 管道内强制提取（100% 执行率），mnemon 依赖 LLM 纪律性。解法：在 remember 管道内加 Ollama 自动 enrichment，保持零外部依赖。

### 认证摩擦对比

| | Cognee | Graphiti | mnemon |
|--|--------|---------|--------|
| 自身服务 | Cognee Docker | Neo4j + Graphiti Docker | 无（Go 二进制） |
| LLM 提取 | 需独立 API key | 需独立 API key | 用宿主 LLM（已认证） |
| 用户额外成本 | Cognee key + LLM key | Neo4j + LLM key | $0 |
| OAuth 即可用 | 否 | 否 | 是 |

## 六、mnemon 在 OpenClaw 中的定位

### 互补模型

```
┌─────────────────────────────────────────┐
│              OpenClaw Agent              │
│                                         │
│  ┌─────────────┐    ┌───────────────┐   │
│  │ 内置 memory  │    │   mnemon      │   │
│  │ (日志层)     │    │  (图谱层)     │   │
│  │              │    │               │   │
│  │ MEMORY.md    │    │ 4-graph       │   │
│  │ daily logs   │    │ beam search   │   │
│  │ vector+BM25  │    │ intent-aware  │   │
│  │              │    │ causal chain  │   │
│  │ 适合:        │    │ 适合:         │   │
│  │ · 当日上下文  │    │ · 跨期知识    │   │
│  │ · 快速笔记   │    │ · 因果推理    │   │
│  │ · 低延迟检索  │    │ · 决策追溯    │   │
│  └─────────────┘    └───────────────┘   │
└─────────────────────────────────────────┘
```

### 竞争优势总结

1. **零额外认证**: 用宿主 Claude OAuth，不需独立 API key（命中 12+ 个社区痛点）
2. **零额外服务**: Go 二进制，无 Docker/Neo4j/Python（命中 HN 安装摩擦反馈）
3. **四图 > 单层图**: Cognee 单层语义图，Graphiti 双层，mnemon 四层
4. **意图感知检索**: 唯一实现 MAGMA intent-aware traversal 的方案
5. **开箱即用**: `openclaw plugins install mnemon` → 完整图记忆能力

### 市场空白

```
         OpenClaw 已覆盖            真空地带（mnemon 填补）
         ─────────────             ──────────────────────
         ✅ 基础记忆 CRUD           ✦ 四图 + 意图感知 beam search
         ✅ 语义/关键词搜索         ✦ 因果图 + 方向推断
         ✅ 自动 recall/capture     ✦ LLM 监督式链接
         ✅ 时间衰减                ✦ Diff 冲突检测
         ✅ 知识图谱 (Cognee, 弱)   ✦ GC + 免疫 + 审计日志
                                    ✦ 图谱可视化
                                    ✦ 零依赖 / 零额外认证
```

社区需求已验证（#2910, #2542, #3320 等），官方明确不做，现有方案要么太新（Cognee 6 星）、要么太重（Graphiti 需 Neo4j）、要么闭源（Penfield）。mnemon 以零依赖、本地优先的图记忆引擎，填补这个空白。
