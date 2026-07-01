# msmgr

`msmgr` is a small Go CLI for managing Meilisearch and related local workflows. It can check service health, manage indexes and documents, run search queries, and migrate document IDs.

## Commands

### Top-level

| Command | Description |
| --- | --- |
| `msmgr hello` | Print a simple startup message. |
| `msmgr health` | Check Meilisearch health and print the status. |
| `msmgr search <query> [--index uid] [--limit n]` | Search one index or all indexes for a query. |
| `msmgr indexes ...` | Manage Meilisearch indexes. |
| `msmgr documents ...` | Manage documents inside an index. |
| `msmgr help` | Print the built-in usage text. |

### `indexes`

| Command | Description |
| --- | --- |
| `msmgr indexes create <uid> [primaryKey]` | Create an index, optionally with a primary key. |
| `msmgr indexes delete <uid>` | Delete an index. |
| `msmgr indexes get <uid>` | Fetch a single index. |
| `msmgr indexes list` | List indexes. |

### `documents`

| Command | Description |
| --- | --- |
| `msmgr documents create <index> <path> [--wait]` | Create a document from a file and optionally wait for the task. |
| `msmgr documents get <index> <id>` | Fetch a single document by ID. |
| `msmgr documents migrate-ids <index> [--apply]` | Build or apply a document ID migration plan. |
| `msmgr documents delete <index> <id>` | Delete a document by ID. |
| `msmgr documents list <index>` | List documents in an index. |

## Build and Test

```sh
make fmt
make test
make build
```

## Configuration

- Meilisearch settings come from `msmgr.json` or `MSMGR_CONFIG`, with `MEILI_HTTP_ADDR` and `MEILI_API_KEY` as environment overrides.
- Keep real API keys out of version control and use `msmgr.example.json` as the starting point for local configuration.

## Repository Layout

- `cmd/msmgr/main.go` is the executable entry point.
- `internal/cli/` wires command-line behavior.
- `internal/meili/` contains Meilisearch API logic.
- `scripts/test.sh` wraps the Go test run for CI or manual verification.
