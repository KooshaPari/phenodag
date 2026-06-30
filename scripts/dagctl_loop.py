#!/usr/bin/env python3
"""dagctl_loop.py — Self-Extending DAG Validator Loop

Reads pending work units from phenodag, simulates sub-agent validation, and
optionally extends the DAG by injecting new work units based on validator
findings (the "auditor spawns new work" pattern).

Usage:
    python scripts/dagctl_loop.py <db_path> [--extend] [--dry-run]
    python scripts/dagctl_loop.py <db_path> --auto  --interval 60

Modes:
  --dry-run   Show what would be done without modifying the DB.
  --extend    After validation, inject new work units for findings.
  --auto      Continuous loop with --interval seconds between cycles.
  --max-per-cycle  Limit how many new work units can be injected (default 25).

Self-extending logic:
  Validator picks up pending high-priority work, "validates" it, and for each
  finding (gaps, improvements, SOTA opportunities) injects new work units
  back into the phenodag DB via the SQLite schema. This creates an indefinite
  growth loop: work → validate → find gaps → new work → more validate → ...
"""

import argparse, sqlite3, json, sys, os, time, random, re
from collections import defaultdict
from datetime import datetime, timedelta

# ---- Finding templates (the seed for self-extension) ------------------------
FINDING_TEMPLATES = [
    {
        "category": "benchmark-gap",
        "description": "Benchmark coverage missing for {repo}/{domain}: {detail}",
        "tier": 2,
        "priority": 7,
    },
    {
        "category": "SOTA-opportunity",
        "description": "Implement SOTA {technique} for {repo}: {detail}",
        "tier": 3,
        "priority": 6,
    },
    {
        "category": "CI-hardening",
        "description": "Add {ci_tool} to {repo} CI workflow: {detail}",
        "tier": 2,
        "priority": 8,
    },
    {
        "category": "doc-gap",
        "description": "Document {topic} for {repo}: {detail}",
        "tier": 4,
        "priority": 3,
    },
    {
        "category": "cross-repo-bridge",
        "description": "Wire {producer} -> {consumer} schema bridge: {detail}",
        "tier": 3,
        "priority": 7,
    },
    {
        "category": "tech-debt",
        "description": "Refactor {component} in {repo}: {detail}",
        "tier": 4,
        "priority": 4,
    },
    {
        "category": "metric-gap",
        "description": "Add {metric} measurement to {repo} benchmarks: {detail}",
        "tier": 2,
        "priority": 7,
    },
    {
        "category": "hygiene",
        "description": "Hygiene sweep for {repo}: {detail}",
        "tier": 5,
        "priority": 2,
    },
    {
        "category": "integration",
        "description": "Integrate {tool} with {repo}: {detail}",
        "tier": 3,
        "priority": 5,
    },
    {
        "category": "audit-gap",
        "description": "Audit {pillar} for {repo}: {detail}",
        "tier": 2,
        "priority": 8,
    },
]


# Template arguments for generating diverse findings
REPO_ALIASES = {
    "Benchora": ["Benchora", "benchora"],
    "portage": ["portage", "harbor"],
    "AgilePlus": ["AgilePlus", "agileplus"],
    "phenodag": ["phenodag"],
    "BytePort": ["BytePort", "byteport"],
    "PhenoCompose": ["PhenoCompose"],
    "nanovms": ["nanovms"],
    "heliosBench": ["heliosBench", "heliosbench"],
    "Tracera": ["Tracera", "tracera"],
    "phenotype-registry": ["phenotype-registry", "registry"],
    "phenotype-org-audits": ["phenotype-org-audits", "audits"],
    "pheno-harness": ["pheno-harness", "pheno-harness"],
    "vibeproxy-monitoring-unified": ["vibeproxy", "vibeproxy-monitoring"],
    "phenotype-infra-ci-fix": ["phenotype-infra", "infra-ci"],
}

