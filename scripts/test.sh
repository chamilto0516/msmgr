#!/usr/bin/env bash

set -euo pipefail

mkdir -p .cache/go-build
GOCACHE="$(pwd)/.cache/go-build" go test ./...
