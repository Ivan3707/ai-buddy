# Отчёт о покрытии тестами

Замер от 15.11.2026, ветка `release/submission`. Кейс требует больше 80% покрытия строк прикладного кода **по каждой части отдельно**. Все наборы ниже воспроизводятся одной командой в Docker (см. [README](../../README.md#тесты-и-покрытие)), и каждый падает, если покрытие строк ниже 80%.

| Часть | Команда | Тестов | Строки | Ветвления | Отчёт |
|---|---|---|---|---|---|
| Go backend | `docker compose -f compose.test.yaml run --rm go` | 14 пакетов: unit, интеграция с PostgreSQL, E2E HTTP | **82,4%** (statements) | — | `coverage/go/index.html`, `coverage/go/coverage.out` |
| Python AI | `docker compose -f compose.test.yaml run --rm python` | 125 | **99,8%** | — | `coverage/ai/index.html`, `coverage/ai/coverage.xml` (Cobertura) |
| Web (React) | `docker compose -f compose.test.yaml run --rm node` | 137 | **85,3%** | 82,3% | `coverage/web/index.html`, `coverage/web/lcov.info` |
| Chrome-расширение | там же | 142 | **99,9%** | 95,8% | `coverage/extension/lcov.info` |
| API-клиент `packages/client` | там же | 6 | **90,6%** | 100% | `coverage/client.lcov.info` |

Папка `coverage/` в Git не попадает (она в `.gitignore`), отчёты пересоздаются командой выше.

## Методика по частям

**Go.** Команда: `go test -p 1 -count=1 -coverpkg=./... -coverprofile=coverage.out ./...` с `RUN_E2E=1`. Тесты идут на временном `postgres:16-alpine` в tmpfs; приложенческая БД не используется. Межпакетный `-coverpkg` засчитывает строку, если её выполнил тест любого пакета. Знаменатель — весь код backend, включая служебные команды. Ничего не исключено.

| Пакет | Покрыто / всего statements | % |
|---|---|---|
| `internal/handler` (HTTP API) | 995 / 1159 | 85,8 |
| `internal/service` (бизнес-правила) | 1289 / 1523 | 84,6 |
| `internal/repository` (SQL) | 942 / 1109 | 84,9 |
| `internal/middleware` (JWT, RBAC, метрики) | 61 / 67 | 91,0 |
| `internal/security` (пароли, JWT) | 67 / 80 | 83,8 |
| `internal/app`, `internal/database`, `migrations` | 78 / 93 | 83,9 |
| `cmd/*` (служебные команды: init-catalog, sync-hr, seed-demo, нагрузочные утилиты) | 310 / 511 | 60,7 |
| **Итого** | **3742 / 4542** | **82,4** |

**Python.** Команда: `pytest --cov=app`. Тесты работают в режиме `LLM_PROVIDER=mock` и с поддельным HTTP-транспортом, поэтому в сеть не ходят. Знаменатель — весь пакет `apps/ai/app`. Каталог `eval/` — инструмент оценки качества модели, а не код сервиса, поэтому в знаменатель не входит.

**Web.** Команда: `vitest run --coverage` (v8). Знаменатель — `apps/web/src/**/*.{ts,tsx}` без `main.tsx` (точка монтирования React) и самих тестов. Общий виджет Бади (`apps/extension/buddy/widget.js`) подключается в сайт через alias и меряется в части расширения.

**Расширение.** Команда: `node --test --experimental-test-coverage`. Знаменатель — весь прикладной код: `lib/**`, `buddy/**`, `background.js`. Тесты гоняют виджет в jsdom так же, как это делает пользователь: вход, поток ответа, карточки действий, формы, вкладки, источники, оценки. Chrome API подменяются. Строковые шаблоны CSS и HTML node считает выполненными строками, поэтому для `widget.js` честнее смотреть на покрытие ветвлений: 94%.

## Чем ещё проверяется качество

- **CI** (GitHub Actions, `.github/workflows/ci.yml`): Go с race detector, `go vet`, `gofmt`, pytest, проверка OpenAPI-контрактов, тесты и сборка web, подъём всего стека в Docker и live-проверка контрактов.
- **Eval AI-агента** (`apps/ai/eval/`): 67 размеченных кейсов и независимый holdout из 30. Проверяются поиск, источники, отказы, действия, противоречия и атаки; подробности в [README eval](../../apps/ai/eval/README.md).
- **Нагрузка** (`ops/load/`): k6 на 10/25/50 виртуальных пользователей, графики в презентации.
- **Ручная проверка** по ролям: [`docs/manual-testing.md`](../manual-testing.md).

Подробный отчёт по тестам backend (race detector, P0/P1-сценарии): [`backend-test-report.md`](backend-test-report.md).
