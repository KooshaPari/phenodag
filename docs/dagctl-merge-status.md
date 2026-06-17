# dagctl → phenodag merge status (sd-dagctl pool)

- **Date:** 2026-06-17
- **Side-DAG:** `sd-dagctl` (mcp-fleet-60 preset)
- **Repos:**
  - phenodag — `C:/Users/koosh/dev/phenodag` (canonical; v0.3.0 on `main`)
  - dagctl — `C:/Users/koosh/dev/dagctl` (remote: [KooshaPari/dagctl](https://github.com/KooshaPari/dagctl), v3.3.0, **not archived**)
- **ADR:** [ADR-dag-superset-merge.md](./adr/ADR-dag-superset-merge.md) (Accepted)

## sd-dagctl pool task status

| Task ID | Title | Status | Notes |
| --- | --- | --- | --- |
| **sd-dagctl-01** | ADR superset | **Done** | `docs/adr/ADR-dag-superset-merge.md` landed on `main`; status Accepted |
| **sd-dagctl-02** | remoteclaim port | **Done** | `internal/remoteclaim/` + `remote-*` CLI commands on `main` |
| **sd-dagctl-03** | preset tests | **Done (stubs)** | Unit stubs for `mcp-fleet-60` core/side shape + `sd-dagctl` IDs; integration pick/claim deferred |
| **sd-dagctl-04** | release rc | **Deferred** | Cut `v1.0.0-rc.1` after superset PR #1 rebased/closed and README/changelog updated |
| **sd-dagctl-05** | archive dagctl | **Deferred** | Requires final `v3.3.x` redirect README + GitHub archive action; repo still active |

## dagctl repo inventory

| Location | Present | Version / branch |
| --- | --- | --- |
| Local clone | Yes | `C:/Users/koosh/dev/dagctl`, `main` @ v3.3.0 |
| GitHub remote | Yes | https://github.com/KooshaPari/dagctl |
| Archived | No | Latest release: `v3.3.0` (2026-06-12) |

## Merge now (already on phenodag `main`)

These dagctl capabilities are **merged** into phenodag via PR #2 and the superset port:

| Capability | phenodag location |
| --- | --- |
| `internal/remoteclaim` package | `internal/remoteclaim/*.go` + tests |
| Remote claim CLI (`remote-claim`, `remote-heartbeat`, …) | `phenodag_extras.go` |
| v3 engine (`seed3`, `extend3-v2`, `extend3-v3`) | `phenodag_v3.go` |
| Meta commands (`worktree-claim`, `agent-stats`, `diff`, `critical-path`) | `phenodag_extras.go` |
| Test/maintenance (`doctor`, `thrash`, `sweep`, `dispatch`) | `phenodag_extras.go` |
| Viz (`gantt`, `mermaid`, `burndown`, `dashboard`, `csv`, `html`) | `phenodag_extras.go` |
| Dedup extras (`dedup-explain`, simhash) | `phenodag_dedup2.go` |
| Task ops (`add`, `merge`, `next`, `where`, `topo`, `promote`, `completion`) | `phenodag_extras.go` |
| HTML template embed | `dagctl_dag_template.html` |
| Superset merge ADR | `docs/adr/ADR-dag-superset-merge.md` |

## Defer (post-rc or next phase)

| dagctl-only item | Reason to defer | Target phase |
| --- | --- | --- |
| **`seed-requirements`** (FR/NFR catalog) | Tracera-specific; no phenodag consumer yet | Phase 4 downstream |
| **`extend3`** (v1 extend, not v2/v3) | Superseded by `extend3-v2` / `extend3-v3` ports; no preset YAML | Drop or alias to `extend3-v2` |
| **Real POSIX `flock` in `withLock`** | phenodag uses SQLite WAL + `SetMaxOpenConns(1)`; flock lives in `internal/remoteclaim/flock.go` only | Phase 2 schema union |
| **Hardcoded `repoSet` (24 fleet repos)** | phenodag scan is path-driven; preset YAML preferred | Optional preset YAML |
| **`FLEET_DAG_v3.db` default DB name** | phenodag defaults to `phenodag.db`; auto-detect planned in ADR | Phase 2 |
| **`-db` single-dash flag aliases** | phenodag uses `--db`; shim in ADR Phase 4 | v1.x compat window |
| **`v1.0.0-rc.1` release** | Needs changelog, README command union table, CI green on all platforms | sd-dagctl-04 |
| **Archive dagctl repo** | Downstream `go install` notice needed; final tag + README redirect | sd-dagctl-05 |
| **mcp-fleet-60 pick/claim integration test** | Requires temp DB + subprocess; stub counts only for now | Follow-up PR |
| **Close stale PR #1** | Superset content already on `main` via #2; rebase or close draft | Housekeeping |

## phenodag-only (keep; do not regress)

| Capability | Why keep |
| --- | --- |
| YAML preset loader (`seed --preset`) | Data-driven presets without recompile |
| 5 built-in presets (`v3-180`, `melosviz-185`, `agileplus-50`, `tracera-50`, `mcp-fleet-60`) | Fleet-specific corpora |
| Single-file core + split ports | Readable core, modular extras |
| `Makefile` build/test/install | CI and local dev ergonomics |

## Recommended next steps

1. **Close or supersede PR #1** — content is on `main`; avoid duplicate review surface.
2. **Merge this PR** — sd-dagctl-03 preset test stubs + merge-status doc.
3. **sd-dagctl-04** — bump to `v1.0.0-rc.1`, update README command table, tag release.
4. **sd-dagctl-05** — dagctl README redirect → phenodag, tag `v3.3.1`, GitHub archive.
