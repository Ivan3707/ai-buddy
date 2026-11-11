# AI-buddy · Hack&Change 2026

**AI-Buddy** — демонстрационная платформа адаптации стажёров: персональные чек-листы, этапы онбординга, отслеживание прогресса, отправка результатов заданий на проверку наставнику (*buddy*) и AI-чат с ответами по демонстрационной базе знаний.

> Проект — MVP для хакатона. Данные и сценарии адаптации демонстрационные; это не официальная программа онбординга Авито. Не размещайте систему в открытом интернете без дополнительной защиты.

## Технологии

| Компонент | Стек |
|---|---|
| Backend | Go 1.26, стандартный `net/http`, `pgx`, JWT |
| База данных | PostgreSQL 16 |
| Миграции | Goose, SQL |
| Web | React 19, TypeScript 5.8, Vite 6 |
| AI | Python 3.12, FastAPI, демонстрационная KB |
| Расширение | Chrome Extension Manifest V3 |
| Инфраструктура | Docker Compose |
| Тесты | Go tests, pytest, Node test runner, Playwright |

## Быстрый запуск на чистой машине

### 1. Установить инструменты

Для приведённого ниже **текущего** сценария необходимы:

1. **Git** — клонировать репозиторий.
2. **Docker Desktop** (Windows/macOS) либо **Docker Engine + Compose v2** (Linux) — запустить PostgreSQL, AI, backend и web.
3. **Go 1.26** — установить Goose и выполнить `bootstrap-buddy`.
4. **Goose** — применить SQL-миграции.

**Node.js 22.12+** нужен для локальной разработки web, сборки расширения и UI-тестов. **Python 3.12** нужен для локальной разработки AI и его тестов. Для запуска готовых Compose-сервисов отдельно устанавливать Node.js и Python не требуется.

Проверьте установку:

```bash
git --version
docker --version
docker compose version
go version
```

Установите Goose (при наличии Go):

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
```

Убедитесь, что каталог бинарников Go (`$(go env GOPATH)/bin`) входит в `PATH`. Проверьте `goose -version`. На Windows Git Bash при необходимости используйте `"$(go env GOPATH)/bin/goose.exe" -version`.

### 2. Клонировать репозиторий

```bash
git clone https://github.com/nvkmaksim/ai-buddy.git
cd ai-buddy
```

> README описывает изменения, которые ещё могут находиться в ветке `backend` и не быть опубликованы в основной ветке. После публикации используйте ветку/коммит, содержащие миграцию `00003`, seed и `bootstrap-buddy`.

### 3. Создать корневой `.env`

```bash
cp .env.example .env
```

В PowerShell: `Copy-Item .env.example .env`.

Приведите **корневой** `.env` к следующему виду, задав собственные значения секретов:

```dotenv
WEB_PORT=5173
BACKEND_PORT=8080
DB_USER=app
DB_PASSWORD=REPLACE_WITH_A_STRONG_LOCAL_PASSWORD
DB_NAME=app
JWT_SECRET=REPLACE_WITH_A_LONG_RANDOM_SECRET
```

Для генерации случайного JWT-секрета можно использовать Python:

```bash
python -c 'import secrets; print(secrets.token_hex(32))'
```

Или OpenSSL: `openssl rand -hex 32`.

**Важно:** в текущем `.env.example` ранее находилось конкретное значение `JWT_SECRET`. Не используйте его как общий секрет при публикации. Для корректного запуска заполните собственные значения. Не коммитьте `.env`.

Параметры `DB_HOST` и `DB_PORT` в корневом `.env` не нужны для Compose: backend внутри сети подключается к `postgres:5432`; с хост-машины PostgreSQL доступен на `127.0.0.1:5433`.

### 4. Запустить PostgreSQL

```bash
docker compose -p ai-buddy up -d postgres
docker compose -p ai-buddy ps
```

Дождитесь статуса `healthy`. Контейнер базы называется `ai-buddy-postgres`.

### 5. Применить миграции Goose

Из корня репозитория выполните команды в **Git Bash** (или другой POSIX-оболочке):

```bash
set -a
source .env
set +a

export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="host=127.0.0.1 port=5433 user=${DB_USER} password=${DB_PASSWORD} dbname=${DB_NAME} sslmode=disable"

