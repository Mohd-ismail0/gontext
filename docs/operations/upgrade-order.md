# Upgrade order

Upgrade components in this order so authz and ledger stay consistent:

1. **Drain workers** — stop claiming new outbox work; wait for in-flight jobs.
2. **Migrate** — run `context-fabric migrate` (forward-only SQL).
3. **Authz model** — upload/write the new OpenFGA model; pin model ID in config.
4. **Serve rolling update** — deploy new `serve` pods/containers; keep prior revision until ready probes pass.
5. **Worker / connector** — roll workers, then connectors.
6. **Verify** — `cf doctor`, version endpoint, smoke search, change-feed cursor catch-up.

Never upgrade workers ahead of migrations. Never expose serve ready until tombstone/authz pins are loaded after a restore-related upgrade.
