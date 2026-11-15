-- Synthetic ML onboarding content for fresh Goose v1; run through cmd/init-catalog.
-- Safe on repeated executions when the existing ML checklist matches exactly.
-- Never changes an existing checklist or any user's progress.
BEGIN;
SELECT pg_advisory_xact_lock(20261112, 24002);
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
  FROM checklists WHERE type='professional' AND profession='ml';
  IF FOUND THEN
    IF existing_name IS DISTINCT FROM 'ML-инженер — демо v2' THEN
      RAISE EXCEPTION 'Existing ML checklist has another name; refusing to overwrite';
    END IF;

    SELECT COALESCE(jsonb_agg(jsonb_build_array(
      position,title,week_start,week_end,goal,expected_result
    ) ORDER BY position),'[]'::jsonb)
    INTO actual FROM checklist_stages WHERE checklist_id=target_checklist_id;
    SELECT jsonb_agg(jsonb_build_array(
      position,title,week_start,week_end,goal,expected_result
    ) ORDER BY position)
    INTO expected FROM (VALUES
      (1,'Окружение и данные',1,2,'Подготовить безопасное рабочее окружение','Воспроизводимое локальное окружение'),
      (2,'Первая модель',2,4,'Обучить бейзлайн','Воспроизводимые метрики бейзлайна'),
      (3,'Доставка результата',4,6,'Поделиться результатами','Демо-артефакт прошёл ревью')
    ) AS stages(position,title,week_start,week_end,goal,expected_result);
    IF actual IS DISTINCT FROM expected THEN
      RAISE EXCEPTION 'Existing ML checklist stages differ from демо v2; refusing to overwrite';
    END IF;

    SELECT COALESCE(jsonb_agg(jsonb_build_array(
      i.position,s.position,i.title,i.description,i.expected_result,
      i.requires_review,i.estimated_minutes,i.assigned_user_id
    ) ORDER BY i.position),'[]'::jsonb)
    INTO actual
    FROM checklist_items i
    JOIN checklist_stages s ON s.id=i.stage_id
    WHERE i.checklist_id=target_checklist_id;
    SELECT jsonb_agg(jsonb_build_array(
      position,stage_position,title,description,expected_result,
      requires_review,estimated_minutes,NULL
    ) ORDER BY position)
    INTO expected FROM (VALUES
      (1,1,'Настроить ML-окружение','Установить разрешённые инструменты и прочитать правила доступа к данным','Рабочее окружение',false,90),
      (2,1,'Изучить выборку данных','Описать поля и проблемы качества данных','Заметки по данным',true,120),
      (3,2,'Собрать бейзлайн','Сделать воспроизводимое разбиение на обучение и валидацию','Ноутбук с бейзлайном',true,180),
      (4,2,'Оценить модель','Измерить качество и разобрать ошибки модели','Отчёт об оценке',true,120),
      (5,3,'Оформить эксперимент','Зафиксировать параметры и артефакт модели','Воспроизводимый эксперимент',false,120),
      (6,3,'Представить результаты','Рассказать о метриках, ограничениях и следующих шагах','Ревью наставника',true,90)
    ) AS items(position,stage_position,title,description,expected_result,requires_review,estimated_minutes);
    IF actual IS DISTINCT FROM expected THEN
      RAISE EXCEPTION 'Existing ML checklist items differ from демо v2; refusing to overwrite';
    END IF;

    RETURN;
  END IF;

  INSERT INTO checklists(name,type,profession) VALUES('ML-инженер — демо v2','professional','ml') RETURNING id INTO target_checklist_id;
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
   VALUES(target_checklist_id,'Окружение и данные',1,2,'Подготовить безопасное рабочее окружение','Воспроизводимое локальное окружение',1) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
   (target_checklist_id,stage_id,'Настроить ML-окружение','Установить разрешённые инструменты и прочитать правила доступа к данным','Рабочее окружение',false,90,1),
   (target_checklist_id,stage_id,'Изучить выборку данных','Описать поля и проблемы качества данных','Заметки по данным',true,120,2);
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
   VALUES(target_checklist_id,'Первая модель',2,4,'Обучить бейзлайн','Воспроизводимые метрики бейзлайна',2) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
   (target_checklist_id,stage_id,'Собрать бейзлайн','Сделать воспроизводимое разбиение на обучение и валидацию','Ноутбук с бейзлайном',true,180,3),
   (target_checklist_id,stage_id,'Оценить модель','Измерить качество и разобрать ошибки модели','Отчёт об оценке',true,120,4);
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
   VALUES(target_checklist_id,'Доставка результата',4,6,'Поделиться результатами','Демо-артефакт прошёл ревью',3) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
   (target_checklist_id,stage_id,'Оформить эксперимент','Зафиксировать параметры и артефакт модели','Воспроизводимый эксперимент',false,120,5),
   (target_checklist_id,stage_id,'Представить результаты','Рассказать о метриках, ограничениях и следующих шагах','Ревью наставника',true,90,6);
END $$;
COMMIT;
