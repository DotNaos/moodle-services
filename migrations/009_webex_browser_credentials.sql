alter table moodle_accounts
  add column if not exists encrypted_webex_credentials text,
  add column if not exists webex_credentials_updated_at timestamptz;
