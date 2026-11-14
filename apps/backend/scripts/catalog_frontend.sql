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
    IF existing_name IS DISTINCT FROM 'Frontend Developer — Demo v1' THEN
      RAISE EXCEPTION 'Existing Frontend checklist has another name; refusing to overwrite';
    END IF;
    SELECT COALESCE(jsonb_agg(jsonb_build_array(position,title,week_start,week_end,goal,expected_result) ORDER BY position),'[]'::jsonb)
      INTO actual FROM checklist_stages WHERE checklist_id=target_checklist_id;
    SELECT jsonb_agg(jsonb_build_array(position,title,week_start,week_end,goal,expected_result) ORDER BY position)
      INTO expected FROM (VALUES
        (1,'Product and workspace',1,2,'Understand the product and prepare a safe workspace','Local web application runs'),
        (2,'First interface',2,4,'Deliver an accessible feature','Reviewed UI change'),
        (3,'Quality and delivery',4,6,'Ship confidently','Tested production-ready change')
      ) AS stages(position,title,week_start,week_end,goal,expected_result);
    IF actual IS DISTINCT FROM expected THEN RAISE EXCEPTION 'Existing Frontend stages differ from Demo v1'; END IF;

    SELECT COALESCE(jsonb_agg(jsonb_build_array(i.position,s.position,i.title,i.description,i.expected_result,i.requires_review,i.estimated_minutes,i.assigned_user_id) ORDER BY i.position),'[]'::jsonb)
      INTO actual FROM checklist_items i JOIN checklist_stages s ON s.id=i.stage_id WHERE i.checklist_id=target_checklist_id;
    SELECT jsonb_agg(jsonb_build_array(position,stage_position,title,description,expected_result,requires_review,estimated_minutes,NULL) ORDER BY position)
      INTO expected FROM (VALUES
        (1,1,'Run the web workspace','Install approved dependencies and start the existing application','Working local frontend',false,90),
        (2,1,'Review design system','Document colors, typography and reusable components','Short UI inventory',true,90),
        (3,2,'Build a responsive screen','Implement the assigned screen with loading and error states','Responsive implementation',true,180),
        (4,2,'Check accessibility','Verify keyboard navigation, labels and contrast','Accessibility notes',true,120),
        (5,3,'Add automated checks','Cover the main interaction with component or browser tests','Passing test scenario',false,150),
        (6,3,'Present the change','Explain trade-offs, metrics and remaining risks','Buddy-approved demo',true,90)
      ) AS items(position,stage_position,title,description,expected_result,requires_review,estimated_minutes);
    IF actual IS DISTINCT FROM expected THEN RAISE EXCEPTION 'Existing Frontend items differ from Demo v1'; END IF;
    RETURN;
  END IF;

  INSERT INTO checklists(name,type,profession) VALUES('Frontend Developer — Demo v1','professional','frontend') RETURNING id INTO target_checklist_id;
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
    VALUES(target_checklist_id,'Product and workspace',1,2,'Understand the product and prepare a safe workspace','Local web application runs',1) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
    (target_checklist_id,stage_id,'Run the web workspace','Install approved dependencies and start the existing application','Working local frontend',false,90,1),
    (target_checklist_id,stage_id,'Review design system','Document colors, typography and reusable components','Short UI inventory',true,90,2);
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
    VALUES(target_checklist_id,'First interface',2,4,'Deliver an accessible feature','Reviewed UI change',2) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
    (target_checklist_id,stage_id,'Build a responsive screen','Implement the assigned screen with loading and error states','Responsive implementation',true,180,3),
    (target_checklist_id,stage_id,'Check accessibility','Verify keyboard navigation, labels and contrast','Accessibility notes',true,120,4);
  INSERT INTO checklist_stages(checklist_id,title,week_start,week_end,goal,expected_result,position)
    VALUES(target_checklist_id,'Quality and delivery',4,6,'Ship confidently','Tested production-ready change',3) RETURNING id INTO stage_id;
  INSERT INTO checklist_items(checklist_id,stage_id,title,description,expected_result,requires_review,estimated_minutes,position) VALUES
    (target_checklist_id,stage_id,'Add automated checks','Cover the main interaction with component or browser tests','Passing test scenario',false,150,5),
    (target_checklist_id,stage_id,'Present the change','Explain trade-offs, metrics and remaining risks','Buddy-approved demo',true,90,6);
END $$;
COMMIT;
