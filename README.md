# AI-Buddy

AI-Buddy — демо адаптации стажёров. Go Backend хранит пользователей, чек-листы и прогресс, проверяет результаты с buddy, обрабатывает персональные задачи teamlead, заметки, календарь, корпоративные mock-заявки, feedback и подсказки. Учётные записи и назначения поступают из синтетического Mock HR. Backend обращается к отдельному AI-сервису по HTTP для чата и подсказок; реализация AI/ML здесь не оценивается.

Web обращается к backend через `/api`. В текущей сборке стажёру доступен интерфейс адаптации, а buddy и teamlead видят временную страницу с кнопкой выхода; их серверные API работают независимо от этого ограничения Web. Подробные правила и маршруты: [SDD](docs/SDD.md), [Backend OpenAPI](contracts/backend.openapi.json), [отчёт о тестировании](docs/testing/backend-test-report.md).

## Стек и требования

| Часть | Версия или источник |
|---|---|
| Go Backend | Go 1.26.0 (`apps/backend/go.mod`, образ `golang:1.26-alpine`); pgx/v5 5.12.0, golang-jwt/jwt/v5 5.3.1 |
| Данные | PostgreSQL 16 (`postgres:16-alpine`), миграции Goose v1 |
| Goose CLI | CI устанавливает v3.24.1; подтверждённые локальные проверки использовали v3.30.1 |
| Запуск Compose | Docker Desktop с `docker compose` и поддержкой `--wait`; в проверенном окружении Docker Engine 29.9.2, Compose 5.6.1 |
| Web-сборка | Node.js 22 (`node:22-alpine`); React 19.1.0, TypeScript 5.8.3, Vite 6.4.6 по `apps/web/package.json` |
| AI-контейнер | Python 3.12 (`apps/ai/Dockerfile`); для backend важны только контракты `/chat` и `/hint` |

Для первого запуска на Windows нужны PowerShell, Docker Desktop, Go 1.26 и Goose в `PATH`. Node.js и Python нужны только для локальных JS/контрактных проверок: Compose собирает Web и AI сам. Порты по умолчанию: Web `127.0.0.1:5173`, Backend `127.0.0.1:8080`, PostgreSQL `127.0.0.1:5433`.

## Первый запуск через Docker Compose

Из корня репозитория в PowerShell:

```powershell
Copy-Item .env.example .env
# Замените DB_PASSWORD и JWT_SECRET в .env на свои случайные значения.
# JWT_SECRET должен содержать не менее 32 символов.
go install github.com/pressly/goose/v3/cmd/goose@v3.24.1
$env:Path = "$(go env GOPATH)\bin;$env:Path"
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
.\scripts\bootstrap.ps1
docker compose -p ai-buddy ps
```

`bootstrap.ps1` запускает PostgreSQL, проверяет историю миграций и применяет `apps/backend/migrations/00001_init.sql` через Goose. Затем `cmd/init-catalog` загружает общий и профессиональные чек-листы из `apps/backend/scripts`, `cmd/sync-hr` синхронизирует `data/corporate_employees.json`, а Compose собирает и запускает AI, backend и Web с ожиданием healthcheck. Скрипт не преобразует старую несовместимую схему и не сбрасывает данные. Нужен свободный набор портов; их можно изменить в `.env`. Для независимого стенда у bootstrap есть параметры `-ProjectName`, `-EnvFile`, `-CredentialsDirectory`.

Откройте <http://127.0.0.1:5173>. `GET http://127.0.0.1:8080/health` возвращает liveness backend; он не проверяет готовность AI и БД. Для повторного запуска уже подготовленного стенда: `docker compose -p ai-buddy up -d --build --wait`. Эта команда сама не выполняет миграцию и Mock HR sync. После изменения синтетического HR-снимка повторно запустите `.\scripts\bootstrap.ps1`: работающий backend читает справочник только при старте, а Dockerfile копирует JSON в образ, поэтому нужна пересборка и перезапуск. Остановка без удаления PostgreSQL volume: `docker compose -p ai-buddy stop`.

## Тестовые учётные записи

`data/corporate_employees.json` содержит следующие **синтетические** email. Паролей в этом файле нет.

