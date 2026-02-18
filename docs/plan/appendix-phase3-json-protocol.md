# 附录：Phase 3 JSON 协议设计说明

## 背景

Phase 3 需要解决 4 个 MAGMA 差异中的 3 个 LLM 依赖项（D3 因果推理、D4 实体抽取、D7 叙事分组），但 mnemon 作为纯 Go CLI 不内嵌任何 LLM API key，也不引入新依赖。

核心矛盾：**需要 LLM 能力，但不能依赖 LLM。**

## 解法：Claude-in-the-loop + JSON 契约

采用结构化 JSON 作为 mnemon 与外部 LLM agent（Claude）之间的通信协议，形成"出题—答题"协作模式：

```
mnemon (粗筛，高召回)  ──JSON stdout──▶  Claude (精排，高精度)  ──CLI 回调──▶  mnemon (写入)
```

mnemon 负责候选生成（关键词匹配、token overlap、时间窗口聚类），以 JSON 输出；Claude 负责语义判断（因果关系是否成立、实体是否遗漏、聚类是否连贯），通过回调 mnemon 命令完成写入。

## JSON 输出点

### 1. `causal_candidates`（D3 因果推理）

`mnemon remember` 在检测到因果信号时，输出候选数组：

```json
{
  "causal_candidates": [
    {
      "id": "abc-123",
      "content": "Chose Redis for caching",
      "causal_signal": "because",
      "token_overlap": 0.35,
      "suggested_sub_type": "causes"
    }
  ]
}
```

Claude 评估后回调：
```bash
mnemon link <src> <tgt> --type causal --weight 0.8 --meta '{"sub_type":"causes","reason":"..."}'
```

### 2. `entity_hints`（D4 实体抽取）

`mnemon remember` 输出正则提取的实体及提示：

```json
{
  "entities": ["HttpServer", "config.yml"],
  "entity_hints": "Auto-extracted by regex. Consider running `mnemon enrich <id> --entities \"X,Y\" --rebuild-edges` if important entities were missed."
}
```

Claude 判断是否遗漏重要实体后回调：
```bash
mnemon enrich <id> --entities "Redis,Memcached" --rebuild-edges
```

### 3. `consolidate` 建议模式（D7 叙事分组）

`mnemon consolidate` 输出聚类结果及建议命令：

```json
{
  "clusters": [
    {
      "cluster_id": 1,
      "time_range": {"start": "...", "end": "..."},
      "insights": [{"id": "abc", "content": "...", "category": "..."}],
      "suggested_title": "Project setup and tooling decisions",
      "shared_entities": ["React", "TypeScript"]
    }
  ],
  "actions": {
    "create": "mnemon consolidate --create --title \"<title>\" --members \"<id1>,<id2>,<id3>\""
  }
}
```

Claude 审核后回调：
```bash
mnemon consolidate --create --title "Project setup" --members "abc,def,ghi"
```

## 为什么选择 JSON

| 考量 | JSON 方案 | 替代方案（内嵌 LLM API） |
|------|----------|------------------------|
| 依赖 | 零新依赖，纯 stdout | 需 API key、HTTP client、重试逻辑 |
| 可解析性 | Claude 原生理解 JSON 结构 | — |
| 解耦 | mnemon 与 LLM 完全解耦，可替换任意 agent | 强绑定特定 LLM provider |
| 可操作性 | JSON 中附带命令模板，agent 可直接执行 | 内部闭环，外部不可观测 |
| 可测试性 | E2E 测试只需验证 JSON 输出格式 | 需 mock LLM 响应 |
| 成本 | 零额外 API 调用成本 | 每次 remember 都产生 API 调用 |

## 设计原则

1. **粗筛在 CLI，精排在 agent** — mnemon 用确定性算法（关键词、overlap、时间窗口）做高召回候选生成，LLM 做高精度语义判断。两者各司其职。

2. **JSON 是接口契约** — 字段名、类型、含义固定，mnemon 版本升级时保持向后兼容。

3. **建议而非命令** — JSON 输出的是"候选"和"建议"，最终决定权在 agent。`suggested_sub_type` 可能被 Claude 推翻，`suggested_title` 可能被改写。

4. **可选参与** — 即使 agent 不处理这些 JSON 字段，mnemon 的核心功能（remember/recall/link）仍然完整运行。Claude-in-the-loop 是增强层，不是必需层。
