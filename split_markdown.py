from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import sys
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any
from urllib import error, request


HEADING_RE = re.compile(r"^(#{1,6})\s+(.*)$")
NON_ALNUM_RE = re.compile(r"[^A-Za-z0-9]+")
WORD_RE = re.compile(r"[A-Za-z0-9]+")


@dataclass
class SectionNode:
    level: int
    title: str
    start_line: int
    parent: SectionNode | None = None
    children: list["SectionNode"] = field(default_factory=list)
    content_lines: list[str] = field(default_factory=list)

    def heading_text(self) -> str:
        return f"{'#' * self.level} {self.title}".rstrip()

    def path_titles(self) -> list[str]:
        titles: list[str] = []
        node: SectionNode | None = self
        while node and node.level > 0:
            titles.append(node.title)
            node = node.parent
        return list(reversed(titles))

    def own_body_text(self) -> str:
        if not self.content_lines:
            return ""
        return "\n".join(self.content_lines).strip()

    def subtree_markdown(self) -> str:
        parts: list[str] = []
        if self.level > 0:
            parts.append(self.heading_text())
        if self.content_lines:
            body = "\n".join(self.content_lines).rstrip()
            if body:
                parts.append(body)
        for child in self.children:
            child_md = child.subtree_markdown().rstrip()
            if child_md:
                parts.append(child_md)
        return "\n\n".join(part for part in parts if part).strip()


@dataclass
class Chunk:
    source_file: str
    document_title: str
    heading_path: list[str]
    chunk_index: int
    chunk_text: str
    section_level: int
    output_filename: str = ""
    chunk_id: str = ""


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Split Markdown files into smaller heading-based chunks."
    )
    parser.add_argument("input_path", type=Path, help="Markdown file or directory.")
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=Path("test_file/output"),
        help="Directory for chunk files and manifest.",
    )
    parser.add_argument(
        "--manifest",
        type=Path,
        default=None,
        help="Override manifest path. Defaults to <output-dir>/manifest.jsonl.",
    )
    parser.add_argument(
        "--split-level",
        type=int,
        default=2,
        choices=range(1, 7),
        help="Base heading level to start chunking from.",
    )
    parser.add_argument(
        "--max-heading-level",
        type=int,
        default=3,
        choices=range(1, 7),
        help="Deepest heading level allowed for recursive splitting.",
    )
    parser.add_argument(
        "--min-chars",
        type=int,
        default=220,
        help="Merge smaller fragments into their parent until this size when possible.",
    )
    parser.add_argument(
        "--max-chars",
        type=int,
        default=1800,
        help="Split large sections when child headings are available above this size.",
    )
    parser.add_argument(
        "--use-llm",
        action="store_true",
        help="Use the OpenAI-compatible endpoint for moniker and descriptor naming.",
    )
    parser.add_argument(
        "--llm-config",
        type=Path,
        default=Path("LLM_Connection.txt"),
        help="Path to connection details for the OpenAI-compatible endpoint.",
    )
    parser.add_argument(
        "--model",
        default=None,
        help="Override model name from the config file.",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print planned chunks without writing files.",
    )
    return parser.parse_args()


def discover_markdown_files(input_path: Path) -> list[Path]:
    if input_path.is_file():
        return [input_path]
    if input_path.is_dir():
        return sorted(path for path in input_path.rglob("*.md") if path.is_file())
    raise FileNotFoundError(f"Input path not found: {input_path}")


def parse_markdown_sections(text: str) -> SectionNode:
    root = SectionNode(level=0, title="", start_line=0)
    stack: list[SectionNode] = [root]

    for line_number, line in enumerate(text.splitlines(), start=1):
        match = HEADING_RE.match(line)
        if match:
            level = len(match.group(1))
            title = match.group(2).strip()
            while stack and stack[-1].level >= level:
                stack.pop()
            parent = stack[-1]
            node = SectionNode(level=level, title=title, start_line=line_number, parent=parent)
            parent.children.append(node)
            stack.append(node)
            continue

        stack[-1].content_lines.append(line)

    return root


def build_chunk_text(node: SectionNode, intro_body: str | None = None) -> str:
    parts = [f"{'#' * node.level} {node.title}"]
    body = node.own_body_text() if intro_body is None else intro_body.strip()
    if body:
        parts.append(body)
    return "\n\n".join(parts).strip()


