# MeiliSearch Manager CLI Plan

## Phase 1

Establish the CLI skeleton, Go module, build flow, and test harness using only the Go standard library.

## Phase 2

Add shared configuration and HTTP client code:

- Read `MEILI_HTTP_ADDR` and `MEILI_API_KEY`.
- Centralize request creation, JSON encoding, response decoding, and error handling.
- Add table-driven unit tests with `net/http/httptest`.

## Phase 3

Implement index CRUD commands:

- `indexes create`
- `indexes list`
- `indexes get`
- `indexes update`
- `indexes delete`

## Phase 4

Implement document CRUD commands:

- `documents add`
- `documents get`
- `documents update`
- `documents delete`
- `documents list`

## Phase 5

Implement operational commands:

- `health`
- `search`
- `version`
- `stats`

## Health Check Investigation Tasks

- Start with `GET /health` to verify liveness.
- Evaluate `GET /version` for quick instance identification in diagnostics.
- Evaluate `GET /stats` for database size, document totals, and index counts.
- Inspect per-index stats to surface document counts and storage growth.
- Track task backlog and failures to detect unhealthy write pipelines.
- Decide whether a composite health command should return non-zero exit codes on degraded states.
