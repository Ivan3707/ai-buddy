-- Synthetic Frontend onboarding content for fresh Goose v1; run through cmd/init-catalog.
-- Exact repeats are no-ops; conflicting existing content is never overwritten.
BEGIN;
SELECT pg_advisory_xact_lock(20261113, 24003);
DO $$
DECLARE
  v integer;
  target_checklist_id uuid;
  stage_id uuid;
  existing_name text;
  actual jsonb;
  expected jsonb;
BEGIN
  SELECT COALESCE(MAX(version_id),0) INTO v FROM goose_db_version WHERE is_applied;
  IF v<1 THEN RAISE EXCEPTION 'Goose v1 or newer required, found %',v; END IF;

  SELECT id,name INTO target_checklist_id,existing_name
  FROM checklists WHERE type='professional' AND profession='frontend';
  IF FOUND THEN
    IF existing_name IS DISTINCT FROM 'Frontend-разработчик — демо v2' THEN
      RAISE EXCEPTION 'Existing Frontend checklist has another name; refusing to overwrite';
    END IF;
    SELECT COALESCE(jsonb_agg(jsonb_build_array(position,title,week_start,week_end,goal,expected_result) ORDER BY position),'[]'::jsonb)
      INTO actual FROM checklist_stages WHERE checklist_id=target_checklist_id;
    SELECT jsonb_agg(jsonb_build_array(position,title,week_start,week_end,goal,expected_result) ORDER BY position)
      INTO expected FROM (VALUES
        (1,'Продукт и рабочее окружение',1,2,'Понять продукт и подготовить рабочее окружение','Веб-приложение запускается локально'),
        (2,'Первый интерфейс',2,4,'Сделать доступную фичу','Изменение UI прошло ревью'),
        (3,'Качество и доставка',4,6,'Уверенно выкатывать изменения','Протестированное изменение, готовое к релизу')
      ) AS stages(position,title,week_start,week_end,goal,expected_result);
    IF actual IS DISTINCT FROM expected THEN RAISE EXCEPTION 'Existing Frontend stages differ from демо v2'; END IF;

    SELECT COALESCE(jsonb_agg(jsonb_build_array(i.position,s.position,i.title,i.description,i.expected_result,i.requires_review,i.estimated_minutes,i.assigned_user_id) ORDER BY i.position),'[]'::jsonb)
      INTO actual FROM checklist_items i JOIN checklist_stages s ON s.id=i.stage_id WHERE i.checklist_id=target_checklist_id;
    SELECT jsonb_agg(jsonb_build_array(position,stage_position,title,description,expected_result,requires_review,estimated_minutes,NULL) ORDER BY position)
      INTO expected FROM (VALUES
        (1,1,'Запустить веб-проект команды','Установить разрешённые зависимости и запустить существующее приложение локально','Фронтенд работает локально',false,90),
        (2,1,'Изучить дизайн-систему','Описать цвета, типографику и переиспользуемые компоненты','Короткий конспект по UI-компонентам',true,90),
        (3,2,'Сверстать адаптивный экран','Реализовать выданный экран с состояниями загрузки и ошибки','Адаптивная реализация экрана',true,180),
        (4,2,'Проверить доступность','Проверить навигацию с клавиатуры, подписи и контраст','Заметки по доступности',true,120),
        (5,3,'Добавить автотесты','Покрыть основной сценарий компонентными или браузерными тестами','Зелёный тестовый сценарий',false,150),
        (6,3,'Представить изменение','Рассказать о компромиссах, метриках и оставшихся рисках','Демо одобрено наставником',true,90)
      ) AS items(position,stage_position,title,description,expected_result,requires_review,estimated_minutes);
    IF actual IS DISTINCT FROM expected THEN RAISE EXCEPTION 'Existing Frontend items differ from демо v2'; END IF;
    RETURN;
  END IF;

  INSERT INTO checklists(name,type,profession) VALUES('Frontend-разработчик — демо v2','professional','frontend') RETURNING id INTO target_checklist_id;
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
    VALUES(target_checklist_id,'Продукт и рабочее окружение',1,2,'Понять продукт и подготовить рабочее окружение','Веб-приложение запускается локально',1) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
    (target_checklist_id,stage_id,'Запустить веб-проект команды','Установить разрешённые зависимости и запустить существующее приложение локально','Фронтенд работает локально',false,90,1),
    (target_checklist_id,stage_id,'Изучить дизайн-систему','Описать цвета, типографику и переиспользуемые компоненты','Короткий конспект по UI-компонентам',true,90,2);
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
    VALUES(target_checklist_id,'Первый интерфейс',2,4,'Сделать доступную фичу','Изменение UI прошло ревью',2) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
    (target_checklist_id,stage_id,'Сверстать адаптивный экран','Реализовать выданный экран с состояниями загрузки и ошибки','Адаптивная реализация экрана',true,180,3),
    (target_checklist_id,stage_id,'Проверить доступность','Проверить навигацию с клавиатуры, подписи и контраст','Заметки по доступности',true,120,4);
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
    VALUES(target_checklist_id,'Качество и доставка',4,6,'Уверенно выкатывать изменения','Протестированное изменение, готовое к релизу',3) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
    (target_checklist_id,stage_id,'Добавить автотесты','Покрыть основной сценарий компонентными или браузерными тестами','Зелёный тестовый сценарий',false,150,5),
    (target_checklist_id,stage_id,'Представить изменение','Рассказать о компромиссах, метриках и оставшихся рисках','Демо одобрено наставником',true,90,6);
END $$;
COMMIT;
