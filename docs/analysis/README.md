# Mnemon 架构分析报告

> 学术成果到工业实现的转化分析
>
> 基于 MAGMA 论文 (arXiv:2601.03236)、MAMGA 官方实现、memcp 实现的深度对比

## 目录

1. [当前项目实现架构](./01-implementation-architecture.md)
2. [典型场景时序图](./02-sequence-diagrams.md)
3. [与 MAGMA 论文的对比分析](./03-magma-paper-comparison.md)
4. [与 MAMGA/memcp 实现的对比分析](./04-implementation-comparison.md)
5. [工业化简化的收益与风险评估](./05-tradeoff-assessment.md)

## 一句话总结

Mnemon 是 MAGMA 论文的 **工业化简化实现**：保留了四图架构的核心骨架（temporal / entity / causal / semantic），用 regex + 字典替代 LLM 提取，用 CLI-in-the-loop 替代 LLM-in-the-loop，用 SQLite 替代 NetworkX + FAISS。在牺牲约 15-20% 图质量（主要来自 causal 和 entity 图的精度损失）的代价下，获得了零外部依赖、零 API 费用、编译型单二进制的工业级部署优势。

## 引用

- **MAGMA 论文**: Dongming Jiang et al., "MAGMA: A Multi-Graph based Agentic Memory Architecture for AI Agents", arXiv:2601.03236, Jan 2026
- **MAMGA 实现**: https://github.com/FredJiang0324/MAMGA (Python, NetworkX + FAISS)
- **memcp 实现**: https://github.com/maydali28/memcp (Python, FastMCP + SQLite)
