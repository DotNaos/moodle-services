# API endpoints

Use this page when you want the exact HTTP endpoints exposed by `moodle serve`.

## Base URL

The default server address is `http://127.0.0.1:8080`.

## Built-in API reference

Open the live reference in your browser:

- `http://127.0.0.1:8080/docs`
- `http://127.0.0.1:8080/scalar`

The raw OpenAPI document is available at:

- `http://127.0.0.1:8080/openapi.json`

## Endpoints

- `GET /healthz`
  Returns `{"status":"ok"}` when the server can use a valid Moodle session.
- `GET /api/courses`
  Returns your enrolled courses as JSON.
- `GET /api/courses/{courseID}/resources`
  Returns files and resources for one course as JSON.
- `GET /api/courses/{courseID}/page`
  Returns the course page as a reader-friendly text outline.
- `GET /api/courses/{courseID}/resources/{resourceID}/text`
  Returns extracted text for one file resource. Add `?raw=true` to skip PDF text cleanup.
- `GET /api/courses/{courseID}/resources/{resourceID}/ocr`
  Runs Docker-backed OCR for one PDF resource. Optional query parameters: `engine`, `out`, `format`, `timeout`, `docker-platform`, `gpu`, `formula`, `code`, `keepArtifacts`.
- `GET /api/study-pipeline/courses`
  Scans a local school workspace and returns raw, extracted, curated, and Reader readiness status for course material. Optional query parameters: `workspace`, `term`.
- `GET /api/study-pipeline/courses/{courseSlug}`
  Returns study pipeline status for one course slug. Optional query parameters: `workspace`, `term`.
- `GET /api/timetable`
  Returns timetable events from the configured calendar. Optional query parameters: `days`, `nextWeek`, `unique`.
- `GET /api/current-lecture`
  Returns today's current or next lecture, the matched course, and ranked materials. Optional query parameters: `workspace`, `at`.
- `GET /api/nav?path=current`
  Resolves a Moodle navigation path. Optional query parameters: `print`, `workspace`, `at`.
- `GET /api/mobile/qr/inspect?link=<moodlemobile-link>`
  Explains a Moodle mobile QR link without redeeming it.
- `GET /api/version`
  Returns the running CLI version metadata.

## Deliberately not exposed

The API is not a generic CLI mirror. Browser actions, local filesystem writes, token bootstrap, updating the binary, shell completion generation, and log streaming are intentionally not published as API endpoints.

## Quick check

```sh
open http://127.0.0.1:8080/docs
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/api/courses
curl http://127.0.0.1:8080/api/courses/18236/resources
curl http://127.0.0.1:8080/api/courses/18236/page
curl "http://127.0.0.1:8080/api/courses/18236/resources/12345/ocr?engine=docling&timeout=900"
curl "http://127.0.0.1:8080/api/study-pipeline/courses?workspace=/Users/oli/school&term=FS26"
curl http://127.0.0.1:8080/api/timetable?days=30
curl http://127.0.0.1:8080/api/version
```

Local study pipeline review can run without a Moodle session:

```sh
moodle serve --addr 127.0.0.1:8091 --skip-session-check --study-workspace /Users/oli/school
```

## Error shape

Errors are returned as JSON:

```json
{"error":"..."}
```
