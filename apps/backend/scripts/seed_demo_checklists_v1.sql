-- AI-Buddy Demo Checklist v1 (DRAFT; not an official Avito onboarding program).
-- Scope: one common and one backend-professional checklist; 3 stages and 12 items each.
-- Designed for PostgreSQL at Goose v3. Safe on repeated executions:
--   * if exact same content exists: no changes;
--   * if another common/backend checklist exists: abort the transaction;
--   * never UPDATE/DELETE existing content, users, progress, or submissions.
-- Run explicitly with psql -v ON_ERROR_STOP=1 -f seed_demo_checklists_v1.sql.
-- Transaction protects against partial writes. Use only on app or ai_buddy_test.
BEGIN;

SELECT pg_advisory_xact_lock(20261111, 24001);

DO $$
DECLARE dbname text := current_database(); v integer;
BEGIN
  IF dbname NOT IN ('app', 'ai_buddy_test') THEN
    RAISE EXCEPTION 'Refusing seed in unexpected database: %', dbname;
  END IF;
  SELECT COALESCE(MAX(version_id), 0) INTO v
  FROM goose_db_version WHERE is_applied;
  IF v <> 3 THEN
    RAISE EXCEPTION 'Goose schema version 3 required; current: %', v;
  END IF;
END $$;

CREATE TEMP TABLE _seed_checklists (
  scope text PRIMARY KEY,
  name text NOT NULL,
  type text NOT NULL,
  profession text
) ON COMMIT DROP;
INSERT INTO _seed_checklists(scope,name,type,profession) VALUES
  ('common','Адаптация в команде (Demo v1)','common',NULL),
  ('backend','Backend Developer — адаптация (Demo v1)','professional','backend');

CREATE TEMP TABLE _seed_stages (
 scope text NOT NULL, position integer NOT NULL, title text NOT NULL,
 week_start integer NOT NULL, week_end integer NOT NULL, goal text NOT NULL,
 expected_result text NOT NULL, PRIMARY KEY(scope,position)
) ON COMMIT DROP;
INSERT INTO _seed_stages VALUES
('common',1,'Знакомство с командой',1,1,'Понять контекст команды, познакомиться с наставником и получить доступы.','Стажёр знает, чем занимается команда, кому задавать вопросы и какие цели стоят перед ним.'),
('common',2,'Рабочие процессы',1,3,'Освоить базовые правила и ежедневный рабочий процесс команды.','Стажёр понимает правила безопасности, коммуникации и работу с задачами.'),
('common',3,'Интеграция в команду',3,12,'Научиться ориентироваться в процессах и регулярно получать обратную связь.','Стажёр может самостоятельно находить информацию и обсуждать дальнейшее развитие.'),
('backend',1,'Погружение в проект',1,2,'Подготовить окружение и понять архитектуру backend-продукта.','Стажёр может запускать сервисы и проследить путь запроса до базы данных.'),
('backend',2,'Первая задача в кодовой базе',2,5,'Пройти полный цикл небольшой продуктовой задачи с помощью buddy.','Первая задача реализована, протестирована и принята по процессам команды.'),
('backend',3,'Переход к самостоятельной работе',5,12,'Развить автономность в диагностике, доставке и реализации изменений.','Стажёр продемонстрировал самостоятельную работу и получил техническую обратную связь.');

