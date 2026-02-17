# 5. 工业化简化的收益与风险评估

## 5.1 总体评估矩阵

| 简化决策 | 影响范围 | 收益 | 风险 | 缓解措施 | 最终判断 |
|----------|---------|------|------|----------|----------|
| regex 替代 LLM 实体提取 | Entity 图质量 | 零成本, 零延迟 | 代词、隐式实体丢失 | `--entities` flag 让 Claude 补充 | ✅ 合理 |
| regex 替代 LLM 因果推断 | Causal 图质量 | 零成本, 同步执行 | 隐式因果丢失 (~30-40%) | causal_candidates + Claude 评估 | ⚠️ 可接受但有损失 |
| 去除 Episode/Session/Narrative | 记忆组织粒度 | 架构简洁 | 缺少会话级语义 | source 字段 + temporal backbone | ✅ 合理（场景差异） |
| 去除 Multi-hop 分解 | 复杂查询能力 | 无管道内 LLM 依赖 | 无 | Claude 外部自然分解 | ✅ 最佳选择 |
| 去除 Best-of-N | 答案质量 | 系统定位清晰 | 无 | 不适用（Mnemon 不生成答案） | ✅ 最佳选择 |
| SQLite 替代 FAISS | 向量搜索性能 | 统一存储, 事务一致 | O(n) 全扫描 | 1000 insight 上限可控 | ✅ 合理（当前规模） |
| Ollama 替代 OpenAI embed | 向量质量 | 零 API 成本, 本地运行 | 模型能力差异 | nomic-embed-text 质量足够 | ✅ 合理 |
| 同步写入替代双流 | 写入架构 | 简洁, 无并发问题 | 写入延迟含全部边生成 | Go 编译型足够快 (~10ms) | ✅ 合理 |
| CLI-in-the-loop 替代 LLM-in-pipe | LLM 集成方式 | 零依赖, 可换 LLM | Claude 判断有额外 round-trip | Signals 元数据辅助判断 | ✅ 最佳选择 |
| 1000 insight 上限 | 存储容量 | 控制搜索延迟 | 长期使用可能不够 | auto-prune + gc 管理 | ⚠️ 可能需要提高 |

## 5.2 核心风险详细分析

### 风险 1：因果图质量 — 最大的简化损失

**量化评估**：

```
MAGMA 论文消融实验:
  - 完整系统 LoCoMo judge score: 0.70
  - 去掉 causal 图: -0.042 (约 -6%)
  - 去掉 LLM 因果推断 (只留 temporal proximity): -0.056 (约 -8%)

Mnemon 的位置: 介于"完整因果"和"无因果"之间
  - regex 能捕获显式因果 (~60-70% 的因果关系有语言标记)
  - 估计损失: -3% ~ -5% (相比完整 LLM 因果)
```

**风险场景**：
- "选了 Redis" → 后来 "延迟降到 5ms"：没有因果关键词，regex 无法发现
- "重构了 API 层" → 后来 "测试覆盖率从 60% 提升到 85%"：隐式因果

**缓解措施有效性**：
- `causal_candidates` 通过 2-hop BFS 邻域发现潜在因果，Claude 评估
- embedding 相似度发现的隐式候选标记为 `(implicit: embedding similarity)`
- 实际使用中，编程助手场景的因果关系比对话场景更显式

**结论**：可接受。在编程场景下 regex 覆盖率可能达 70-80%（因为技术讨论的因果表达更规范），加上 Claude 补充的因果链接，实际损失可控。

### 风险 2：全表扫描的向量搜索 — 容量瓶颈

**量化评估**：

```
当前性能 (实测估算):
  - 100 insights: ~1ms (768d float64 余弦, Go 编译型)
  - 500 insights: ~5ms
  - 1000 insights: ~10ms
  - 5000 insights (假设提高上限): ~50ms

参考:
  - FAISS IndexFlatL2 (1000, 384d): ~0.5ms
  - FAISS 优势在 10K+ 规模才显著
```

**当前状态**：1000 insight 上限下，全表扫描完全可接受。

**未来风险**：如果用户需要更大容量（5000+），搜索延迟会线性增长。

**缓解路径**（按优先级）：
1. 提高上限到 3000-5000，观察实际延迟
2. 加内存缓存：启动时加载所有 embedding 到内存
3. 最终方案：Go HNSW 库（如 `viterin/vek`），但增加复杂度

### 风险 3：无 Episode 分割 — 跨会话查询退化

**风险场景**：
- "上周二的那次讨论说了什么？" → 无 session 边界信息
- "这个项目讨论过几次数据库选型？" → 无 episode 聚合

**影响评估**：
- 在 LoCoMo 这类对话 benchmark 上影响较大
- 在编程助手场景下影响很小（Claude 会话通常聚焦单一任务）
- `source` 字段 + temporal backbone 提供弱替代

**结论**：低风险。编程助手的使用模式与对话记忆不同。

### 风险 4：1000 Insight 容量上限

**使用场景估算**：

```
典型使用频率:
  - 每天 5-15 条 insight (积极使用)
  - 每月 150-450 条
  - 1000 条上限 = 2-6 个月

达到上限后:
  - auto-prune 删除最低 EI 的非免疫 insight
  - 免疫条件: importance >= 4 OR access_count >= 3
  - 高质量的 decision/preference 不会被清理
  - 被清理的主要是低重要度、无人访问的 context/general 条目
```

