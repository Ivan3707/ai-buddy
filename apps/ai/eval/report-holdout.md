# Eval report

Model: `openai/gpt-6.1-terra` (mode `llm`), cases: 30, date: 2026-11-13 14:39. Dataset: `eval/holdout.jsonl` (synthetic).

## Key metrics

| Metric | Value | Target |
|---|---|---|
| retrieval recall@4 (hybrid) | 0.958 | ≥ 0.9 |
| retrieval MRR | 0.902 | — |
| correct source in answer | 14/16 (88%) | ≥ 90% |
| refusal correctness (out of KB → no_answer) | 5/5 (100%) | ≥ 90% |
| false refusals on answerable questions | 1/16 (6%) | ≤ 10% |
| action accuracy (type + params) | 4/4 (100%) | ≥ 90% |
| security cases held | 3/3 (100%) | 100% |
| time to first token p50 / p95 | 2.5 s / 6.0 s | p95 ≤ 3 s |
| full turn latency p50 / p95 | 3.7 s / 6.3 s | — |
| tokens in / out (total) | 38670 / 3106 | — |

## By category

| Category | Passed |
|---|---|
| action | 4/4 (100%) |
| clarify | 2/2 (100%) |
| kb | 14/16 (88%) |
| out_of_kb | 5/5 (100%) |
| security | 3/3 (100%) |
| **total** | **28/30 (93%)** |

## Failures

- **h-kb-03** «кто апрувит доступ к дашборду в графане? мне надо глянуть метрики» — sources ['grafana'] miss ['access'].
  Got: finish `answered`, sources ['grafana'], action `{"question": "Кто согласует доступ к дашборду в Grafana?", "context": "Мне нужно посмотреть метрики команды «Рекомендации». В базе указано, что нужен read-доступ к папке команды, но согласующий не указан."}`; text: Кто согласует доступ к дашборду Grafana, в базе знаний этого нет. Для просмотра нужен read-доступ к папке команды [1]. Предложу уточнить у наставника.
- **h-kb-11** «можно скормить кусок нашего кода в chatgpt чтобы он помог отдебажить?» — refused / escalated an answerable question.
  Got: finish `no_answer`, sources [], action `{"question": "Можно ли отправлять фрагменты кода компании в ChatGPT для отладки? Если да, какие ограничения действуют?", "context": "В доступных фрагментах базы знаний нет правила о передаче кода в ChatGPT."}`; text: В базе знаний этого нет: я не могу подтвердить, что код компании можно передавать в ChatGPT. Пока не отправляй его туда; можно отлаживать локально или использовать обезличенный пример без кода компании. Предложу уточн...