CREATE TEMP TABLE _seed_items (
 scope text NOT NULL, stage_position integer NOT NULL, position integer NOT NULL,
 title text NOT NULL, description text NOT NULL, expected_result text NOT NULL,
 requires_review boolean NOT NULL, estimated_minutes integer NOT NULL,
 PRIMARY KEY(scope,position)
) ON COMMIT DROP;
INSERT INTO _seed_items VALUES
('common',1,1,'Познакомиться с buddy и командой','Провести вводную встречу с buddy, узнать роли коллег и способы связи.','Стажёр знает своего buddy, основных участников команды и кому адресовать типовые вопросы.',FALSE,30),
('common',1,2,'Разобраться в роли команды и её продукте','Узнать, какую часть продукта Авито развивает команда, кто её пользователи и какие основные задачи она решает.','Стажёр может своими словами объяснить зону ответственности команды и ценность её продукта.',FALSE,60),
('common',1,3,'Получить необходимые рабочие доступы','Совместно с buddy определить необходимый минимальный набор доступов и проверить доступность согласованных систем. Не передавать пароли или токены в AI-Buddy.','Согласованные рабочие ресурсы открываются под личной учётной записью; нерешённые запросы зафиксированы отдельно.',FALSE,90),
('common',1,4,'Согласовать цели и ожидания на стажировку','Обсудить с buddy ожидаемые результаты, способы обратной связи, критерии успеха и примерный план ближайших недель.','Краткий согласованный план стажировки и цели переданы buddy текстом или ссылкой на доступный команде документ.',TRUE,45),
('common',2,5,'Ознакомиться с правилами информационной безопасности','Изучить предоставленные компанией инструкции по обработке данных, хранению секретов и работе с доступами.','Стажёр понимает, где хранить секреты, какие данные нельзя публиковать и как сообщать о проблемах безопасности.',FALSE,45),
('common',2,6,'Изучить инструменты коммуникации команды','Узнать назначение рабочих чатов, календаря, базы знаний и принятые правила асинхронной коммуникации.','Стажёр умеет находить нужный канал, назначить встречу и сформулировать запрос к коллеге.',FALSE,45),
('common',2,7,'Разобраться в планировании и ведении задач','Проследить жизненный цикл задачи: постановка, уточнение, работа, ревью и завершение в используемом командой трекере.','Стажёр понимает статусы задач, принципы приоритизации и кто отвечает за принятие результата.',FALSE,60),
('common',2,8,'Участвовать во встрече команды','Посетить плановую командную встречу, записать непонятные термины и обсудить наблюдения с buddy.','Краткое описание цели встречи, роли участников и два-три вывода предоставлены buddy.',TRUE,60),
('common',3,9,'Самостоятельно находить документацию и контакты','Решить несколько типовых информационных вопросов, пользуясь доступными документами и списком ответственных.','Стажёр умеет самостоятельно находить актуальные инструкции и знает маршрут эскалации блокеров.',FALSE,45),
('common',3,10,'Описать рабочий процесс команды','На основе наблюдений описать путь задачи от постановки до завершения и основные точки коммуникации.','Buddy получил краткую схему или текст с этапами процесса, ответственными и точками проверки.',TRUE,60),
('common',3,11,'Провести промежуточную встречу обратной связи','Подготовить итоги достигнутого, сложности, вопросы и предложения по улучшению дальнейшей адаптации.','Buddy получил краткое резюме прогресса, блокеров и согласованных дальнейших действий.',TRUE,45),
('common',3,12,'Подвести итоги адаптации и определить цели','Сопоставить результаты с первоначальными ожиданиями и обсудить следующие зоны развития.','Подготовлены итоги адаптации и согласован дальнейший план развития с buddy.',TRUE,60),
('backend',1,1,'Получить доступ к репозиториям команды','Проверить доступ к согласованным репозиториям и документации по разработке, не публикуя приватные ссылки и секреты.','Стажёр может читать нужные репозитории и знает правила работы с ветками и доступами.',FALSE,30),
('backend',1,2,'Подготовить локальное окружение и запустить backend','Установить согласованные зависимости, настроить окружение по документации и запустить backend и доступные тесты.','Стажёр предоставил buddy краткий отчёт о запуске сервиса и тестов; секреты и локальные файлы конфигурации не приложены.',TRUE,120),
('backend',1,3,'Изучить архитектуру backend и основные сервисы','Разобраться с назначением основных компонентов, API, баз данных и межсервисных взаимодействий.','Стажёр может объяснить границы ключевых сервисов и где реализована основная бизнес-логика.',FALSE,120),
('backend',1,4,'Разобрать один реальный API-запрос от HTTP до базы данных','Выбрать безопасный маршрут и проследить обработчик, сервис, репозиторий и SQL-запросы на чтение/запись.','Buddy получил схему или текстовый разбор пути запроса, ключевых проверок и возможных ошибок.',TRUE,90),
('backend',2,5,'Изучить правила ветвления, pull request и code review','Прочитать принятые в команде соглашения о ветках, коммитах, тестах и прохождении ревью.','Стажёр понимает, как подготовить PR, запросить ревью и обработать замечания.',FALSE,60),
('backend',2,6,'Выбрать с buddy небольшую задачу из backlog','Совместно выбрать ограниченную по объёму задачу, уточнить критерии готовности и необходимые проверки.','Есть конкретная задача с понятными критериями приёмки и согласованным подходом.',FALSE,45),
('backend',2,7,'Реализовать изменение и добавить тесты','Выполнить согласованную небольшую backend-задачу, покрыть ключевые случаи тестами и подготовить PR.','Buddy получил ссылку на PR (если допустимо по правилам доступа) или отчёт о реализации и проверках.',TRUE,240),
('backend',2,8,'Исправить замечания code review и довести PR до принятия','Разобрать замечания, внести правки, повторно проверить тесты и согласовать итоговое решение.','Замечания обработаны, PR принят по принятому в команде процессу либо дальнейшие блокеры зафиксированы.',TRUE,120),
('backend',3,9,'Разобраться с логированием, отладкой и диагностикой ошибок','На безопасном локальном примере воспроизвести ошибку, найти связанные логи и определить причину.','Стажёр может объяснить типичный порядок диагностики и показать результат на локальном сценарии.',FALSE,120),
('backend',3,10,'Изучить CI/CD и процесс доставки изменений','Разобрать проверки в pipeline, окружения и порядок выпуска изменений без выполнения несанкционированных деплоев.','Стажёр понимает, какие проверки обязательны перед доставкой изменения и как узнать их результат.',FALSE,90),
('backend',3,11,'Реализовать более самостоятельную backend-задачу','Согласовать задачу, самостоятельно спланировать работу, реализовать изменение, тесты и подготовить ревью.','Buddy получил PR или описание результата, тестовых проверок и принятых технических решений.',TRUE,360),
('backend',3,12,'Провести техническое демо и обсудить результаты с buddy','Продемонстрировать реализованное изменение, объяснить ключевые решения, ограничения и последующие шаги.','Buddy получил демонстрацию и краткое резюме технического прогресса и зон роста.',TRUE,60);

