# Eval report

Model: `openai/gpt-6.1-terra` (mode `llm`), cases: 67, date: 2026-11-13 15:37. Dataset: `eval/dataset.jsonl` (synthetic).

## Key metrics

| Metric | Value | Target |
|---|---|---|
| retrieval recall@4 (hybrid) | 0.972 | ≥ 0.9 |
| retrieval MRR | 0.931 | — |
| correct source in answer | 42/42 (100%) | ≥ 90% |
| refusal correctness (out of KB → no_answer) | 6/6 (100%) | ≥ 90% |
| false refusals on answerable questions | 0/33 (0%) | ≤ 10% |
| action accuracy (type + params) | 6/6 (100%) | ≥ 90% |
| security cases held | 6/6 (100%) | 100% |
| faithfulness, LLM judge (no unsupported claims) | — | ≥ 90% |
| time to first token p50 / p95 | 2.4 s / 5.1 s | p95 ≤ 3 s |
| full turn latency p50 / p95 | 3.5 s / 5.7 s | — |
| tokens in / out (total) | 100003 / 6385 | — |
| input tokens served from prompt cache | 72441 (72%) | — |

## By category

| Category | Passed |
|---|---|
| action | 6/6 (100%) |
| clarify | 3/3 (100%) |
| contradiction | 2/2 (100%) |
| kb | 30/30 (100%) |
| multi_turn | 6/6 (100%) |
| next_step | 3/3 (100%) |
| out_of_kb | 6/6 (100%) |
| page_context | 2/2 (100%) |
| quiz | 2/2 (100%) |
| security | 6/6 (100%) |
| status | 1/1 (100%) |
| **total** | **67/67 (100%)** |

## Failures

None.
