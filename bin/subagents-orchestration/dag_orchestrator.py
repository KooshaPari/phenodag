#!/usr/bin/env python3
"""
Forge Session Recovery DAG Orchestrator

Analyzes the forge conversation database, identifies incomplete sessions,
categorizes them, and creates either:
  1. Resume commands for individual sessions (forge --conversation-id [id] ...)
  2. Aggregate fresh session prompts for related tasks

Usage:
  python3 dag_orchestrator.py --phase collect    # Extract and categorize
  python3 dag_orchestrator.py --phase analyze    # Build aggregate groups
  python3 dag_orchestrator.py --phase plan       # Generate launch commands
  python3 dag_orchestrator.py --phase launch     # Execute (dry-run by default)
"""

import argparse
import json
import sqlite3
import collections
import re
import os
from datetime import datetime, timezone

DB_PATH = "/Users/kooshapari/forge/.forge.db"
INVENTORY_PATH = "/Users/kooshapari/.forge/forge_resume_runs/inventory_2026_06_13.json"
PLAN_PATH = "/Users/kooshapari/.forge/forge_resume_runs/dag_plan_2026_06_13.json"
LAUNCH_SCRIPT_PATH = "/Users/kooshapari/.forge/forge_resume_runs/dag_launch_2026_06_13.sh"
RESUME_DIR = "/Users/kooshapari/.forge/forge_resume_runs"

def collect():
    """Extract all active conversations from the DB."""
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    cursor = conn.cursor()
    cursor.execute("""
        SELECT conversation_id, title, workspace_id, created_at, updated_at, metrics
        FROM conversations
        WHERE context IS NOT NULL
        ORDER BY updated_at DESC
    """)
    rows = []
    for row in cursor.fetchall():
        metrics = row["metrics"]
        try:
            metrics_obj = json.loads(metrics) if metrics else {}
        except:
            metrics_obj = {}
        rows.append({
            "id": row["conversation_id"],
            "title": row["title"] or "",
            "workspace_id": row["workspace_id"],
            "created_at": row["created_at"],
            "updated_at": row["updated_at"],
            "metrics": metrics_obj,
        })
    with open(INVENTORY_PATH, "w") as f:
        json.dump(rows, f, indent=2)
    print(f"[collect] Wrote {len(rows)} active conversations to {INVENTORY_PATH}")
    return rows

def load_inventory():
    with open(INVENTORY_PATH) as f:
        return json.load(f)

def categorize(data):
    """Categorize conversations by title patterns."""
    groups = collections.defaultdict(list)
    for item in data:
        title = item["title"]
        # Skip the current session (if identifiable)
        if title == "" and item["id"] == "9349f7d4-eb21-4595-afd9-e4ada62536fd":
            continue
        # Categorize by pattern
        if "[PARENT]" in title:
            groups["PARENT"].append(item)
        elif "[SUBAGENT]" in title:
            groups["SUBAGENT"].append(item)
        elif "V3 DAG L5" in title or "V3 DAG L4" in title:
            groups["V3_DAG"].append(item)
        elif "Task side-" in title:
            groups["SIDE_TASK"].append(item)
        elif "TASK ID:" in title or re.search(r'arc-\d+', title, re.I):
            groups["ARC_TASK"].append(item)
        elif "WP-" in title:
            groups["WP_TASK"].append(item)
        elif "governance" in title.lower() or "L1.4" in title:
            groups["GOVERNANCE"].append(item)
        elif "CI" in title or "cargo" in title.lower() or "coverage" in title.lower():
            groups["CI_CARGO"].append(item)
        elif "ontology" in title.lower() or "intent graph" in title.lower():
            groups["ONTOLOGY"].append(item)
        elif "audit" in title.lower():
            groups["AUDIT"].append(item)
        elif "Implement" in title or "implement" in title.lower():
            groups["IMPLEMENT"].append(item)
        elif "Test" in title or "test" in title.lower():
            groups["TEST"].append(item)
        elif "Fix" in title or "fix" in title.lower():
            groups["FIX"].append(item)
        elif "Create" in title or "create" in title.lower():
            groups["CREATE"].append(item)
        elif "Add" in title or "add" in title.lower():
            groups["ADD"].append(item)
        elif "Resume" in title or "resume" in title.lower():
            groups["RESUME"].append(item)
        elif "Worktree" in title or "worktree" in title.lower():
            groups["WORKTREE"].append(item)
        elif "PR" in title or "pr " in title.lower():
            groups["PR"].append(item)
        else:
            groups["OTHER"].append(item)
    return groups

