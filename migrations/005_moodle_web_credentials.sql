alter table moodle_accounts
  add column if not exists encrypted_webex_session_json text,
  add column if not exists webex_session_updated_at timestamptz;