def should_split_node(node: SectionNode, max_heading_level: int, max_chars: int) -> bool:
    if not node.children or node.level >= max_heading_level:
        return False
    subtree_len = len(node.subtree_markdown())
    return subtree_len > max_chars or len(node.children) > 1


def split_node(
    node: SectionNode,
    *,
    source_file: str,
    document_title: str,
    max_heading_level: int,
    min_chars: int,
    max_chars: int,
) -> list[Chunk]:
    subtree_text = node.subtree_markdown()
    if not should_split_node(node, max_heading_level, max_chars):
        return [
            Chunk(
                source_file=source_file,
                document_title=document_title,
                heading_path=node.path_titles(),
                chunk_index=0,
                chunk_text=subtree_text,
                section_level=node.level,
            )
        ]

    chunks: list[Chunk] = []
    intro_body = node.own_body_text()
    if intro_body and len(intro_body) >= min_chars:
        chunks.append(
            Chunk(
                source_file=source_file,
                document_title=document_title,
                heading_path=node.path_titles(),
                chunk_index=0,
                chunk_text=build_chunk_text(node, intro_body),
                section_level=node.level,
            )
        )

    for child in node.children:
        chunks.extend(
            split_node(
                child,
                source_file=source_file,
                document_title=document_title,
                max_heading_level=max_heading_level,
                min_chars=min_chars,
                max_chars=max_chars,
            )
        )

    if not chunks:
        chunks.append(
            Chunk(
                source_file=source_file,
                document_title=document_title,
                heading_path=node.path_titles(),
                chunk_index=0,
                chunk_text=subtree_text,
                section_level=node.level,
            )
        )
    return chunks


def build_chunks(
    markdown_path: Path,
    *,
    split_level: int,
    max_heading_level: int,
    min_chars: int,
    max_chars: int,
) -> tuple[str, list[Chunk], str]:
    text = markdown_path.read_text(encoding="utf-8")
    root = parse_markdown_sections(text)
    document_title = markdown_path.stem
    h1_node: SectionNode | None = None
    if root.children and root.children[0].level == 1:
        h1_node = root.children[0]
        document_title = h1_node.title

    target_nodes: list[SectionNode] = []

    def walk(node: SectionNode) -> None:
        if node.level == split_level:
            target_nodes.append(node)
            return
        for child in node.children:
            walk(child)

    walk(root)
    if not target_nodes:
        target_nodes = [child for child in root.children] or [root]

    chunks: list[Chunk] = []
    if h1_node is not None and split_level > 1:
        h1_intro = h1_node.own_body_text()
        if h1_intro:
            chunks.append(
                Chunk(
                    source_file=markdown_path.name,
                    document_title=document_title,
                    heading_path=h1_node.path_titles(),
                    chunk_index=0,
                    chunk_text=build_chunk_text(h1_node, h1_intro),
                    section_level=h1_node.level,
                )
            )

    for node in target_nodes:
        chunks.extend(
            split_node(
                node,
                source_file=markdown_path.name,
                document_title=document_title,
                max_heading_level=max_heading_level,
                min_chars=min_chars,
                max_chars=max_chars,
            )
        )

    for index, chunk in enumerate(chunks, start=1):
        chunk.chunk_index = index
        chunk.chunk_id = stable_chunk_id(markdown_path.name, chunk.heading_path, index)

    return text, chunks, document_title


def stable_chunk_id(source_file: str, heading_path: list[str], index: int) -> str:
    raw = f"{source_file}|{' > '.join(heading_path)}|{index}"
    return hashlib.sha1(raw.encode("utf-8")).hexdigest()[:16]


def slug_words(value: str, max_words: int = 8) -> str:
    words = WORD_RE.findall(value.lower())
    if not words:
        return "section"
    return "_".join(words[:max_words])


def sanitize_moniker(value: str) -> str:
    cleaned = NON_ALNUM_RE.sub("", value).upper()
    if len(cleaned) >= 10:
        return cleaned[:10]
    digest = hashlib.sha1(value.encode("utf-8")).hexdigest().upper()
    return (cleaned + digest)[:10]


def fallback_moniker(filename: str, full_text: str) -> str:
    stem = NON_ALNUM_RE.sub("", Path(filename).stem).upper()
    digest = hashlib.sha1(f"{filename}\n{full_text}".encode("utf-8")).hexdigest().upper()
    return (stem + digest)[:10]