def build_plan(data):
    """Build the DAG plan: aggregate groups and individual resume commands."""
    groups = categorize(data)
    plan = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "total_conversations": len(data),
        "aggregate_sessions": [],
        "individual_resumes": [],
        "skipped": []
    }

    # --- AGGREGATE GROUPS ---
    # Group 1: AgilePlus Ontology (all related, create one fresh session)
    ontology_items = groups.get("ONTOLOGY", [])
    # Also include the 4 ontology tasks from the current session
    ontology_items += [i for i in groups.get("OTHER", []) if "ontology" in i["title"].lower() or "intent graph" in i["title"].lower()]
    if ontology_items:
        tasks = []
        for item in ontology_items:
            tasks.append({
                "id": item["id"],
                "title": item["title"],
                "updated_at": item["updated_at"]
            })
        plan["aggregate_sessions"].append({
            "name": "AgilePlus Ontology & Intent Graph",
            "type": "aggregate_fresh",
            "strategy": "Create a single fresh forge session that covers all ontology tasks.",
            "tasks": tasks,
            "prompt": "You are resuming a multi-part AgilePlus ontology implementation.\n" +
                      "Complete all of these tasks in order:\n" +
                      "\n".join([f"{t['id'][:8]}: {t['title'][:120]}" for t in tasks]) +
                      "\n\nExecute each task fully before moving to the next. Report completion."
        })

    # Group 2: V3 DAG tasks - group by level
    v3_items = groups.get("V3_DAG", [])
    l5_items = [i for i in v3_items if "L5" in i["title"]]
    l4_items = [i for i in v3_items if "L4" in i["title"]]
    if l5_items:
        plan["aggregate_sessions"].append({
            "name": "V3 DAG L5 Tasks",
            "type": "aggregate_resume",
            "tasks": [{"id": i["id"], "title": i["title"], "updated_at": i["updated_at"]} for i in l5_items],
            "strategy": "Resume each L5 task individually in parallel batches.",
            "batch_size": 5
        })
    if l4_items:
        plan["aggregate_sessions"].append({
            "name": "V3 DAG L4 Tasks",
            "type": "aggregate_resume",
            "tasks": [{"id": i["id"], "title": i["title"], "updated_at": i["updated_at"]} for i in l4_items],
            "strategy": "Resume each L4 task individually in parallel batches.",
            "batch_size": 5
        })

    # Group 3: WP Tasks - group by type
    wp_items = groups.get("WP_TASK", [])
    if wp_items:
        plan["aggregate_sessions"].append({
            "name": "MelosViz WP Backend Tasks",
            "type": "aggregate_resume",
            "tasks": [{"id": i["id"], "title": i["title"], "updated_at": i["updated_at"]} for i in wp_items],
            "strategy": "Resume each WP task individually.",
            "batch_size": 3
        })

    # Group 4: Side Tasks - group by project
    side_items = groups.get("SIDE_TASK", [])
    side_by_project = collections.defaultdict(list)
    for item in side_items:
        title = item["title"]
        # Extract project name from title like "Task side-232: Extend tracera-core..."
        m = re.search(r'(?:Repo|repo):\s*([^\s,]+)', title)
        if m:
            project = m.group(1).split("/")[-1]
        else:
            m = re.search(r'(?:side-\d+):\s*([A-Z][a-z]+)', title)
            project = m.group(1) if m else "UNKNOWN"
        side_by_project[project].append(item)
    for project, items in side_by_project.items():
        plan["aggregate_sessions"].append({
            "name": f"Side Tasks: {project}",
            "type": "aggregate_resume",
            "tasks": [{"id": i["id"], "title": i["title"], "updated_at": i["updated_at"]} for i in items],
            "strategy": f"Resume {project} side tasks individually.",
            "batch_size": 3
        })

    # Group 5: Arc Tasks - group by arc ID
    arc_items = groups.get("ARC_TASK", [])
    arc_by_id = collections.defaultdict(list)
    for item in arc_items:
        m = re.search(r'arc-(\d+)-(\d+)', item["title"], re.I)
        if m:
            arc_id = f"arc-{m.group(1)}"
        else:
            arc_id = "ARC_MISC"
        arc_by_id[arc_id].append(item)
    for arc_id, items in arc_by_id.items():
        plan["aggregate_sessions"].append({
            "name": f"Arc Tasks: {arc_id}",
            "type": "aggregate_resume",
            "tasks": [{"id": i["id"], "title": i["title"], "updated_at": i["updated_at"]} for i in items],
            "strategy": f"Resume {arc_id} tasks individually.",
            "batch_size": 3
        })

    # Group 6: Governance tasks - aggregate by repo
    gov_items = groups.get("GOVERNANCE", [])
    if gov_items:
        plan["aggregate_sessions"].append({
            "name": "L1.4 Governance Tasks",
            "type": "aggregate_resume",
            "tasks": [{"id": i["id"], "title": i["title"], "updated_at": i["updated_at"]} for i in gov_items],
            "strategy": "Resume governance tasks individually.",
            "batch_size": 5
        })

    # Group 7: CI/Cargo tasks - aggregate into one fresh session
    ci_items = groups.get("CI_CARGO", [])
    if ci_items:
        plan["aggregate_sessions"].append({
            "name": "CI & Cargo Baseline Tasks",
            "type": "aggregate_fresh",
            "tasks": [{"id": i["id"], "title": i["title"], "updated_at": i["updated_at"]} for i in ci_items],
            "strategy": "Create a fresh session that runs CI/cargo fixes across all repos.",
            "prompt": "You are fixing CI and cargo baseline issues across multiple repos.\n" +
                      "Execute these tasks:\n" +
                      "\n".join([f"{i['id'][:8]}: {i['title'][:120]}" for i in ci_items]) +
                      "\n\nComplete each repo's fixes before moving to the next."
        })

    # --- INDIVIDUAL RESUMES ---
    # For items that are too large/complex to aggregate, resume individually
    # Include the 232 known subagent tasks from all_subagents_resume.md
    known_ids = set()
    known_resume_file = os.path.join(RESUME_DIR, "all_subagents_resume.md")
    if os.path.exists(known_resume_file):
        with open(known_resume_file) as f:
            content = f.read()
        for m in re.finditer(r'\*\*ID:\*\* `([a-f0-9-]+)`', content):
            known_ids.add(m.group(1))
    for item in data:
        if item["id"] in known_ids:
            plan["individual_resumes"].append({
                "id": item["id"],
                "title": item["title"],
                "updated_at": item["updated_at"],
                "strategy": "resume",
                "reason": "Known subagent task from all_subagents_resume.md"
            })

    # --- SKIPPED ---
    # Skip PARENT/SUBAGENT pairs that are likely completed
    for item in groups.get("PARENT", []) + groups.get("SUBAGENT", []):
        # Only include recent ones (last 7 days) as potentially incomplete
        updated_str = item["updated_at"]
        if "Z" in updated_str:
            updated_str = updated_str.replace("Z", "+00:00")
        if "+" in updated_str or "-" in updated_str[-6:]:
            updated = datetime.fromisoformat(updated_str)
        else:
            updated = datetime.fromisoformat(updated_str).replace(tzinfo=timezone.utc)
        now = datetime.now(timezone.utc)
        age = now - updated
        if age.days > 7:
            plan["skipped"].append({
                "id": item["id"],
                "title": item["title"][:80],
                "reason": f"Stale (>7 days, age={age.days}d)"
            })

    return plan

