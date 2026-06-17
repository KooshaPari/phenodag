# ADR: phenodag + dagctl SUPERSET merge

- **Status:** Accepted
- **Date:** 2026-06-16
- **Repos:**
  - `phenodag` — `C:/Users/koosh/Dev/phenodag` (this repo), v0.3.0
  - `dagctl` — `C:/Users/koosh/Dev/dagctl`, v3.2.0
- **Driver:** superset union; **retire nothing**
- **Author:** KooshaPari <kooshapari@gmail.com>
- **Co-author:** Claude Opus 4.8 <noreply@anthropic.com>

## Context

`phenodag` and `dagctl` are sibling CLIs for the same problem space — a
multi-agent multi-project fleet DAG backed by a single SQLite file. They
diverged in style (single-file vs split source) and in surface area
(15 vs 38+ commands) but share the same data model primitives: tasks,
edges, claims, duplicate groups, repos, side-DAGs, agents, WAL SQLite,
`pick`/`claim`/`done`/`dupes`/`export` core flow.

Maintaining both is duplication tax. Neither repo is a strict subset of
the other — each carries features the other does not. A union merge
preserves everything, retires nothing.

## Inventory: what each uniquely has

### phenodag-only (not in dagctl)

| Capability | Where | Why it matters |
| --- | --- | --- |
| **YAML preset loader** | `phenodag.go` + `gopkg.in/yaml.v3` dep | `seed --preset v3-180\|melosviz-185\|agileplus-50\|tracera-50` is data-driven; users can author new presets without recompiling |
| **4 shipped presets** | `phenodag.go` | 180 / 185 / 50 / 50 task corpora out of the box |
| **Single-file Go source** | `phenodag.go` (1606 lines) | One file, one `go build` — easy fork / vendor / read in full |
| **`Makefile`** | root | `build` / `test` / `install` targets |
| **15-command core surface** | `phenodag.go` | init, seed, status, validate, pick, claim, release, heartbeat, reclaim, done, fail, fill, scan, dupes, export |
| **Mangled-git + no-git tolerant scanner** | `cmdScan` | handles clone-of-clone worktrees and non-git dirs |
| **No-build preset authoring** | `presets/empty.yaml` | users add tasks via YAML, not Go |
| **MIT license + AI-DD badge block** | `README.md` | governance metadata for the AI-DD metaproject |

### dagctl-only (not in phenodag)

