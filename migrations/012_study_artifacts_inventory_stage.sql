alter table study_artifacts
  drop constraint if exists study_artifacts_stage_check,
  add constraint study_artifacts_stage_check
    check (stage in ('inventory', 'raw', 'extracted', 'curated'));
