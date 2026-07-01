#!/usr/bin/env python3
"""
Meilisearch File Ingestion Service
===================================
Scans index subfolders under send_to_meilisearch_Index/ for .txt and .md files,
uploads each file as a document to the corresponding Meilisearch index, and
moves the file to archive/<index>/ on success or error/<index>/ on failure.

Usage:
    python3 ingest_to_meilisearch.py              # normal run
    python3 ingest_to_meilisearch.py --dry-run     # preview without changes

Designed to run via cron (e.g. every hour) with zero external dependencies.
"""

import os
import sys
import json
import time
import shutil
import logging
import urllib.request
import urllib.error
from datetime import datetime
from pathlib import Path

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
BASE_DIR = Path(__file__).resolve().parent
MEILI_URL = os.environ.get("MEILI_HTTP_ADDR", "http://localhost:7700")
MEILI_API_KEY = os.environ.get("MEILI_MASTER_KEY", "")  # empty = no auth
SUPPORTED_EXTENSIONS = {".txt", ".md"}
LOG_FILE = BASE_DIR / "ingestion.log"
TASK_POLL_INTERVAL = 1.0   # seconds between task-status polls
TASK_TIMEOUT = 300          # max seconds to wait for a single task
RESERVED_DIRS = {"archive", "error"}  # skip these when scanning for index dirs

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
logger = logging.getLogger("meilisearch-ingest")
logger.setLevel(logging.DEBUG)

# File handler — append, with timestamps
_fh = logging.FileHandler(LOG_FILE, encoding="utf-8")
_fh.setLevel(logging.DEBUG)
_fh.setFormatter(logging.Formatter(
    "%(asctime)s  %(levelname)-8s  %(message)s", datefmt="%Y-%m-%d %H:%M:%S"
))
logger.addHandler(_fh)

# Console handler — only INFO and above
_ch = logging.StreamHandler(sys.stdout)
_ch.setLevel(logging.INFO)
_ch.setFormatter(logging.Formatter("%(levelname)-8s  %(message)s"))
logger.addHandler(_ch)

# ---------------------------------------------------------------------------
# HTTP helpers
# ---------------------------------------------------------------------------

def _build_headers():
    """Return common HTTP headers, including auth if configured."""
    headers = {"Content-Type": "application/json"}
    if MEILI_API_KEY:
        headers["Authorization"] = f"Bearer {MEILI_API_KEY}"
    return headers


def meili_request(path, method="GET", body=None):
    """
    Send an HTTP request to Meilisearch and return the parsed JSON response.
    Raises on HTTP or connection errors.
    """
    url = f"{MEILI_URL}{path}"
    data = json.dumps(body).encode("utf-8") if body is not None else None
    req = urllib.request.Request(url, data=data, headers=_build_headers(), method=method)

    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        error_body = exc.read().decode("utf-8", errors="replace")
        logger.error("HTTP %s %s → %s: %s", method, path, exc.code, error_body)
        raise
    except urllib.error.URLError as exc:
        logger.error("Connection error %s %s → %s", method, path, exc.reason)
        raise


def wait_for_task(task_uid):
    """
    Poll a Meilisearch async task until it resolves.
    Returns the final task dict.  Raises on failure or timeout.
    """
    deadline = time.monotonic() + TASK_TIMEOUT
    while True:
        task = meili_request(f"/tasks/{task_uid}")
        status = task.get("status")
        if status == "succeeded":
            logger.debug("Task %s succeeded.", task_uid)
            return task
        if status == "failed":
            error = task.get("error", {})
            raise RuntimeError(
                f"Task {task_uid} failed: {error.get('message', 'unknown error')}"
            )
        if time.monotonic() > deadline:
            raise TimeoutError(f"Task {task_uid} timed out after {TASK_TIMEOUT}s")
        time.sleep(TASK_POLL_INTERVAL)

# ---------------------------------------------------------------------------
# Index discovery
# ---------------------------------------------------------------------------

def get_meilisearch_indexes():
    """Return a set of index UIDs currently defined in Meilisearch."""
    try:
        resp = meili_request("/indexes?limit=100")
        return {idx["uid"] for idx in resp.get("results", [])}
    except Exception:
        logger.exception("Failed to fetch index list from Meilisearch.")
        return set()


def discover_index_folders():
    """
    Return a list of (folder_path, index_uid) tuples for subdirectories of
    BASE_DIR that are not reserved names (archive, error).
    """
    folders = []
    for entry in sorted(BASE_DIR.iterdir()):
        if entry.is_dir() and entry.name not in RESERVED_DIRS:
            folders.append((entry, entry.name))
    return folders

