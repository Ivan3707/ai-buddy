---
id: git-workflow
title: Git и merge request
url: https://wiki.demo.local/process/git
updated: 2024-11-05
---
## Ветки
Ветку создают от main и называют по шаблону feature/<KEY>-короткое-описание, где KEY — ключ задачи в Jira, например feature/REC-142-fix-ranking.

## Коммиты
Сообщение коммита начинается с ключа задачи и описывает, что изменено: «REC-142: fix ranking for empty history».

## Merge request
Для мержа merge request нужно минимум 2 одобрения (аппрува) от участников команды и зелёный CI. Мержит автор после получения одобрений.
