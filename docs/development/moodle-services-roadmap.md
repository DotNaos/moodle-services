# Moodle Services Roadmap

This document captures the agreed direction for consolidating the Moodle projects.
It is intended to survive chat/context loss and should be detailed enough for another
developer or agent to continue the work.

## Decision Summary

- The old `moodle-cli` project is now **Moodle Services**.
- The GitHub repository is `moodle-services`.
- Keep the installed command name as `moodle` for now, because it is ergonomic and
  avoids breaking local workflows.
- Make Moodle Services the single backend authority for Moodle access, server APIs,
  ChatGPT/MCP integration, authentication, Neon storage, PDF handling, and OpenAPI.
- Keep `moodle-clients` as the single user-facing client monorepo for mobile, web,
  and browser extension UI.
- Do not keep parallel Moodle backends in TypeScript. TypeScript clients should call
  the Moodle Services API instead of reimplementing Moodle Mobile API calls.
- Generate an OpenAPI spec from the Go API and use that spec to generate a typed
  TypeScript API client.

## Repository Ownership

### `moodle-services`

Current path:

```text
/Users/oli/projects.school/tools/moodle/moodle-services
```

Target responsibility:

- Go CLI binary.
- Hosted Go API.
- ChatGPT App / MCP endpoint.
- Moodle Mobile QR token exchange.
- Moodle Mobile API access.
- Moodle course, material, file, PDF, calendar, and search services.
- API-key authentication.
- Later Clerk integration.
- Neon Postgres storage.
- OpenAPI spec generation.
- PDF proxying and extraction.
- Server-side security controls.

Target shape:

```text
cmd/moodle/
  CLI entrypoint. The binary can keep the name `moodle`.

api/
  Thin Vercel entrypoints only. No business logic.

internal/moodle/
  Low-level Moodle integration:
  QR link parsing, QR token exchange, Moodle Mobile API calls,
  courses, resources, downloads, PDF extraction, calendar parsing.

internal/moodleservice/
  Shared application use cases:
  list courses, list materials, read material text, open PDF,
  search, calendar events, current/next lecture support.

internal/auth/
  API keys, session cookies, future Clerk user mapping.

internal/store/
  Neon/Postgres access with explicit SQL.

internal/crypto/
  Encryption/decryption of Moodle sessions and sensitive URLs.

internal/httpapi/
  REST API transport and OpenAPI route registration.

internal/chatgptapp/
  MCP tools, ChatGPT widget resources, ChatGPT-specific metadata.

migrations/
  SQL migrations for Neon.

docs/
  Architecture, deployment, API, and operator documentation.
```

### `moodle-clients`

Current path:

```text
/Users/oli/projects.school/tools/moodle/moodle-clients
```

Target responsibility:

- Expo mobile app.
- Expo web/PWA app.
- Chrome extension.
- Shared app UI.
- QR scanner UI.
- API-key management UI.
- Client-side session UX.
- Typed TypeScript API client generated from Moodle Services OpenAPI.

Target shape:

```text
apps/mobile/
  Native Expo shell, camera permissions, secure storage integration.

apps/web/
  Web/PWA shell and deployed browser UI.

apps/extension/
  Moodle browser extension and Moodle DOM integration.

packages/app/
  Shared mobile/web app UI and screens.

packages/api-client/
  Generated or wrapped TypeScript client for the Moodle Services API.

packages/shared-types/
  Shared client-side UI/domain types only when they are not generated.
```

`moodle-clients` should not permanently contain its own Moodle backend. Existing
Moodle proxy and Moodle Mobile API code should be phased out after the hosted API is
ready.

## Projects To Consolidate Or Archive

### Keep Active

- `moodle-services`: backend, CLI, API, ChatGPT, Neon, Moodle integration.
- `moodle-clients`: mobile, web, extension, UI.
- `obsidian-moodle-explorer`: may stay separate as an Obsidian distribution wrapper,
  but it should be a thin client over the CLI or hosted API.

### Fold Then Archive

- `moodle-bridge`: useful OAuth/pairing ideas, but it duplicates backend work.
  Move durable ideas into `moodle-services` or `moodle-clients`, then archive.
- `moodle-custom-ui`: effectively superseded by `moodle-clients/apps/extension`.
  Confirm no missing functionality, then archive.

### Archive / Delete If No Deployment Depends On It

- `llm-poc`: stale proof of concept derived from bridge work.

## Backend Architecture

The Go backend is the source of truth. Avoid a TypeScript API layer that calls a Go
API and then exposes another API. That would reintroduce duplicate auth, duplicate
errors, duplicate models, and duplicate debugging paths.

