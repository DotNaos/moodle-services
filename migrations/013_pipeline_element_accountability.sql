alter table study_pipeline_runs
  add column if not exists curation_checklist jsonb,
  add column if not exists element_decisions jsonb not null default '[]'::jsonb;

create index if not exists study_pipeline_runs_element_decisions_gin_idx
  on study_pipeline_runs using gin (element_decisions);