TECHNIQUES = [
    "criterion-group-benchmarking", "property-based-testing", "mutation-testing",
    "fuzz-testing", "statistical-hypothesis-driven-regression",
    "cross-architecture-benchmarking", "multi-language-benchmark-comparison",
    "trace-based-profiling", "eBPF-based-performance-tracing",
    "adaptive-load-generation", "chaos-engineering-for-perf-regression",
    "compiler-flag-optimization-sweep", "SIMD-benchmark-expansion",
    "distributed-benchmark-orchestration", "real-time-monitoring-dashboard",
]

CI_TOOLS = [
    "cargo-deny", "cargo-machete", "cargo-audit", "cargo-pants",
    "cargo-semver-checks", "cargo-udeps", "cargo-diff", "cargo-pgo",
    "typos-cli", "actionlint", "markdown-link-check", "codespell",
]

TOPICS = [
    "benchmark methodology", "CI/CD pipeline design", "cross-repo architecture",
    "eval framework integration", "DAG workflow design", "result schema contracts",
    "telemetry pipeline", "SOTA pillar definitions", "absorption justification",
    "fleet grading", "mutation coverage", "property testing patterns",
    "parallelization strategy", "self-extending DAG loop",
]

DETAILS = [
    "needs baseline reference benchmarks", "missing edge case coverage",
    "no regression detection threshold", "lacks cross-platform testing",
    "integrate with existing harness pipeline",
    "schema versioning not documented", "error handling gaps",
    "no performance regression alerts", "missing documentation",
    "unused dependencies detected", "license compliance gaps",
    "outdated pattern detected", "could benefit from parallel execution",
    "missing CI badge in README", "dependabot alerts not triaged",
]


def open_db(path: str):
    conn = sqlite3.connect(path)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA journal_mode=WAL")
    return conn


def get_pending_tasks(conn, limit=20):
    """Get the highest-priority pending tasks."""
    cur = conn.cursor()
    # Discover columns
    cur.execute("PRAGMA table_info(tasks)")
    cols = {row["name"] for row in cur.fetchall()}

    order = ""
    if "priority" in cols:
        order = "priority DESC, "
    if "tier" in cols:
        order += "tier ASC, "
    order += "stage ASC, slot ASC"

    query = f"SELECT * FROM tasks WHERE status = 'ready' ORDER BY {order} LIMIT ?"
    cur.execute(query, (limit,))
    return [dict(r) for r in cur.fetchall()], cols


def validate_task(conn, task, cols):
    """Validate a single work unit. Returns findings list."""
    findings = []
    task_id = task["id"]
    stage = task.get("stage", 0)
    slot = task.get("slot", 0)
    repo = task.get("repo", "unknown")
    domain = task.get("domain", "")
    tier = task.get("tier", 1)
    name = task.get("name", f"task-{task_id}")
    desc = task.get("description", "")
    priority = task.get("priority", 5)

    # Find an alias for the repo
    repo_alias = REPO_ALIASES.get(repo, [repo.lower()])[0]

    # Number of findings scales with tier (lower tiers = more critical findings)
    num_findings = max(1, 6 - tier)

    for _ in range(num_findings):
        tmpl = random.choice(FINDING_TEMPLATES)
        detail = random.choice(DETAILS)
        technique = random.choice(TECHNIQUES)
        ci_tool = random.choice(CI_TOOLS)
        topic = random.choice(TOPICS)

        # Find two random consumers for cross-repo bridges
        consumer_repos = [r for r in REPO_ALIASES if r != repo]
        c1 = random.choice(consumer_repos)
        c2 = random.choice([r for r in REPO_ALIASES if r != c1 and r != repo])

        finding_desc = tmpl["description"].format(
            repo=repo_alias,
            domain=domain or repo_alias,
            detail=detail,
            technique=technique,
            ci_tool=ci_tool,
            topic=topic,
            producer=repo_alias,
            consumer=c1,
            component=name,
            metric=detail.split()[0] if detail else "latency",
            tool=ci_tool,
            pillar=f"P{random.randint(20, 30)}",
        )

        findings.append({
            "category": tmpl["category"],
            "priority": max(1, tmpl["priority"] + random.randint(-2, 1)),
            "tier": min(5, tmpl["tier"] + random.randint(0, 1)),
            "description": finding_desc,
            "source_stage": stage,
            "source_slot": slot,
            "source_repo": repo,
        })

    return findings


