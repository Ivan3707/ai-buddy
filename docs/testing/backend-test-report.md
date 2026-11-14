# AI-Buddy — отчёт о тестировании Go Backend MVP

Состояние на 15 ноября 2026 года. Этот документ суммирует **сохранённые результаты**, полученные 14–15 ноября, и описывает только Go backend и его интеграционные контракты. Документация не заменяет повторный прогон для нового коммита. [Инструкция запуска](../../README.md), [SDD](../SDD.md), [Backend OpenAPI](../../contracts/backend.openapi.json).

## Окружение и методика

- Финальные Go-тесты и покрытие: официальный `golang:1.26.0-bookworm`, Go 1.26.0, Linux/amd64. Race detector: `CGO_ENABLED=1`, GCC 12.2.0. PostgreSQL `postgres:16-alpine` (сервер 16.15), Goose v3.30.1.
- Тестовые PostgreSQL запускались в отдельных временных Docker-контейнерах с `tmpfs`, без существующих volumes и основной БД. Для финального покрытия использовались отдельные `ai_buddy_verify_coverage_improved` и `ai_buddy_verify_catalog`, обе на Goose v1. Все Go-пакеты запускались последовательно (`-p 1`) из-за глобально уникальных фикстур чек-листов.
- Unit и handler-тесты используют изолированные зависимости; repository/service integration и HTTP E2E используют `TEST_DATABASE_URL`. Для CLI-теста каталога задана отдельная `TEST_CATALOG_DATABASE_URL`; полный E2E включён `RUN_E2E=1`. Внешний AI в Go-тестах заменяется управляемыми HTTP-ответами, а корпоративный источник — Mock HR.
- Статическая проверка OpenAPI: Windows, Python 3.13.15 в изолированной `.venv`, `jsonschema` 4.26.0, `openapi-spec-validator` 0.9.0. Live-проверка контрактов после последнего изменения OpenAPI не запускалась.

## Проверки и результаты

| Направление | Подтверждённый результат | Основание в локальном `test-results/` |
|---|---|---|
| P0 | Конкурентный refresh/logout, повторное использование refresh token, HTTP-изоляция заметок/календаря, назначение персональных задач: целевые тесты, полный Go-прогон, opt-in E2E и vet прошли | `backend-p0-20261114/run-summary.txt`, `p0-go-test-final.txt`, `full-go-test-acl-enabled.txt`, `opt-in-e2e.txt` |
| P1-A/B | Изоляция ролей, смена профессии и старый трек, HR sync, конкурентные переходы прогресса, retry/fallback hint и feedback: целевые, полный Go и полный opt-in E2E прошли | `backend-p1-20261114/run-summary.txt`, `targeted-final-after-timeout.txt`, `full-go-test-final.txt`, `full-opt-in-e2e-final.txt` |
| Финальный полный Go | `go test -p 1 -count=1 ./...` с отдельными БД и `RUN_E2E=1`: exit 0, включая opt-in E2E | `backend-coverage-improved/test-final-full.txt`, `final-full-exit-code.txt` |
| Race detector | `go test -race -p 1 -count=1 ./...` с теми же тестовыми БД и `RUN_E2E=1`: exit 0, сообщений `DATA RACE` нет | `backend-coverage-improved/race.txt`, `race-exit-code.txt`; отдельные обычный и opt-in прогоны — `backend-race-p2-20261114/` |
| Статический анализ | `go vet ./...`: exit 0, без диагностик | `backend-coverage-improved/vet.txt`, `vet-exit-code.txt` |
| Контракты | Обе OpenAPI-спецификации валидны; общие `ChatRequest`/`ChatResponse`, чат и совместимость AI `/hint` прошли. Три намеренных расхождения валидатор отклонил | `contracts-validation-20261115/validation.txt` |

Предварительные диагностические прогоны и ошибки тестовых фикстур сохранены отдельно; таблица использует только финальные прогоны с зафиксированным кодом выхода 0. Реального production-дефекта в этих сериях не подтверждено. Отсутствие сообщения race detector относится только к исполненным путям.

