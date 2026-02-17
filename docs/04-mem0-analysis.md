# 04. Mem0 技术深度剖析

---

## 4.1 两阶段 Pipeline

Mem0 的核心是一个 LLM 驱动的两阶段处理流水线：

```
输入（对话/文档）
    │
    ▼
┌─────────────────────────────┐
│ Phase 1: 事实提取            │  ← LLM 调用 #1
│ 用 FACT_RETRIEVAL_PROMPT     │
│ 从非结构化文本中提炼出       │
│ 结构化的 fact 列表           │
└─────────────────────────────┘
    │
    ▼
┌─────────────────────────────┐
│ Phase 2: 冲突解决            │  ← LLM 调用 #2
│ 对每条 fact:                 │
│  1. 向量搜索已有记忆         │
│  2. LLM 判断: ADD/UPDATE/    │
│     DELETE/NOOP              │
│  3. 执行对应操作             │
└─────────────────────────────┘
    │
    ▼
  存储（向量 DB + 图谱 + 历史日志）
```

---

## 4.2 LLM 决策：ADD / UPDATE / DELETE / NOOP

对每条提取出的 fact，Mem0 通过 LLM 做四种决策：

| 决策 | 触发条件 | 操作 |
|------|---------|------|
| **ADD** | 全新事实，无相似记忆 | 生成 embedding，写入向量 DB |
| **UPDATE** | 与已有记忆相关但信息已更新 | 更新内容，保留 ID |
| **DELETE** | 已有记忆过时或被否定 | 从存储中移除 |
| **NOOP** | 与已有记忆完全重复 | 跳过，不做任何操作 |

---

## 4.3 双存储路径

Mem0 同时维护两条存储路径：

**向量路径（`_add_to_vector_store`）：**
- Embedding 提取的 facts
- 向量搜索已有记忆（带 user_id/agent_id/run_id 过滤）
- 执行 LLM 决策的 ADD/UPDATE/DELETE
- 所有变更记录到 SQLite 历史日志
- Payload 包含：内容、hash、时间戳、session 标识、自定义字段

**图谱路径（`_add_to_graph`，enable_graph=True 时启用）：**
- 通过 `EXTRACT_ENTITIES_TOOL` 用 LLM 提取实体
- Neo4j 余弦相似度搜索相似实体（阈值默认 0.7）
- 通过 `RELATIONS_TOOL` 用 LLM 确定关系
- `DELETE_MEMORY_TOOL_GRAPH` 解决冲突
- Cypher 查询持久化节点和关系

---

## 4.4 完整数据流

```
Input Messages
    ↓
解析/标准化（处理 vision 等多模态输入）
    ↓
提取 session 过滤器（user_id, agent_id, run_id 校验）
    ↓
LLM 事实提取 → JSON fact 列表
    ↓
并行处理：
  ├─ 向量路径：
  │  ├─ Embed facts
  │  ├─ 搜索已有记忆（vector_store.search）
  │  ├─ LLM 决策（ADD/UPDATE/DELETE/NONE）
  │  ├─ 执行操作（_create/_update/_delete_memory）
  │  └─ 写入向量 DB + metadata
  │
  └─ 图谱路径（如启用）：
     ├─ LLM 提取实体
     ├─ 搜索相似实体（Neo4j similarity search）
     ├─ LLM 确定关系
     ├─ 冲突解决（删除过时边）
     └─ 持久化实体和关系
    ↓
历史记录
    ↓
返回结果（memory_ids, status, relations）
```