| Capability | Where | Why it matters |
| --- | --- | --- |
| **internal/remoteclaim package** | `internal/remoteclaim/{types,store,sqlite,flock,github,local}.go` + tests | TTL + heartbeat + fencing-epoch claim model; `Claim` struct with `State`/`Kind`/`Reason` |
| **Remote claim transport** | `dagctl_remote_claim.go` | GitHub issues used as coordination bus; `FLEET_REMOTE_CLAIMS.db` separate DB |
| **POSIX `flock` file locking** | `internal/remoteclaim/flock.go` | `LOCK_EX\|LOCK_NB` lockfile; phenodag's `withLock` is a documented no-op stub |
| **38+ command surface** | 9 source files split by feature | see list below |
| **`worktree-claim`** | `dagctl_meta2.go` | claim a worktree as a first-class resource |
| **`agent-stats`** | `dagctl_meta2.go` | per-agent completion stats |
| **`diff`** | `dagctl_meta2.go` | task/claim diff view |
| **`critical-path`** | `dagctl_meta2.go` | critical-path analysis |
| **`doctor`** | `dagctl_test2.go` | DB health check |
| **`thrash`** | `dagctl_test2.go` | DB stress test |
| **`sweep`** | `dagctl_test2.go` | clean up old claims/agents |
| **`dispatch`** | `dagctl_test2.go` | dispatch work to a sub-agent |
| **`gantt` / `mermaid` / `burndown`** | `dagctl_viz2.go` | visualisations (Mermaid + ASCII) |
| **`dashboard`** | `dagctl_extras.go` | dashboard print |
| **`where`** | `dagctl_extras.go` | find task by partial id |
| **`topo`** | `dagctl_extras.go` | topological order |
| **`csv` / `html`** | `dagctl_extras.go` | extra export formats |
| **`promote`** | `dagctl_extras.go` | promote side-DAG task into a stage |
| **`completion`** | `dagctl_extras.go` | completion stats |
| **`merge`** | `dagctl.go` | merge two tasks |
| **`next`** | `dagctl.go` | show next task to work on |
| **`add`** | `dagctl.go` | add a new task |
| **`dedup-explain`** | `dagctl_dedup2.go` | explain why two tasks are duplicates |
| **simhash + n-gram dedup** | `dagctl_dedup2.go` | richer similarity primitives |
| **`seed-requirements`** | — | seed FR/NFR catalogs (Tracera traceability) |
| **`extend3` / `extend3-v2`** | `dagctl_v3_seed.go`, `dagctl_v3_extend2.go`, `dagctl_v3_extend3.go` | additive layer + side-DAG growth (L7, L8) |
| **HTML template embed** | `dagctl_dag_template.html` | interactive HTML export |
| **Domain constants** | `dagctl.go` | `Subproject`, `Status`, `Kind` enums (audit/hygiene/tooling/governance/sota/test/release) |
| **POSIX `flock` integration** | `withLock` actually locks | phenodag's stub is replaced by a real lockfile |
| **Hardcoded `repoSet`** | `dagctl.go` | 24 named fleet repos seeded into scan |
| **`FLEET_DAG_v3.db` default** | `dagctl.go` | v3-shaped default DB name |

### Shared (both)

- SQLite (modernc.org/sqlite, pure Go) + WAL
- Tasks / edges / claims / agents / repos / side_dags / duplicate_groups tables
- `pick` (atomic) / `claim` / `done` / `fail` / `heartbeat` / `reclaim` / `dupes` / `fill` / `export` / `scan` / `init` / `seed` / `status` / `validate` / `release`
- Hybrid similarity `0.6 × Jaccard + 0.2 × Levenshtein + 0.2 × repo_overlap`
- Stale-threshold reclaim (30 min)
- Width × stages minima-not-caps

## Decision

Merge into a **single superset binary** in the `phenodag` repo. Naming
and binary name TBD; current placeholder: `phenodag` stays the canonical
name (it's the modular successor and the GitHub Releases binary is
already under that name per `README.md`). The `dagctl` repo is archived,
not deleted, with a final release tagged and a `README.md` redirect
pointing at `phenodag` for active development.

**Retire nothing.** Every command, every table, every package from both
sides lands in the superset. Commands are gated behind a `cmd/` router;
deprecated dagctl-only commands may alias to phenodag's command names
(`worktree-claim` → `claim --worktree`) but remain callable by their
dagctl names for one major version for downstream scripts.

## Merge plan

### Phase 0 — inventory & ADR (this commit)

- ✅ This ADR lands in `docs/adr/ADR-dag-superset-merge.md`.
- ✅ Branch: `feat/dag-superset-merge`.

### Phase 1 — package adoption

1. **Copy `dagctl/internal/remoteclaim/`** verbatim into
   `phenodag/internal/remoteclaim/`. The package is already
   self-contained (no dagctl-internal imports); only path is to add it
   to the phenodag module.
2. **Copy `dagctl/dagctl_*.go` source files** (8 files) into a new
   `phenodag/cmd/legacy_dagctl/` subpackage or directly into
   `phenodag.go` as `cmd*` functions, depending on the desired source
   layout. Preserve file-per-feature split from dagctl.
3. **Copy `dagctl/dagctl_dag_template.html`** into the embedded assets.
4. **Add `golang.org/x/sys` or syscall import for `flock`** — phenodag's
   `withLock` stub at `phenodag.go:116` is replaced with a real POSIX
   `flock` implementation from `dagctl/internal/remoteclaim/flock.go`.