## Покрытие Go backend

Финальный прогон: `go test -p 1 -count=1 -coverpkg=./... -coverprofile=coverage.out ./...` при `RUN_E2E=1` в Go 1.26.0 на изолированном PostgreSQL. Тесты и обработка профиля завершились с exit 0. Профиль охватывает **14 Go-пакетов**; `internal/model` не содержит исполняемых statements. Production-пакеты не исключались.

**2 085 из 2 568 statements = 81,19% statement coverage** по межпакетной методике `-coverpkg=./...`. `go tool cover -func` округляет итог до 81,2%. Это не branch coverage и не покрытие Web, AI/ML или всей команды.

| Пакет | Покрыто / всего | % |
|---|---:|---:|
| `cmd/api` | 0 / 2 | 0,0 |
| `cmd/cleanup-load` | 31 / 63 | 49,2 |
| `cmd/init-catalog` | 23 / 33 | 69,7 |
| `cmd/retry-feedback` | 10 / 16 | 62,5 |
| `cmd/seed-load` | 63 / 120 | 52,5 |
| `cmd/sync-hr` | 64 / 102 | 62,7 |
| `internal/app` | 30 / 32 | 93,8 |
| `internal/database` | 28 / 33 | 84,8 |
| `internal/handler` | 529 / 611 | 86,6 |
| `internal/middleware` | 59 / 67 | 88,1 |
| `internal/repository` | 476 / 549 | 86,7 |
| `internal/security` | 67 / 80 | 83,8 |
| `internal/service` | 705 / 860 | 82,0 |
| `internal/model` | исполняемых statements нет | — |

Небольшие итоговые артефакты включены рядом с отчётом: [общий расчёт](backend-coverage-totals.txt) и [пакеты CSV](backend-coverage-packages.csv). Они скопированы из локального финального прогона и сверены с `func-final.txt` и `coverage.out`. Полный профиль и HTML остаются в игнорируемом `test-results/backend-coverage-improved/`: их не нужно добавлять в Git целиком; профиль можно получить снова приведённой командой на отдельной БД.

## Нагрузочные материалы k6

В локальном игнорируемом `load-results-presentation/` есть 9 завершённых трёхминутных прогонов: сценарии `public`, `read`, `mixed` при 10, 25 и 50 VU. Их `manifest.json` помечены `complete`, а `result.json` для каждого показывает `http_error_rate = 0` и `functional_success_rate = 1` в учитываемой workload-фазе. Это результаты отдельного нагрузочного стенда; они **не повторялись** при подготовке документации и не являются гарантией production-производительности. Сырые метрики и снимки мониторинга остаются локальными, в Git целиком не добавляются.

## Ограничения и материалы для Git

- `cmd/api` не тестируется напрямую; путь запуска частично проверен через `internal/app`. `cmd/seed-load` тестирует защитные условия, но не выполняет реальную загрузку. `VERIFY_DATABASE_CONNECTION=1` и `DB_*` для отдельного live-теста БД не задавались.
- Go E2E проверяет HTTP через тестовый router и PostgreSQL. Состояние текущего Web для buddy/teamlead — временная страница с Logout; это ограничение интерфейса, а не отсутствие соответствующих backend-маршрутов. Качество AI/ML здесь не оценивалось.
- Для Git нужны этот отчёт, два небольших артефакта покрытия, актуальная SDD/контракты и **все 21 новых Go `_test.go` файла**, пока не отслеживаемых Git; четыре уже отслеживаемых изменённых Go-теста также должны войти в итоговый diff. `tests/e2e/performance.spec.ts` относится к отдельному Web/нагрузочному направлению и требует проверки владельцем перед включением как подтверждение backend.
- `test-results/`, `load-results-presentation/`, `.env` и `secrets/` игнорируются Git. Не добавляйте их целиком, а также приватный `инфа.md` или резервный `codex-tests-backup.patch` как материалы сдачи.
