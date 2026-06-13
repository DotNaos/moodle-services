create table if not exists study_pipeline_feedback (
  id uuid primary key default gen_random_uuid(),
  course_id text not null,
  target_id text not null,
  target_kind text not null,
  feedback_type text not null,
  message text not null default '',
  source_run_id uuid references study_pipeline_runs(id) on delete set null,
  source_artifact_id text,
  status text not null default 'open' check (status in ('open', 'triaged', 'resolved', 'dismissed')),
  created_by uuid references users(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists study_pipeline_feedback_course_idx
  on study_pipeline_feedback (course_id, created_at desc);

create table if not exists study_pipeline_proposals (
  id uuid primary key default gen_random_uuid(),
  course_id text not null,
  target_id text not null,
  target_kind text not null,
  title text not null,
  content_preview text not null default '',
  source_run_id uuid references study_pipeline_runs(id) on delete set null,
  source_artifact_id text,
  model text,
  status text not null default 'private' check (status in ('private', 'submitted_for_review', 'promoted', 'dismissed')),
  created_by uuid references users(id) on delete set null,
  submitted_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists study_pipeline_proposals_course_idx
  on study_pipeline_proposals (course_id, created_at desc);
