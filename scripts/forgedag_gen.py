#!/usr/bin/env python3
"""forge-DAG generator: massively multi-tier work-unit preset for phenodag.

Produces a YAML preset file compatible with `phenodag seed-yaml`.
The DAG is structured into 4 tiers, each having core + side DAGs.
Tier-1 (executor units) → Tier-2 (audit/grade units) → Tier-3 (cross-repo
integration units) → Tier-4 (self-extending / meta-DAG units).

Usage:
    python scripts/forgedag_gen.py [--tier 1|2|3|4] [--repo-filter R1,R2]
"""

import argparse, sys, yaml, json, hashlib, os
from datetime import date

# ---- known repos -----------------------------------------------------------
TIER_MAP = {
    1: ("executor", [
        # Repos that execute SOTA: benchmark, evaluate, compile, deploy
        ("Benchora",     "bench"),
        ("portage",      "eval"),
        ("heliosBench",  "bench"),
        ("phenodag",     "dag"),
        ("Tracera",      "eval"),
        ("phenotype-org-audits", "audit"),
    ]),
    2: ("audit/grade", [
        # Repos that audit, grade, verify
        ("phenotype-registry", "registry"),
        ("AgilePlus",          "spine"),
        ("phenotype-infra-ci-fix", "infra"),
        ("pheno-harness",      "eval"),
    ]),
    3: ("integration", [
        # Cross-repo bridge repos
        ("BytePort",     "infra"),
        ("PhenoCompose", "infra"),
        ("nanovms",      "infra"),
        ("vibeproxy-monitoring-unified", "monitor"),
    ]),
    4: ("meta/self-extend", [
        # Self-extending / meta-DAG repos
        ("phenodag",          "dag"),
        ("AgilePlus",         "spine"),
        ("phenotype-registry","registry"),
        ("phenotype-org-audits", "audit"),
    ]),
}

# ---- work-unit categories per domain ---------------------------------------
CATEGORIES = {
    "bench": [
        "criterion-profile", "bench-regression", "bench-new-suite",
        "microbench", "bench-ci-perf", "bench-docs",
    ],
    "eval": [
        "eval-scenario", "eval-swe-bench", "eval-rlvr",
        "eval-accuracy", "eval-trace", "eval-regression",
    ],
    "dag": [
        "dag-preset", "dag-seed", "dag-status", "dag-validate",
        "dag-self-extend", "dag-orchestrate",
    ],
    "audit": [
        "audit-pillar", "audit-scorecard", "audit-fleet",
        "audit-repo", "audit-cross-cutting",
    ],
    "registry": [
        "registry-absorb", "registry-grade", "registry-scan",
        "registry-fleet",
    ],
    "spine": [
        "spine-absorb", "spine-sync", "spine-dag-manifest",
    ],
    "infra": [
        "infra-build", "infra-release", "infra-docker",
        "infra-cross-toolchain", "infra-hygiene",
    ],
    "monitor": [
        "monitor-dash", "monitor-alert", "monitor-heal",
    ],
}

# ---- helper: deterministic but spread-out task names -----------------------
def make_task_name(repo: str, stage: int, slot: int, cat: str) -> str:
    return f"{repo}-s{stage:03d}-sl{slot:03d}-{cat}"

# ---- build core stage ------------------------------------------------------
def build_core_stage(tier_name: str, repos: list, stage: int,
                     stage_width: int, side_dags: list):
    """Build a core stage dict for the YAML preset."""
    tasks = []
    for slot in range(1, stage_width + 1):
        idx = (slot - 1) % len(repos)
        repo, domain = repos[idx]
        cats = CATEGORIES[domain]
        cat = cats[(slot - 1) % len(cats)]
        status = "pending"
        # Tier-1 gets running; higher tiers get pending
        if tier_name == "executor" and stage <= 2:
            status = "pending"
        elif tier_name == "meta/self-extend":
            status = "pending"  # meta tasks always pending (self-extending)
        tasks.append({
            "stage": stage,
            "slot": slot,
            "repo": repo,
            "domain": domain,
            "category": cat,
            "status": status,
            "name": make_task_name(repo, stage, slot, cat),
        })
    return {
        "stage": stage,
        "tasks": tasks,
    }