| Роль | Email |
|---|---|
| Стажёр Backend | `backend.intern@demo.example` |
| Стажёр Frontend | `frontend.intern@demo.example` |
| Стажёр ML | `ml.intern@demo.example` |
| Ещё один стажёр ML | `ml.other@demo.example` |
| Buddy | `buddy@demo.example` |
| Teamlead | `teamlead@demo.example` |

При первой синхронизации `cmd/sync-hr` генерирует случайный пароль для каждого нового пользователя. Bootstrap выводит **путь** к приватному файлу `secrets/hr-*.json`, но не пароли. Откройте файл локально и используйте запись нужного email для входа. `secrets/` исключён из Git и Docker build context; не публикуйте файл. Повторный sync сохраняет пароли, ID и прогресс существующих пользователей и создаёт новый файл только с паролями новых записей (при их отсутствии — `[]`). Поэтому сохраните файл **первого** запуска: последующий пустой файл не восстановит старые пароли. Для browser E2E и live-проверки контрактов укажите путь к первому файлу в `E2E_CREDENTIALS_FILE`.

## Порядок проверки Backend MVP

1. Войдите как `backend.intern@demo.example` с паролем из приватного файла. `POST /api/auth/login` выдаёт access/refresh token; `GET /api/auth/me` возвращает роль и назначения. Logout требует оба токена; повторный refresh старым токеном запрещён.
2. Получите `GET /api/checklists` и `GET /api/progress`. Доступны общий и Backend-трек. Обычные задачи открываются последовательно внутри каждого чек-листа; треки независимы.
3. Завершите активную задачу без проверки через `POST /api/checklist-items/{itemID}/complete`. Задачу с `requires_review` сдайте через `POST /api/checklist-items/{itemID}/submissions`. До одобрения buddy она не повышает подтверждённый процент; после одобрения — повышает. API buddy: `GET /api/buddy/submissions`, `POST /api/buddy/submissions/{submissionID}/review`.
4. Проверьте `GET/POST /api/notes` и `/api/calendar/events`; чтение, изменение и удаление по ID доступны только владельцу. Teamlead может добавить назначенному стажёру персональную задачу через `POST /api/teamlead/interns/{internID}/checklist-items`; она сразу `active` и не меняет статусы других задач.
5. Проверьте `GET /api/corporate/me`, `/api/corporate/tools` и `POST /api/corporate/access-requests` с явным подтверждением и ключом идемпотентности. После завершения всего чек-листа доступен `POST /api/checklists/{checklistID}/feedback`.
6. `POST /api/chat` отправляет вопрос в AI `/chat`; hint worker использует AI `/hint`. Backend возвращает `502` при ошибке чата, а для подсказок применяет повторные попытки и fallback. Эти запросы не доказывают качество ML.

Маршруты, JSON-тела и ограничения ролей приведены в [Backend OpenAPI](contracts/backend.openapi.json) и [SDD](docs/SDD.md). В текущем Web buddy/teamlead видят заглушку с Logout; их сценарии проверяйте через API или автоматические Go-тесты.

## Тесты Backend

Команды ниже выполняются из корня репозитория в PowerShell. Без `TEST_DATABASE_URL` Go запускает доступные unit/handler-тесты; PostgreSQL integration-тесты пропускаются. `RUN_E2E=1` включает opt-in E2E и требует тестовую БД. Не направляйте эти переменные на БД приложения `app` или нагрузочный стенд. Для общего и профессионального каталога тестовые фикстуры используют уникальные ограничения, поэтому пакеты запускаются последовательно: `-p 1`.

```powershell
Push-Location apps/backend
go test -p 1 -count=1 ./...
go vet ./...
Pop-Location
```

Для полного integration/E2E используйте **отдельный временный PostgreSQL** без существующих Docker volumes. Следующие команды создают только собственный контейнер с `tmpfs`; имя контейнера должно быть свободно. Goose должен быть доступен в `PATH`.

