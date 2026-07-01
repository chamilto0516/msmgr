# msmgr

`msmgr` is a small Go CLI for managing Meilisearch and related local workflows. It can check service health, manage indexes and documents, run search queries, migrate document IDs, and split Markdown into Meilisearch-ready chunks.

## Commands

### Top-level

| Command | Description |
| --- | --- |
| `msmgr hello` | Print a simple startup message. |
| `msmgr health` | Check Meilisearch health and print the status. |
| `msmgr search <query> [--index uid] [--limit n]` | Search one index or all indexes for a query. |
| `msmgr indexes ...` | Manage Meilisearch indexes. |
| `msmgr documents ...` | Manage documents inside an index. |
| `msmgr split-markdown ...` | Split Markdown into smaller chunks and emit a JSONL manifest. |
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

### `split-markdown`

```sh
msmgr [--timeout seconds] split-markdown <input-path> \
  [--output-dir dir] \
  [--manifest path] \
  [--split-level n] \
  [--max-heading-level n] \
  [--min-chars n] \
  [--max-chars n] \
  [--use-llm] \
  [--dry-run]
```

Defaults:

- `--split-level 2`
- `--max-heading-level 3`
- `--min-chars 220`
- `--max-chars 1800`
- `--output-dir test_file/output`
- `--manifest <output-dir>/manifest.jsonl`

What it does:

- Parses heading structure from a Markdown file or a directory of Markdown files
- Splits primarily at `##` by default
- Recursively splits large sections at deeper headings up to `###` by default
- Preserves heading ancestry in a JSONL manifest
- Names chunk files as `MONIKER_descriptor_words.md`
- Uses the shared `msmgr.json` / `MSMGR_CONFIG` / `MSMGR_LLM_*` configuration path for LLM-backed naming
- Falls back to deterministic names if the LLM call fails

Example:

```sh
./bin/msmgr split-markdown path/to/input.md --output-dir test/output --use-llm
```

Preview chunk boundaries without writing files:

```sh
./bin/msmgr split-markdown path/to/input.md --dry-run
```

Output:

- Chunk files in the selected output directory
- `manifest.jsonl` with one JSON object per chunk, unless `--manifest` overrides it

Each manifest record includes:

- `id`
- `source_file`
- `document_title`
- `heading_path`
- `section_level`
- `chunk_index`
- `output_filename`
- `text`

## Build and Test

```sh
make fmt
make test
make build
```

## Configuration

- Meilisearch settings come from `msmgr.json` or `MSMGR_CONFIG`, with `MEILI_HTTP_ADDR` and `MEILI_API_KEY` as environment overrides.
- Keep real API keys out of version control and use `msmgr.example.json` as the starting point for local configuration.
- The splitter uses the same `msmgr.json` / `MSMGR_CONFIG` / `MSMGR_LLM_*` configuration path as the rest of `msmgr`.

## Repository Layout

- `cmd/msmgr/main.go` is the executable entry point.
- `internal/cli/` wires command-line behavior.
- `internal/meili/` contains Meilisearch API logic.
- `internal/llm/` contains the OpenAI-compatible client used by the splitter.
- `internal/splitmd/` contains the Markdown splitting implementation.
