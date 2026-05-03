create table if not exists mobile_bridge_requests (
  challenge_hash text primary key,
  clerk_user_id text not null,
  origin text not null,
  endpoint text not null,
  app_name text not null default '',
  state text not null default '',
  expires_at timestamptz not null,
  completed_at timestamptz,
  consumed_at timestamptz,
  user_id uuid references users(id) on delete set null,
  encrypted_api_key text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index if not exists mobile_bridge_requests_clerk_user_idx
  on mobile_bridge_requests(clerk_user_id);

create index if not exists mobile_bridge_requests_active_idx
  on mobile_bridge_requests(expires_at)
  where completed_at is null;
