# Eval report

Model: `qwen3.5:9b` (mode `llm`), cases: 55, date: 2026-11-13 18:09. Dataset: `eval/dataset.jsonl` (synthetic).

## Key metrics

| Metric | Value | Target |
|---|---|---|
| retrieval recall@4 (BM25 baseline) | 0.942 | ≥ 0.9 |
| retrieval MRR | 0.877 | — |
| correct source in answer | 31/34 (91%) | ≥ 90% |
| refusal correctness (out of KB → no_answer) | 1/6 (17%) | ≥ 90% |
| false refusals on answerable questions | 0/33 (0%) | ≤ 10% |
| action accuracy (type + params) | 3/6 (50%) | ≥ 90% |
| security cases held | 4/4 (100%) | 100% |
| time to first token p50 / p95 | 1.3 s / 2.2 s | p95 ≤ 3 s |
| full turn latency p50 / p95 | 3.7 s / 7.4 s | — |
| tokens in / out (total) | 91657 / 6182 | — |

## By category

| Category | Passed |
|---|---|
| action | 3/6 (50%) |
| clarify | 3/3 (100%) |
| contradiction | 2/2 (100%) |
| kb | 28/30 (93%) |
| next_step | 2/3 (67%) |
| out_of_kb | 1/6 (17%) |
| security | 4/4 (100%) |
| status | 1/1 (100%) |
| **total** | **44/55 (80%)** |

## Failures

- **kb-15** «Во сколько стендап?» — text lacks '10:30'.
  Got: finish `answered`, sources ['first-week'], action `None`; text: Стендап проходит каждое утро (точное время не указано). Точное расписание смотри на канале команды [1]. Если опасаешься не успеть, лучше написать в канал заранее.
- **kb-19** «Откуда брать данные для обучения модели?» — sources [] miss ['ml-onboarding'].
  Got: finish `answered`, sources [], action `None`; text: Данные для обучения берутся из витрин, которые размещены в **Redash** и **Hive**.  Для получения доступа к конкретной витрине нужно создать заявку с уровнем `read` у её владельца.  Следующий шаг: определи, какая витри...
- **next-01** «Что мне делать дальше?» — sources [] miss ['ml-onboarding', 'onboarding-plan'].
  Got: finish `answered`, sources [], action `None`; text: Покажи себя на максимум с активной задачей: воссоздай baseline модель рекомендаций (задача t5).  Преврати её в план действий: 1) Найди репозиторий проекта Рекомендации и код модели/скрипт inference. 2) Собери окружени...
- **act-01** «Оформи отгул на 20 ноября» — proposal None != leave_request.
  Got: finish `answered`, sources ['first-week', 'leave'], action `None`; text: Тебе нужно оформить заявку на отгул на 20 ноября, но я пока не вижу в списке открытых заявок ([open_requests]) эту дату. Кроме того, согласно [4], в первую неделю стажировки брать отгулы не рекомендуется — уточни, сей...
- **act-02** «Хочу в отпуск с 1 по 14 декабря, оформи заявку» — proposal None != leave_request.
  Got: finish `answered`, sources ['leave'], action `None`; text: Здравствуйте, Аня! Сроки подачи заявок строго соблюдены: для отпуска нужно не менее 14 календарных дней [1]. Сейчас 13 ноября, поэтому до 1 декабря осталось меньше времени.  Чтобы я мог подготовить заявку на отпуск в ...
- **act-06** «Спроси у наставника, можно ли мне поехать на конференцию за счёт компании» — proposal None != mentor_question.
  Got: finish `answered`, sources ['mentor'], action `None`; text: В базе знаний нет информации о том, как оплачиваются поездки на конференции или есть ли такие компенсации для стажёров. Согласно правилам эскалации [1], если ответ на вопрос есть только у наставника или в источниках п...
- **oos-01** «Какой дресс-код на корпоративе?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources ['security', 'code-review'], action `None`; text: Дресс-код для корпоратива в базе знаний не указан [1], [2], [3], [4]. Передаю этот вопрос нашему наставнику.
- **oos-02** «Какая зарплата будет у стажёра после стажировки?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources ['onboarding-plan'], action `{"question": "Какая будет зарплата после успешного завершения стажировки? Где посмотреть информацию об оплате труда и условиях для финальных стадёров?", "context": "Стажёра интересует размер зарплаты после окончания стажировки. В базе знаний нет данных по оплате труда и компенсациям [3], [4]."}`; text: В базе знаний нет информации о зарплате и компенсациях [3], [4]. Это правило относится к процессам работы, а не к финансовым условиям.  Передаю твой вопрос наставнику для уточнения в реальных источниках.
- **oos-03** «Как оформить командировку в другой город?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources ['work-schedule', 'wiki-faq', 'leave', 'onboarding-plan'], action `None`; text: В базе знаний информации о том, как оформить командировку в другой город, нет [1, 2, 3, 4]. Это вопрос, который нужно передать наставнику или руководителю.
- **oos-05** «Какой ДМС положен стажёрам?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources [], action `None`; text: В базе знаний информации о медицинском страховании (ДМС) для стажеров нет. Нужно передать этот вопрос наставнику, чтобы уточнить детали или получить актуальные источники.
- **oos-06** «Когда будет новогодний корпоратив?» — finish=answered, expected no_answer.
  Got: finish `answered`, sources ['it-setup', 'security'], action `None`; text: Новогоднего корпоратива в базе знаний [1], [2], [3], [4] нет — это относится к организационным мероприятиям, а не к ИТ или безопасности.  Попроси у наставника Олега актуальную информацию о планах на конец года.
