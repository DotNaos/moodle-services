alter table study_pipeline_runs
  add column if not exists source_id text,
  add column if not exists resource_id text,
  add column if not exists file_hash text,
  add column if not exists engine text not null default 'unknown',
  add column if not exists config_hash text not null default 'config:default',
  add column if not exists ownership text not null default 'shared',
  add column if not exists created_by uuid references users(id) on delete set null,
  add column if not exists artifact_refs jsonb not null default '[]'::jsonb;

update study_pipeline_runs
set source_id = 'source:moodle-course:' || course_id
where source_id is null or source_id = '';

alter table study_pipeline_runs
  alter column source_id set not null;

alter table study_pipeline_runs
  drop constraint if exists study_pipeline_runs_stage_check,
  add constraint study_pipeline_runs_stage_check
    check (stage in ('inventory', 'raw', 'extracted', 'curated'));

alter table study_pipeline_runs
  drop constraint if exists study_pipeline_runs_status_check,
  add constraint study_pipeline_runs_status_check
    check (status in ('queued', 'running', 'succeeded', 'failed', 'stale'));

alter table study_pipeline_runs
  drop constraint if exists study_pipeline_runs_ownership_check,
  add constraint study_pipeline_runs_ownership_check
    check (ownership in ('shared', 'user_owned'));

create index if not exists study_pipeline_runs_source_stage_idx
  on study_pipeline_runs (source_id, coalesce(resource_id, ''), stage, created_at desc);

create table if not exists active_run_selections (
  source_id text not null,
  resource_id text not null default '',
  stage text not null check (stage in ('inventory', 'raw', 'extracted', 'curated')),
  active_run_id uuid not null references study_pipeline_runs(id) on delete cascade,
  selected_by uuid references users(id) on delete set null,
  selected_at timestamptz not null default now(),
  reason text not null default '',
  primary key (source_id, resource_id, stage)
);