def generate_launch_script(plan):
    """Generate a shell script with all launch commands."""
    lines = ["#!/bin/bash", "# Auto-generated DAG launch script", f"# Generated: {plan['generated_at']}", ""]

    # Aggregate fresh sessions
    for agg in plan["aggregate_sessions"]:
        if agg["type"] == "aggregate_fresh":
            lines.append(f"# --- AGGREGATE FRESH: {agg['name']} ---")
            lines.append(f"# Tasks: {len(agg['tasks'])}")
            # Create a fresh session with a combined prompt
            prompt = agg["prompt"].replace('"', '\\"')
            lines.append(f"forge -p \"{prompt[:500]}...\" -C /Users/kooshapari/CodeProjects/Phenotype/repos")
            lines.append("")
        elif agg["type"] == "aggregate_resume":
            lines.append(f"# --- AGGREGATE RESUME: {agg['name']} ---")
            lines.append(f"# Tasks: {len(agg['tasks'])}, Batch size: {agg.get('batch_size', 5)}")
            for task in agg["tasks"]:
                title = task["title"].replace('"', '\\"')[:100]
                lines.append(f"forge --conversation-id {task['id']} -p \"Continue: {title}...\" -C /Users/kooshapari/CodeProjects/Phenotype/repos")
            lines.append("")

    # Individual resumes
    lines.append("# --- INDIVIDUAL RESUMES ---")
    for task in plan["individual_resumes"]:
        title = task["title"].replace('"', '\\"')[:100]
        lines.append(f"forge --conversation-id {task['id']} -p \"Continue: {title}...\" -C /Users/kooshapari/CodeProjects/Phenotype/repos")
    lines.append("")

    lines.append("# --- DONE ---")
    with open(LAUNCH_SCRIPT_PATH, "w") as f:
        f.write("\n".join(lines))
    os.chmod(LAUNCH_SCRIPT_PATH, 0o755)
    print(f"[generate] Wrote {len(lines)} lines to {LAUNCH_SCRIPT_PATH}")

    # Also write the plan JSON
    with open(PLAN_PATH, "w") as f:
        json.dump(plan, f, indent=2)
    print(f"[generate] Wrote plan to {PLAN_PATH}")

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--phase", choices=["collect", "analyze", "plan", "launch", "all"], default="all")
    args = parser.parse_args()

    if args.phase in ("collect", "all"):
        collect()

    if args.phase in ("analyze", "plan", "all"):
        data = load_inventory()
        plan = build_plan(data)
        generate_launch_script(plan)
        print(f"[analyze] Aggregate sessions: {len(plan['aggregate_sessions'])}")
        print(f"[analyze] Individual resumes: {len(plan['individual_resumes'])}")
        print(f"[analyze] Skipped: {len(plan['skipped'])}")

    if args.phase == "launch":
        with open(PLAN_PATH) as f:
            plan = json.load(f)
        print("[launch] Dry-run mode. Set --live to actually execute.")
        print(f"[launch] Would launch {len(plan['aggregate_sessions'])} aggregate sessions")
        print(f"[launch] Would resume {len(plan['individual_resumes'])} individual sessions")

if __name__ == "__main__":
    main()
