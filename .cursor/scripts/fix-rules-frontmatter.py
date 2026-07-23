#!/usr/bin/env python3
"""Bulk-fix .cursor/rules/*.mdc frontmatter for Cursor compatibility."""

from __future__ import annotations

import re
from pathlib import Path

RULES_DIR = Path(__file__).resolve().parent.parent / "rules"

STACK_DISPLAY: dict[str, str] = {
    "golang": "Go",
    "csharp": "C#",
    "cpp": "C++",
    "fsharp": "F#",
    "react-native": "React Native",
    "react": "React",
    "vue": "Vue",
    "nuxt": "Nuxt",
    "angular": "Angular",
    "typescript": "TypeScript",
    "python": "Python",
    "java": "Java",
    "kotlin": "Kotlin",
    "swift": "Swift",
    "rust": "Rust",
    "ruby": "Ruby",
    "php": "PHP",
    "perl": "Perl",
    "dart": "Dart",
    "arkts": "ArkTS",
    "web": "Web",
}

SUFFIX_DESCRIPTIONS: dict[str, str] = {
    "coding-style": "{stack} coding style extending common rules",
    "hooks": "{stack} hooks extending common rules",
    "testing": "{stack} testing extending common rules",
    "security": "{stack} security extending common rules",
    "patterns": "{stack} patterns extending common rules",
    "performance": "{stack} performance extending common rules",
    "design-quality": "{stack} design quality extending common rules",
    "fastapi": "FastAPI patterns extending common rules",
    "accessibility": "{stack} accessibility extending common rules",
    "production-readiness": "{stack} production readiness extending common rules",
}

COMMON_DESCRIPTIONS: dict[str, str] = {
    "common-agents": "Agent orchestration: available agents, parallel execution, multi-perspective analysis",
    "common-code-review": "Code review standards: when to review, checklist, and approval criteria",
    "common-coding-style": "Cross-project coding conventions: immutability, file organization, error handling, input validation",
    "common-development-workflow": "Feature development workflow: planning, TDD, code review, and git operations",
    "common-git-workflow": "Git workflow: commit message format and pull request process",
    "common-hooks": "Hooks system: types, auto-accept permissions, TodoWrite best practices",
    "common-patterns": "Common patterns: skeleton projects, repository pattern, API response format",
    "common-performance": "Performance optimization: model selection, context management, build troubleshooting",
    "common-security": "Security guidelines: mandatory checks, secret management, response protocol",
    "common-testing": "Testing requirements: coverage targets, TDD workflow, troubleshooting",
}


def parse_frontmatter(content: str) -> tuple[dict[str, object], str, bool]:
    if not content.startswith("---"):
        return {}, content, False
    end = content.find("\n---", 3)
    if end == -1:
        return {}, content, False
    raw = content[3:end].strip()
    body = content[end + 4 :]
    if body.startswith("\n"):
        body = body[1:]

    meta: dict[str, object] = {}
    current_key: str | None = None
    list_items: list[str] = []

    for line in raw.splitlines():
        if line.startswith("  - ") and current_key is not None:
            list_items.append(line[4:].strip().strip('"').strip("'"))
            continue
        if current_key is not None and list_items:
            meta[current_key] = list_items
            list_items = []
        m = re.match(r"^([A-Za-z]+):\s*(.*)$", line)
        if not m:
            continue
        key, value = m.group(1), m.group(2).strip()
        current_key = key
        if not value:
            list_items = []
            continue
        if value.startswith("[") and value.endswith("]"):
            inner = value[1:-1].strip()
            if inner:
                meta[key] = [
                    item.strip().strip('"').strip("'")
                    for item in inner.split(",")
                    if item.strip()
                ]
            else:
                meta[key] = []
        elif value in ("true", "false"):
            meta[key] = value == "true"
        else:
            meta[key] = value.strip('"').strip("'")
        current_key = None

    if current_key is not None and list_items:
        meta[current_key] = list_items

    return meta, body, True


def infer_description(stem: str) -> str:
    if stem in COMMON_DESCRIPTIONS:
        return COMMON_DESCRIPTIONS[stem]

    if stem.startswith("common-"):
        suffix = stem.removeprefix("common-").replace("-", " ")
        return f"Common {suffix} extending shared conventions"

    for stack in sorted(STACK_DISPLAY, key=len, reverse=True):
        prefix = f"{stack}-"
        if stem.startswith(prefix):
            suffix = stem[len(prefix) :]
            template = SUFFIX_DESCRIPTIONS.get(suffix)
            if template:
                display = STACK_DISPLAY[stack]
                return template.format(stack=display)
            return f"{STACK_DISPLAY[stack]} {suffix.replace('-', ' ')} extending common rules"

    return f"{stem.replace('-', ' ').title()} extending common rules"


def format_globs(globs: list[str]) -> str:
    quoted = ", ".join(f'"{g}"' for g in globs)
    return f"globs: [{quoted}]"


def render_frontmatter(meta: dict[str, object]) -> str:
    lines = ["---"]
    if "description" in meta:
        desc = str(meta["description"]).replace('"', '\\"')
        lines.append(f'description: "{desc}"')
    if "globs" in meta:
        globs = meta["globs"]
        assert isinstance(globs, list)
        lines.append(format_globs([str(g) for g in globs]))
    if "alwaysApply" in meta:
        lines.append(f"alwaysApply: {str(meta['alwaysApply']).lower()}")
    lines.append("---")
    return "\n".join(lines)


def fix_rule(path: Path) -> bool:
    original = path.read_text(encoding="utf-8")
    stem = path.stem
    meta, body, had_frontmatter = parse_frontmatter(original)

    if "paths" in meta:
        paths = meta.pop("paths")
        assert isinstance(paths, list)
        meta["globs"] = paths

    if "description" not in meta:
        meta["description"] = infer_description(stem)

    is_common = stem.startswith("common-")
    if is_common:
        meta["alwaysApply"] = True
        meta.pop("globs", None)
    else:
        meta["alwaysApply"] = False
        if "globs" not in meta:
            raise ValueError(f"{path.name}: stack rule missing globs after conversion")

    new_content = render_frontmatter(meta) + "\n" + body
    if new_content != original:
        path.write_text(new_content, encoding="utf-8")
        return True
    return False


def main() -> None:
    changed: list[str] = []
    for path in sorted(RULES_DIR.glob("*.mdc")):
        if fix_rule(path):
            changed.append(path.name)
    print(f"Changed {len(changed)} rule files:")
    for name in changed:
        print(f"  - {name}")


if __name__ == "__main__":
    main()
