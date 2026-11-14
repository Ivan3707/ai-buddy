# AI-Buddy

AI-Buddy — платформа адаптации стажёров: Go Backend, PostgreSQL, React Web, Python AI и расширение Chrome. Buddy проверяет задания; Teamlead видит профессиональный прогресс и добавляет персональные задания. Учётные записи и назначения Buddy/Teamlead приходят из Mock HR.

## Запуск с чистой БД

Нужны Go 1.26, Node.js 22, Docker Compose и Goose (`go install github.com/pressly/goose/v3/cmd/goose@v3.24.1`). `00001_init.sql` создаёт итоговую схему прежней v9, но имеет версию Goose 1. **Команды ниже предназначены только для новой пустой БД.** Не применяйте эту миграцию поверх существующих БД с историей v8/v9 и не удаляйте существующие БД или Docker volumes.

Из корня проекта в Windows PowerShell:

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
# Задайте DB_PASSWORD и JWT_SECRET в .env, затем загрузите переменные:
Get-Content .env | Where-Object { $_ -match '^[A-Z_]+=' } | ForEach-Object {
  $key, $value = $_ -split '=', 2
  [Environment]::SetEnvironmentVariable($key, $value, 'Process')
}
docker compose up -d --wait postgres
$history = docker compose exec -T postgres psql -U $env:DB_USER -d $env:DB_NAME -Atc "SELECT to_regclass('public.goose_db_version')"
if ($LASTEXITCODE -ne 0 -or $history -match 'goose_db_version') { throw 'Нужна новая пустая БД; текущую не мигрировать' }
$dsn = "host=127.0.0.1 port=5433 user=$env:DB_USER password=$env:DB_PASSWORD dbname=$env:DB_NAME sslmode=disable"
goose -dir apps/backend/migrations postgres $dsn up
Set-Location apps/backend
$env:DB_HOST = '127.0.0.1'
$env:DB_PORT = '5433'
$env:DB_SSLMODE = 'disable'
go run ./cmd/init-catalog
$credentials = Join-Path $env:LOCALAPPDATA ('ai-buddy\credentials\hr-' + (Get-Date -Format 'yyyyMMdd-HHmmss') + '.json')
go run ./cmd/sync-hr -snapshot ../../data/corporate_employees.json -credentials $credentials
Set-Location ../..
docker compose up --build -d --wait
```

Web: <http://127.0.0.1:5173>; Backend: <http://127.0.0.1:8080/health>. Если том PostgreSQL уже существует, выберите новую БД и создайте её отдельно до запуска команд; не меняйте историю миграций старой БД. `docker compose --env-file .env config --quiet` проверяет конфигурацию, `docker compose ps` показывает состояние сервисов. `docker compose stop` останавливает сервисы, сохраняя данные.

## Первичные пароли

`cmd/sync-hr` генерирует случайные пароли только для новых сотрудников. В PostgreSQL сохраняются только Argon2id-хеши. JSON-файл создаётся по абсолютному пути `-credentials` вне Git-репозитория, с закрытыми правами и без замены существующего файла. Рекомендуемая папка Windows: `%LOCALAPPDATA%\ai-buddy\credentials\`. Повторная синхронизация сохраняет пароли и прогресс существующих пользователей; её новый файл содержит `[]`, если новых аккаунтов нет.

Полный файл публикуется до `COMMIT`. При ошибке записи транзакция откатывается. Если результат `COMMIT` неизвестен, сохраните файл: в нём остаются первичные пароли на случай успешного commit. Проверьте состояние БД перед повторной синхронизацией. Не выводите содержимое файла в логи и не храните открытые пароли в `data/corporate_employees.json`, `.env` или репозитории.

## Инициализация и команды

1. Goose создаёт только схему (`apps/backend/migrations/00001_init.sql`).
2. `cmd/init-catalog` в одной транзакции применяет общий, Backend, Frontend и ML чек-листы из `apps/backend/scripts`. Точный повтор не меняет данные; конфликт приводит к отказу без сброса прогресса.
3. `cmd/sync-hr` читает `data/corporate_employees.json`, создаёт и обновляет сотрудников, связывает Buddy/Teamlead и добавляет недостающий прогресс. Отсутствующие в новом снимке HR пользователи сохраняются.
4. `cmd/api` запускает API. `cmd/retry-feedback` повторяет отправку обратной связи. `cmd/seed-load` и `cmd/cleanup-load` обслуживают отдельный нагрузочный стенд.

Ручной `PUT /api/buddy/interns/{internID}/teamlead` удалён. На существующей нагрузочной БД v9 миграционный контейнер проверяет схему и не применяет новую историю; чистая нагрузочная БД получает v1. Рабочий нагрузочный стенд сейчас находится на v8: его данные не менялись, а новый запуск с этой кодовой базой безопасно остановится. Обновление до v9 разрешено **позже отдельной операцией** после backup и согласования окна. Не направляйте демо и тестовые команды на рабочее нагрузочное окружение.

## Проверки

```powershell
Set-Location apps/backend
go mod tidy
go fmt ./...
go vet ./...
go build ./...
go test -count=1 -p 1 ./...
Set-Location ../..
python scripts/check_contracts.py
npm ci
npm test
npm run build
```

Go-интеграционным тестам нужен `TEST_DATABASE_URL` с отдельной чистой БД `ai_buddy_verify_*` или свежей `ai_buddy_test`, созданной новой миграцией. Без этой переменной PostgreSQL-тесты будут пропущены. Для HTTP E2E задайте `RUN_E2E=1`. Browser E2E (`npx playwright test`) получает `E2E_CREDENTIALS_FILE`, указывающий на файл HR Sync вне репозитория; сначала нужен запущенный чистый стенд. AI-тесты: `python -m pip install -r apps/ai/requirements-dev.txt`, затем `python -m pytest -q apps/ai`. Не запускайте тесты против `app` или `ai_buddy_load`.

Live-проверка контрактов (`CHECK_LIVE=1 python scripts/check_contracts.py`) использует тот же `E2E_CREDENTIALS_FILE`. В файле должна быть учётная запись `buddy@demo.example`; повторный `sync-hr` не выдаёт пароль уже существующего Buddy, поэтому сохраните файл первичной синхронизации. Проверка `/health` публичная, `/api/chat` требует Bearer JWT; рабочие чек-листы доступны через авторизованные `/api/checklists` и `/api/progress`.

## Структура

- `apps/backend` — API, команды, миграция и SQL справочников.
- `apps/web`, `apps/extension`, `apps/ai` — Web, Chrome MV3 и AI-сервис.
- `packages/client` — клиент Backend API; `contracts` — OpenAPI.
- `tests/e2e` — браузерные тесты; `ops/load` — нагрузка и мониторинг.
- `data/corporate_employees.json` — Mock HR без паролей.
