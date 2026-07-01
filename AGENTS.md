# Repository Guidelines

## Project Structure & Module Organization

This repository is a small Go CLI for managing Meilisearch. Keep the executable entry point in `cmd/msmgr/main.go`, command wiring in `internal/cli/`, and operational notes in root files such as `meilisearch_startup_info.txt`. Build output belongs in `bin/`, and the local Go build cache stays in `.cache/go-build/`.

Add new packages under `internal/` by responsibility, not by API endpoint count. For example, future Meilisearch HTTP logic should live in a focused package such as `internal/meili/`.

## Build, Test, and Development Commands

Use the checked-in commands instead of ad hoc shell history:

```sh
make fmt    # run gofmt on cmd/ and internal/
make test   # run go test ./... with local cache
make build  # compile ./cmd/msmgr to ./bin/msmgr
./bin/msmgr hello
./bin/msmgr health
./bin/msmgr search "history of rome"
./bin/msmgr indexes create test id
./bin/msmgr indexes delete test
cp msmgr.example.json msmgr.json
./bin/msmgr indexes list
./bin/msmgr documents create test ./notes/example.md
./bin/msmgr documents migrate-ids test
./bin/msmgr documents migrate-ids test --apply
./bin/msmgr documents delete test doc1
./bin/msmgr documents delete-all test
./bin/msmgr documents list <index>
./bin/msmgr help
```

## Coding Style & Naming Conventions

Format Go code with `gofmt`; the repository already assumes standard Go formatting and tab indentation. Use exported `CamelCase` names only when a symbol must cross package boundaries, keep internal helpers in `camelCase`, and keep packages small and explicit. Prefer standard library packages unless a dependency clearly reduces complexity.

## Testing Guidelines

Use Go's built-in `testing` package and place tests beside the code they cover, as in `internal/cli/app_test.go`. Name tests by behavior, such as `TestRunHelp` or `TestRunRejectsUnknownCommand`. Cover both success paths and command errors, then run `make test` before proposing changes.

## Commit & Pull Request Guidelines

The repository now has usable Git history. Use short imperative subjects such as `Add health command`, `Wire Meilisearch client`, or `Add markdown splitter command`.

Pull requests should explain the user-visible change, list validation performed, and call out any new environment variables, API assumptions, or follow-up work.

## Security & Configuration Tips

Do not commit real Meilisearch keys or machine-specific endpoints. Keep values such as `MEILI_HTTP_ADDR` and any API key in environment variables, and use the startup notes only as local development guidance.
