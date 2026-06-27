#!/usr/bin/env python3
"""DAG Control (dagctl): bridge between phenodag SQLite DB and forge task tool.

Subcommands:
  orchestrate  -- Read phenodag DB, emit forge task-agent invocation batches.
  status       -- Show DAG status summary (stages, tiers, completion %).
  edges        -- Show DAG dependency edges.
  agents       -- Show agent assignments and load.

Usage:
    python scripts/dagctl.py orchestrate <db_path> --batch-size 8
    python scripts/dagctl.py status <db_path>
    python scripts/dagctl.py edges <db_path> --stage 3
"""

import argparse, sqlite3, json, sys, os, subprocess, time
from collections import defaultdict
from datetime import datetime

# ---- DB helpers -------------------------------------------------------------
DB_SCHEMA = {
    "tasks": """
        CREATE TABLE IF NOT EXISTS tasks (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            stage INTEGER NOT NULL,
            slot INTEGER NOT NULL,
            status TEXT DEFAULT 'pending',
            subproject TEXT DEFAULT '',
            category TEXT DEFAULT '',
            kind TEXT DEFAULT '',
            priority INTEGER DEFAULT 5,
            description TEXT DEFAULT '',
            assigned_agent TEXT DEFAULT '',
            tier INTEGER DEFAULT 1,
            repo TEXT DEFAULT '',
            domain TEXT DEFAULT '',
            name TEXT DEFAULT '',
            created_at TEXT DEFAULT (datetime('now')),
            started_at TEXT,
            completed_at TEXT,
            UNIQUE(stage, slot)
        )
    """,
    "edges": """
        CREATE TABLE IF NOT EXISTS edges (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            from_stage INTEGER,
            from_slot INTEGER,
            to_stage INTEGER,
            to_slot INTEGER,
            kind TEXT DEFAULT 'depends',
            FOREIGN KEY(from_stage, from_slot) REFERENCES tasks(stage, slot),
            FOREIGN KEY(to_stage, to_slot) REFERENCES tasks(stage, slot)
        )
    """,
}

def open_db(path: str):
    """Open a phenodag DB and ensure the expected schema exists."""
    conn = sqlite3.connect(path)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA journal_mode=WAL")
    # Ensure tables exist (phenodag may use a different schema; we work with
    # the columns that are present)
    return conn

# ---- orchestrate ------------------------------------------------------------
def cmd_orchestrate(args):
    """Read phenodag DB, batch pending tasks, emit forge task-agent invocations."""
    conn = open_db(args.db)
    cur = conn.cursor()

    # Discover what columns actually exist
    cur.execute("PRAGMA table_info(tasks)")
    cols = {row["name"] for row in cur.fetchall()}
    has_priority = "priority" in cols
    has_tier = "tier" in cols
    has_repo = "repo" in cols
    has_domain = "domain" in cols
    has_name = "name" in cols
    has_description = "description" in cols
    has_assigned = "assigned_agent" in cols

    # Fetch pending tasks ordered by priority (desc), then stage, then slot
    order_cols = []
    if has_tier:
        order_cols.append("tier ASC")
    if has_priority:
        order_cols.append("priority DESC")
    order_cols.append("stage ASC, slot ASC")

    limit_clause = f"LIMIT {args.batch_size}" if args.batch_size else ""
    order_clause = ", ".join(order_cols) if order_cols else "stage ASC, slot ASC"

    # Use the actual phenodag status (default 'ready' for newly-seeded tasks)
    # Filter: only ready/pending tasks. Allow override via --status
    target_status = getattr(args, "status", "ready")
    query = f"SELECT * FROM tasks WHERE status = ? ORDER BY {order_clause} {limit_clause}"
    cur.execute(query, (target_status,))
    rows = cur.fetchall()

    if not rows:
        print(json.dumps({"batch": [], "total_pending": 0, "total_tasks": 0}))
        return

    batch = []
    for r in rows:
        rdict = dict(r)
        # Map real phenodag columns to forge-friendly fields
        # subproject is the repo hint (e.g. "forge-mcp-fleet" -> "McpFleet")
        # category is the domain (e.g. "core", "forge", "registry")
        # kind is the work type (e.g. "task", "validator")
        # If subproject is empty, infer from the ID prefix (e.g. "forge-dag-v1-c0001" -> "forge")
        raw_subproject = rdict.get("subproject") or ""
        if raw_subproject:
            repo = raw_subproject
        else:
            # Infer from id: "forge-dag-v1-c0001" -> "forge"
            id_prefix = rdict["id"].split("-")[0] if rdict.get("id") else ""
            repo = id_prefix or "unknown"
        domain = rdict.get("category") or ""
        kind = rdict.get("kind") or "task"
        stage = rdict.get("stage", 0)
        slot = rdict.get("slot", 0)
        # tier: all seeded tasks are L0 for now; future YAML can carry tier
        tier = 1
        # Compose a stable name + description
        repo_cap = "".join(p.capitalize() for p in str(repo).split("-"))
        name = f"{repo_cap}-{kind}-s{stage}-sl{slot}-{rdict['id'].split('-')[-1]}"
        if not rdict.get("description"):
            description = (
                f"Execute {kind} work on {repo} (stage {stage} slot {slot}); "
                f"domain={domain}. Read the registry entry and run the canonical "
                f"validator pattern for {kind} on {repo}."
            )
        else:
            description = rdict["description"]
        priority = rdict.get("priority", 5)

        # Determine forge agent_id based on kind
        if kind in ("validator", "audit"):
            agent = "forge"  # validator runs the cycle
        else:
            agent = "forge"
        complexity = "complex" if kind in ("validator", "audit") else "medium"

        batch.append({
            "id": rdict["id"],
            "stage": stage,
            "slot": slot,
            "tier": tier,
            "repo": repo,
            "domain": domain,
            "kind": kind,
            "name": name,
            "description": description,
            "priority": priority,
            "agent": agent,
            "complexity": complexity,
            # forge task tool invocation shape
            "forge_task": {
                "agent_id": agent,
                "tasks": [
                    f"Execute work unit: {name}",
                    f"Repo: {repo}",
                    f"Domain: {domain}",
                    f"Kind: {kind}",
                    f"Description: {description}",
                ],
            },
        })

    # Counts: ready = status='ready' (matches phenodag seed convention)
    cur.execute("SELECT COUNT(*) as c FROM tasks WHERE status = 'ready'")
    total_pending = cur.fetchone()["c"]
    cur.execute("SELECT COUNT(*) as c FROM tasks")
    total_tasks = cur.fetchone()["c"]  # noqa: F841
    output = {
        "batch": batch,
        "batch_size": len(batch),
        "total_pending": total_pending,
        "total_tasks": total_tasks,
        "timestamp": datetime.utcnow().isoformat(),
    }

    # Write batch to file if --output specified
    if args.output:
        with open(args.output, "w", encoding="utf-8") as f:
            json.dump(output, f, indent=2, default=str)
        print(f"Wrote batch of {len(batch)} to {args.output}")
    else:
        print(json.dumps(output, indent=2, default=str))

    conn.close()


