# 02. 设计哲学：LLM-CLI 即游戏引擎

---

## 2.1 核心类比：映射关系

| 游戏开发 | Agent 生态 |
|---------|-----------|
| 游戏引擎（Unity/Unreal） | LLM-CLI（Claude Code/Cursor） |
| 脚本层（C#/Blueprint） | Markdown Skills |
| 原生插件（C++ Plugin） | Binary + Skill 定义 |
| Gameplay 逻辑 | AGENT.md / SOUL.md |
| **MMO World Server** | **Mnemon Daemon** |
| 游戏作品 | 具体 Agent 应用 |
| 美术/关卡资源 | Prompt / Context / 数据 |

---

## 2.2 分层架构

```
Layer 0: LLM 模型本身              ← GPU/硬件
Layer 1: LLM-CLI (agentic loop)    ← 游戏引擎
Layer 2: Skills (md 定义)           ← 脚本/蓝图
Layer 3: Skills (bin + md)          ← 原生插件
Layer 4: 具体 Agent 应用            ← 游戏成品
       ↕
Mnemon Daemon (独立运行)             ← MMO World Server
```

---

## 2.3 核心洞察

- **引擎层不需要自己建**——大厂持续优化（Anthropic/OpenAI/Cursor 等），开发者只需引入即用
- **Skill 层边际成本极低**——写 md 很快，类似 Unity Blueprint 让非程序员也能定义 agent 行为
- **记忆层（Mnemon）是唯一需要深耕且不可替代的部分**——记忆有复利效应，是 agent 从"工具"变成"助手"的分界线
- **LLM 本身就是 runtime，skill 描述就是程序**——自然语言是新平台的"编程语言"

---

## 2.4 与当前 Agent 框架的对比

LangChain/CrewAI/AutoGen 本质是"用代码编排 LLM 调用"，相当于游戏开发中"没有引擎，每个游戏从 OpenGL 调用写起"的阶段。核心问题：

- 抽象层次错误——把 LLM 当需要被"编排"的 API，而非有自主推理能力的执行核心
- 过度工程化——用 Python DAG 定义行为流程，LLM 读了 md 就知道该怎么做
- LLM 本身就是最好的编排器

---

## 2.5 设计哲学：Tools are Organs, Skills are Textbooks

这与 OpenClaw 的哲学完全一致：

- **Binary = Organ（器官）**——能不能做。提供记忆存取、图谱遍历、生命周期管理等能力
- **Skill（.md）= Textbook（教材）**——怎么做。教 LLM 何时检索记忆、如何判断去重、怎样调用命令

Binary 封装了所有不需要 LLM 的逻辑，Skill 只教 LLM 做需要智能判断的部分。**记忆管理逻辑从 prompt 变成代码——确定性、可测试、可移植**。