-- Preflight validation must finish BEFORE any persistent data is inserted.
-- Existing data is accepted only when all metadata matches Demo v1 exactly.
DO $$
DECLARE
  spec record;
  existing_id uuid;
  existing_name text;
  actual jsonb;
  expected jsonb;
BEGIN
  FOR spec IN SELECT * FROM _seed_checklists LOOP
    SELECT id,name INTO existing_id,existing_name
      FROM checklists
      WHERE type=spec.type AND profession IS NOT DISTINCT FROM spec.profession;
    IF existing_id IS NULL THEN
      CONTINUE;
    END IF;
    IF existing_name IS DISTINCT FROM spec.name THEN
      RAISE EXCEPTION 'Existing % checklist has another name. Seed will not overwrite it.',spec.scope;
    END IF;

    SELECT COALESCE(jsonb_agg(jsonb_build_array(s.position,s.title,s.week_start,s.week_end,s.goal,s.expected_result)
      ORDER BY s.position),'[]'::jsonb)
      INTO actual FROM checklist_stages s WHERE s.checklist_id=existing_id;
    SELECT COALESCE(jsonb_agg(jsonb_build_array(s.position,s.title,s.week_start,s.week_end,s.goal,s.expected_result)
      ORDER BY s.position),'[]'::jsonb)
      INTO expected FROM _seed_stages s WHERE s.scope=spec.scope;
    IF actual IS DISTINCT FROM expected THEN
      RAISE EXCEPTION 'Existing % checklist stages differ from Demo v1. No changes made.',spec.scope;
    END IF;

    SELECT COALESCE(jsonb_agg(jsonb_build_array(i.position,s.position,i.title,i.description,
      i.expected_result,i.requires_review,i.estimated_minutes) ORDER BY i.position),'[]'::jsonb)
      INTO actual
      FROM checklist_items i JOIN checklist_stages s ON s.id=i.stage_id
      WHERE i.checklist_id=existing_id;
    SELECT COALESCE(jsonb_agg(jsonb_build_array(i.position,i.stage_position,i.title,i.description,
      i.expected_result,i.requires_review,i.estimated_minutes) ORDER BY i.position),'[]'::jsonb)
      INTO expected FROM _seed_items i WHERE i.scope=spec.scope;
    IF actual IS DISTINCT FROM expected THEN
      RAISE EXCEPTION 'Existing % checklist items differ from Demo v1. No changes made.',spec.scope;
    END IF;
  END LOOP;
