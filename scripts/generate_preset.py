#!/usr/bin/env python3
"""
generate_preset.py — canonical YAML preset generator for phenodag.

Usage:
    python scripts/generate_preset.py <name> <stages> <width> <side_dag_count> <side_size> [--repo <default_repo>]

Examples:
    # Re-generate v3-180 (6 stages x 20 width + 12 side-DAGs x 5)
    python scripts/generate_preset.py v3-180 6 20 12 5 --repo v3

    # New fleet: forge-120 (8 stages x 15 width + 0 side)
    python scripts/generate_preset.py forge-120 8 15 0 0 --repo forge

The generated file goes to `presets/<name>.yaml` and is also emitted to stdout.
The Go-side `phenodag seed --preset <name>` consumes this file via
`internal/preset` (added in this same change).
"""

import argparse
import sys
from pathlib import Path

PRESETS_DIR = Path(__file__).resolve().parent.parent / "presets"


def build_yaml(name: str, stages: int, width: int, side_dag_count: int, side_size: int, repo: str) -> str:
    core_total = stages * width
    side_total = side_dag_count * side_size
    total = core_total + side_total
    lines = []
    lines.append(f"name: {name}")
    lines.append(
        f"description: {name} fleet ({stages} stages x {width} width = {core_total} core"
        + (f" + {side_dag_count} side-DAGs x {side_size} = {side_total} side" if side_dag_count else "")
        + f"; total {total} tasks)"
    )
    lines.append("core:")
    lines.append(f"  stages: {stages}")
    lines.append(f"  width: {width}")
    lines.append("side_dags:")
    if side_dag_count == 0:
        lines.append("  []")
    else:
        for i in range(1, side_dag_count + 1):
            sd_id = f"sd-{name}-{i:03d}"
            sd_name = f"{name} side {i}"
            sd_desc = f"Auto-generated side-DAG {i}/{side_dag_count} for {name} fleet"
            lines.append(f"  - id: {sd_id}")
            lines.append(f"    name: {sd_name}")
            lines.append(f"    description: {sd_desc}")
            lines.append(f"    size: {side_size}")
            lines.append(f"    repo: {repo}")
    lines.append("")  # trailing newline
    return "\n".join(lines)


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument("name", help="preset name (e.g. v3-180)")
    p.add_argument("stages", type=int, help="number of core stages")
    p.add_argument("width", type=int, help="core width per stage")
    p.add_argument("side_dag_count", type=int, help="number of side-DAGs")
    p.add_argument("side_size", type=int, help="tasks per side-DAG")
    p.add_argument("--repo", default="phenodag", help="default repo for side-DAGs")
    p.add_argument("--out", help="output file (default: presets/<name>.yaml)")
    args = p.parse_args()

    yaml_text = build_yaml(args.name, args.stages, args.width, args.side_dag_count, args.side_size, args.repo)

    out_path = Path(args.out) if args.out else PRESETS_DIR / f"{args.name}.yaml"
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(yaml_text, encoding="utf-8")
    print(f"wrote {out_path} ({len(yaml_text)} bytes)", file=sys.stderr)
    print(yaml_text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
