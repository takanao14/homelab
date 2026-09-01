#!/usr/bin/env python3
"""Validate ADR identifiers, lifecycle metadata, index entries, and links."""

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
ADR_DIR = ROOT / "docs" / "adr"
INDEX_PATH = ADR_DIR / "README.md"

FILE_RE = re.compile(r"^(\d{4})-[a-z0-9][a-z0-9-]*\.md$")
HEADING_RE = re.compile(r"^# ADR-(\d{4}): (.+)$", re.MULTILINE)
STATUS_RE = re.compile(r"^- \*\*Status:\*\* (.+)$", re.MULTILINE)
SUPERSEDED_RE = re.compile(
    r"Superseded by \[ADR-(\d{4})\]\(([^)]+\.md)\)"
)
INDEX_ROW_RE = re.compile(
    r"^\| \[(\d{4})\]\(([^)]+\.md)\) \| (.+) \| (.+) \|$",
    re.MULTILINE,
)
LINK_RE = re.compile(r"\[[^\]]+\]\(([^)]+)\)")
SIMPLE_STATUSES = {"Proposed", "Accepted", "Deprecated", "Rejected"}


def normalized_status(status: str) -> str | None:
    if status in SIMPLE_STATUSES:
        return status
    match = SUPERSEDED_RE.fullmatch(status)
    if match:
        return f"Superseded by ADR-{match.group(1)}"
    return None


def validate_relative_links(path: Path, text: str, errors: list[str]) -> None:
    for raw_target in LINK_RE.findall(text):
        target = raw_target.strip().strip("<>")
        if target.startswith(("#", "/", "http://", "https://", "mailto:")):
            continue
        target = target.split("#", 1)[0]
        if not target:
            continue
        resolved = (path.parent / target).resolve()
        if not resolved.exists():
            errors.append(f"{path.relative_to(ROOT)}: broken link: {raw_target}")


def main() -> int:
    errors: list[str] = []
    records: dict[str, tuple[str, str, str]] = {}
    ids: dict[str, str] = {}

    for path in sorted(ADR_DIR.glob("[0-9][0-9][0-9][0-9]-*.md")):
        relative = str(path.relative_to(ADR_DIR))
        file_match = FILE_RE.fullmatch(path.name)
        if not file_match:
            errors.append(f"{relative}: invalid ADR filename")
            continue
        file_id = file_match.group(1)
        if file_id in ids:
            errors.append(f"duplicate ADR-{file_id}: {ids[file_id]} and {relative}")
        ids[file_id] = relative

        text = path.read_text(encoding="utf-8")
        heading_match = HEADING_RE.search(text)
        if not heading_match:
            errors.append(f"{relative}: missing ADR heading")
            continue
        heading_id, title = heading_match.groups()
        if heading_id != file_id:
            errors.append(
                f"{relative}: heading ADR-{heading_id} does not match filename"
            )

        status_matches = STATUS_RE.findall(text)
        if len(status_matches) != 1:
            errors.append(f"{relative}: expected exactly one Status field")
            continue
        status = status_matches[0]
        display_status = normalized_status(status)
        if display_status is None:
            errors.append(f"{relative}: invalid Status value: {status}")
            continue

        if status == "Proposed" and not re.search(
            r"^- \*\*Review (?:by|trigger):\*\* ", text, re.MULTILINE
        ):
            errors.append(f"{relative}: Proposed ADR needs Review by/trigger")

        superseded_match = SUPERSEDED_RE.fullmatch(status)
        if superseded_match:
            target = ADR_DIR / superseded_match.group(2)
            if not target.is_file():
                errors.append(f"{relative}: superseding ADR link does not exist")

        validate_relative_links(path, text, errors)
        records[relative] = (file_id, title, display_status)

    if ids:
        expected_ids = {f"{number:04d}" for number in range(1, max(map(int, ids)) + 1)}
        for missing_id in sorted(expected_ids - set(ids)):
            errors.append(f"missing ADR-{missing_id}; ADR identifiers must not be reused")

    index_text = INDEX_PATH.read_text(encoding="utf-8")
    index_records: dict[str, tuple[str, str, str]] = {}
    for adr_id, filename, title, status in INDEX_ROW_RE.findall(index_text):
        if filename in index_records:
            errors.append(f"index contains duplicate entry for {filename}")
        index_records[filename] = (adr_id, title, status)

    for filename in sorted(set(records) - set(index_records)):
        errors.append(f"index is missing {filename}")
    for filename in sorted(set(index_records) - set(records)):
        errors.append(f"index references unknown ADR file {filename}")
    for filename in sorted(set(records) & set(index_records)):
        if records[filename] != index_records[filename]:
            errors.append(
                f"index metadata differs for {filename}: "
                f"file={records[filename]!r} index={index_records[filename]!r}"
            )

    validate_relative_links(INDEX_PATH, index_text, errors)

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print(f"Validated {len(records)} ADRs.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
