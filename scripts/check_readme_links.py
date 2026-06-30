"""Tier-3 #3 README link checker for phenodag.

Walks a Markdown file, extracts every `[text](url)` and bare URL, and
verifies that the local file:// targets exist on disk.  Network URLs
are NOT fetched (no network access by design — this is a CI-friendly
check that runs offline).

Fails (exit 1) if any local link is broken, so a typo in README.md
catches a reviewer in the same PR.

Run:  python scripts/check_readme_links.py README.md
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

# Match `[text](url)` and bare URLs.  Both forms are common in READMEs.
_MD_LINK = re.compile(r"\[([^\]]+)\]\(([^)]+)\)")
_BARE_URL = re.compile(r"(?<![\(\[])(https?://[^\s)<>]+|file://[^\s)<>]+)(?![\)\]])")


def check_markdown(md_path: Path) -> int:
    """Return 0 on success, 1 on any broken local link."""
    if not md_path.exists():
        sys.stderr.write(f"FAIL: {md_path} does not exist\n")
        return 1
    text = md_path.read_text(encoding="utf-8")
    md_dir = md_path.parent

    targets: set[str] = set()
    for m in _MD_LINK.finditer(text):
        url = m.group(2).strip()
        # Strip optional title (`url "title"`)
        if " " in url and not url.startswith("<"):
            url = url.split()[0]
        if url.startswith(("http://", "https://", "mailto:", "#")):
            continue
        targets.add(url)
    for m in _BARE_URL.finditer(text):
        url = m.group(1)
        if url.startswith(("http://", "https://", "mailto:")):
            continue
        targets.add(url)

    broken: list[str] = []
    for url in sorted(targets):
        # Strip the optional fragment for the existence check.
        clean = url.split("#", 1)[0].rstrip("/")
        if not clean:
            continue
        target = (md_dir / clean).resolve() if not clean.startswith("/") else Path(clean)
        if not target.exists():
            broken.append(f"  broken: [{url}]  -> {target}")

    if broken:
        sys.stderr.write(f"FAIL: {md_path} has {len(broken)} broken local link(s):\n")
        for b in broken:
            sys.stderr.write(b + "\n")
        return 1
    print(f"OK: {md_path} ({len(targets)} local link(s), all resolve)")
    return 0


def main() -> int:
    if len(sys.argv) != 2:
        sys.stderr.write("usage: check_readme_links.py <README.md>\n")
        return 2
    return check_markdown(Path(sys.argv[1]))


if __name__ == "__main__":
    sys.exit(main())
