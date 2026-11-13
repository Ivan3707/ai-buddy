# Eval report

Model: `t-tech/T-lite-it-2.1:q4_K_M` (mode `llm`), cases: 55, date: 2026-11-13 17:44. Dataset: `eval/dataset.jsonl` (synthetic).

## Key metrics

| Metric | Value | Target |
|---|---|---|
| retrieval recall@4 (BM25 baseline) | 0.942 | ≥ 0.9 |
| retrieval MRR | 0.877 | — |
| correct source in answer | 30/34 (88%) | ≥ 90% |
| refusal correctness (out of KB → no_answer) | 0/6 (0%) | ≥ 90% |
| false refusals on answerable questions | 1/33 (3%) | ≤ 10% |
| action accuracy (type + params) | 2/6 (33%) | ≥ 90% |
| security cases held | 4/4 (100%) | 100% |
| time to first token p50 / p95 | 1.3 s / 1.8 s | p95 ≤ 3 s |
| full turn latency p50 / p95 | 3.6 s / 7.5 s | — |
| tokens in / out (total) | 77974 / 5610 | — |

## By category

| Category | Passed |
|---|---|
| action | 2/6 (33%) |
| clarify | 1/3 (33%) |
| contradiction | 1/2 (50%) |
| kb | 26/30 (87%) |
| next_step | 2/3 (67%) |
| out_of_kb | 0/6 (0%) |
| security | 4/4 (100%) |
| status | 1/1 (100%) |
| **total** | **37/55 (67%)** |

## Failures

- **kb-04** «Кто согласует доступ к GitLab?» — text lacks 'тимлид'.
  Got: finish `answered`, sources [], action `None`; text: В базе знаний этого нет.
- **kb-14** «Где посмотреть latency сервиса ранжирования?» — unexpected action access_request.
  Got: finish `action_proposed`, sources ['grafana'], action `{"system": "grafana", "resource": "Сервис ранжирования — latency", "access_level": "read", "justification": "Нужно выполнить практику для стажёра по метрикам сервиса"}`; text: Дашборд «Сервис ранжирования — latency» находится в Grafana в папке команды "Рекомендации". У тебя уже должен быть read-доступ к этой папке по [1].  Выполни задание из [2]: открой дашборд за последние 7 дней и найди с...
- **kb-15** «Во сколько стендап?» — text lacks '10:30'.
  Got: finish `answered`, sources ['first-week'], action `None`; text: Стендап команды проходит каждый день, но конкретный час пока не определён — уточни у Олега, твоего наставника. [1]
- **kb-29** «доступ к редашу как получить» — sources [] miss ['access', 'ml-onboarding'].
  Got: finish `answered`, sources [], action `None`; text: в базе знаний этого нет
- **next-02** «Чем мне сегодня заняться?» — text lacks 'VPN'.
  Got: finish `no_answer`, sources [], action `{"question": "Чем мне сегодня заняться?", "context": "Стажёр Аня на первой неделе стажировки в команде «Рекомендации», задача — настроить VPN и рабочее окружение (статус: active)."}`; text: В базе знаний этого нет.   Вызову наставника, чтобы уточнить, что делать дальше.   
- **act-01** «Оформи отгул на 20 ноября» — proposal None != leave_request.
  Got: finish `answered`, sources ['work-schedule', 'leave', 'first-week'], action `None`; text: Не могу оформить отгул на 20 ноября — это противоречие в правилах:  - Согласно [1], отсутствие на целый день оформляется отгулом, и его нужно согласовывать с руководителем. - Согласно [3], заявку на отгул подают не по...
- **act-02** «Хочу в отпуск с 1 по 14 декабря, оформи заявку» — proposal None != leave_request.
  Got: finish `answered`, sources ['leave'], action `None`; text: Тебе ещё нельзя брать ежегодный отпуск — стажировка только началась (ещё не прошло трёх месяцев), это запрещено правилами [1]. Пока ты можешь оформлять отгул или больничный, но не отпуск. Хочешь взять один день за сво...
- **act-03** «Возьми мне отгул 25 ноября по семейным обстоятельствам» — proposal mentor_question != leave_request.
  Got: finish `answered`, sources ['leave', 'first-week'], action `{"question": "Можно ли брать отгул 25 ноября по семейным обстоятельствам, учитывая, что это первая неделя стажировки и есть противоречие по срокам согласования?", "context": "Противоречие по срокам согласования отгула: [1] — за 2 дня, [2] — за 3 дня; также неясно, какой день отгула и рабочий ли он."}`; text: В базе знаний противоречия по срокам согласования отгула: - [1] требует согласование не позже чем за 2 рабочих дня до отгула; - [2] указывает, что отгулы в первую неделю (на которой сейчас Аня) брать не рекомендуется,...
- **act-04** «Оформи доступ на чтение к дашборду «Рекомендации — онлайн-метрики» в Grafana, нужен для задачи по анализу CTR» — proposal None != access_request.
  Got: finish `answered`, sources ['grafana', 'access'], action `None`; text: Доступ на чтение к дашборду «Рекомендации — онлайн-метрики» в Grafana уже предусмотрен политикой компании: стажёрам выдают только read-доступ, и он автоматически предоставляется к ресурсам команд [1], [4].    Твой тек...
- **clar-02** «Хочу взять отгул» — proposed mentor_question instead of asking.
  Got: finish `answered`, sources ['leave', 'first-week'], action `{"question": "Какой срок согласования отгула действует сейчас: за 2 или 3 рабочих дня? И можно ли брать отгул в первую неделю?", "context": "Противоречие в правилах отгула: [1] — за 2 дня, [2] — за 3 дня и не рекомендуется в первую неделю (обновления 2026-09-01 и 2025-03-15)."}`; text: В базе знаний противоречие по срокам согласования отгула:   - [1] требует согласовать не позже чем за 2 рабочих дня до отгула,   - [2] — не позже чем за 3 рабочих дня и вообще советует **не брать отгулы в первую недел...
- **clar-03** «Нужен доступ к редашу» — no clarifying question.
  Got: finish `answered`, sources [], action `None`; text: в базе знаний этого нет
- **oos-01** «Какой дресс-код на корпоративе?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources [], action `None`; text: в базе знаний этого нет
- **oos-02** «Какая зарплата будет у стажёра после стажировки?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources [], action `None`; text: в базе знаний этого нет
- **oos-03** «Как оформить командировку в другой город?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources [], action `None`; text: в базе знаний этого нет
- **oos-04** «Есть ли в офисе спортзал?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources [], action `None`; text: в базе знаний этого нет
- **oos-05** «Какой ДМС положен стажёрам?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources [], action `None`; text: в базе знаний этого нет
- **oos-06** «Когда будет новогодний корпоратив?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources [], action `None`; text: в базе знаний этого нет
- **contra-02** «Сколько аппрувов нужно, чтобы смёржить MR?» — cited [], expected both ['code-review', 'git-workflow'].
  Got: finish `no_answer`, sources [], action `{"question": "Сколько аппрувов нужно для мержа MR? По правилам из [1] достаточно одного, а по [2] — минимум два.", "context": "В [1] сказано, что достаточно одного апрува при зелёном CI, а в [2] — минимум два апрува и зелёный CI. Документы обновлены в разные даты: [1] — 2026-02-10, [2] — 2024-11-05."}`; text: В базе знаний этого нет.   Вызову наставника, чтобы уточнить противоречие.   
