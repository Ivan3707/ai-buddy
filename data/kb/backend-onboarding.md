---
id: backend-onboarding
title: Онбординг backend-стажёра
url: https://wiki.demo.local/onboarding/backend
updated: 2026-08-11
---
## Стек и шаблон сервиса
Сервисы пишутся на Go по шаблону service-template: HTTP-хендлеры, PostgreSQL, миграции, метрики и трейсинг из коробки.

## Локальный запуск
Каждый сервис запускается локально через docker compose по README репозитория. Перед MR прогоняй go test и линтер.

## Деплой
Деплой идёт через CI после мержа в main. Стажёры деплоят только на dev-стенд; выкладку в прод делает тимлид или дежурный.