def fallback_descriptor(chunk: Chunk) -> str:
    if len(chunk.heading_path) >= 3:
        source_text = " ".join(chunk.heading_path[-2:])
    else:
        source_text = chunk.heading_path[-1]
    return slug_words(source_text, max_words=8)


def unique_filename(base_name: str, seen: dict[str, int]) -> str:
    count = seen.get(base_name, 0) + 1
    seen[base_name] = count
    if count == 1:
        return f"{base_name}.md"
    return f"{base_name}_{count:02d}.md"


def parse_llm_config(config_path: Path) -> dict[str, str]:
    config: dict[str, str] = {}
    for line in config_path.read_text(encoding="utf-8").splitlines():
        if ":" not in line:
            continue
        key, value = line.split(":", 1)
        config[key.strip().lower()] = value.strip()
    return config


class OpenAICompatibleClient:
    def __init__(self, base_url: str, api_key: str, model: str, timeout: int = 60) -> None:
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.model = model
        self.timeout = timeout

    def chat_json(self, messages: list[dict[str, str]], schema_name: str) -> dict[str, Any]:
        payload = {
            "model": self.model,
            "messages": messages,
            "temperature": 0.2,
            "response_format": {
                "type": "json_schema",
                "json_schema": {
                    "name": schema_name,
                    "schema": response_schema(schema_name),
                },
            },
        }
        data = json.dumps(payload).encode("utf-8")
        req = request.Request(
            url=f"{self.base_url}/chat/completions",
            data=data,
            method="POST",
            headers={
                "Authorization": f"Bearer {self.api_key}",
                "Content-Type": "application/json",
            },
        )
        try:
            with request.urlopen(req, timeout=self.timeout) as response:
                parsed = json.loads(response.read().decode("utf-8"))
        except error.HTTPError as exc:
            detail = exc.read().decode("utf-8", errors="replace")
            raise RuntimeError(f"LLM request failed with HTTP {exc.code}: {detail}") from exc
        except error.URLError as exc:
            raise RuntimeError(f"LLM request failed: {exc.reason}") from exc

        try:
            content = parsed["choices"][0]["message"]["content"]
        except (KeyError, IndexError, TypeError) as exc:
            raise RuntimeError(f"Unexpected LLM response: {parsed}") from exc

        if isinstance(content, list):
            text_parts = [part.get("text", "") for part in content if isinstance(part, dict)]
            content = "".join(text_parts)
        return json.loads(content)


def response_schema(schema_name: str) -> dict[str, Any]:
    if schema_name == "document_moniker":
        return {
            "type": "object",
            "properties": {
                "moniker": {
                    "type": "string",
                    "description": "Exactly 10 uppercase alphanumeric characters.",
                    "pattern": "^[A-Z0-9]{10}$",
                }
            },
            "required": ["moniker"],
            "additionalProperties": False,
        }

    return {
        "type": "object",
        "properties": {
            "descriptor": {
                "type": "string",
                "description": "Up to 8 lowercase underscore-separated descriptive words.",
                "pattern": "^[a-z0-9]+(?:_[a-z0-9]+){0,7}$",
            }
        },
        "required": ["descriptor"],
        "additionalProperties": False,
    }


def build_client(args: argparse.Namespace) -> OpenAICompatibleClient | None:
    if not args.use_llm:
        return None

    config = parse_llm_config(args.llm_config)
    api_key = config.get("llm key") or os.environ.get("OPENAI_API_KEY")
    base_url = config.get("host") or os.environ.get("OPENAI_BASE_URL")
    model = args.model or config.get("model name") or os.environ.get("OPENAI_MODEL")

    if not api_key or not base_url or not model:
        raise RuntimeError("Missing LLM settings. Provide key, host, and model.")

    return OpenAICompatibleClient(base_url=base_url, api_key=api_key, model=model)


def generate_moniker(
    client: OpenAICompatibleClient | None,
    source_file: str,
    document_title: str,
    full_text: str,
) -> str:
    fallback = fallback_moniker(source_file, full_text)
    if client is None:
        return fallback

    prompt = (
        "Create a stable project moniker for this markdown document.\n"
        "Requirements:\n"
        "- Exactly 10 uppercase alphanumeric characters\n"
        "- No punctuation, spaces, or underscores\n"
        "- Must reflect the filename and overall document subject\n"
        "- Prefer pronounceable or mnemonic output when possible\n"
    )
    messages = [
        {"role": "system", "content": "You generate strict JSON only."},
        {
            "role": "user",
            "content": (
                f"{prompt}\nFilename: {source_file}\nDocument title: {document_title}\n\n"
                f"Document contents:\n{full_text[:12000]}"
            ),
        },
    ]
    try:
        result = client.chat_json(messages, "document_moniker")
        return sanitize_moniker(result["moniker"])
    except Exception:
        return fallback


