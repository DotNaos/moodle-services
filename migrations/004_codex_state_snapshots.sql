create table if not exists user_storage_accounts (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  provider text not null,
  account_label text not null default '',
  encrypted_oauth_token_json text not null,
  scopes jsonb not null default '[]'::jsonb,
  status text not null default 'active',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (user_id, provider)
);

create index if not exists user_storage_accounts_user_provider_idx
  on user_storage_accounts(user_id, provider);

create table if not exists codex_state_snapshots (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references users(id) on delete cascade,
  kind text not null,
  storage_backend text not null default 'postgres',
  storage_account_id uuid references user_storage_accounts(id) on delete set null,
  object_key text,
  encrypted_zip text,
  zip_sha256 text not null,
  zip_size_bytes integer not null,
  metadata jsonb not null default '{}'::jsonb,
  created_at timestamptz not null default now(),
  constraint codex_state_snapshots_storage_check
    check (
      (storage_backend = 'postgres' and encrypted_zip is not null and object_key is null and storage_account_id is null)
      or
      (storage_backend <> 'postgres' and object_key is not null and storage_account_id is not null)
    )
);

create index if not exists codex_state_snapshots_user_kind_created_idx
  on codex_state_snapshots(user_id, kind, created_at desc);
