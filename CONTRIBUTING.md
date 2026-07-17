# Contributing to taichi

> 🌐 Languages: [English](CONTRIBUTING.md) | [中文](CONTRIBUTING.zh.md)

First of all, thank you for taking the time to contribute to taichi! 🎉

This document describes how to set up a development environment and the conventions we follow. Whether you are fixing a typo, reporting a bug, or proposing a new feature, every contribution is welcome.

## Table of Contents

- [Contributing to taichi](#contributing-to-taichi)
  - [Table of Contents](#table-of-contents)
  - [Development Environment](#development-environment)
  - [Development Workflow](#development-workflow)
  - [Branch Naming](#branch-naming)
  - [Commit Messages](#commit-messages)
  - [Code Standards](#code-standards)
  - [Pull Request Process](#pull-request-process)
  - [Reporting Bugs](#reporting-bugs)
  - [Proposing Features](#proposing-features)
  - [Adding a New Skill](#adding-a-new-skill)
  - [Adding a New Environment Type](#adding-a-new-environment-type)
  - [Third-Party Plugins](#third-party-plugins)
  - [Code of Conduct](#code-of-conduct)
  - [License](#license)

## Development Environment

To build and test taichi locally you need:

- **Go** 1.26 or newer
- **make**
- **golangci-lint** (for linting)

Clone your fork and build the CLI:

```bash
git clone https://github.com/<your-username>/taichi.git
cd taichi
make build
```

Run the test suite:

```bash
make test
```

Run linters:

```bash
make lint
```

## Development Workflow

1. **Fork** the repository on GitHub.
2. **Clone** your fork locally.
3. Create a **feature branch** from `main` (see [Branch Naming](#branch-naming)).
4. **Commit** your changes following [Conventional Commits](#commit-messages).
5. **Push** the branch to your fork.
6. Open a **Pull Request** against the upstream `main` branch.

## Branch Naming

Use a short, descriptive prefix followed by a slash and a kebab-case summary:

- `feat/<short-description>` — new feature
- `fix/<short-description>` — bug fix
- `docs/<short-description>` — documentation only
- `refactor/<short-description>` — code refactor without behavior change

Example: `feat/add-grpc-skill`

## Commit Messages

We follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

```
<type>(<optional scope>): <description>

[optional body]

[optional footer(s)]
```

Recognized types:

- `feat` — a new feature
- `fix` — a bug fix
- `docs` — documentation only
- `refactor` — code change that neither fixes a bug nor adds a feature
- `test` — adding or updating tests
- `chore` — maintenance tasks, tooling, dependencies
- `breaking` — a change that breaks backward compatibility (also use the `BREAKING CHANGE:` footer)

Example:

```
feat(prober): add HTTP probe skill
```

## Code Standards

All contributions must pass:

- `gofmt -l .` (no output)
- `go vet ./...`
- `golangci-lint run`

Core logic must be covered by unit tests. When fixing a bug, add a regression test that reproduces the issue.

## Pull Request Process

1. Fill in the [PR template](.github/PULL_REQUEST_TEMPLATE.md).
2. Describe **what** changed and **why**.
3. Link the related issue (e.g. `Closes #123`).
4. Ensure CI is green.
5. Request review from maintainers.
6. Address review feedback by pushing additional commits (do not force-push during review unless asked).

## Reporting Bugs

Before opening a bug report:

1. Search [existing issues](https://github.com/tickraft/taichi/issues) to avoid duplicates.
2. Reproduce the issue on the latest `main`.

Use the [Bug Report template](.github/ISSUE_TEMPLATE/bug_report.md) and provide:

- A clear description of the problem.
- Step-by-step reproduction instructions.
- Expected behavior vs. actual behavior.
- Environment details: taichi version, OS, Go version, and a minimal relevant config snippet.
- Relevant logs (redact secrets).

## Proposing Features

1. Open a [discussion](https://github.com/tickraft/taichi/discussions) or an issue using the Feature Request template to describe the problem and the proposed direction.
2. Allow maintainers and the community to weigh in on the design before investing in implementation.
3. Once the direction is agreed, proceed with the standard development workflow.

## Adding a New Skill

A skill is the unit of test execution in taichi. To add a new built-in skill:

1. Study the implementation pattern under `pkg/skill/api`.
2. Implement the `TestSkill` interface for your skill.
3. Register the skill in `cmd/taichi/run.go`.
4. Add tests that cover both happy-path and failure scenarios.

## Adding a New Environment Type

To support a new environment (e.g. a new container runtime or remote sandbox):

1. Implement the `env.Environment` interface.
2. Register the implementation via `env.New`.

## Third-Party Plugins

taichi is plugin-driven and language-agnostic. You do **not** need to modify taichi itself to add a plugin — you can implement skills in any language using the SDKs under `sdks/`. See that directory for examples.

## Code of Conduct

Participation in this project is governed by the [Code of Conduct](CODE_OF_CONDUCT.md). By participating you agree to abide by its terms.

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE). No Contributor License Agreement (CLA) is required.
