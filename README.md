# Markdown Splitter

`msmgr split-markdown` splits Markdown files into smaller heading-based chunks for indexing workflows such as MeiliSearch.

## CLI Shape

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

## What it does

- Parses heading structure from `.md` files or directories of `.md` files
- Splits primarily at `##` by default
- Recursively splits large sections at deeper headings up to `###` by default
- Preserves heading ancestry in a JSONL manifest
- Names chunk files as `MONIKER_descriptor_words.md`
- Supports an OpenAI-compatible `chat/completions` endpoint for moniker and descriptor generation
- Falls back to deterministic names if the LLM call fails

## Example

```sh
./bin/msmgr split-markdown test/input/bill_hamilton_linkedin_profile_notes.md --output-dir test/output --use-llm
```

Preview chunk boundaries without writing files:

```sh
./bin/msmgr split-markdown test/input/bill_hamilton_linkedin_profile_notes.md --dry-run
```

Run it through the build target:

```sh
make split-markdown ARGS='test/input/bill_hamilton_linkedin_profile_notes.md --output-dir test/output --use-llm'
```

## Output

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

## Notes

- The splitter reads LLM settings from the same `msmgr.json` / `MSMGR_CONFIG` / `MSMGR_LLM_*` configuration used by the rest of `msmgr`.
- For production use, keep the LLM key in environment variables or another secret store.
- If you want the default output directory to stay out of version control, point `--output-dir` at a scratch path such as `test/output`.
