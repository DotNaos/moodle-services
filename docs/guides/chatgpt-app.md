# ChatGPT Moodle App

This project exposes a private ChatGPT App endpoint at `/api/mcp`.

## Configuration

Configure secrets in the hosting environment. Do not commit these values.

## API key mode

The hosted app reads user credentials from Neon Postgres. `DATABASE_URL`,
`APP_ENCRYPTION_KEY`, and `API_KEY_HASH_SECRET` must be configured in hosted
environments.

Run the schema in `docs/guides/chatgpt-app-schema.sql`, then connect each user
through the web login or insert user-specific rows for private development:

- `user_id`: stable local user id, for example `oli`
- `api_key_hash`: SHA-256 hex digest of the generated API key
- `mobile_session_json`: JSON from `~/.moodle/mobile-session.json`
- `calendar_url`: school ICS subscription URL

Use the MCP URL with the generated API key:

```text
https://moodle-services.vercel.app/api/mcp?key=<generated-api-key>
```

This query-string API key is an interim private setup so ChatGPT Developer Mode can connect without custom request headers. Replace it with OAuth before broader sharing.

## No global Moodle session fallback

Hosted API requests must never use a global Moodle account from environment
variables. If `DATABASE_URL` is missing, request-backed APIs fail instead of
falling back to `MOODLE_MOBILE_SESSION_JSON`, `MOODLE_MOBILE_TOKEN`, or
`MOODLE_SESSION_JSON`.

Calendar:

- `MOODLE_CALENDAR_URL`: school ICS subscription URL

## ChatGPT setup

1. Deploy the project to an HTTPS host such as Vercel.
2. Open ChatGPT settings and enable Developer Mode under Apps advanced settings.
3. Create a new app from the deployed MCP URL, for example `https://example.vercel.app/api/mcp`.
4. Refresh the app in ChatGPT after changing tools or metadata.

## Exposed tools

- `search`: searches Moodle courses, materials, PDFs, and calendar events.
- `fetch`: fetches details for a `search` result id.
- `list_courses`: shows enrolled Moodle courses.
- `list_course_materials`: shows materials for one course.
- `list_calendar_events`: shows upcoming school calendar events.
- `read_material_text`: extracts text from a Moodle material or PDF.
- `render_pdf_viewer`: opens a real embedded PDF viewer in the ChatGPT widget.
- `open_pdf_location`: reopens the PDF viewer at a requested page or search text.
- `get_pdf_view_state`: tells the model how the widget reports the currently visible PDF page.
- `capture_pdf_view`: returns the latest widget-reported screenshot of the visible PDF page.
- `save_pdf_view_state`: widget-only reporting hook for current page and screenshot state.

Raw Moodle file URLs are not returned to the model. PDFs are fetched through `/api/pdf`, which requires the same API key as `/api/mcp` and is only sent to the widget as tool metadata.

All tools are read-only.
