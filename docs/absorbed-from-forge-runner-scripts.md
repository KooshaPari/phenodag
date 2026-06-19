# forge-runner-scripts absorption

**Date:** 2026-06-18
**Source:** `KooshaPari/forge-runner-scripts` (33 files, 5,597 LOC)
**Target:** `KooshaPari/phenodag` (canonical dispatch substrate per ADR-013)
**L5 ID:** L5-113
**Audit:** `findings/2026-06-18-L5-113-forge-runner-scripts-absorption-audit.md`

## 5-step substrate placement (per ADR-023 Rule 3)

1. **Intent:** the 17 shell scripts in `forge-runner-scripts/bin/` and `bin/autoqueue/` were a 2026-06-13 snapshot of the dispatch substrate. They served their purpose: drive 232+ forge subagent conversations for the v6/v7 wave.
2. **Placement:** per ADR-013, dispatching lives in `phenodag` (Go-typed, modular, tested, in-tree CI). The shell scripts were a pre-Go-rewrite prototype.
3. **Target:** `phenodag` (this repo). 23/25 items already `SUPERSEDED_PARITY/BETTER` by `phenodag:feat/absorption-patches` head `9b271ce`.
4. **Parity status:** 23 superseded, 7 unique provenance items preserved in `docs/absorbed-from-forge-runner-scripts/`, 0 last-resort exceptions.
5. **Last-resort:** none. Source repo archived (not deleted) by user via GitHub UI after this PR merges.

## What was migrated

- `PROVENANCE.md` — 5-step substrate placement rule + audit trail
- `INDEX.md` — file index pattern
- `specs/loop_feature_{request,spec}.md` — loop FR/spec (now adopted as de-facto standard by phenodag)
- `specs/loop_task.txt` — loop task spec
- `docs/INSTALL.md` — proven `forge --conversation-id` CLI install procedure
- `docs/ARCHITECTURE.md` — pre-Go architecture
- `commands/loop.md` — loop command reference

## What was NOT migrated (and why)

- `bin/subagents-orchestration/*.sh` (17 shell scripts) — fully superseded by `phenodag` Go modules. No unique logic.
- 782 hardcoded `forge --conversation-id <uuid>` invocations across the 17 scripts — point-in-time session state; would launch stale forge conversations.
- `install.sh` — replaced by `phenodag/Makefile` install target.
- `bin/autoqueue/*.sh` — replaced by `phenodag/queue/` module.

## References

- L5-113 audit: `findings/2026-06-18-L5-113-forge-runner-scripts-absorption-audit.md`
- ADR-013: `docs/adr/2026-06-15/ADR-013-pheno-mcp-router-substrate.md` (analogue for mcp-router)
- Prior retirements: dagctl → phenodag, kwality → phenotype-tooling, AuthKit-ts → AuthKit, dinoforge-packs → Dino
- 4-repo retirement wave (L5-109 → L5-113): `findings/2026-06-18-L5-109-4-repo-retirement.md`