def generate_descriptor(client: OpenAICompatibleClient | None, chunk: Chunk) -> str:
    fallback = fallback_descriptor(chunk)
    if client is None:
        return fallback

    messages = [
        {"role": "system", "content": "You generate strict JSON only."},
        {
            "role": "user",
            "content": (
                "Create a lowercase underscore-separated descriptor for this markdown chunk.\n"
                "Requirements:\n"
                "- Up to 8 words\n"
                "- Only lowercase letters and digits\n"
                "- Use underscores between words\n"
                "- Focus on what this section is about, not generic filler\n\n"
                f"Heading path: {' > '.join(chunk.heading_path)}\n"
                f"Chunk text:\n{chunk.chunk_text[:6000]}"
            ),
        },
    ]
    try:
        result = client.chat_json(messages, "chunk_descriptor")
        return slug_words(result["descriptor"], max_words=8)
    except Exception:
        return fallback


def prepare_output_records(
    chunks: list[Chunk],
    *,
    moniker: str,
    client: OpenAICompatibleClient | None,
    seen_names: dict[str, int],
) -> list[dict[str, Any]]:
    records: list[dict[str, Any]] = []
    for chunk in chunks:
        descriptor = generate_descriptor(client, chunk)
        base_name = f"{moniker}_{descriptor}"
        filename = unique_filename(base_name, seen_names)
        chunk.output_filename = filename
        records.append(
            {
                "id": chunk.chunk_id,
                "source_file": chunk.source_file,
                "document_title": chunk.document_title,
                "heading_path": chunk.heading_path,
                "section_level": chunk.section_level,
                "chunk_index": chunk.chunk_index,
                "output_filename": filename,
                "text": chunk.chunk_text,
            }
        )
    return records


def write_outputs(
    records: list[dict[str, Any]],
    *,
    output_dir: Path,
    manifest_path: Path,
    dry_run: bool,
) -> None:
    if not dry_run:
        output_dir.mkdir(parents=True, exist_ok=True)

    for record in records:
        chunk_path = output_dir / record["output_filename"]
        if dry_run:
            print(f"[DRY RUN] {record['output_filename']} :: {' > '.join(record['heading_path'])}")
            continue
        chunk_path.write_text(record["text"].strip() + "\n", encoding="utf-8")

    if dry_run:
        return

    with manifest_path.open("w", encoding="utf-8") as handle:
        for record in records:
            handle.write(json.dumps(record, ensure_ascii=True) + "\n")


def process_file(
    markdown_path: Path,
    args: argparse.Namespace,
    client: OpenAICompatibleClient | None,
    seen_names: dict[str, int],
) -> tuple[str, list[dict[str, Any]]]:
    full_text, chunks, document_title = build_chunks(
        markdown_path,
        split_level=args.split_level,
        max_heading_level=args.max_heading_level,
        min_chars=args.min_chars,
        max_chars=args.max_chars,
    )
    moniker = generate_moniker(client, markdown_path.name, document_title, full_text)
    records = prepare_output_records(
        chunks,
        moniker=moniker,
        client=client,
        seen_names=seen_names,
    )
    return moniker, records


def main() -> int:
    args = parse_args()

    try:
        client = build_client(args)
        markdown_files = discover_markdown_files(args.input_path)
        if not markdown_files:
            raise RuntimeError("No Markdown files found.")

        manifest_path = args.manifest or args.output_dir / "manifest.jsonl"
        all_records: list[dict[str, Any]] = []
        seen_names: dict[str, int] = {}

        for markdown_file in markdown_files:
            moniker, records = process_file(markdown_file, args, client, seen_names)
            all_records.extend(records)
            print(
                f"Processed {markdown_file.name}: {len(records)} chunks, moniker={moniker}, "
                f"output_dir={args.output_dir}"
            )
            if client is not None:
                time.sleep(0.2)

        write_outputs(
            all_records,
            output_dir=args.output_dir,
            manifest_path=manifest_path,
            dry_run=args.dry_run,
        )
        return 0
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
