# 07. 记忆采集策略：宽进严出

---

## 7.1 MAGMA 的设计模式

MAGMA 的 dual-stream 写入架构明确回答了"精选 vs 广泛采集"的问题：

**Fast Path（Synaptic Ingestion）**——几乎所有交互事件都写入。做 event segmentation、生成 embedding、挂到 temporal backbone 上。非阻塞，不做复杂判断。temporal graph 是 immutable 的，只追加不删除。

**Slow Path（Structural Consolidation）**——异步由 LLM 分析最近写入事件，推断 causal 和 entity 关系，做图谱 densification。选择性发生在这里，不是选择"记不记"，而是选择"怎么连"。

---

## 7.2 为什么"先全收"更合理

- **信息价值的后验性**——写入时不知道一条信息未来是否重要，记忆的价值在被检索时才显现
- **过滤的假阳性代价太高**——误判丢失重要信息是不可逆的，多存冗余信息的代价只是存储和检索噪声
- **人类记忆的类比**——感官记忆几乎全收，通过巩固和检索自然衰减，不在写入时决定遗忘

---

## 7.3 分层处理策略

```
写入层:  低门槛，几乎全收，打元数据标签（时间戳/来源/上下文类型/置信度）
         ↓
整理层:  async agents 做关联、去重、合并（MAGMA slow path）
         ↓
检索层:  query-adaptive，按意图选图、选路径（Adaptive Traversal Policy）
         ↓
呈现层:  compact context construction，只给 LLM 看相关子图
```

唯一在写入层做过滤的场景：**隐私和安全**（密码、密钥、敏感个人信息）。

核心原则：**智能不在门口，在内部。**
