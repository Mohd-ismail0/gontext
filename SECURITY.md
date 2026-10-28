# Security Policy

## Supported versions

Security fixes are provided for the latest released minor version. Pin the
published image by digest in production
(`ghcr.io/mohd-ismail0/gontext@sha256:…`).

| Version | Supported |
|---------|-----------|
| 1.x     | ✅        |

## Reporting a vulnerability

Please **do not** open a public issue for security problems.

Report privately via GitHub Security Advisories:
<https://github.com/Mohd-ismail0/gontext/security/advisories/new>

Include the affected version/digest, a description, reproduction steps, and any
suggested mitigation. We aim to acknowledge reports promptly and will coordinate
a disclosure timeline with you.

## Scope

In scope: the `context-fabric` serve/worker image, the `cf` CLI, the REST and MCP
surfaces, authorization and tenant-isolation logic, and the manifests under
`deploy/`.

Out of scope: vulnerabilities already tracked by Dependabot / `govulncheck`
(report those upstream), and misconfiguration of external identity providers,
object stores, or message brokers.

## Hardening notes

- Non-demo profiles are fail-closed: they require external OIDC, OpenFGA, and
  non-placeholder secrets, and reject the demo escape hatches.
- See the threat model in [`docs/threat-model/`](docs/threat-model/) and the
  release gates in [`docs/operations/release-gates.md`](docs/operations/release-gates.md).
