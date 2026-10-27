# Voice decision latency comparison

User constraints: Apple Silicon, decision-step p95 <=100ms. Pure pinyin is not a priority. Compare the existing Julia CPU implementation and downloaded Chinese Decision 2B Q8 Metal service; retain accuracy results and do not switch production engines.

- Build deterministic Chinese voice fixtures: 0/4/12 history pairs, 1 or3 named decisions, three latest utterances per group. These are performance workloads, not new accuracy evidence.
- Add a reusable standard-library closed-loop HTTP benchmark. Test nearest-rank p95/p99, malformed multiquestion response detection and error budget accounting before implementation.
- Warm models, preserve HTTP connections per worker; measure120 requests per group at1/4 outstanding clients, 100ms budget. Run models separately to avoid simultaneous inference contention. Record input tokens and every request/error; reject invalid fixtures rather than hiding errors.
- Verify actual model identities, versions and GPU offload. Explain that history/question counts and the Python server's serial per-question lock affect latency; no latency inference from parameter count alone.
- Save results and reproduce calculations, get independent review, run Python/Go checks, then commit/push/merge/verify main and clean up only this task's branch under standing authorization.

Progress: existing16 tests pass (13 prior +3 new); new benchmark tests first failed due to absent implementation, then passed. Production inference code is unchanged.

Results: both models completed12cells x120requests=1440requests each, zero errors. Raw nearest-rank metrics were independently recomputed.2B single-question p95=47/86/132ms for0/4/12history pairs; three-question p95=139/254/396ms. Current serial backend exceeds100ms under4outstanding requests in every tested group. Review found no blocking measurement issue; report explicitly includes first-worker TCP setup/client validation and closed-loop limitations. Production inference code unchanged.
