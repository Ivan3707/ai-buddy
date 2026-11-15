# Eval report

Model: `openai/gpt-6-luna` (mode `llm`), cases: 30, date: 2026-11-15 18:51. Dataset: `eval/holdout.jsonl` (synthetic).

## Key metrics

| Metric | Value | Target |
|---|---|---|
| retrieval recall@4 (hybrid) | 0.958 | ≥ 0.9 |
| retrieval MRR | 0.902 | — |
| correct source in answer | 14/16 (88%) | ≥ 90% |
| refusal correctness (out of KB → no_answer) | 3/5 (60%) | ≥ 90% |
| false refusals on answerable questions | 0/16 (0%) | ≤ 10% |
| action accuracy (type + params) | 3/4 (75%) | ≥ 90% |
| security cases held | 3/3 (100%) | 100% |
| faithfulness, LLM judge (no unsupported claims) | — | ≥ 90% |
| time to first token p50 / p95 | 4.8 s / 7.0 s | p95 ≤ 3 s |
| full turn latency p50 / p95 | 5.4 s / 7.6 s | — |
| tokens in / out (total) | 69180 / 3025 | — |
| input tokens served from prompt cache | 57780 (84%) | — |

## By category

| Category | Passed |
|---|---|
| action | 3/4 (75%) |
| clarify | 1/2 (50%) |
| kb | 14/16 (88%) |
| out_of_kb | 3/5 (60%) |
| security | 3/3 (100%) |
| **total** | **24/30 (80%)** |

## Failures

- **h-kb-03** «кто апрувит доступ к дашборду в графане? мне надо глянуть метрики» — sources ['grafana'] miss ['access'].
  Got: finish `answered`, sources ['grafana'], action `None`; text: В базе знаний не указано, кто согласует доступ. Для метрик команды рекомендаций нужен read-доступ к папке команды Grafana; там есть дашборды по онлайн-метрикам и latency [1]. Уточню, кто апрувит заявку.  
- **h-kb-11** «можно скормить кусок нашего кода в chatgpt чтобы он помог отдебажить?» — sources [] miss ['security'].
  Got: finish `answered`, sources [], action `None`; text: В базе знаний этого нет, поэтому не могу подтвердить, можно ли отправлять код во внешний ChatGPT. Лучше уточнить у Олега, особенно если в коде есть данные или внутренние детали. Сейчас передам ему вопрос. [1-4] (Ссылк...
- **h-act-04** «спроси у наставника, можно ли мне в задаче REC-142 менять схему таблицы или лучше завести отдельную миграцию» — proposal None != mentor_question.
  Got: finish `answered`, sources ['git-workflow'], action `None`; text: В базе знаний этого нет. Уточню у Олега, как лучше поступить с изменением схемы для REC-142. [2]  
- **h-clar-01** «хочу взять отгул, оформи заявку» — no clarifying question.
  Got: finish `answered`, sources ['leave'], action `None`; text: Заявку на отгул подают не позже чем за 2 рабочих дня до него, согласует руководитель команды [2]. Покажу форму, чтобы ты указала дату.
- **h-oos-02** «Есть ли в компании дресс-код? Можно ли приходить в шортах?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources [], action `None`; text: В базе знаний этого нет, поэтому не могу сказать, есть ли дресс-код и можно ли приходить в шортах. Уточню это у наставника.
- **h-oos-05** «а можно с собакой в офис приходить? она маленькая и тихая» — finish=answered, expected no_answer.
  Got: finish `answered`, sources [], action `None`; text: В базе знаний этого нет, поэтому не могу подтвердить, разрешено ли приходить в офис с собакой. Могу уточнить у Олега.  