# ---------------------------------------------------------------------------
# Document helpers
# ---------------------------------------------------------------------------

def slugify(filename):
    """Convert a filename (without extension) into a safe document ID."""
    stem = Path(filename).stem
    return "".join(
        c if c.isalnum() or c in ("-", "_") else "_" for c in stem
    ).strip("_").lower()


def read_file_content(filepath):
    """Read a text file and return its contents."""
    with open(filepath, "r", encoding="utf-8") as fh:
        return fh.read()

# ---------------------------------------------------------------------------
# File movement
# ---------------------------------------------------------------------------

def move_file(src, dest_dir):
    """
    Move *src* into *dest_dir*.  If a file with the same name already exists,
    append a timestamp to avoid overwriting.
    """
    dest_dir.mkdir(parents=True, exist_ok=True)
    dest = dest_dir / src.name
    if dest.exists():
        ts = datetime.now().strftime("%Y%m%d_%H%M%S")
        stem = src.stem
        suffix = src.suffix
        dest = dest_dir / f"{stem}_{ts}{suffix}"
    shutil.move(str(src), str(dest))
    logger.debug("Moved %s → %s", src.name, dest)
    return dest

# ---------------------------------------------------------------------------
# Core processing
# ---------------------------------------------------------------------------

def process_file(filepath, index_uid, dry_run=False):
    """
    Upload a single file to the given Meilisearch index.
    Returns True on success, False on failure.
    """
    filename = filepath.name
    doc_id = slugify(filename)
    logger.info("Processing: %s → index '%s' (id=%s)", filename, index_uid, doc_id)

    if dry_run:
        logger.info("  [DRY-RUN] Would upload %s to %s", filename, index_uid)
        return True

    try:
        content = read_file_content(filepath)
    except Exception:
        logger.exception("  Failed to read file: %s", filepath)
        return False

    document = {
        "id": doc_id,
        "title": filepath.stem,
        "path": f"{index_uid}/{filename}",
        "content": content,
    }

    try:
        resp = meili_request(
            f"/indexes/{index_uid}/documents", method="POST", body=[document]
        )
        task_uid = resp.get("taskUid")
        if task_uid is None:
            logger.error("  No taskUid returned: %s", resp)
            return False
        wait_for_task(task_uid)
        logger.info("  ✓ Uploaded successfully (task %s).", task_uid)
        return True
    except Exception:
        logger.exception("  ✗ Upload failed for %s", filename)
        return False

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    dry_run = "--dry-run" in sys.argv

    logger.info("=" * 60)
    logger.info("Meilisearch ingestion run started%s", " [DRY-RUN]" if dry_run else "")
    logger.info("Base directory : %s", BASE_DIR)
    logger.info("Meilisearch URL: %s", MEILI_URL)

    # Discover what Meilisearch knows about
    known_indexes = get_meilisearch_indexes()
    if not known_indexes:
        logger.warning("No indexes found in Meilisearch (or server unreachable). Aborting.")
        return

    logger.info("Known indexes  : %s", ", ".join(sorted(known_indexes)))

    # Discover local index folders
    index_folders = discover_index_folders()
    if not index_folders:
        logger.info("No index folders found. Nothing to do.")
        return

    total_processed = 0
    total_success = 0
    total_error = 0

    for folder, index_uid in index_folders:
        if index_uid not in known_indexes:
            logger.warning(
                "Folder '%s' does not match any Meilisearch index — skipping.",
                index_uid,
            )
            continue

        # Collect eligible files
        files = sorted(
            f for f in folder.iterdir()
            if f.is_file() and f.suffix.lower() in SUPPORTED_EXTENSIONS
        )

        if not files:
            logger.debug("No eligible files in %s/", index_uid)
            continue

        logger.info("Found %d file(s) in %s/", len(files), index_uid)

        for filepath in files:
            total_processed += 1
            success = process_file(filepath, index_uid, dry_run=dry_run)

            if dry_run:
                total_success += 1
                continue

            if success:
                total_success += 1
                archive_dir = BASE_DIR / "archive" / index_uid
                move_file(filepath, archive_dir)
            else:
                total_error += 1
                error_dir = BASE_DIR / "error" / index_uid
                move_file(filepath, error_dir)

    logger.info("-" * 40)
    logger.info(
        "Run complete: %d processed, %d succeeded, %d failed.",
        total_processed, total_success, total_error,
    )
    logger.info("=" * 60)


if __name__ == "__main__":
    main()