**实际风险**：
- 长期使用后，一些有价值但重要度标记不高的 fact 可能被清理
- 用户可能不知道 insight 被自动清理了

**缓解措施**：
- `gc --threshold` 让 Claude 主动审查候选
- 可以提高上限到 3000-5000（SQLite 性能足够支撑）
- 定期 `embed --status` 监控总量

## 5.3 简化带来的独特优势

### 优势 1：零成本运行

```
MAMGA 运行成本估算 (月):
  - 写入: 500 insights × $0.002/条 (提取+因果) = $1.00
  - 查询: 1000 次 × $0.001/次 (生成+选择) = $1.00
  - Embedding: $0.50 (text-embedding-3-small)
  - 合计: ~$2.50/月

memcp 运行成本估算 (月):
  - 可选 LLM: ~$0.30 (Haiku 子代理)
  - Embedding: $0 (本地 Model2Vec)
  - 合计: ~$0.30/月

Mnemon 运行成本:
  - $0/月 (所有计算本地完成)
```

看似微不足道的成本差异，在规模化时意义重大：
- 10 个代理 × 12 个月 × MAMGA: $300
- 10 个代理 × 12 个月 × Mnemon: $0

### 优势 2：部署简洁性

```
MAMGA 部署:
  - Python 3.x + pip install (NetworkX, FAISS, OpenAI, ...)
  - .env 配置 (OPENAI_API_KEY 必需)
  - 适合: 研究环境, Jupyter notebook

memcp 部署:
  - Python 3.10+ + pip install
  - MCP server 配置
  - Claude Code 专用
  - 适合: Claude Code 用户

Mnemon 部署:
  - 下载单二进制 (或 go install)
  - 可选: brew install ollama && ollama pull nomic-embed-text
  - 适合: 任何环境, 任何 LLM
```

### 优势 3：LLM 无关性

Mnemon 不绑定任何 LLM 提供商：
- 当前用 Claude → 可以切换到 GPT / Gemini / 本地模型
- skill 提示是纯文本，可适配任何系统提示格式
- CLI 输出是标准 JSON，任何程序都可以解析

### 优势 4：可审计性

```
# 查看所有操作记录
mnemon log --limit 50

# 每条操作都有:
# - 时间戳
# - 操作类型 (remember, recall, recall:smart, link, forget, gc, embed)
# - 关联 insight ID
# - 详情字段
```

MAMGA 和 memcp 都缺少完整的操作审计日志。

### 优势 5：Signals 透明度

Mnemon 是唯一一个将 reranking 信号暴露给调用方的实现：

```json
"signals": {
  "keyword": 0.8,     // token 匹配强度
  "entity": 0.5,      // 实体匹配度
  "similarity": 0.7,  // 向量相似度
  "graph": 0.9        // 图遍历分数
}
```

这让 Claude 能做出比管道内部 LLM 更好的判断，因为 Claude 有完整的对话上下文。

## 5.4 综合建议

### 短期不变（当前设计合理）

1. ✅ CLI-in-the-loop 架构
2. ✅ 零 API 依赖
3. ✅ SQLite 统一存储
4. ✅ regex + 字典 + `--entities` 的实体提取
5. ✅ Signals 元数据输出
6. ✅ Diff 去重机制

### 中期可优化

1. **KeywordEnricher**：embedding 前附加 entities + tags 到内容，提升向量质量。实现成本低（~20 行代码）。
2. **IDF 加权**：keyword search 中引入 IDF，提升搜索精度。不需要完整 BM25，一个简单的 IDF 表即可。
3. **容量上限**：从 1000 提升到 3000-5000，同时观察搜索延迟。
4. **Beam search early stopping**：当连续 N 层没有发现高分新节点时提前停止，减少无效遍历。

### 长期可考虑

1. **可选 LLM 因果推断**：提供 `--llm-causal` flag，当用户配置了 LLM API 时启用 slow path 因果推断。保持默认行为不变（零依赖），但给高级用户选项。
2. **向量索引**：当容量需求超过 5000 时，引入 Go HNSW 库。
3. **MCP 适配层**：在 CLI 之上提供可选的 MCP server wrapper，支持 Claude Code 以外的 MCP 客户端。不改变核心架构。

### 不建议实现

1. ❌ Episode / Session 节点类型（场景不匹配）
2. ❌ Multi-hop 问题分解（Claude 外部做得更好）
3. ❌ Best-of-N 答案选择（系统定位不同）
4. ❌ 3-zone Archive 模型（soft delete 足够）
5. ❌ Context-as-Variable / RLM（CLI 工具不需要）

## 5.5 结论

Mnemon 在学术成果转化为工业实现的过程中做出了一系列务实的取舍。核心判断准则是：

> **保留架构骨架，简化实现路径，用 LLM 外部协作弥补管道内部的能力损失。**

具体而言：

1. **四图架构的核心价值得到保留**：多视图分离、意图自适应是 MAGMA 最重要的贡献，Mnemon 完整实现
2. **LLM 密集操作用规则+外部协作替代**：regex 提取 + Claude 补充 ≈ LLM 内部提取，但成本为零
3. **生命周期管理是纯增量**：论文没有的功能，对生产环境至关重要
4. **Signals 透明度是独特创新**：让外部 LLM 能做出比管道内部 LLM 更好的判断

最终，Mnemon 证明了一个观点：**在 LLM 能力越来越强的时代，记忆系统不需要自己内嵌 LLM —— 它只需要提供足够好的结构和足够透明的信号，让外部的 LLM 做出最终判断。**
