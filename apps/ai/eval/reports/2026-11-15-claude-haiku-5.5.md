# Eval report

Model: `anthropic/claude-haiku-5.5` (mode `llm`), cases: 68, date: 2026-11-15 18:47. Dataset: `eval/dataset.jsonl` (synthetic).

## Key metrics

| Metric | Value | Target |
|---|---|---|
| retrieval recall@4 (hybrid) | 0.963 | ≥ 0.9 |
| retrieval MRR | 0.932 | — |
| correct source in answer | 43/43 (100%) | ≥ 90% |
| refusal correctness (out of KB → no_answer) | 3/6 (50%) | ≥ 90% |
| false refusals on answerable questions | 0/33 (0%) | ≤ 10% |
| action accuracy (type + params) | 4/6 (67%) | ≥ 90% |
| security cases held | 6/6 (100%) | 100% |
| faithfulness, LLM judge (no unsupported claims) | — | ≥ 90% |
| time to first token p50 / p95 | 4.9 s / 8.4 s | p95 ≤ 3 s |
| full turn latency p50 / p95 | 6.2 s / 10.0 s | — |
| tokens in / out (total) | 272853 / 29890 | — |
| input tokens served from prompt cache | 0 (0%) | — |

## By category

| Category | Passed |
|---|---|
| action | 4/6 (67%) |
| clarify | 3/3 (100%) |
| contradiction | 2/2 (100%) |
| kb | 30/30 (100%) |
| multi_turn | 6/7 (86%) |
| next_step | 3/3 (100%) |
| out_of_kb | 3/6 (50%) |
| page_context | 2/2 (100%) |
| quiz | 2/2 (100%) |
| security | 6/6 (100%) |
| status | 1/1 (100%) |
| **total** | **62/68 (91%)** |

## Failures

- **act-01** «Оформи отгул на 20 ноября» — proposal None != leave_request.
  Got: finish `answered`, sources ['leave', 'first-week'], action `None`; text: Отгул на пятницу 20 ноября можно подать сейчас, срок не горит. По [1] заявку нужно подать не позже чем за 2 рабочих дня, то есть до среды 18 ноября. Но в памятке первой недели [4] (от 2025-03-15) написано «не позже че...
- **act-03** «Возьми мне отгул 25 ноября по семейным обстоятельствам» — proposal None != leave_request.
  Got: finish `answered`, sources ['leave', 'first-week'], action `None`; text: Ок, оформлю отгул на 25 ноября (среда). Заявка появится карточкой, и ты подтвердишь её сам.  Срок подачи в документах разный. По [1] (обновлён 2026-09-01) заявку нужно подать не позже чем за 2 рабочих дня, то есть до ...
- **oos-02** «Какая зарплата будет у стажёра после стажировки?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources ['onboarding-plan', 'leave', 'work-schedule'], action `{"question": "Олег, подскажи, пожалуйста, какая зарплата предусмотрена у стажёра после окончания стажировки и кто может ответить на этот вопрос точнее: ты или HR?", "context": "В базе знаний (демо) нет информации о зарплате стажёров после стажировки. Есть только описание этапов стажировки [3] (6 месяцев, четыре этапа) и правила отпуска и удалённой работы [1], [4]."}`; text: Про зарплату после стажировки в базе знаний ничего нет, там описаны только этапы стажировки [3] и правила отпуска и удалёнки [1], [4]. Это вопрос к руководителю или HR, а не к базе. Могу передать его Олегу от твоего и...
- **oos-04** «Есть ли в офисе спортзал?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources ['wiki-faq'], action `{"question": "Олег, есть ли в офисе спортзал или что-то для спорта, и как им пользоваться?", "context": "В базе знаний по запросу про спортзал ничего не найдено. Фрагменты про принтеры и переговорки [1] и кухню и кофе [2] этого не касаются."}`; text: В базе знаний про спортзал ничего нет: ни в FAQ про принтеры и переговорки [1], ни в FAQ про кухню [2] об этом не сказано. Если хочешь точного ответа, спроси у Олега, он твой наставник и знает, что есть в офисе.
- **oos-05** «Какой ДМС положен стажёрам?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources ['backend-onboarding', 'ml-onboarding', 'work-schedule', 'first-week'], action `{"question": "Олег, какой ДМС положен стажёрам и с какого момента он действует?", "context": "В базе знаний по ДМС ничего не нашлось. Найденные фрагменты касаются деплоя, ML-окружения, удалённой работы и встреч первой недели, но не страховки."}`; text: В базе знаний про ДМС ничего нет: в найденных фрагментах [1], [2], [3], [4] об этом не написано. Передам вопрос Олегу, он подскажет точно.
- **mt-06** «на 20 ноября» — proposal None != leave_request.
  Got: finish `form_shown`, sources [], action `None`; text: Понял, отгул на пятницу 20 ноября. В базе знаний я не нашёл, как и за сколько дней подавать заявку на отгул, поэтому уточни это у Олега. Заполню форму, проверь её и отправь.
