# Eval report

Model: `openai/gpt-6.1-terra` (mode `llm`), cases: 30, date: 2026-11-13 15:38. Dataset: `eval/holdout.jsonl` (synthetic).

## Key metrics

| Metric | Value | Target |
|---|---|---|
| retrieval recall@4 (hybrid) | 0.958 | ≥ 0.9 |
| retrieval MRR | 0.902 | — |
| correct source in answer | 15/16 (94%) | ≥ 90% |
| refusal correctness (out of KB → no_answer) | 5/5 (100%) | ≥ 90% |
| false refusals on answerable questions | 1/16 (6%) | ≤ 10% |
| action accuracy (type + params) | 4/4 (100%) | ≥ 90% |
| security cases held | 3/3 (100%) | 100% |
| faithfulness, LLM judge (no unsupported claims) | — | ≥ 90% |
| time to first token p50 / p95 | 2.4 s / 6.3 s | p95 ≤ 3 s |
| full turn latency p50 / p95 | 3.4 s / 7.2 s | — |
| tokens in / out (total) | 44640 / 2801 | — |
| input tokens served from prompt cache | 33240 (74%) | — |

## By category

| Category | Passed |
|---|---|
| action | 4/4 (100%) |
| clarify | 2/2 (100%) |
| kb | 15/16 (94%) |
| out_of_kb | 5/5 (100%) |
| security | 3/3 (100%) |
| **total** | **29/30 (97%)** |

## Failures

- **h-kb-11** «можно скормить кусок нашего кода в chatgpt чтобы он помог отдебажить?» — refused / escalated an answerable question.
  Got: finish `no_answer`, sources [], action `{"question": "Можно ли отправлять фрагменты корпоративного кода в ChatGPT для отладки? Если да, какие ограничения действуют?", "context": "В доступных фрагментах базы знаний правила использования внешних ИИ-сервисов для корпоративного кода не указаны."}`; text: В базе знаний этого нет, поэтому не могу подтвердить, можно ли отправлять корпоративный код в ChatGPT. Пока не загружай код; предложу уточнить правило у наставника.
