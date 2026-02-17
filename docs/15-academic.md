# 15. 学术支撑

---

## 15.1 直接相关论文

| 论文 | 时间 | 核心贡献 | 与 Mnemon 的关系 |
|------|------|---------|----------------|
| **MAGMA** (UT Dallas & UF) | 2026.01 | 四图结构 + 异步整合 + Adaptive Traversal | 核心理论基座 |
| **Sleep-time Compute** (Letta/Berkeley) | 2025.04 | 维护者/使用者分离，sleep-time 处理提升效率 | 支撑多 agent 维护设计 |
| **Zep/Graphiti** | 2025.01 | 时间感知知识图谱，DMR 94.8% | 图谱实现参考 |
| **LightMem** | 2025.10 | 三层记忆 + sleep-time update，token 降低 117× | 支撑分层架构 |
| **Memory in the Age of AI Agents** | 2025.12 | 最全面综述，memory 为 first-class primitive | 理论框架定位 |
| **Graphs Meet AI Agents** | 2025.06 | 图结构记忆优于向量检索 | 支撑 graph-native 选择 |
| **Multi-agent Memory Survey** (TechRxiv) | — | 三种记忆托管模式，external hosting | 支撑独立 daemon 设计 |

---

## 15.2 MAGMA 发表状态

MAGMA 目前是 arXiv 预印本（arXiv:2601.03236），**尚未确认被任何顶会接收**。距提交仅一个多月，大概率在审稿周期中。可能目标：ICML 2026 / ACL 2026 / NeurIPS 2026。

建议：不要单押 MAGMA，同时锚定已发表的 Graphiti、Sleep-time Compute 等工作。

---

## 15.3 学术定位总结

各设计要素都有论文支撑，但没有一篇整合成完整架构。Mnemon 作为完全独立的、LLM-agnostic 的记忆 daemon，是这些研究方向的**自然但尚未被明确提出的汇聚点**。
