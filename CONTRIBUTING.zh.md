# 贡献 Taichi

> 🌐 语言: [English](CONTRIBUTING.md) | [中文](CONTRIBUTING.zh.md)

首先，感谢你抽出时间参与 Taichi 的贡献！🎉

本文档说明如何搭建开发环境以及我们遵循的约定。无论是修正错别字、报告 bug，还是提议新功能，我们都欢迎你的参与。

## 目录

- [开发环境](#开发环境)
- [开发流程](#开发流程)
- [分支命名](#分支命名)
- [提交信息](#提交信息)
- [代码规范](#代码规范)
- [Pull Request 流程](#pull-request-流程)
- [报告 Bug](#报告-bug)
- [提议新功能](#提议新功能)
- [添加新 Skill](#添加新-skill)
- [添加新环境类型](#添加新环境类型)
- [第三方插件](#第三方插件)
- [行为准则](#行为准则)
- [许可证](#许可证)

## 开发环境

在本地构建与测试 Taichi 需要：

- **Go** 1.26 或更高版本
- **make**
- **golangci-lint**（用于代码检查）

克隆你的 fork 并构建 CLI：

```bash
git clone https://github.com/<your-username>/taichi.git
cd taichi
make build
```

运行测试套件：

```bash
make test
```

运行代码检查：

```bash
make lint
```

## 开发流程

1. 在 GitHub 上 **Fork** 仓库。
2. 将你的 fork **克隆**到本地。
3. 从 `main` 创建**特性分支**（参见 [分支命名](#分支命名)）。
4. 按 [Conventional Commits](#提交信息) 规范**提交**变更。
5. 将分支**推送**到你的 fork。
6. 向上游 `main` 分支发起 **Pull Request**。

## 分支命名

使用简短描述性前缀加斜杠，再跟 kebab-case 概述：

- `feat/<简短描述>` — 新功能
- `fix/<简短描述>` — Bug 修复
- `docs/<简短描述>` — 仅文档
- `refactor/<简短描述>` — 不改变行为的重构

示例：`feat/add-grpc-skill`

## 提交信息

我们遵循 [Conventional Commits](https://www.conventionalcommits.org/) 规范：

```
<类型>(<可选作用域>): <描述>

[可选正文]

[可选页脚]
```

支持的类型：

- `feat` — 新功能
- `fix` — Bug 修复
- `perf` — 提升性能的代码变更
- `refactor` — 既不修复 bug 也不新增功能的代码变更
- `revert` — 回退之前的提交
- `docs` — 仅文档
- `style` — 代码风格格式化（空格、格式、缺失分号等）
- `test` — 新增或更新测试
- `build` — 构建系统或外部依赖（Makefile、go.mod 等）
- `ci` — CI/CD 流水线配置与脚本
- `chore` — 维护任务、工具、依赖
- `breaking` — 破坏向后兼容的变更（同时使用 `BREAKING CHANGE:` 页脚）

示例：

```
feat(prober): add HTTP probe skill
```

## 代码规范

所有贡献必须通过：

- `gofmt -l .`（无输出）
- `go vet ./...`
- `golangci-lint run`

核心逻辑必须有单元测试覆盖。修复 bug 时，请添加能复现该问题的回归测试。

## Pull Request 流程

1. 填写 [PR 模板](.github/PULL_REQUEST_TEMPLATE.md)。
2. 说明**改了什么**以及**为什么改**。
3. 关联相关 issue（例如 `Closes #123`）。
4. 确保 CI 通过。
5. 请求维护者 review。
6. 通过追加提交来回应 review 意见（除非被要求，否则 review 期间不要 force-push）。

## 报告 Bug

在提交 bug 报告之前：

1. 搜索[现有 issue](https://github.com/tickraft/taichi/issues) 以避免重复。
2. 在最新的 `main` 上复现该问题。

使用 [Bug 报告模板](.github/ISSUE_TEMPLATE/bug_report.md) 并提供：

- 对问题的清晰描述。
- 逐步复现步骤。
- 期望行为与实际行为。
- 环境信息：Taichi 版本、操作系统、Go 版本，以及最小化的相关配置片段。
- 相关日志（请去除敏感信息）。

## 提议新功能

1. 使用功能请求模板开一个[讨论](https://github.com/tickraft/taichi/discussions)或 issue，描述问题与提议方向。
2. 在投入实现之前，让维护者和社区就设计方向达成一致。
3. 方向确定后，按标准开发流程继续。

## 添加新 Skill

Skill 是 Taichi 中测试执行的最小单元。要新增内置 skill：

1. 学习 `pkg/skill/api` 下的实现模式。
2. 为你的 skill 实现 `TestSkill` 接口。
3. 在 `cmd/taichi/run.go` 中注册该 skill。
4. 添加覆盖成功路径与失败场景的测试。

## 添加新环境类型

要支持新的环境（例如新的容器运行时或远程沙箱）：

1. 实现 `env.Environment` 接口。
2. 通过 `env.New` 注册实现。

## 第三方插件

Taichi 是插件驱动且语言无关的。你**无需**修改 Taichi 本身即可添加插件 —— 可以使用 `sdks/` 下的 SDK 以任意语言实现 skill。参见该目录下的示例。

## 行为准则

本项目的参与受 [行为准则](CODE_OF_CONDUCT.zh.md) 约束。参与即表示你同意遵守其条款。

## 许可证

贡献即表示你同意你的贡献内容将在 [Apache License 2.0](LICENSE) 下授权。无需签署 CLA（贡献者许可协议）。
