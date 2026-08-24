# Retrieval evaluation corpus (FTS + graph gate)

Held-out organizational tasks used to measure sparse/graph retrieval before
enabling dense embeddings (ADR 0015).

## Metrics

| Metric | Description |
|--------|-------------|
| Task completion | Human/judge: packet sufficient for the stated task |
| Recall@K | Fraction of gold resource IDs in top-K citations/nodes |
| MRR / nDCG | Rank quality of first relevant citation |
| Citation precision | Cited resources that are gold-relevant |
| Tool-call count | Retrieval rounds (target ≤3) |
| Tokens | Approximate packet JSON size / 4 |
| p50 / p95 latency | End-to-end tool latency |
| AuthZ checks | BatchCheck size per request |
| Abstention correctness | Denied/missing cases correctly abstain or request_access |

## Dense retrieval gate

Enable dense fusion only if held-out evaluation shows a **statistically credible
material gain** over FTS+graph on Recall@K and nDCG while staying within
latency/cost SLOs in `docs/operations/slo-baselines.md`. Embed only opted-in,
hash-deduplicated summaries/chunks asynchronously—never raw evidence by default.

## Corpus format

See `eval/retrieval/corpus.jsonl` and runner `eval/retrieval/score.go`.
