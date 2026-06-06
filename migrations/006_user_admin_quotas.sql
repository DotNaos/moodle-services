alter table users
	add column if not exists is_admin boolean not null default false,
	add column if not exists codex_state_quota_bytes bigint;

create index if not exists users_admin_idx on users (is_admin) where is_admin = true;
