# Security Policy

> 🌐 Languages: [English](SECURITY.md) | [中文](SECURITY.zh.md)

## Supported Versions

Only the **latest published release** of Taichi receives security fixes.

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| < older | :x: (upgrade first) |

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please report suspected vulnerabilities privately via email to
**security@tickraft.com**.

To help us triage and reproduce the issue, please include:

- A description of the problem and its potential impact.
- Step-by-step instructions to reproduce the vulnerability.
- The scope of affected components and configurations.
- A suggested fix or mitigation, if you have one.

## Response Timeline

- We will acknowledge receipt of your report within **72 hours**.
- We will provide an initial assessment within **7 days**, including a
  preliminary determination of severity and a remediation plan.
- We will keep you informed of progress until the issue is resolved or closed.

## Disclosure Policy

- We follow a coordinated disclosure process.
- Once a fix is available and released, we will publish a
  [GitHub Security Advisory](https://github.com/tickraft/taichi/security/advisories)
  crediting the reporter (unless they prefer to remain anonymous).
- We kindly ask that you do not disclose the vulnerability publicly until a fix
  has been released.

## Out of Scope

Vulnerabilities in third-party dependencies are not covered by this policy.
Please report them to the upstream maintainers directly. We use Dependabot to
track and update vulnerable dependencies; you can open a regular issue to
request an urgent dependency bump.

## Acknowledgements

We are grateful to everyone who responsibly discloses security issues.
Reporters who wish to be credited will be acknowledged in the relevant release
notes.
