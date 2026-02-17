# Mnemon: Project Vision

> **Mnemon** — An open-source memory daemon for LLM agents

---

## 命名

**Mnemon**，源自古希腊语 μνήμων（mnḗmōn），由 μνάομαι（"铭记"）与施事后缀 -μων 构成，意为"铭记者、善于记忆之人"。荷马在《奥德赛》中以 "καὶ γὰρ μνήμων εἰμί"（我记得很清楚）描述这一特质。在古希腊城邦制度中，Mnemones 是专职的记录官员，在财产交易与法律程序中承担见证与存档职责，是口述传统向书面记录过渡时期的制度性记忆载体。

该词同源于记忆女神 Mnemosyne（Μνημοσύνη）——宙斯与她结合诞生了九位缪斯，象征记忆是一切知识与创造的源泉。现代英语中的 mnemonic（助记法）亦由此派生。

---

## 项目定位

Mnemon 是一个**独立运行的 memory daemon**——为 LLM agent 提供持久化、可共享、图原生的记忆基础设施。

它不是嵌入某个 agent 框架的库或插件，而是一个 24/7 运行的独立服务，可被多种消费者（LLM-CLI、Web UI、CLI 工具、其他 Agent）同时访问。

### 核心理念

- **记忆是 agent 的灵魂**——没有可靠的长期记忆，agent 从"工具"无法进化为"助手"
- **记忆层有复利效应**——使用越久，积累越多，价值越高，这是唯一需要深耕且不可替代的部分
- **独立 daemon 是正确的架构**——记忆不应绑定在某个 agent 会话的生命周期里

---

## 文档索引

| 编号 | 文件 | 主题 |
|------|------|------|
| 00 | [00-vision.md](00-vision.md) | 项目愿景与命名 |
| 01 | [01-problem.md](01-problem.md) | 问题陈述：为什么需要 Memory |
| 02 | [02-philosophy.md](02-philosophy.md) | 设计哲学：LLM-CLI 即游戏引擎 |
| 03 | [03-landscape.md](03-landscape.md) | 现有方案概览与对比 |
| 04 | [04-mem0-analysis.md](04-mem0-analysis.md) | Mem0 技术深度剖析 |
| 05 | [05-memcp-analysis.md](05-memcp-analysis.md) | MemCP 技术深度剖析 |
| 06 | [06-architecture.md](06-architecture.md) | Mnemon 核心架构设计 |
| 07 | [07-memory-strategy.md](07-memory-strategy.md) | 记忆采集策略：宽进严出 |
| 08 | [08-cowork.md](08-cowork.md) | Cowork 双实例异步架构 |
| 09 | [09-multi-agent.md](09-multi-agent.md) | 多 Agent 记忆维护 |
| 10 | [10-rag-comparison.md](10-rag-comparison.md) | 与 RAG 方案的定位对比 |
| 11 | [11-community.md](11-community.md) | 社区共识与市场验证 |
| 12 | [12-vector-and-pipeline.md](12-vector-and-pipeline.md) | 向量检索层与学习 Pipeline |
| 13 | [13-binary-evolution.md](13-binary-evolution.md) | 架构演进：Binary + Skill |
| 14 | [14-decentralization.md](14-decentralization.md) | 去中心化架构适配性分析 |
| 15 | [15-academic.md](15-academic.md) | 学术支撑 |
| 16 | [16-implementation.md](16-implementation.md) | 实施路径与待办事项 |
| 17 | [17-references.md](17-references.md) | 参考资料 |

---

*A Practical Memory Daemon based on MAGMA Agentic Memory Architecture*