# ---- status -----------------------------------------------------------------
def cmd_status(args):
    """Show DAG status summary."""
    conn = open_db(args.db)
    cur = conn.cursor()

    # Discover columns
    cur.execute("PRAGMA table_info(tasks)")
    cols = {row["name"]: row for row in cur.fetchall()}
    has_tier = "tier" in cols

    # Count by status
    cur.execute("SELECT status, COUNT(*) as c FROM tasks GROUP BY status")
    by_status = {row["status"]: row["c"] for row in cur.fetchall()}

    # Count by stage
    cur.execute("SELECT stage, status, COUNT(*) as c FROM tasks GROUP BY stage, status ORDER BY stage")
    by_stage = defaultdict(dict)
    for row in cur.fetchall():
        by_stage[row["stage"]][row["status"]] = row["c"]

    total = sum(by_status.values())
    completed = by_status.get("completed", 0) + by_status.get("passed", 0)
    pct = round(completed / total * 100, 1) if total else 0

    # Tier breakdown (if available)
    if has_tier and "tier" in {r["name"] for r in cur.execute("PRAGMA table_info(tasks)")}:
        cur.execute("SELECT tier, status, COUNT(*) as c FROM tasks GROUP BY tier, status ORDER BY tier")
        by_tier = defaultdict(dict)
        for row in cur.fetchall():
            by_tier[row["tier"]][row["status"]] = row["c"]
    else:
        by_tier = {}

    print(json.dumps({
        "total": total,
        "completed": completed,
        "completion_pct": pct,
        "by_status": dict(by_status),
        "stages": len(by_stage),
        "by_stage": dict(by_stage),
        "by_tier": dict(by_tier),
        "timestamp": datetime.utcnow().isoformat(),
    }, indent=2, default=str))
    conn.close()


# ---- edges ------------------------------------------------------------------
def cmd_edges(args):
    """Show DAG edges."""
    conn = open_db(args.db)
    cur = conn.cursor()
    if args.stage:
        cur.execute("SELECT * FROM edges WHERE from_stage = ? OR to_stage = ? ORDER BY from_stage, from_slot",
                    (args.stage, args.stage))
    else:
        cur.execute("SELECT * FROM edges ORDER BY from_stage, from_slot")
    rows = cur.fetchall()
    print(json.dumps([{
        "from": f"S{r['from_stage']}.{r['from_slot']}",
        "to": f"S{r['to_stage']}.{r['to_slot']}",
        "kind": r["kind"],
    } for r in rows], indent=2, default=str))
    conn.close()


# ---- main -------------------------------------------------------------------
def main():
    parser = argparse.ArgumentParser(description="DAG Control (dagctl)")
    parser.add_argument("--db", type=str, default="dag.db",
                        help="phenodag SQLite DB path")
    sub = parser.add_subparsers(dest="command", required=True)

    # Orchestrate
    p_orch = sub.add_parser("orchestrate", help="Emit forge task-agent batch")
    p_orch.add_argument("db", nargs="?", type=str, default=None,
                        help="phenodag DB path (overrides --db)")
    p_orch.add_argument("--batch-size", type=int, default=8,
                        help="Max units per batch (default 8)")
    p_orch.add_argument("--output", type=str, default="",
                        help="Write batch JSON to file")

    # Status
    p_stat = sub.add_parser("status", help="DAG status summary")
    p_stat.add_argument("db", nargs="?", type=str, default=None)

    # Edges
    p_edges = sub.add_parser("edges", help="DAG edges")
    p_edges.add_argument("db", nargs="?", type=str, default=None)
    p_edges.add_argument("--stage", type=int, default=0)

    args = parser.parse_args()

    # Resolve db path: positional takes precedence over --db
    if hasattr(args, 'db') and args.db:
        db_path = args.db
    else:
        db_path = args.db  # from the global --db default

    if args.command == "orchestrate":
        cmd_orchestrate(args)
    elif args.command == "status":
        cmd_status(args)
    elif args.command == "edges":
        cmd_edges(args)


if __name__ == "__main__":
    main()