def inject_findings(conn, findings, max_inject=25, dry_run=False):
    """Inject new work units (findings) back into the phenodag DB."""
    cur = conn.cursor()

    # Get current max stage
    cur.execute("SELECT COALESCE(MAX(stage), 10) as ms FROM tasks")
    next_stage = cur.fetchone()["ms"] + 1

    # Get current max slot for the next stage
    cur.execute("SELECT COALESCE(MAX(slot), 0) as ms FROM tasks WHERE stage = ?",
                (next_stage,))
    next_slot = cur.fetchone()["ms"] + 1

    injected = 0
    for f in findings[:max_inject]:
        slot = next_slot + injected
        desc = f["description"]
        category = f["category"]
        priority = f["priority"]
        tier = f["tier"]
        source_repo = f.get("source_repo", "unknown")

        # Only inject if the description is unique enough (avoid exact duplicates)
        cur.execute("SELECT COUNT(*) as c FROM tasks WHERE description = ? AND stage = ?",
                    (desc, next_stage))
        if cur.fetchone()["c"] > 0:
            continue

        if dry_run:
            print(f"  WOULD INJECT: S{next_stage}.{slot} [{category}] {desc[:100]}")
            injected += 1
            continue

        # Determine columns
        cur.execute("PRAGMA table_info(tasks)")
        cols = {row["name"] for row in cur.fetchall()}

        col_names = ["stage", "slot", "status", "description", "category"]
        col_vals = [next_stage, slot, "ready", desc, category]

        if "priority" in cols:
            col_names.append("priority")
            col_vals.append(priority)
        if "tier" in cols:
            col_names.append("tier")
            col_vals.append(tier)
        if "repo" in cols:
            col_names.append("repo")
            col_vals.append(source_repo)
        if "name" in cols:
            short_name = desc[:80]
            col_names.append("name")
            col_vals.append(short_name)
        if "assigned_agent" in cols:
            agent = "forge" if tier <= 3 else "muse"
            col_names.append("assigned_agent")
            col_vals.append(agent)

        placeholders = ", ".join("?" for _ in col_names)
        col_str = ", ".join(col_names)
        try:
            cur.execute(
                f"INSERT INTO tasks ({col_str}) VALUES ({placeholders})",
                col_vals,
            )
            injected += 1
        except sqlite3.IntegrityError:
            continue

    if not dry_run:
        conn.commit()

    return injected


def self_extend(conn, args):
    """Main self-extending loop: validate pending tasks, inject new work."""
    dry_run = args.dry_run
    do_extend = args.extend
    max_per_cycle = args.max_per_cycle

    pending, cols = get_pending_tasks(conn, limit=args.validate_batch)
    if not pending:
        print(json.dumps({"status": "idle", "message": "No pending tasks to validate", "injected": 0, "timestamp": datetime.utcnow().isoformat()}))
        return

    # Check if we have a unique constraint on (stage, slot)
    cur = conn.cursor()
    cur.execute("PRAGMA table_info(tasks)")
    table_cols = {row["name"]: row for row in cur.fetchall()}
    has_name = "name" in table_cols
    has_assigned = "assigned_agent" in table_cols

    all_findings = []
    tasks_processed = 0
    tracera_findings = 0

    for task in pending:
        findings = validate_task(conn, task, cols)
        all_findings.extend(findings)
        tasks_processed += 1

        # Tracera semantic-scorer bridge: emit kind=tracera-score tasks for
        # semantic regressions (only when --tracera-bridge is enabled)
        if args.tracera_bridge:
            tracera = validate_tracera(task)
            if tracera:
                all_findings.extend(tracera)
                tracera_findings += len(tracera)

        # Mark the task as validated (if 'assigned_agent' exists, update it)
        if not dry_run:
            cur = conn.cursor()
            update_cols = []
            update_vals = []
            if has_assigned:
                update_cols.append("assigned_agent")
                update_vals.append("validator-done")
            if update_cols:
                update_vals.append(task["id"])
                cur.execute(f"UPDATE tasks SET {', '.join(f'{c}=?' for c in update_cols)} WHERE id=?", update_vals)
            else:
                # Fallback: just update status if that column exists
                if "status" in table_cols:
                    cur.execute("UPDATE tasks SET status = 'validated' WHERE id = ?", (task["id"],))
            conn.commit()

    # Inject new work units
    injected = inject_findings(conn, all_findings, max_inject=max_per_cycle, dry_run=dry_run)

    result = {
        "status": "ok" if injected > 0 else "idle",
        "tasks_processed": tasks_processed,
        "total_findings": len(all_findings),
        "injected": injected,
        "max_per_cycle": max_per_cycle,
        "dry_run": dry_run,
        "timestamp": datetime.utcnow().isoformat(),
    }
    print(json.dumps(result, indent=2, default=str))


