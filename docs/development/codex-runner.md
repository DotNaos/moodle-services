# Codex Runner State Storage

The Moodle web UI should not run Codex inside a Vercel Function. Codex needs a
managed sandbox or runner that can keep a small authenticated Codex home between
runs.

Initial storage model:

- Keep Moodle credentials encrypted in Neon Postgres.
- Keep Codex state as small zip snapshots in `codex_state_snapshots`.
- Encrypt zip snapshots with the existing `MOODLE_ENCRYPTION_KEY` box before
  persisting them.
- Store the encrypted zip in Postgres for the first implementation.
- Store only compressed runtime state, not large Moodle PDFs or generated
  exports.
- Use `kind` to separate state:
  - `codex-auth`: Codex auth files created by the ChatGPT/Codex login flow.
  - `codex-session`: Codex session files.
  - `codex-memory`: Codex memory/config files.
  - `codex-artifacts`: small outputs that should survive sandbox teardown.

The first managed runner can use Vercel Sandbox. If that becomes too limited,
Fly Machines or E2B can hydrate and persist the same snapshots without changing
the database schema.

Preferred long-term storage:

- Ask each user to connect Google Drive.
- Request the narrow `https://www.googleapis.com/auth/drive.appdata` scope.
- Store encrypted Codex zips in the user's hidden Google Drive app data folder.
- Keep only the encrypted Google refresh token, provider metadata, object key,
  hash, size, and timestamps in Postgres.
- Hydrate sandbox state from Drive on sandbox startup and persist a new snapshot
  back to Drive before teardown.

This keeps user Codex sessions and artifacts out of our database and uses the
user's own cloud storage quota. It also gives us a clean provider abstraction:
OneDrive or Backblaze B2 can use the same `user_storage_accounts` and
`codex_state_snapshots` shape later.

Backblaze B2 is the preferred object-storage upgrade path if snapshots grow
beyond the small Postgres limit and we do not want to require user Drive
connection for a particular deployment. B2 is S3-compatible and currently
advertises the first 10 GB of storage as free. In that version,
`codex_state_snapshots` keeps the metadata, hash, size, and object key while
encrypted zip bytes live in a private B2 bucket. The runner should still decrypt
only inside the sandbox.

Ephemeral-only mode is acceptable for the earliest Codex runner prototype: keep
Codex auth and sessions inside the sandbox and accept that teardown means a new
Codex login. That mode should not be used once we expect the integration to work
across browser sessions or sandbox restarts.

Current internal API:

- `POST /api/auth/clerk/codex/state`
  - Requires `X-Moodle-Internal-Secret` and `X-Clerk-User-Id`.
  - Body: `kind`, `zipBase64`, optional `metadata`.
  - Validates that the payload is a zip archive with safe relative paths.
  - Encrypts and stores the snapshot.
- `GET /api/auth/clerk/codex/state?kind=codex-auth`
  - Requires `X-Moodle-Internal-Secret` and `X-Clerk-User-Id`.
  - Returns the latest decrypted `zipBase64` for that user and kind.

This deliberately avoids a separate object storage dependency for now. If state
grows beyond a few megabytes, move the encrypted zip bytes to B2 or another
S3-compatible store and keep only the object key, hash, and metadata in
Postgres.