goose -dir apps/backend/migrations up
goose -dir apps/backend/migrations status
```

Должны примениться миграции:

- `00001_init.sql` — пользователи, чек-листы, задания, прогресс;
- `00002_checklist_stages_and_buddy.sql` — этапы и роль buddy;
- `00003_checklist_submissions_and_progress.sql` — результаты заданий, ревью и дополнительные ограничения.

**Замечание по безопасности:** не публикуйте вывод команд с секретами; не сохраняйте строку подключения в Git. Если пароль содержит символы, значимые для синтаксиса строки подключения, потребуется корректное экранирование.

### 6. Заполнить демонстрационные чек-листы

Seed требует базу с именем **`app`** (либо `ai_buddy_test`) и полностью применённую миграцию `00003`.

```bash
docker exec -i ai-buddy-postgres \
  psql -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 \
  < apps/backend/scripts/seed_demo_checklists_v1.sql
```

Ожидается два чек-листа: `common` и `professional/backend`, по 3 этапа и 12 заданий каждый. Скрипт повторно запускать можно при **точном совпадении** данных; если существующие чек-листы отличаются, он завершится ошибкой вместо перезаписи.

### 7. Создать первую учётную запись buddy

Команда `apps/backend/cmd/bootstrap-buddy` — необходимая административная утилита, а не временный тест.

В Git Bash из корня проекта:

```bash
export DB_HOST=127.0.0.1
export DB_PORT=5433
export DB_USER DB_PASSWORD DB_NAME
export DB_SSLMODE=disable

read -r -p 'Buddy email: ' BOOTSTRAP_BUDDY_EMAIL
read -r -s -p 'Buddy password (min 12 characters): ' BOOTSTRAP_BUDDY_PASSWORD
echo
export BOOTSTRAP_BUDDY_EMAIL BOOTSTRAP_BUDDY_PASSWORD

(cd apps/backend && go run ./cmd/bootstrap-buddy)

unset BOOTSTRAP_BUDDY_PASSWORD BOOTSTRAP_BUDDY_EMAIL
```

Пароль не выводится при вводе. Если пользователь с таким email уже существует, команда **не меняет** его пароль. Не добавляйте `BOOTSTRAP_BUDDY_PASSWORD` в `.env.example` или Git.

### 8. Собрать и запустить приложение

```bash
docker compose -p ai-buddy up --build -d --wait
docker compose -p ai-buddy ps
```

Адреса:

| Сервис | URL |
|---|---|
| Web | http://127.0.0.1:5173 |
| Backend health | http://127.0.0.1:8080/health |
| PostgreSQL | `127.0.0.1:5433` (не HTTP) |
| AI | Только внутри Docker-сети, `http://ai:8001` |

Откройте Web, войдите под созданным buddy, создайте стажёра с профессией `backend`, затем проверьте отображение чек-листов, отправку задания и его ревью.

Логи и остановка:

```bash
docker compose -p ai-buddy logs -f backend
docker compose -p ai-buddy logs -f postgres
docker compose -p ai-buddy down
```

`docker compose down` **не удаляет** именованный том БД. Команда `docker compose down -v` удалит данные PostgreSQL — используйте её только если осознанно хотите начать с чистой базы.

## Переменные окружения

| Переменная | Где используется | Назначение |
|---|---|---|
| `WEB_PORT` | Compose | Порт веб-интерфейса на хосте, по умолчанию `5173` |
| `BACKEND_PORT` | Compose | Порт Go API на хосте, по умолчанию `8080` |
| `DB_USER` | Compose / Go | Пользователь PostgreSQL |
| `DB_PASSWORD` | Compose / Go | Пароль PostgreSQL |
| `DB_NAME` | Compose / Go | Имя базы; demo seed ожидает `app` |
| `DB_HOST` | Go | `postgres` внутри Compose, `127.0.0.1` локально |
| `DB_PORT` | Go | `5432` внутри Compose, `5433` локально |
| `DB_SSLMODE` | Go | Для локального Compose — `disable` |
| `JWT_SECRET` | Backend | Ключ подписи JWT, обязателен |
| `AI_URL` | Backend | Адрес AI-сервиса |
| `DATA_DIR` | Backend / AI | Каталог демонстрационных JSON-данных |
| `BOOTSTRAP_BUDDY_EMAIL` | Bootstrap | Email первоначального buddy |
| `BOOTSTRAP_BUDDY_PASSWORD` | Bootstrap | Пароль первоначального buddy, минимум 12 символов |
| `TEST_DATABASE_URL` | Интеграционные Go-тесты | DSN отдельной тестовой базы |