5. **Add YAML preset migration path**: dagctl's hardcoded seed3/extend3
   are also emitted as `v3-180.yaml` style presets so they work with
   phenodag's `seed --preset` flow. (Both stay callable; presets are
   just one more way to seed.)

### Phase 2 — schema & command union

1. **Add `subproject`, `status`, `kind` enum columns** if not already
   present. Verify against `dagctl.go:48-80` constants. Map
   `phenodag.kind` ↔ `dagctl.Kind` 1:1.
2. **Add `FLEET_REMOTE_CLAIMS.db` opt-in support** behind
   `--remote-claim-db` flag and `REMOTE_CLAIMS_DB` env var. Default off
   for back-compat.
3. **Command surface union**: every dagctl-only command listed above
   becomes a `phenodag <cmd>` subcommand. Flag names mapped
   `-db` → `--db`, `-agent` → `--agent`, etc. (phenodag already uses
   double-dash; dagctl mixes both — normalise to `--` on merge.)
4. **Default DB name**: keep `FLEET_DAG_v3.db` only when an existing
   file with that name is detected, otherwise default to
   `phenodag.db` (current phenodag default).

### Phase 3 — testing & release

1. **Port `dagctl_test2.go` and `internal/remoteclaim/*_test.go`** into
   `phenodag/`. Verify `go test ./...` is green with 0 races at 5+
   parallel agents (the existing phenodag concurrency claim).
2. **Cut a `v1.0.0-rc.1` release** on the merged `main`.
3. **Tag a final `v3.2.x` on the dagctl repo** with a `README.md` note:
   *"Superseded by `phenodag` ≥ v1.0.0. See
   https://github.com/KooshaPari/phenodag for active development."*
4. **Archive (do not delete) the `dagctl` repo** with the GitHub
   `archive` action; preserve git history, releases, and Issues.

### Phase 4 — downstream

1. **Changelog** entry per dagctl-only command mapping.
2. **Deprecation window**: 1 major version (i.e. v1.x of phenodag keeps
   every dagctl command name callable; v2.0 may alias-warn then remove).
3. **Backwards-compat shim** for `dagctl`-style flags (`-db` accepted as
   alias for `--db`).

## Non-goals

- **No behaviour change** to the shared core (`pick`/`claim`/`done`/
  `dupes`/`fill`/`export`/`scan`) beyond bug fixes found during the
  merge.
- **No schema break**: any new column is nullable or has a `DEFAULT ''`
  migration guarded with `ALTER TABLE … DEFAULT` (phenodag's
  `migrate()` pattern at `phenodag.go:54` is the template).
- **No README rewrite yet**: README updates land in the Phase 3 release
  commit, not in the ADR commit.

## Risks

| Risk | Mitigation |
| --- | --- |
| `go.mod` dep conflict (dagctl uses `gopkg.in/yaml.v3` already; phenodag also uses it) | Resolved at copy time; confirm `go mod tidy` after Phase 1. |
| Flag-style mismatch (`-db` vs `--db`) | Alias shim in Phase 4; warn in v1.x; remove in v2.0. |
| `FLEET_DAG_v3.db` vs `phenodag.db` default collision | Auto-detect existing file by name; emit a one-time warning on first run. |
| dagctl test suite assumes POSIX flock path; Windows CI may not exercise it | Skip flock tests on `windows/*` in `internal/remoteclaim/flock_test.go` (already done in dagctl). |
| Archive of dagctl repo could surprise downstream scripts that `go install` it | Final `v3.2.x` tag + `README.md` redirect + GitHub archive notice gives one release cycle of notice. |

## Decision record

- **2026-06-16**: ADR proposed on `feat/dag-superset-merge`.
- **2026-06-16**: ADR Accepted; Phase 1 package adoption landed on `main`.
- **2026-06-17**: sd-dagctl pool (tasks 01–03) complete; 04–05 deferred per
  [dagctl-merge-status.md](../dagctl-merge-status.md).