Preferred flow:

```text
Moodle / Calendar
      ^
      |
Moodle Services Go backend
      ^
      |
OpenAPI JSON
      ^
      |
Generated TypeScript API client
      ^
      |
Mobile / Web / Extension / Obsidian / ChatGPT frontend surfaces
```

Transport-specific code should be thin:

- CLI parses flags and prints results.
- REST API maps HTTP requests to service calls.
- ChatGPT MCP maps tool calls to service calls.
- Vercel handlers load config and delegate.
- TypeScript clients call the generated HTTP client.

## Authentication Plan

Initial mode:

- QR login from Moodle Mobile QR code.
- API-key access for API and ChatGPT.
- Neon-only user/account storage.

Later mode:

- Clerk login added on top.
- Existing API keys remain supported.
- Clerk user ID maps to the same internal user record.

QR login flow:

1. User opens Moodle on laptop and displays the Moodle Mobile QR code.
2. User opens Moodle Services or Moodle Clients web/mobile UI on phone.
3. UI scans the QR code with the camera.
4. Client sends QR payload to Moodle Services.
5. Moodle Services parses the `moodlemobile://...` link.
6. Moodle Services exchanges the QR login key with Moodle over HTTP.
7. Moodle Services validates the returned Moodle Mobile token.
8. Moodle Services encrypts and stores the Moodle session in Neon.
9. User can create API keys.
10. API, ChatGPT, and clients use the API key or later Clerk session.

Username/password login is a fallback only. It is more fragile because school SSO,
2FA, and browser-only login flows are harder to support reliably on Vercel.

## Security Requirements

Default rule:

```text
No authenticated Moodle data may be returned without a valid API key,
valid web session, or later valid Clerk user.
```

API keys:

- Generate high-entropy random keys.
- Show each full key only once.
- Store only a hash/HMAC in Neon.
- Store a short key prefix for display and debugging.
- Support naming, last-used timestamp, scopes, and revocation.
- Never log full API keys.

Moodle sessions:

- Store encrypted Moodle Mobile session JSON in Neon.
- Do not store raw Moodle tokens in logs.
- Do not return raw Moodle file URLs with tokens to clients or ChatGPT.
- Use server-side PDF/file proxy endpoints.
- Keep encryption key in Vercel environment secrets, not in the database.

HTTP/API:

- HTTPS only in production.
- Restrict CORS to known origins where possible.
- Use `HttpOnly`, `Secure`, `SameSite=Lax` cookies for web sessions.
- Add rate limits to QR login, API-key creation, and auth failures.
- Keep public endpoints deliberate and read-oriented.

Database:

- Use Neon Postgres.
- Use the pooled connection string for serverless deployments.
- Use explicit SQL first; do not introduce a large ORM.
- Add migrations under `migrations/`.

## OpenAPI Requirement

The Moodle Services REST API must generate and serve an OpenAPI spec.

Required endpoints:

```text
GET /api/openapi.json
GET /api/docs
```

The OpenAPI document should be generated from the Go API route/schema definitions
or kept as a checked, validated artifact if generation is too expensive at runtime.

The spec must cover:

- Authentication endpoints.
- API-key management endpoints.
- Moodle course/material/calendar/search endpoints.
- PDF/file endpoints.
- Error response shapes.
- Security schemes.
- Operation IDs stable enough for client generation.

The OpenAPI spec is the contract between `moodle-services` and `moodle-clients`.
TypeScript request and response types should not be manually duplicated.

## TypeScript Client Generation

Use OpenAPI-based client generation in `moodle-clients`.

Preferred direction:

- Generate a TypeScript API client from `moodle-services` OpenAPI JSON.
- Put generated or generated-wrapped code in:

```text
moodle-clients/packages/api-client
```

Candidate tooling:

- `@hey-api/openapi-ts`: generates TypeScript SDK/client code from OpenAPI.
- `openapi-typescript` plus `openapi-fetch`: generates types and a typed fetch
  client with minimal runtime.

Implementation should evaluate which option best fits the client repo at the time,
but the important architectural decision is fixed:

```text
Go API -> OpenAPI JSON -> generated TypeScript client
```

Example target workflow:

```text
pnpm --filter @moodle-clients/api-client generate
```

Possible generator input:

```text
https://moodle-services.vercel.app/api/openapi.json
```

Possible local input:

```text
../moodle-services/docs/generated/openapi.json
```

Generated client code should not be hand-edited. If wrapper ergonomics are needed,
create small hand-written wrappers around generated functions.

