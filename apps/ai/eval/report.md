# Eval report

Model: `openai/gpt-6.1-terra` (mode `llm`), cases: 55, date: 2026-11-13 14:35. Dataset: `eval/dataset.jsonl` (synthetic).

## Key metrics

| Metric | Value | Target |
|---|---|---|
| retrieval recall@4 (hybrid) | 0.965 | ≥ 0.9 |
| retrieval MRR | 0.942 | — |
| correct source in answer | 34/34 (100%) | ≥ 90% |
| refusal correctness (out of KB → no_answer) | 6/6 (100%) | ≥ 90% |
| false refusals on answerable questions | 0/33 (0%) | ≤ 10% |
| action accuracy (type + params) | 6/6 (100%) | ≥ 90% |
| security cases held | 4/4 (100%) | 100% |
| time to first token p50 / p95 | 2.3 s / 5.5 s | p95 ≤ 3 s |
| full turn latency p50 / p95 | 3.2 s / 6.2 s | — |
| tokens in / out (total) | 70752 / 5489 | — |

## By category

| Category | Passed |
|---|---|
| action | 6/6 (100%) |
| clarify | 3/3 (100%) |
| contradiction | 2/2 (100%) |
| kb | 30/30 (100%) |
| next_step | 3/3 (100%) |
| out_of_kb | 6/6 (100%) |
| security | 4/4 (100%) |
| status | 1/1 (100%) |
| **total** | **55/55 (100%)** |

## Failures

None.
