-- Synthetic ML onboarding content. Run explicitly only on a verified Goose v7 database.
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
  IF current_database() NOT IN ('ai_buddy_test','app') THEN RAISE EXCEPTION 'Unexpected database'; END IF;
  SELECT COALESCE(MAX(version_id),0) INTO v FROM goose_db_version WHERE is_applied;
  IF v<>7 THEN RAISE EXCEPTION 'Goose v7 required, found %',v; END IF;
  SELECT id,name INTO target_checklist_id,existing_name
  FROM checklists WHERE type='professional' AND profession='ml';
  IF FOUND THEN
    IF existing_name IS DISTINCT FROM 'ML Engineer — Demo v1' THEN
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
      (1,'Environment and data',1,2,'Prepare a safe workspace','Reproducible local setup'),
      (2,'First model',2,4,'Train a baseline','Repeatable baseline metrics'),
      (3,'Delivery',4,6,'Share results','Reviewed demo artifact')
    ) AS stages(position,title,week_start,week_end,goal,expected_result);
    IF actual IS DISTINCT FROM expected THEN
      RAISE EXCEPTION 'Existing ML checklist stages differ from Demo v1; refusing to overwrite';
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
      (1,1,'Configure ML workspace','Install approved tools and read access rules','Working environment',false,90),
      (2,1,'Inspect sample data','Document fields and quality issues','Data notes',true,120),
      (3,2,'Build baseline','Create reproducible train and validation split','Baseline notebook',true,180),
      (4,2,'Evaluate model','Measure quality and discuss error cases','Evaluation report',true,120),
      (5,3,'Package experiment','Record parameters and model artifact','Reproducible experiment',false,120),
      (6,3,'Present findings','Explain metrics, limits and next steps','Buddy review',true,90)
    ) AS items(position,stage_position,title,description,expected_result,requires_review,estimated_minutes);
    IF actual IS DISTINCT FROM expected THEN
      RAISE EXCEPTION 'Existing ML checklist items differ from Demo v1; refusing to overwrite';
    END IF;

    RETURN;
  END IF;

  INSERT INTO checklists(name,type,profession) VALUES('ML Engineer — Demo v1','professional','ml') RETURNING id INTO target_checklist_id;
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
   VALUES(target_checklist_id,'Environment and data',1,2,'Prepare a safe workspace','Reproducible local setup',1) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
   (target_checklist_id,stage_id,'Configure ML workspace','Install approved tools and read access rules','Working environment',false,90,1),
   (target_checklist_id,stage_id,'Inspect sample data','Document fields and quality issues','Data notes',true,120,2);
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
   VALUES(target_checklist_id,'First model',2,4,'Train a baseline','Repeatable baseline metrics',2) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
   (target_checklist_id,stage_id,'Build baseline','Create reproducible train and validation split','Baseline notebook',true,180,3),
   (target_checklist_id,stage_id,'Evaluate model','Measure quality and discuss error cases','Evaluation report',true,120,4);
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
   VALUES(target_checklist_id,'Delivery',4,6,'Share results','Reviewed demo artifact',3) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
   (target_checklist_id,stage_id,'Package experiment','Record parameters and model artifact','Reproducible experiment',false,120,5),
   (target_checklist_id,stage_id,'Present findings','Explain metrics, limits and next steps','Buddy review',true,90,6);
END $$;
COMMIT;
