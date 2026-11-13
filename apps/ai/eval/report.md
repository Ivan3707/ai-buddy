# Eval report

Model: `openai/gpt-6.1-terra` (mode `llm`), cases: 55, date: 2026-11-13 14:07. Dataset: `eval/dataset.jsonl` (synthetic).

## Key metrics

| Metric | Value | Target |
|---|---|---|
| retrieval recall@4 (BM25 baseline) | 0.942 | ≥ 0.9 |
| retrieval MRR | 0.877 | — |
| correct source in answer | 33/34 (97%) | ≥ 90% |
| refusal correctness (out of KB → no_answer) | 6/6 (100%) | ≥ 90% |
| false refusals on answerable questions | 0/33 (0%) | ≤ 10% |
| action accuracy (type + params) | 6/6 (100%) | ≥ 90% |
| security cases held | 4/4 (100%) | 100% |
| time to first token p50 / p95 | 2.3 s / 5.1 s | p95 ≤ 3 s |
| full turn latency p50 / p95 | 3.3 s / 5.9 s | — |
| tokens in / out (total) | 69640 / 5348 | — |

## By category

| Category | Passed |
|---|---|
| action | 6/6 (100%) |
| clarify | 3/3 (100%) |
| contradiction | 2/2 (100%) |
| kb | 29/30 (97%) |
| next_step | 3/3 (100%) |
| out_of_kb | 6/6 (100%) |
| security | 4/4 (100%) |
| status | 1/1 (100%) |
| **total** | **54/55 (98%)** |

## Failures

- **kb-15** «Во сколько стендап?» — text lacks '10:30'.
  Got: finish `answered`, sources ['first-week'], action `{"question": "Во сколько проходит ежедневный стендап команды?", "context": "В базе знаний указано, что стендап ежедневный, но время не указано."}`; text: В базе знаний этого нет: указано только, что стендап ежедневный [1]. Предлагаю уточнить время у Олега.