**Два `.env`:** Docker Compose автоматически читает **корневой** `.env`; Go-приложение само по себе `.env` **не загружает**, а читает переменные процесса через `os.Getenv`. Поэтому `apps/backend/.env` необязателен: при локальном запуске экспортируйте переменные из корневого файла и задайте `DB_HOST=127.0.0.1`, `DB_PORT=5433`. Не дублируйте секреты без необходимости. `apps/backend/.env.test` также не требуется для Docker Compose; его использование зависит от способа запуска локальных тестов.

**Предупреждение:** если PostgreSQL уже инициализирован, изменение `DB_USER`, `DB_PASSWORD` или `DB_NAME` в `.env` не перенастраивает автоматически существующий том. Сохраните прежние параметры или выполните осознанную миграцию конфигурации.

## Локальная разработка

### Go backend

Запустите PostgreSQL через Compose и примените миграции (шаги выше). Затем в Git Bash из корня:

```bash
set -a
source .env
set +a
export DB_HOST=127.0.0.1 DB_PORT=5433 DB_SSLMODE=disable
export AI_URL=http://127.0.0.1:8001

cd apps/backend
go run .
```

При запуске без Docker backend ожидает доступный AI-сервис. Для него откройте отдельный терминал:

### Python AI

```bash
python -m venv .venv
# Git Bash на Windows:
source .venv/Scripts/activate
# Linux/macOS: source .venv/bin/activate
python -m pip install -r apps/ai/requirements-dev.txt
python -m uvicorn app.main:app --app-dir apps/ai --host 127.0.0.1 --port 8001
```

### React Web

Из корня в отдельном терминале:

```bash
npm ci
npm run dev --workspace @ai-buddy/web
```

Vite проксирует `/api` в backend. При локальном запуске сервисы должны быть доступны на согласованных портах.

## Тестирование

### Go

```bash
cd apps/backend
go test ./...
go vet ./...
```

Для интеграционных тестов нужна отдельная БД `ai_buddy_test` с актуальной схемой. Создайте её при работающем PostgreSQL:

```bash
# Из корня проекта, после загрузки переменных .env
docker exec ai-buddy-postgres psql -U "$DB_USER" -d "$DB_NAME" \
  -c 'CREATE DATABASE ai_buddy_test;'
```

Команда создания базы нужна только один раз; при повторном запуске PostgreSQL сообщит, что база существует.

Примените миграции к тестовой БД:

```bash
export TEST_DATABASE_URL="host=127.0.0.1 port=5433 user=${DB_USER} password=${DB_PASSWORD} dbname=ai_buddy_test sslmode=disable"
GOOSE_DBSTRING="$TEST_DATABASE_URL" goose -dir apps/backend/migrations up
```

В зависимости от конкретных тестов может потребоваться seed в `ai_buddy_test`. Не запускайте тесты против основной базы `app`, если тесты изменяют данные.

> Проверка тестов на чистой машине ещё не выполнена. Некоторые интеграционные тесты используют `TEST_DATABASE_URL`, а Dockerfile backend содержит `RUN go test ./...` во время сборки. Это нужно проверить отдельно: контейнер сборки не получает тестовую PostgreSQL автоматически.

## Расширение Chrome (Manifest V3)

```sh
npm ci
npm run build --workspace @ai-buddy/extension
```

В Chrome откройте `chrome://extensions`, включите режим разработчика → «Загрузить распакованное расширение» → выберите `apps/extension/dist`. Закрепите AI-buddy, нажмите иконку и отправьте вопрос. Backend должен быть запущен. Расширение не читает страницы, не запрашивает cookies, history или tabs. Только разрешение на локальный Backend. Popup и веб используют `packages/client`.

## Проверки

```sh
# из apps/backend
go test -race ./...
go vet ./...
# из apps/ai (Python-окружение активировано)
python -m pytest -q
# из корня
python scripts/check_contracts.py
npm test
npm run build
npx playwright install chromium
# при работающем Compose-стенде
npm run test:e2e
```

Live-проверка контрактов: `CHECK_LIVE=1 python scripts/check_contracts.py` (PowerShell: `$env:CHECK_LIVE='1'; python scripts/check_contracts.py`). E2E открывает веб и настоящее распакованное расширение в тестовом Chromium. Проверяет ответ по цепочке UI → Go → Python, роли и чекбоксы. CI выполняет unit-тесты, vet, сборки, валидацию OpenAPI, Compose, live-контракты и браузерные тесты. Падения AI и некорректный JSON отдельно проверяются Go-тестами.

