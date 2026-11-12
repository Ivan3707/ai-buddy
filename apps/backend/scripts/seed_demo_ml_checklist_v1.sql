-- Synthetic ML onboarding content. Run explicitly only on a verified Goose v7 database.
-- Never changes an existing checklist or any user's progress.
BEGIN;
SELECT pg_advisory_xact_lock(20261112, 24002);
DO $$
DECLARE v integer; checklist_id uuid; stage_id uuid; existing integer;
BEGIN
  IF current_database() NOT IN ('ai_buddy_test','app') THEN RAISE EXCEPTION 'Unexpected database'; END IF;
  SELECT COALESCE(MAX(version_id),0) INTO v FROM goose_db_version WHERE is_applied;
  IF v<>7 THEN RAISE EXCEPTION 'Goose v7 required, found %',v; END IF;
  SELECT count(*) INTO existing FROM checklists WHERE type='professional' AND profession='ml';
  IF existing<>0 THEN RAISE EXCEPTION 'ML checklist already exists; refusing to overwrite'; END IF;
  INSERT INTO checklists(name,type,profession) VALUES('ML Engineer — Demo v1','professional','ml') RETURNING id INTO checklist_id;
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
   VALUES(checklist_id,'Environment and data',1,2,'Prepare a safe workspace','Reproducible local setup',1) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
   (checklist_id,stage_id,'Configure ML workspace','Install approved tools and read access rules','Working environment',false,90,1),
   (checklist_id,stage_id,'Inspect sample data','Document fields and quality issues','Data notes',true,120,2);
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
   VALUES(checklist_id,'First model',2,4,'Train a baseline','Repeatable baseline metrics',2) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
   (checklist_id,stage_id,'Build baseline','Create reproducible train and validation split','Baseline notebook',true,180,3),
   (checklist_id,stage_id,'Evaluate model','Measure quality and discuss error cases','Evaluation report',true,120,4);
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
   VALUES(checklist_id,'Delivery',4,6,'Share results','Reviewed demo artifact',3) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
   (checklist_id,stage_id,'Package experiment','Record parameters and model artifact','Reproducible experiment',false,120,5),
   (checklist_id,stage_id,'Present findings','Explain metrics, limits and next steps','Buddy review',true,90,6);
END $$;
COMMIT;
