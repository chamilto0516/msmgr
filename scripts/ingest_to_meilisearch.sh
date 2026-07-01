#!/usr/bin/env bash

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BASE_DIR="${BASE_DIR:-$SCRIPT_DIR}"
MSMGR_DIR="${MSMGR_DIR:-$(cd "$SCRIPT_DIR/../msmgr" 2>/dev/null && pwd)}"
MSMGR_BIN="${MSMGR_BIN:-$MSMGR_DIR/bin/msmgr}"
LOG_FILE="${LOG_FILE:-$BASE_DIR/ingestion.log}"

SUPPORTED_EXTENSIONS=("txt" "md")
RESERVED_DIRS=("archive" "error")

DRY_RUN=0
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=1
fi

timestamp() {
  date +"%Y-%m-%d %H:%M:%S"
}

log() {
  local level="$1"
  shift
  local message="$*"
  printf "%s  %-8s  %s\n" "$(timestamp)" "$level" "$message" | tee -a "$LOG_FILE"
}

is_reserved_dir() {
  local name="$1"
  local reserved
  for reserved in "${RESERVED_DIRS[@]}"; do
    if [[ "$name" == "$reserved" ]]; then
      return 0
    fi
  done
  return 1
}

is_supported_file() {
  local path="$1"
  local ext="${path##*.}"
  ext="${ext,,}"
  local supported
  for supported in "${SUPPORTED_EXTENSIONS[@]}"; do
    if [[ "$ext" == "$supported" ]]; then
      return 0
    fi
  done
  return 1
}

move_file() {
  local src="$1"
  local dest_dir="$2"
  mkdir -p "$dest_dir"

  local base_name
  base_name="$(basename "$src")"
  local dest="$dest_dir/$base_name"

  if [[ -e "$dest" ]]; then
    local stem="${base_name%.*}"
    local ext=""
    if [[ "$base_name" == *.* ]]; then
      ext=".${base_name##*.}"
    fi
    dest="$dest_dir/${stem}_$(date +%Y%m%d_%H%M%S)$ext"
  fi

  mv "$src" "$dest"
  log "DEBUG" "Moved $(basename "$src") -> $dest"
}

require_msmgr() {
  if [[ -z "$MSMGR_DIR" || ! -x "$MSMGR_BIN" ]]; then
    log "ERROR" "msmgr binary not found or not executable at $MSMGR_BIN"
    exit 1
  fi
}

index_exists() {
  "$MSMGR_BIN" indexes get "$1" >/dev/null 2>&1
}

create_document() {
  local index_uid="$1"
  local file_path="$2"
  "$MSMGR_BIN" documents create "$index_uid" "$file_path" --wait
}

main() {
  require_msmgr
  mkdir -p "$BASE_DIR/archive" "$BASE_DIR/error"

  log "INFO" "============================================================"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    log "INFO" "Meilisearch ingestion run started [DRY-RUN]"
  else
    log "INFO" "Meilisearch ingestion run started"
  fi
  log "INFO" "Base directory : $BASE_DIR"
  log "INFO" "msmgr binary   : $MSMGR_BIN"

  local total_processed=0
  local total_success=0
  local total_error=0

  local folder
  for folder in "$BASE_DIR"/*; do
    [[ -d "$folder" ]] || continue

    local index_uid
    index_uid="$(basename "$folder")"
    if is_reserved_dir "$index_uid"; then
      continue
    fi

    if ! index_exists "$index_uid"; then
      log "WARN" "Folder '$index_uid' does not match any Meilisearch index, skipping."
      continue
    fi

    shopt -s nullglob
    local files=()
    local candidate
    for candidate in "$folder"/*; do
      [[ -f "$candidate" ]] || continue
      if is_supported_file "$candidate"; then
        files+=("$candidate")
      fi
    done
    shopt -u nullglob

    if [[ "${#files[@]}" -eq 0 ]]; then
      continue
    fi

    log "INFO" "Found ${#files[@]} file(s) in ${index_uid}/"

    local file
    for file in "${files[@]}"; do
      total_processed=$((total_processed + 1))
      log "INFO" "Processing: $(basename "$file") -> index '$index_uid'"

      if [[ "$DRY_RUN" -eq 1 ]]; then
        log "INFO" "[DRY-RUN] Would upload $(basename "$file") to $index_uid"
        total_success=$((total_success + 1))
        continue
      fi

      local output
      if output="$(create_document "$index_uid" "$file" 2>&1)"; then
        log "INFO" "$output"
        move_file "$file" "$BASE_DIR/archive/$index_uid"
        total_success=$((total_success + 1))
      else
        log "ERROR" "$output"
        move_file "$file" "$BASE_DIR/error/$index_uid"
        total_error=$((total_error + 1))
      fi
    done
  done

  log "INFO" "----------------------------------------"
  log "INFO" "Run complete: $total_processed processed, $total_success succeeded, $total_error failed."
  log "INFO" "============================================================"
}

main "$@"