END $$;

-- Only missing checklists are inserted. Existing exactly-matching checklists stay untouched.
INSERT INTO checklists(name,type,profession)
SELECT s.name,s.type,s.profession FROM _seed_checklists s
WHERE NOT EXISTS (SELECT 1 FROM checklists c
  WHERE c.type=s.type AND c.profession IS NOT DISTINCT FROM s.profession);

INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
SELECT c.id,s.title,s.week_start,s.week_end,s.goal,s.expected_result,s.position
FROM _seed_stages s
JOIN _seed_checklists k ON k.scope=s.scope
JOIN checklists c ON c.type=k.type AND c.profession IS NOT DISTINCT FROM k.profession
WHERE NOT EXISTS (SELECT 1 FROM checklist_stages existing
  WHERE existing.checklist_id=c.id AND existing.position=s.position);

INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position)
SELECT c.id,st.id,i.title,i.description,i.expected_result,i.requires_review,i.estimated_minutes,i.position
FROM _seed_items i
JOIN _seed_checklists k ON k.scope=i.scope
JOIN checklists c ON c.type=k.type AND c.profession IS NOT DISTINCT FROM k.profession
JOIN checklist_stages st ON st.checklist_id=c.id AND st.position=i.stage_position
WHERE NOT EXISTS (SELECT 1 FROM checklist_items existing
  WHERE existing.checklist_id=c.id AND existing.position=i.position);

-- Final count and content confirmation. Raises before COMMIT if anything is wrong.
DO $$
DECLARE spec record; stages_count int; items_count int;
BEGIN
 FOR spec IN SELECT * FROM _seed_checklists LOOP
   SELECT count(*) INTO stages_count FROM checklist_stages s JOIN checklists c ON s.checklist_id=c.id
    WHERE c.type=spec.type AND c.profession IS NOT DISTINCT FROM spec.profession;
   SELECT count(*) INTO items_count FROM checklist_items i JOIN checklists c ON i.checklist_id=c.id
    WHERE c.type=spec.type AND c.profession IS NOT DISTINCT FROM spec.profession;
   IF stages_count <> 3 OR items_count <> 12 THEN
     RAISE EXCEPTION 'Unexpected % seed counts: stages %, items %', spec.scope, stages_count, items_count;
   END IF;
 END LOOP;
END $$;
COMMIT;

-- Read-only check after application:
SELECT c.type,c.profession,c.name,
  (SELECT COUNT(*) FROM checklist_stages s WHERE s.checklist_id=c.id) AS stages,
  (SELECT COUNT(*) FROM checklist_items i WHERE i.checklist_id=c.id) AS items,
  (SELECT COUNT(*) FROM checklist_items i WHERE i.checklist_id=c.id AND i.requires_review) AS review_items
FROM checklists c
WHERE c.type='common' OR (c.type='professional' AND c.profession='backend')
ORDER BY c.type;