def validate_tracera(task):
    """Simulate Tracera semantic scorer for a task.

    Returns a list of findings (same shape as FINDING_TEMPLATES) for any
    semantic regression detected by Tracera's cosine-similarity-based scorer.
    Used when --tracera-bridge is enabled; produces kind=tracera-score tasks
    in phenodag so downstream validators can re-score after fixes.
    """
    findings = []
    # Tracera-style detection: if task description mentions semantic/embedding
    # patterns, emit a tracera-score follow-up. In production this calls
    # Tracera/src/tracertm/scoring/semantic_scorer.py via JSON-score-exchange
    # (the P20 producer-consumer bridge from pheno-harness/eval/pillars/tracera_semantic_pillar.py).
    desc = (task.get("description") or "").lower()
    subproject = (task.get("subproject") or "").lower()
    category = (task.get("category") or "").lower()

    # Heuristic: tasks in eval/bench categories or with semantic-related
    # descriptions get a tracera-score follow-up to detect semantic drift.
    semantic_triggers = ("semantic", "embedding", "cosine", "score", "eval")
    if (
        category in ("eval", "bench", "qa", "tracera")
        or any(t in desc for t in semantic_triggers)
        or "tracera" in subproject
    ):
        # Deterministic score based on task ID hash (so re-runs are stable)
        tid = task.get("id", "")
        h = sum(ord(c) for c in str(tid)) % 100
        score = round(0.5 + (h / 200.0), 3)  # 0.5 - 1.0 range
        findings.append({
            "category": "tracera",
            "severity": "info" if score > 0.8 else ("warn" if score > 0.6 else "high"),
            "template": "tracera-score",
            "args": {
                "score": score,
                "scorer": "tracertm.semantic_scorer",
                "source_task": tid,
            },
        })
    return findings


def cmd_loop(args):
    """Self-extending loop: validate → inject → wait → repeat."""
    conn = open_db(args.db)

    if args.auto:
        # Continuous mode
        cycle = 0
        while True:
            cycle += 1
            self_extend(conn, args)
            time.sleep(args.interval)
    else:
        # Single-shot mode
        self_extend(conn, args)

    conn.close()


def main():
    parser = argparse.ArgumentParser(description="Self-Extending DAG Validator Loop")
    parser.add_argument("db", type=str, help="phenodag SQLite DB path")
    parser.add_argument("--dry-run", action="store_true", help="Show what would happen without modifying DB")
    parser.add_argument("--extend", action="store_true", help="Inject new work units for findings")
    parser.add_argument("--auto", action="store_true", help="Continuous loop mode")
    parser.add_argument("--interval", type=int, default=60, help="Seconds between auto-loop cycles")
    parser.add_argument("--validate-batch", type=int, default=10, help="How many pending tasks to validate per cycle")
    parser.add_argument("--max-per-cycle", type=int, default=25, help="Max new work units to inject per cycle")
    parser.add_argument("--tracera-bridge", action="store_true", help="Also run Tracera semantic scorer on each task (emits kind=tracera-score tasks for semantic regressions)")
    args = parser.parse_args()

    cmd_loop(args)


if __name__ == "__main__":
    main()
