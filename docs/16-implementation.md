# 16. 实施路径与待办事项

---

## 16.1 三阶段渐进落地

### Phase 1：Cowork 核心验证（最小可行）

**目标**：验证双实例 + MemCP 的可行性

**工作内容：**

```
Step 1: 基础设施（1 天）
  ├─ 安装 MemCP（pip install memcp）
  ├─ 配置主实例 MCP Server（mcpServers.memcp，只读工具）
  └─ 配置记忆实例 MCP Server（mcpServers.memcp，完整工具集）

Step 2: 主实例配置（0.5 天）
  ├─ System Prompt 加入 MEMORY USAGE 指令（~50 tokens）
  └─ 验证：memcp_recall / memcp_related 可正常调用

Step 3: 记忆实例配置（0.5 天）
  ├─ System Prompt 加入完整 MEMORY PROTOCOL（~500 tokens）
  └─ 手动测试：喂入对话内容 → 观察提取/去重/冲突解决

Step 4: Scheduler 集成（1 天）
  ├─ scheduler 新增 memory-processing 任务
  ├─ 每 30 分钟触发记忆实例处理最近 daily log
  └─ 验证：对话 → daily log → 记忆实例处理 → MemCP → 主实例可召回
```

**验证标准：**
- 主实例对话零延迟增加（无 MEMORY PROTOCOL 开销）
- 记忆实例能从 daily log 中正确提取关键事实
- 重复信息被正确跳过（NOOP）
- 矛盾信息被正确处理（UPDATE/DELETE）
- 主实例跨 session 可通过 `memcp_recall` 召回记忆

**依赖**：零。MemCP 零外部依赖，两个 CLI 实例独立运行。

---

### Phase 2：Qdrant 向量层 + 学习 Pipeline

**目标**：支持大规模原始素材的语义检索 + 多渠道自动摄入

**工作内容：**
- Docker 启动 Qdrant
- 配置 Qdrant MCP Server（记忆实例使用）
- scheduler 新增 learning 任务（每 4 小时）
- 实现多渠道内容拉取 → 记忆实例处理 → MemCP + Qdrant 双层存储
- 记忆实例模型优化：Haiku 处理提取/去重，Sonnet 仅处理冲突
- **记忆安全基础**（见 [11-community.md](11-community.md) §11.6）：来源标记（owner/external/visitor）、记忆审计机制、外部渠道记忆隔离

**验证标准：**
- Twitter 内容被定时拉取并处理
- 原始文档可通过 Qdrant 语义搜索到
- 精炼洞察在 MemCP 中可通过图谱遍历关联
- 记忆实例 API 成本 < 主实例的 10%
- 不同来源的记忆可区分标记和隔离查询

**依赖**：Phase 1 验证通过，确认 Cowork 模式有效。

---

### Phase 3：Binary 演进 + 闭环优化

**目标**：将验证有效的设计固化为独立 binary，记忆系统成为核心竞争力

**工作内容：**
- **`mnemon` binary 开发**（见 [13-binary-evolution.md](13-binary-evolution.md)）：将 MemCP 验证有效的 MAGMA 图、搜索、retention 用 Go 重新实现
- **Skill 替代 System Prompt**：用 `memory.md` skill 替代 MEMORY PROTOCOL，token 开销从 ~500 降到 ~100
- **Daemon 模式**：binary 自带后台监听，替代"第二个 CLI 实例"，潜意识层成本趋近于零
- 基于使用数据优化提取质量（在 binary 的 extract 模块中迭代）
- 调优 retention 策略（什么该 Archive，什么该 Purge）
- 建立记忆质量评估指标（召回率、准确率、过时率）
- 借鉴 Self-RAG 的检索质量自评——主实例评估记忆相关性并反馈（见 [10-rag-comparison.md](10-rag-comparison.md) §10.7）
- 探索 Graph RAG 的社区摘要思路——定期生成知识全局概览

**验证标准：**
- `mnemon` binary 可独立运行，零外部依赖
- 同一份 Skill 在 Claude Code 和至少一个其他 CLI 上可用
- daemon 模式有效运作，基础提取不依赖 LLM API
- 跨 session 召回率 > 90%
- 记忆冲突解决准确率 > 85%
- 月度记忆增长可控（retention 有效运作）

---

## 16.2 关键问题解答

| 问题 | 答案 |
|------|------|
| 两个 CLI 实例怎么共享 MemCP？ | 连接同一个 MCP Server 进程，或各自启动独立进程指向同一 SQLite 文件（WAL 模式支持一写多读） |
| 能否同时用 MemCP + 向量 DB？ | 可以，Claude Code 原生支持多 MCP Server |
| 需要改 MemCP 源码吗？ | 不需要，两个 MCP 独立运行 |
| 记忆实例用什么模型？ | Phase 1-2: Haiku/Sonnet；Phase 3: binary daemon 用本地模型，仅复杂冲突需 LLM |
| 成本？ | Phase 1-2: MemCP 零成本 + Haiku 按量；Phase 3: binary daemon 接近零成本 |
| 与现有 MEMORY.md 机制冲突吗？ | 不冲突，MemCP / binary 是增量增强，MEMORY.md 可继续保留 |
| 30 分钟延迟能接受吗？ | 可以——刚聊的内容在上下文窗口里，记忆的价值在跨 session 和 compact 后 |
| Binary 方案是否推翻了 MemCP 方案？ | 不是——Phase 1 用 MemCP 验证设计，Phase 3 用 Go 重实现为 binary，渐进演进 |
| Binary 能完全不依赖 LLM 吗？ | 基础操作（存取删搜）完全不需要；高质量事实提取和冲突解决仍需 LLM |
| 不同 LLM CLI 怎么共享记忆？ | 天然共享——同一个 `mnemon` binary 指向同一 SQLite 文件 |

---

## 16.3 Letta 等价映射（参考）

| Letta 概念 | Mnemon 等价物 |
|-----------|---------------|
| Core Memory（Agent 自编辑） | MEMORY.md + MemCP remember/forget |
| Recall Memory（对话历史） | Session 持久化 + daily log |
| Archival Memory（归档知识） | MemCP retention archive + Qdrant |

---

## 16.4 待办事项

- [ ] 确认 mnemon 图谱架构是否采用 MAGMA 多图分层（episodic/semantic/causal/entity）
- [ ] 设计最小可行方案（v0.1：SQLite + 几张表）先跑通闭环
- [ ] 实现保护机制：软删除、置信度标记、关键记忆不可变标志
- [ ] 设计多 agent 写入的一致性控制（原子性、版本控制、审计日志）
- [ ] 考虑 graph + vector 混合方案（模糊语义匹配需求）
- [ ] 深入阅读 MAGMA 论文，确认 causal graph 层的实现细节
- [ ] 研究 MemCP 实现，评估可复用的工程经验
- [ ] 定义 mnemon skill 的接口规范（读/写/查询/关联操作）
