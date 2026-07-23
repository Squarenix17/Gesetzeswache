#!/usr/bin/env python3
"""Migrate .cursor folder from ECC/Claude Code conventions to Cursor conventions."""

from __future__ import annotations

import os
import re
import shutil
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]  # .cursor/

READONLY_PATTERNS = (
    "reviewer",
    "analyzer",
    "hunter",
    "auditor",
    "evaluator",
    "spec-miner",
    "seo-specialist",
    "comment-analyzer",
    "silent-failure-hunter",
    "type-design-analyzer",
    "pr-test-analyzer",
)

SKIP_DIRS = {".git", "node_modules", "__pycache__"}


def should_skip(path: Path) -> bool:
    return any(part in SKIP_DIRS for part in path.parts)


def is_text_file(path: Path) -> bool:
    return path.suffix.lower() in {
        ".md",
        ".mdc",
        ".js",
        ".mjs",
        ".py",
        ".sh",
        ".json",
        ".yaml",
        ".yml",
        ".txt",
    }


def replace_paths(content: str) -> str:
    replacements = [
        ("~/.claude/agents/", "~/.cursor/agents/"),
        ("~/.claude/skills/", "~/.cursor/skills/"),
        ("~/.claude/rules/", "~/.cursor/rules/"),
        ("~/.claude/hooks/", "~/.cursor/hooks/"),
        ("~/.claude.json", "~/.cursor/cli-config.json"),
        ("~/.claude/settings.json", "~/.cursor/settings.json"),
        (".claude/agents/", ".cursor/agents/"),
        (".claude/skills/", ".cursor/skills/"),
        (".claude/rules/", ".cursor/rules/"),
        (".claude/hooks/", ".cursor/hooks/"),
        (".claude/commands/", ".cursor/commands/"),
        ("`~/.claude/`", "`~/.cursor/`"),
        ("~/.claude/", "~/.cursor/"),
        (".claude/", ".cursor/"),
    ]
    for old, new in replacements:
        content = content.replace(old, new)
    return content


def replace_harness_claude(content: str) -> str:
  """Replace Claude Code harness references with Cursor, preserving product names."""
  patterns = [
      (r"\bClaude Code\b", "Cursor"),
      (r"\bclaude code\b", "cursor"),
      (r"\bClaude Code sessions?\b", "Cursor sessions"),
      (r"for Claude Code", "for Cursor"),
      (r"in Claude Code", "in Cursor"),
      (r"via Claude Code", "via Cursor"),
      (r"Claude Code hook", "Cursor hook"),
      (r"CLAUDE\.md", "AGENTS.md"),
  ]
  for pattern, repl in patterns:
      content = re.sub(pattern, repl, content)
  return content


def replace_ecc_refs(content: str) -> str:
    content = re.sub(r"\becc-statusline\b", "statusline", content)
    content = re.sub(r"\becc-metrics-bridge\b", "metrics-bridge", content)
    content = re.sub(r"\becc-context-monitor\b", "context-monitor", content)
    content = re.sub(r"\becc-metrics-cost-warnings-\b", "metrics-cost-warnings-", content)
    content = re.sub(r"\becc-metrics-\b", "metrics-", content)
    content = re.sub(r"\becc-edited-\b", "edited-", content)
    content = re.sub(r"\becc-ctx-warn-\b", "ctx-warn-", content)
    content = re.sub(r"\becc-agent-data\b", "agent-data", content)
    content = re.sub(r"\becc-install-state\b", "install-state", content)
    return content


def update_agent_frontmatter(content: str, agent_name: str) -> str:
    if not content.startswith("---"):
        return content

    end = content.find("\n---", 3)
    if end == -1:
        return content

    front = content[3:end]
    body = content[end + 4 :]

    lines = []
    has_readonly = False
    has_model = False
    for line in front.splitlines():
        stripped = line.strip()
        if stripped.startswith("tools:"):
            continue
        if stripped.startswith("model:"):
            lines.append("model: inherit")
            has_model = True
            continue
        if stripped.startswith("readonly:"):
            lines.append(line)
            has_readonly = True
            continue
        lines.append(line)

    if not has_model:
        lines.append("model: inherit")

    readonly = any(p in agent_name for p in READONLY_PATTERNS)
    if readonly and not has_readonly:
        lines.append("readonly: true")

    new_front = "---\n" + "\n".join(lines) + "\n---"
    return new_front + body


def rename_ecc_files() -> list[str]:
    renamed: list[str] = []

    renames = [
        (ROOT / "ecc-install-state.json", ROOT / "install-state.json"),
        (ROOT / "scripts" / "hooks" / "ecc-statusline.js", ROOT / "scripts" / "hooks" / "statusline.js"),
        (ROOT / "scripts" / "hooks" / "ecc-metrics-bridge.js", ROOT / "scripts" / "hooks" / "metrics-bridge.js"),
        (ROOT / "scripts" / "hooks" / "ecc-context-monitor.js", ROOT / "scripts" / "hooks" / "context-monitor.js"),
    ]

    for src, dst in renames:
        if src.exists() and not dst.exists():
            shutil.move(str(src), str(dst))
            renamed.append(f"{src.name} -> {dst.name}")

    skill_renames = [
        ("ecc-guide", "guide"),
        ("ecc-recipes", "recipes"),
        ("ecc-tools-cost-audit", "tools-cost-audit"),
    ]
    for old, new in skill_renames:
        for base in (ROOT / "skills", ROOT / ".agents" / "skills"):
            src = base / old
            dst = base / new
            if src.exists() and not dst.exists():
                shutil.move(str(src), str(dst))
                renamed.append(f"{base.name}/{old} -> {new}")

    agents_dir = ROOT / "agents"
    if agents_dir.exists():
        for path in sorted(agents_dir.glob("ecc-*.md")):
            dst = agents_dir / path.name[4:]
            if not dst.exists():
                shutil.move(str(path), str(dst))
                renamed.append(f"agents/{path.name} -> {dst.name}")

    return renamed


def process_text_files() -> list[str]:
    changed: list[str] = []

    for path in ROOT.rglob("*"):
        if should_skip(path) or not path.is_file() or not is_text_file(path):
            continue
        if path.name == "migrate-to-cursor.py":
            continue

        try:
            original = path.read_text(encoding="utf-8")
        except (UnicodeDecodeError, OSError):
            continue

        updated = original
        updated = replace_paths(updated)
        updated = replace_harness_claude(updated)
        updated = replace_ecc_refs(updated)

        if path.parent.name == "agents" and path.suffix == ".md":
            agent_name = path.stem.removeprefix("ecc-")
            updated = update_agent_frontmatter(updated, agent_name)

        # globs -> paths in skill frontmatter
        if path.name == "SKILL.md":
            updated = re.sub(r"^globs:\s*", "paths: ", updated, flags=re.MULTILINE)

        if updated != original:
            path.write_text(updated, encoding="utf-8")
            changed.append(str(path.relative_to(ROOT)))

    return changed


def main() -> None:
    renamed = rename_ecc_files()
    changed = process_text_files()

    print("RENAMED:")
    for item in renamed:
        print(f"  - {item}")
    print(f"\nUPDATED {len(changed)} files")
    for item in changed[:50]:
        print(f"  - {item}")
    if len(changed) > 50:
        print(f"  ... and {len(changed) - 50} more")


if __name__ == "__main__":
    main()
