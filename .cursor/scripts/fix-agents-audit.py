#!/usr/bin/env python3
"""Apply compatibility audit fixes to .cursor/agents/*.md."""

from __future__ import annotations

import re
from pathlib import Path

AGENTS_DIR = Path(__file__).resolve().parents[1] / "agents"

READONLY_AGENTS = {
    "code-explorer",
    "docs-lookup",
    "network-troubleshooter",
    "opensource-sanitizer",
}


def fix_agent(path: Path) -> bool:
    text = path.read_text(encoding="utf-8")
    if not text.startswith("---"):
        return False

    end = text.find("\n---", 3)
    if end == -1:
        return False

    front = text[3:end]
    body = text[end + 4 :]
    lines = []
    has_readonly = False

    for line in front.splitlines():
        if line.strip().startswith("color:"):
            continue
        if line.strip().startswith("readonly:"):
            has_readonly = True
        lines.append(line)

    agent_name = path.stem
    if agent_name in READONLY_AGENTS and not has_readonly:
        lines.append("readonly: true")

    new_front = "---\n" + "\n".join(lines) + "\n---"
    new_text = new_front + body
    if new_text != text:
        path.write_text(new_text, encoding="utf-8")
        return True
    return False


def main() -> None:
    changed = []
    for path in sorted(AGENTS_DIR.glob("*.md")):
        if fix_agent(path):
            changed.append(path.name)
    print(f"Updated {len(changed)} agent files")
    for name in changed:
        print(f"  - {name}")


if __name__ == "__main__":
    main()