## API Surface

The API must stay curated. Do not expose a generic CLI command mirror.

Initial target endpoints:

```text
POST /api/auth/qr/exchange
GET  /api/me

POST   /api/keys
GET    /api/keys
DELETE /api/keys/{keyId}

GET /api/courses
GET /api/courses/{courseId}
GET /api/courses/{courseId}/materials
GET /api/courses/{courseId}/materials/{resourceId}/text
GET /api/courses/{courseId}/materials/{resourceId}/pdf

GET /api/calendar/events
GET /api/search

GET /api/openapi.json
GET /api/docs

POST /api/mcp
GET  /api/pdf
```

The exact paths can still be refined, but every endpoint must have a clear product
reason. Avoid endpoints such as:

```text
POST /api/cli/*
```

## Database Model Draft

Initial tables:

```text
users
  id
  moodle_site_url
  moodle_user_id
  display_name
  clerk_user_id nullable
  created_at
  updated_at

moodle_accounts
  id
  user_id
  school_id
  site_url
  encrypted_mobile_session_json
  token_last_validated_at
  created_at
  updated_at

api_keys
  id
  user_id
  name
  key_prefix
  key_hash
  scopes
  last_used_at
  revoked_at
  created_at

web_sessions
  id
  user_id
  session_hash
  expires_at
  created_at

calendar_subscriptions
  id
  user_id
  encrypted_url
  created_at
  updated_at

pdf_view_states
  id
  user_id
  course_id
  resource_id
  page
  page_count
  screenshot_ref nullable
  updated_at

audit_events
  id
  user_id
  event_type
  ip_hash nullable
  created_at
```

## Migration Plan

### Phase 1: Documentation and naming

- Confirm `Moodle Services` as the project name.
- Rename docs and user-facing wording.
- Keep local project naming aligned with the `moodle-services` repository.
- Keep binary command `moodle`.

### Phase 2: Internal service boundaries

- Introduce `internal/moodleservice`.
- Move duplicated use cases out of CLI and ChatGPT-specific code.
- Keep `internal/moodle` as low-level Moodle integration.
- Move `pkg/chatgptapp` into `internal/chatgptapp` or split it by concern.
- Move auth/store code into `internal/auth` and `internal/store`.

### Phase 3: Hosted auth and Neon

- Add migrations.
- Add encrypted Moodle session storage.
- Add QR exchange login endpoint.
- Add API-key creation, listing, and revocation.
- Use API-key auth for REST API and ChatGPT MCP.

### Phase 4: Curated REST API

- Build REST endpoints over `internal/moodleservice`.
- Keep Vercel handlers thin.
- Add tests for auth, endpoint shape, and no raw Moodle token leakage.

### Phase 5: OpenAPI

- Add OpenAPI generation/serving.
- Add spec validation in tests or CI.
- Commit or publish generated `openapi.json` if useful for clients.
- Ensure stable operation IDs.

### Phase 6: TypeScript client

- Add `moodle-clients/packages/api-client`.
- Configure OpenAPI client generation.
- Replace hand-written Moodle API calls in `moodle-clients` with generated client
  calls and small wrappers.

### Phase 7: Archive duplicates

- Migrate useful `moodle-bridge` ideas.
- Archive `moodle-bridge`.
- Confirm `moodle-clients/apps/extension` supersedes `moodle-custom-ui`.
- Archive `moodle-custom-ui`.
- Archive/delete `llm-poc` if no deployment depends on it.

## First Work Items

Recommended first implementation sequence:

1. Rename project documentation from Moodle CLI to Moodle Services.
2. Create `migrations/001_initial_auth.sql`.
3. Extract `internal/store` and `internal/auth`.
4. Extract `internal/moodleservice`.
5. Add QR exchange REST endpoint.
6. Add API-key management endpoints.
7. Add OpenAPI skeleton and `/api/openapi.json`.
8. Add `moodle-clients/packages/api-client` generation.

## Non-Goals For Now

- Do not introduce microservices.
- Do not create a second TypeScript backend with Moodle access.
- Do not introduce a heavy ORM.
- Do not remove the `moodle` CLI command name yet.
- Do not migrate all clients in one large change.

## References

- OpenAPI TypeScript: https://openapi-ts.dev/introduction
- openapi-fetch: https://openapi-ts.dev/openapi-fetch/
- Hey API OpenAPI TS: https://heyapi.dev/openapi-ts/
- Neon connection pooling: https://neon.com/docs/connect/connection-pooling
- Neon scale to zero: https://neon.com/docs/introduction/scale-to-zero
