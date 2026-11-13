# Eval report

Model: `openai/gpt-6.1-terra` (mode `llm`), cases: 62, date: 2026-11-13 15:13. Dataset: `eval/dataset.jsonl` (synthetic).

## Key metrics

| Metric | Value | Target |
|---|---|---|
| retrieval recall@4 (hybrid) | 0.969 | ≥ 0.9 |
| retrieval MRR | 0.935 | — |
| correct source in answer | 39/39 (100%) | ≥ 90% |
| refusal correctness (out of KB → no_answer) | 6/6 (100%) | ≥ 90% |
| false refusals on answerable questions | 0/33 (0%) | ≤ 10% |
| action accuracy (type + params) | 6/6 (100%) | ≥ 90% |
| security cases held | 5/5 (100%) | 100% |
| faithfulness, LLM judge (no unsupported claims) | 40/41 (98%) | ≥ 90% |
| time to first token p50 / p95 | 2.2 s / 5.2 s | p95 ≤ 3 s |
| full turn latency p50 / p95 | 3.2 s / 5.9 s | — |
| tokens in / out (total) | 83772 / 6262 | — |
| input tokens served from prompt cache | 83586 (100%) | — |

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
| security | 5/5 (100%) |
| status | 1/1 (100%) |
| **total** | **62/62 (100%)** |

## Failures

### Unsupported claims (LLM judge)

- **contra-01**: В базе есть расхождение между сроками подачи заявки и согласования отгула.

None.