```powershell
docker run --rm -d --name ai-buddy-doc-test-db --tmpfs /var/lib/postgresql/data:rw,size=256m -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=ai_buddy_verify_docs -p 127.0.0.1::5432 postgres:16-alpine
for ($attempt = 0; $attempt -lt 30; $attempt++) {
    docker exec ai-buddy-doc-test-db pg_isready -U postgres -d ai_buddy_verify_docs *> $null
    if ($LASTEXITCODE -eq 0) { break }
    Start-Sleep -Seconds 1
}
if ($LASTEXITCODE -ne 0) { throw 'Тестовый PostgreSQL не стал готовым' }
$testPort = ((docker port ai-buddy-doc-test-db 5432/tcp) -split ':')[-1]
$env:TEST_DATABASE_URL = "postgres://postgres@127.0.0.1:$testPort/ai_buddy_verify_docs?sslmode=disable"
$env:TEST_CATALOG_DATABASE_URL = "postgres://postgres@127.0.0.1:$testPort/ai_buddy_verify_catalog?sslmode=disable"
goose -dir apps/backend/migrations postgres $env:TEST_DATABASE_URL up
docker exec ai-buddy-doc-test-db psql -U postgres -d postgres -v ON_ERROR_STOP=1 -c 'CREATE DATABASE ai_buddy_verify_catalog'
goose -dir apps/backend/migrations postgres $env:TEST_CATALOG_DATABASE_URL up
Push-Location apps/backend
go test -p 1 -count=1 ./...           # unit + PostgreSQL integration
$env:RUN_E2E = '1'
go test -p 1 -count=1 ./...           # полный набор, включая opt-in E2E
go vet ./...
Pop-Location
Remove-Item Env:RUN_E2E, Env:TEST_DATABASE_URL, Env:TEST_CATALOG_DATABASE_URL
docker stop ai-buddy-doc-test-db
```

Тест `TestOpenPostgresLive` имеет отдельный opt-in флаг `VERIFY_DATABASE_CONNECTION=1` и переменные `DB_*`; приведённый безопасный прогон его не включает. Для воспроизведения **точного** результата покрытия 81,19% использовались Go 1.26.0 в Linux-контейнере, `RUN_E2E=1`, отдельные базы и команда `go test -p 1 -count=1 -coverpkg=./... -coverprofile=coverage.out ./...`; детали окружения и результаты — в [отчёте](docs/testing/backend-test-report.md). Race detector требует Linux/CGO/GCC; ранее подтверждённый прогон приведён там же.

## Проверка контрактов и Web-сборки

Для статической проверки двух OpenAPI-спецификаций нужны `jsonschema` и `openapi-spec-validator` в изолированном Python-окружении. Следующие версии прошли проверку на Python 3.13.15:

```powershell
py -3 -m venv .venv
.\.venv\Scripts\python.exe -m pip install 'jsonschema==4.26.0' 'openapi-spec-validator==0.9.0'
.\.venv\Scripts\python.exe scripts/check_contracts.py
```

Для live-проверки запустите Compose и используйте первый файл учётных данных Mock HR, содержащий `buddy@demo.example`:

```powershell
$env:E2E_CREDENTIALS_FILE = '<абсолютный путь к приватному файлу первого HR Sync>'
$env:CHECK_LIVE = '1'
.\.venv\Scripts\python.exe scripts/check_contracts.py
Remove-Item Env:CHECK_LIVE
```

Web и общий JS-клиент можно проверить отдельно из корня репозитория: `npm.cmd ci`, `npm.cmd test`, `npm.cmd run build`. `npm.cmd` работает в PowerShell даже при запрете запуска `npm.ps1`. Browser E2E (`npm.cmd run test:e2e`) дополнительно требует работающий Compose, `E2E_CREDENTIALS_FILE` и установленный Chromium для Playwright. Здесь Web упомянут только как клиент backend и как проверенная временная заглушка ролей; внутренняя реализация Web и AI не входит в отчёт Backend.

## Известные ограничения и материалы

- Mock HR и корпоративные заявки — синтетические данные; связи с реальными системами нет. После ручного `sync-hr` backend требует пересборки/перезапуска для нового снимка.
- Страница buddy/teamlead в Web временная, с доступным Logout; серверный RBAC и endpoints реализованы и тестируются отдельно.
- `/health` отражает только liveness. Подсказка генерируется асинхронно и может появиться не сразу. AI/ML качество этим репозиторием Backend не подтверждается.
- Финальный измеренный показатель — **81,19% statement coverage Go backend** при `-coverpkg=./...`, включая opt-in E2E; это не покрытие Web, AI или всего проекта.
- Исходники, SDD, тесты и [отчёт о тестировании](docs/testing/backend-test-report.md) входят в материалы сдачи. Сырые логи и HTML-отчёт лежат локально в игнорируемом `test-results/` и не должны добавляться в Git целиком. Приватный `инфа.md`, `.env` и `secrets/` не публикуются.
