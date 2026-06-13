create table if not exists study_pipeline_runs (
  id uuid primary key default gen_random_uuid(),
  user_id uuid references users(id) on delete cascade,
  course_id text not null,
  stage text not null check (stage in ('inventory', 'raw', 'extracted', 'curated')),
  status text not null check (status in ('queued', 'running', 'succeeded', 'failed')),
  artifact_root text not null,
  error text,
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz not null default now()
);

create index if not exists study_pipeline_runs_course_idx
  on study_pipeline_runs (course_id, created_at desc);

create table if not exists study_resources (
  user_id uuid references users(id) on delete cascade,
  course_id text not null,
  resource_id text not null,
  name text not null,
  url text,
  resource_type text,
  file_type text,
  section_id text,
  section_name text,
  classification text not null,
  raw_path text,
  raw_sha256 text,
  updated_at timestamptz not null default now(),
  primary key (user_id, course_id, resource_id)
);

create table if not exists study_artifacts (
  id uuid primary key default gen_random_uuid(),
  user_id uuid references users(id) on delete cascade,
  course_id text not null,
  resource_id text,
  stage text not null check (stage in ('inventory', 'raw', 'extracted', 'curated')),
  kind text not null,
  path text not null,
  sha256 text,
  source_path text,
  created_at timestamptz not null default now()
);

create index if not exists study_artifacts_course_stage_idx
  on study_artifacts (course_id, stage, created_at desc);

create table if not exists study_task_links (
  user_id uuid references users(id) on delete cascade,
  course_id text not null,
  task_resource_id text not null,
  solution_resource_id text,
  status text not null,
  updated_at timestamptz not null default now(),
  primary key (user_id, course_id, task_resource_id)
);
