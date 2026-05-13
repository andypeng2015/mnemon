# Mnemon Harness

Mnemon Harness 是 Mnemon modular self-evolution harness 的正式中文文档入口。

Mnemon 不替换宿主 agent runtime，而是通过 hooks、skills、subagents、文件系统资产和环境配置，把外置 evolution loop 挂载到已有 agent 上。

## 核心定位

| 主题 | 设计 |
| --- | --- |
| Modular Agent Harness | [中文](modular-agent/DESIGN.md) / [EN](../../harness/modular-agent/DESIGN.md) |
| Memory Loop | [中文](memory-loop/DESIGN.md) / [EN](../../harness/memory-loop/DESIGN.md) / [site](../../site/memory-loop/site.html) |
| Skill Loop | [中文](skill-loop/DESIGN.md) / [EN](../../harness/skill-loop/DESIGN.md) / [site](../../site/skill-loop/site.html) |

## 可安装资产

| Harness Module | 实现 |
| --- | --- |
| Memory Loop | [harness/memory-loop](../../../harness/memory-loop/README.md) |
| Skill Loop | [harness/skill-loop](../../../harness/skill-loop/README.md) |

## 词汇

| 概念 | 含义 |
| --- | --- |
| GUIDE | Markdown policy，用来判断某个 loop 何时应该行动。 |
| setup | 安装并挂载到宿主 agent。 |
| hook | Prime、Remind、Nudge、Compact 等宿主生命周期时机。 |
| protocol | 定义可复用操作的 Markdown skill。 |
| subagent | 用于较重 review 或 consolidation 的后台维护 agent。 |

## 边界

宿主 agent 保留 ReAct loop、prompt assembly、tool routing、native skill runtime、权限模型和 UI。Mnemon 提供可挂载的 harness module，让宿主 agent 获得更持久、更可自进化的能力。

Claude Code 是第一个 reference host，因为它提供 hooks、skills 和 subagents。这个架构的目标不局限于 Claude Code。