# ---- build side DAGs -------------------------------------------------------
def build_side_dags(repos: list, side_size: int, start_stage: int, tier_idx: int):
    """Build side-DAG entries that follow their assigned repo."""
    dags = []
    for i, (repo, domain) in enumerate(repos):
        cats = CATEGORIES[domain]
        cat = cats[i % len(cats)]
        dags.append({
            "id": f"t{tier_idx}-{repo}-side",
            "size": side_size,
            "name": f"t{tier_idx}-side-{repo}-{cat}",
        })
    return dags

# ---- main ------------------------------------------------------------------
def main():
    parser = argparse.ArgumentParser(description="Generate forge-DAG preset YAML")
    parser.add_argument("--tier", type=int, default=0,
                        help="Specific tier (1-4) or 0 for all")
    parser.add_argument("--core-width", type=int, default=20,
                        help="Width of each core stage (default 20)")
    parser.add_argument("--side-size", type=int, default=5,
                        help="Number of tasks per side DAG (default 5)")
    parser.add_argument("--out", type=str, default="",
                        help="Output path (default: presets/forge-dag-v1.yaml)")
    parser.add_argument("--seed-db", type=str, default="",
                        help="phenodag DB path for direct seeding")
    args = parser.parse_args()

    tiers_to_build = [args.tier] if args.tier else sorted(TIER_MAP.keys())

    all_core_stages = []
    all_side_dags = []
    total_expected = 0
    stage_num = 1

    for t in tiers_to_build:
        tier_name, repos = TIER_MAP[t]
        # Core stages per tier
        num_stages = {1: 3, 2: 3, 3: 2, 4: 2}[t]
        if t == 1:
            width = 16  # Tier-1: 16-wide (enough for all 6 executor repos + room)
        elif t == 4:
            width = 8
        else:
            width = args.core_width

        for s in range(num_stages):
            stage = stage_num
            core = build_core_stage(tier_name, repos, stage, width, [])
            all_core_stages.append(core)
            total_expected += len(core["tasks"])
            stage_num += 1

        # Side DAGs per tier
        side_dags = build_side_dags(repos, args.side_size, stage_num, t)
        all_side_dags.extend(side_dags)
        total_expected += len(side_dags) * args.side_size

    # ---- build preset YAML -------------------------------------------------
    # Flat schema matching internal/preset/preset.go:
    #   core: {stages: int, width: int}
    #   side_dags: [{id: str, size: int, name: str}, ...]
    unique_stages = len(set(s["stage"] for s in all_core_stages)) if all_core_stages else 0
    core_width = sum(len(s["tasks"]) for s in all_core_stages) // unique_stages if unique_stages else 0
    preset = {
        "name": "forge-dag-v1",
        "description": f"Forge-DAG v1 — indefinitely-extendable multi-tier DAG. Generated {date.today()}",
        "core": {
            "stages": unique_stages,
            "width": core_width,
        },
        "side_dags": all_side_dags,
    }

    out_path = args.out or f"presets/forge-dag-v1.yaml"
    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    with open(out_path, "w", encoding="utf-8") as f:
        yaml.dump(preset, f, default_flow_style=False, sort_keys=False, allow_unicode=True)

    print(f"Generated: {out_path}")
    print(f"  Total expected work units: {total_expected}")
    print(f"  Core stages: {unique_stages} (width {core_width} = {unique_stages * core_width} tasks)")
    print(f"  Side DAGs: {len(all_side_dags)} (avg size {sum(sd['size'] for sd in all_side_dags)//len(all_side_dags) if all_side_dags else 0} each)")
    print(f"  Core+Side total: {total_expected}")
    print(f"  Tiers: {', '.join(f'T{t}:{TIER_MAP[t][0]}' for t in tiers_to_build)}")
    print()

    # Direct seed if --seed-db given
    if args.seed_db:
        import subprocess
        db_path = args.seed_db
        # First seed via seed-yaml
        cmd = ["phenodag.exe", "seed-yaml", "--preset", "forge-dag-v1",
               "--db", db_path]
        if os.path.exists("phenodag.exe"):
            result = subprocess.run(cmd, capture_output=True, text=True)
            if result.returncode == 0:
                print(f"Seeded {total_expected} units into {db_path}")
            else:
                print(f"Seed output: {result.stdout}")
                print(f"Seed errors: {result.stderr}")
        else:
            print(f"phenodag.exe not found; preset written to {out_path}")
            print(f"Seed manually: phenodag seed-yaml --preset forge-dag-v1 --db {db_path}")

if __name__ == "__main__":
    main()
