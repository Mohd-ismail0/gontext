# ADR 0016: Compose-first production release

## Status

Accepted

## Context

The project has strong architectural intent (one image, profile-driven packaging, fail-closed retrieval, AuthZ outbox, FORCE RLS) but is not yet deployable as a production starter stack. Audit findings include: demo auth on durable Postgres, unpublished images, missing release automation, admin APIs without OpenFGA gates, host-managed backup tooling, and Compose defaults that do not match the starter profile.

## Decision

1. **Primary acceptance target:** single-host Docker Compose **starter** profile with bundled Postgres/MinIO/NATS/OpenFGA, external OIDC, `INDEX_BACKEND=postgres`, and split `serve` + `worker` roles.
2. **Release registry:** publish signed multi-arch images to `ghcr.io/mohd-ismail0/gontext`. Keep the internal binary name `context-fabric` and Go module path `github.com/xsama/context-fabric` for this milestone.
3. **Immutable digest policy:** production deploys pin image digest; semver tags are pointers only. GA requires signature, SBOM, and vulnerability scan on the published digest.
4. **Host-managed backup:** the application image stays minimal. Backup, restore, and object-version replication use host-installed or vendor utility containers — no ops sidecar in the app image.
5. **Production OIDC:** starter requires operator-supplied external IdP (issuer, audience, discovery/JWKS, claim mapping). CI may use a disposable Dex-compatible test IdP.
6. **Promotion order:** Compose starter GA → Coolify (xsama) → Helm (scaled), reusing the same signed digest.
7. **Threat assumptions:** only the TLS gateway is public; metrics and dependencies are private; local/demo auth and `CONTEXT_FABRIC_ALLOW_*` escape hatches are forbidden outside demo.

## Consequences

- Demo and starter paths must be clearly separated in Compose, env examples, and documentation.
- Release gates gain non-waivable P0 security and recovery checks before GA.
- Profiles, Helm, Makefile, and README image references move to `ghcr.io/mohd-ismail0/gontext`.
- `backup` and `restore` image roles are deprecated or documented as host-only; `reconcile` may remain in-process or invoke host scripts.

## Related

- [ADR 0002](0002-deployment-profiles.md) — deployment profiles
- [ADR 0003](0003-bundled-vs-external-infra.md) — bundled vs external infra
- [docs/operations/release-gates.md](../operations/release-gates.md) — release checklist
