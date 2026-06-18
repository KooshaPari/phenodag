#!/usr/bin/env python3
"""
DAG Dispatcher — Uses the `task` tool with forge-agent subagents to execute
sessions from the dag_plan. Creates a dispatch_manifest.json that is consumed
by the task tool dispatch loop.

Reads:
  ~/.forge/forge_resume_runs/dag_plan_2026_06_13.json
Writes:
  ~/.forge/forge_resume_runs/dispatch_manifest.json
"""

import json
import os

PLAN_PATH = "/Users/kooshapari/.forge/forge_resume_runs/dag_plan_2026_06_13.json"
MANIFEST_PATH = "/Users/kooshapari/.forge/forge_resume_runs/dispatch_manifest.json"

BATCH_SIZE = 8  # Subagents per task tool call (parallel limit)

def load_plan():
    with open(PLAN_PATH) as f:
        return json.load(f)

def build_manifest(plan):
    batches = []

    # --- Aggregate Fresh Sessions ---
    for agg in plan.get("aggregate_sessions", []):
        if agg["type"] == "aggregate_fresh":
            prompt = agg.get("prompt", "")
            tasks_text = []
            for t in agg["tasks"]:
                tasks_text.append(f"ID: {t['id']}\n{t['title']}")
            full_prompt = (
                f"You are a forge subagent working on: {agg['name']}\n\n"
                f"Complete all tasks below in order. Report completion status for each.\n\n"
                + "\n---\n".join(tasks_text)
            )
            batches.append({
                "name": agg["name"],
                "type": "aggregate_fresh",
                "prompt": full_prompt,
                "cwd": "/Users/kooshapari/CodeProjects/Phenotype/repos",
                "task_count": len(agg["tasks"]),
                "task_ids": [t["id"] for t in agg["tasks"]]
            })
        elif agg["type"] == "aggregate_resume":
            items = []
            for t in agg["tasks"]:
                items.append({
                    "name": f"resume:{t['id'][:8]}",
                    "type": "resume",
                    "conversation_id": t["id"],
                    "prompt": f"Continue: {t['title'][:200]}",
                    "cwd": "/Users/kooshapari/CodeProjects/Phenotype/repos",
                    "task_id": t["id"]
                })
            for i in range(0, len(items), BATCH_SIZE):
                batch = items[i:i + BATCH_SIZE]
                batches.append({
                    "name": f"resume_batch:{agg['name']}:{i//BATCH_SIZE}",
                    "type": "resume_batch",
                    "tasks": batch,
                    "batch_size": len(batch)
                })

    # --- Individual Resumes ---
    individual = plan.get("individual_resumes", [])
    for i in range(0, len(individual), BATCH_SIZE):
        batch = individual[i:i + BATCH_SIZE]
        tasks = []
        for t in batch:
            tasks.append({
                "name": f"resume:{t['id'][:8]}",
                "type": "resume",
                "conversation_id": t["id"],
                "prompt": f"Continue: {t['title'][:200]}",
                "cwd": "/Users/kooshapari/CodeProjects/Phenotype/repos",
                "task_id": t["id"]
            })
        batches.append({
            "name": f"resume_batch:individual:{i//BATCH_SIZE}",
            "type": "resume_batch",
            "tasks": tasks,
            "batch_size": len(tasks)
        })

    return batches

def save_manifest(batches):
    with open(MANIFEST_PATH, "w") as f:
        json.dump(batches, f, indent=2)
    print(f"[manifest] Wrote {len(batches)} batches to {MANIFEST_PATH}")
    fresh_count = sum(1 for b in batches if b["type"] == "aggregate_fresh")
    resume_batch_count = sum(1 for b in batches if b["type"] == "resume_batch")
    total_resume_tasks = sum(
        b["batch_size"] for b in batches if b["type"] == "resume_batch"
    )
    print(f"  aggregate_fresh sessions: {fresh_count}")
    print(f"  resume batches: {resume_batch_count}")
    print(f"  total resume tasks: {total_resume_tasks}")

if __name__ == "__main__":
    plan = load_plan()
    batches = build_manifest(plan)
    save_manifest(batches)
