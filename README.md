# AI-buddy · Hack&Change 2026

Минимальный монорепозиторий помощника онбординга: чат с источниками и два сценария — общий и по роли. Все данные синтетические, интеграции демонстрационные.

## Быстрый запуск

Требования: Docker Desktop / Docker Engine с Compose v2; Node.js 22.12+ для сборки расширения и тестов UI.

```sh
cp .env.example .env
docker compose -p ai-buddy up --build -d --wait
```

В PowerShell вместо `cp` можно использовать `Copy-Item .env.example .env`.
Откройте http://127.0.0.1:5173. Спросите «Как оформить отпуск?» — получите инструкцию и синтетическую HR-ссылку. Переключите роль, отметьте задачу. Основной API: http://127.0.0.1:8080/health. AI доступен только внутри Compose.

```sh
docker compose -p ai-buddy logs -f
docker compose -p ai-buddy down
```

Отдельные сервисы: `docker compose -p ai-buddy up --build -d ai`, затем `... up --build -d backend` или `... up --build -d web` (зависимости запускаются автоматически). Имена сервисов независимы, перезапуск: `docker compose -p ai-buddy restart ai`.
Порты на хосте задаются в `.env`; расширение по умолчанию использует Backend 8080. Для другого порта перед сборкой задайте `BACKEND_ORIGIN=http://127.0.0.1:18080` (PowerShell: `$env:BACKEND_ORIGIN='http://127.0.0.1:18080'`). Сборка согласованно обновит URL, host permission и CSP. Расширение поддерживает только локальный HTTP Backend на 127.0.0.1.

## Расширение Chrome (Manifest V3)

```sh
npm ci
npm run build --workspace @ai-buddy/extension
```

В Chrome откройте `chrome://extensions`, включите режим разработчика → «Загрузить распакованное расширение» → выберите `apps/extension/dist`. Закрепите AI-buddy, нажмите иконку и отправьте вопрос. Backend должен быть запущен. Расширение не читает страницы, не запрашивает cookies, history или tabs. Только разрешение на локальный Backend. Popup и веб используют `packages/client`.

## Разработка без Docker

Python 3.12 и Go 1.26+, Node.js 22.12+. Команды запускаются из корня, кроме явно указанных переходов; каждый сервис — в отдельном терминале.

```sh
python -m venv .venv
# Windows: .venv/Scripts/Activate.ps1; Linux/macOS: source .venv/bin/activate
python -m pip install -r apps/ai/requirements-dev.txt
python -m uvicorn app.main:app --app-dir apps/ai --host 127.0.0.1 --port 8001
```

```sh
cd apps/backend
go run .
```

```sh
npm ci
npm run dev --workspace @ai-buddy/web
```

Vite перенаправляет `/api` в Backend. `AI_URL` и `DATA_DIR` меняют адрес AI и каталог данных для backend; для AI используется `DATA_DIR`. Встроенные пути работают при указанных рабочих каталогах. `.env` автоматически читает Compose; при запуске без Docker задавайте переменные в оболочке.

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

## Структура и ответственность

| Путь | Назначение | Ответственный |
|---|---|---|
| `apps/ai` | FastAPI `/health`, `/chat`, retrieval-заглушка | ML — пользователь (nvkmaksim) |
| `apps/backend` | Go API, валидация, маршрутизация к AI, сценарии | Ваня |
| `apps/web` | React + TypeScript + Vite | Маша |
| `apps/extension` | Chrome MV3 popup | Маша |
| `packages/client` | Общий HTTP-клиент веба и расширения | Маша + Ваня |
| `contracts` | OpenAPI: публичный и внутренний API | Вся команда; согласовать до изменений |
| `docs` | Архитектура, SDD, демо и проверка | Вся команда; дизайнер — UX, аналитик — данные/метрики |
| `data` | Синтетические KB и onboarding | ML + аналитик |

Один репозиторий, отдельные сервисы и Dockerfile. Работайте в коротких feature-ветках, открывайте PR, избегайте несогласованных изменений API. Lockfile npm хранится в корне. Правила хакатона и исходное вложение не были доступны при создании: соответствие регламенту/официальному стеку не подтверждено.

## Безопасность и пределы прототипа

- Это локальное демо без авторизации, RBAC, хранения прогресса, аудита и rate limiting. Не публикуйте его в интернет в текущем виде. Compose привязывает порты к 127.0.0.1.
- Нет реальных HR/Jira/SSO-интеграций и автоматических действий. `example.com` — фиктивные ссылки, их открытие не создаёт заявку.
- Нет реальной LLM, embeddings или векторной БД. Ответ выбирается по ключевым словам из `data/kb.json`, неизвестный вопрос получает явный отказ. Следующий этап ML — retrieval, цитирование и eval-набор по контракту.
- Задачи можно отмечать, но прогресс исчезает после перезагрузки. Нет бэклогов, канбана, Ганта, учётных записей или командной синхронизации.
- Не коммитьте секреты, реальные документы сотрудников и персональные данные. `.env` игнорируется, реальные ключи пока не нужны. Не логируйте тексты чата при добавлении наблюдаемости.
- Текст отображается как текст, HTML не исполняется. Backend ограничивает размер запроса и время ожидания AI; при сбое отвечает 502. `/health` — liveness отдельного процесса.
- Перед production потребуются auth/RBAC, TLS, проверка источников и URL, защита от prompt injection, управление секретами, контроль хранения/удаления данных, мониторинг и аудит зависимостей.

Подробнее: [архитектура](docs/architecture.md), [SDD](docs/specifications/001-skeleton.md), [демо](docs/demo-scenarios.md).
