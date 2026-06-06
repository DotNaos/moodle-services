create table if not exists webex_recording_cache (
  user_id uuid not null references users(id) on delete cascade,
  course_id text not null,
  recordings_json jsonb not null,
  fetched_at timestamptz not null default now(),
  primary key (user_id, course_id)
);

create index if not exists webex_recording_cache_fetched_at_idx on webex_recording_cache (fetched_at);
