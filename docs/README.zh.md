# taichi 文档索引

> 🌐 语言: [English](README.md) | [中文](README.zh.md)

> taichi 是一个通用的自动化测试编排框架。

## 文档目录

| 文档 | 内容 | 语言 |
|------|------|------|
| [架构设计](./architecture.zh.md) | 框架整体架构图、模块划分、核心流程、依赖关系 | [EN](./architecture.md) / [中文](./architecture.zh.md) |
| [技能接口规范](./skill-interface.zh.md) | TestSkill 接口契约、生命周期、上下文、优先级、注册机制 | [EN](./skill-interface.md) / [中文](./skill-interface.zh.md) |
| [环境配置指南](./configuration.zh.md) | 配置文件 schema、环境类型、字段说明、模板示例 | [EN](./configuration.md) / [中文](./configuration.zh.md) |
| [测试用例编写规范](./test-cases.zh.md) | 内置技能用例写法、断言规则、统一响应契约、最佳实践 | [EN](./test-cases.md) / [中文](./test-cases.zh.md) |
| [扩展开发文档](./extending.zh.md) | 自定义技能、自定义环境、自定义报告格式、钩子机制 | [EN](./extending.md) / [中文](./extending.zh.md) |
| [Agent 集成指南](./agent-integration.zh.md) | 与 AI Agent 双向集成、MCP Server、Copilot 模式、失败上下文 | [EN](./agent-integration.md) / [中文](./agent-integration.zh.md) |

## 快速链接

- 配置文件示例：[`configs/taichi.yaml`](../configs/taichi.yaml)、[`configs/taichi.example.yaml`](../configs/taichi.example.yaml)
- 技能接口定义：[`pkg/skill/skill.go`](../pkg/skill/skill.go)
- 编排器核心：[`pkg/orchestrator/orchestrator.go`](../pkg/orchestrator/orchestrator.go)
- CLI 入口：[`cmd/taichi/`](../cmd/taichi/)

## 快速开始

```bash
# 编译
make build

# 列出配置
./bin/taichi list -c configs/taichi.yaml

# 运行测试
./bin/taichi run -c configs/taichi.yaml

# 查看版本
./bin/taichi version
```

## 目录结构概览

```
taichi/
├── cmd/taichi/          # CLI 入口（run / list / version）
├── pkg/
│   ├── framework/       # 测试核心：类型、断言、报告、服务生命周期
│   ├── autofix/         # 自动修复：错误检测、修复规则引擎
│   ├── skill/           # 技能接口契约与上下文
│   ├── registry/        # 技能注册中心（并发安全）
│   ├── env/             # 环境管理（后端 / 前端）
│   ├── config/          # 配置 schema 与加载
│   ├── report/          # 报告扩展点与多格式输出
│   └── orchestrator/    # 编排核心
├── skills/              # 内置技能实现
│   ├── api/             # API 测试技能
│   ├── grpc/            # gRPC 冒烟检查技能（health / dial / reflect）
│   ├── ui/              # UI / 页面测试技能
│   ├── static/          # 静态资源测试技能
│   └── regression/      # 回归测试技能
├── configs/             # 默认配置与模板
└── docs/                # 本文档目录
```
