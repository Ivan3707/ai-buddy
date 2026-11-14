# AI-Buddy

AI-Buddy — платформа адаптации стажёров: Go Backend, PostgreSQL, React Web, Python AI и расширение Chrome. Buddy проверяет задания; Teamlead видит профессиональный прогресс и добавляет персональные задания. Учётные записи и назначения Buddy/Teamlead приходят из Mock HR.

## Первый запуск на Windows

Нужны Windows PowerShell, Docker Desktop с Compose, Go 1.26 и Goose (`go install github.com/pressly/goose/v3/cmd/goose@v3.24.1`). Для сборки Extension и локальных тестов нужен Node.js 22. Docker собирает Web и AI самостоятельно. Миграция `00001_init.sql` предназначена для пустой БД; bootstrap отказывается применять её к другой истории Goose.

Из корня клонированного репозитория:

```powershell
Copy-Item .env.example .env
# В .env замените DB_PASSWORD и JWT_SECRET на случайные секреты.
# JWT_SECRET должен содержать не менее 32 символов.
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
.\scripts\bootstrap.ps1
```

Разрешение на выполнение скриптов действует только в текущем окне PowerShell. Bootstrap сохраняет учётные данные Mock HR в `secrets/` в корне проекта. Каталог исключён из Git и всех Docker build contexts; файлы с паролями не попадают в образы. При повторном запуске пароли существующих сотрудников не меняются и заново не выдаются.

Скрипт запускает PostgreSQL, применяет Goose, выполняет `init-catalog` и `sync-hr`, затем собирает и запускает приложение с ожиданием healthcheck. Путь к первому файлу в `secrets/` выводится после Sync; сами пароли не выводятся. Сохраните этот файл: повторный Sync не выдаст пароль существующего сотрудника. Для отдельного стенда используйте параметры `-ProjectName`, `-EnvFile`, `-CredentialsDirectory` и отдельные `WEB_PORT`, `BACKEND_PORT`, `POSTGRES_PORT` в его env-файле.

Web: <http://127.0.0.1:5173>; Backend: <http://127.0.0.1:8080/health>. Проверяйте `docker compose ps`: все четыре сервиса должны быть healthy. `docker compose up -d --build` подходит для обычного повторного запуска инициализированного стенда; миграции и HR Sync эта команда не выполняет. Чтобы повторно применить справочник и снимок HR, снова запустите `.\scripts\bootstrap.ps1`. Прежние пароли и прогресс сохраняются; новый файл с `[]` не заменяет первый файл. Остановка без удаления данных: `docker compose stop`.

Типичные ошибки:

- Шаблонный секрет: замените `DB_PASSWORD` и `JWT_SECRET` в `.env`. Изменение пароля в env-файле не меняет пароль роли в существующем PostgreSQL volume.
- Занятый порт: задайте свободные `WEB_PORT`, `BACKEND_PORT`, `POSTGRES_PORT`; для второго стенда задайте отдельный ProjectName.
- Другая история Goose или таблица пользователей без Goose: bootstrap останавливается. Не применяйте миграцию к старой БД v8/v9 без отдельного плана и backup.
- Ошибка создания файла учётных данных: проверьте каталог вне Git и возможность ограничить ACL. При неизвестном результате COMMIT сохраните созданный файл и проверьте БД перед повтором.
- Backend не поднялся после временной недоступности БД: проверьте `docker compose ps` и `docker compose logs backend`; для согласованного запуска используйте `docker compose up -d --wait`.

## Первичные пароли

`cmd/sync-hr` генерирует случайные пароли только для новых сотрудников. В PostgreSQL сохраняются только Argon2id-хеши. По умолчанию bootstrap создаёт `secrets/hr-*.json` с закрытыми правами без замены существующего файла; `-CredentialsDirectory` позволяет выбрать каталог вне репозитория. `secrets/` исключён в `.gitignore` и `.dockerignore`. Ранее выданные файлы в `%LOCALAPPDATA%\ai-buddy\credentials\` не перемещаются и не удаляются. Повторная синхронизация сохраняет пароли и прогресс существующих пользователей; текущий `sync-hr` создаёт новый файл с `[]`, если новых аккаунтов нет, не перезаписывая первый файл.

Полный файл публикуется до `COMMIT`. При ошибке записи транзакция откатывается. Если результат `COMMIT` неизвестен, сохраните файл: в нём остаются первичные пароли на случай успешного commit. Проверьте состояние БД перед повторной синхронизацией. Не выводите содержимое файла в логи и не храните открытые пароли в `data/corporate_employees.json`, `.env` или отслеживаемых Git-файлах.

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