## Структура репозитория

```text
ai-buddy/
├── apps/
│   ├── ai/                    # Python / FastAPI, демо AI
│   ├── backend/               # Go API
│   │   ├── cmd/bootstrap-buddy/ # Первоначальное создание buddy
│   │   ├── internal/          # model, repository, service, security, database
│   │   ├── migrations/        # Goose 00001–00003
│   │   └── scripts/           # SQL seed
│   ├── web/                   # React / TypeScript / Vite
│   └── extension/             # Chrome MV3
├── packages/client/          # Общий HTTP-клиент
├── contracts/                # OpenAPI
├── data/                     # Синтетические данные
├── docs/                     # Архитектура и сценарии
├── tests/e2e/                # Playwright
├── compose.yaml
└── .env.example
```

## Основные модели PostgreSQL

| Таблица | Назначение |
|---|---|
| `users` | Пользователи, роли, профессия, buddy, последний вход |
| `checklists` | Общие и профессиональные чек-листы |
| `checklist_stages` | Этапы адаптации |
| `checklist_items` | Задания, критерии результата, необходимость ревью |
| `user_checklist_progress` | Индивидуальные статусы заданий |
| `checklist_item_submissions` | Попытки отправки результата и решения buddy |

Детальное описание типов, полей и ограничений: [документация моделей](docs/models.md) (добавьте ранее подготовленный файл `AI_BUDDY_MODELS.md` по этому пути).

## API (основные маршруты)

| Метод | Маршрут | Назначение |
|---|---|---|
| `GET` | `/health` | Проверка backend |
| `POST` | `/api/auth/login` | Вход |
| `GET` | `/api/auth/me` | Текущий пользователь |
| `POST` | `/api/users` | Создание стажёра (buddy) |
| `GET` | `/api/checklists` | Чек-листы стажёра |
| `POST` | `/api/checklist-items/{itemID}/complete` | Завершение задания |
| `POST` | `/api/checklist-items/{itemID}/submissions` | Отправка результата |
| `GET` | `/api/checklist-items/{itemID}/submissions` | История попыток |
| `GET` | `/api/buddy/submissions` | Задания на проверке |
| `POST` | `/api/buddy/submissions/{submissionID}/review` | Решение наставника |
| `POST` | `/api/chat` | Демонстрационный AI-чат |
| `GET` | `/api/onboarding` | Демонстрационные сценарии |

Маршруты для ролей защищены JWT middleware и проверками прав доступа.

## Типичные проблемы

**`JWT_SECRET must be configured`** — проверьте корневой `.env` и переменную `JWT_SECRET`.

**`connection refused` к PostgreSQL** — дождитесь `healthy`, проверьте порт: с хоста `5433`, из Compose `5432`.

**`relation ... does not exist`** — миграции Goose не применены к нужной базе.

**Seed отказался работать** — он требует имя БД `app` или `ai_buddy_test`, схему версии 3 и отсутствие конфликтующих чек-листов.

**`user with email ... already exists`** — bootstrap не перезаписывает существующего пользователя. Войдите с уже созданной учётной записью.

**Нет профессионального чек-листа** — демо seed содержит профессиональные задания только для `backend`.

**Сборка backend Dockerfile падает на тестах** — проверьте интеграционные тесты: в текущем Dockerfile тесты запускаются во время сборки без отдельной тестовой БД.

**CI не поднимает Compose** — проверьте обязательные переменные, миграции и seed. Текущий workflow ещё не адаптирован к новой схеме.

## Безопасность и ограничения MVP

- Не коммитьте `.env`, пароли, JWT-секреты, реальные данные сотрудников и внутренние документы.
- PostgreSQL, web и backend привязаны к `127.0.0.1` хост-машины; не считайте это полноценной защитой для production.
- JWT-аутентификация и RBAC реализованы, но полноценные production-контроли (TLS, rate limiting, аудит, управление секретами, мониторинг и политики хранения данных) требуют отдельной работы.
- AI использует демонстрационные данные; ответы не должны рассматриваться как официальные HR-инструкции.
- Для production необходимы отдельные секреты, защищённая инфраструктура, проверка зависимостей и сценариев доступа.

## Документация

- [Архитектура](docs/architecture.md)
- [Демо-сценарии](docs/demo-scenarios.md)
- [Проверки](docs/verification.md)
- [Контракты API](contracts/README.md)
- [Модели данных](docs/models.md) — после добавления файла

---