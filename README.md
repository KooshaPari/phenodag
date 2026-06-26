<!-- AI-DD-META:START -->
<!-- This repository is planned, maintained, and managed by AI Agents only. -->
<!-- Slop issues are expected and intentionally present as part of an HITL-less -->
<!-- /minimized AI-DD metaproject of learning, refining, and building brute-force -->
<!-- training for both agents and the human operator. -->
![Downloads](https://img.shields.io/github/downloads/KooshaPari/phenodag/total?style=flat-square&label=downloads&color=blue)
![GitHub release](https://img.shields.io/github/v/release/KooshaPari/phenodag?style=flat-square&label=release)
![License](https://img.shields.io/github/license/KooshaPari/phenodag?style=flat-square)
![AI-Slop](https://img.shields.io/badge/AI--DD-Slop%20Expected-orange?style=flat-square)
![AI-Only-Maintained](https://img.shields.io/badge/Planned%20%26%20Maintained%20by-AI%20Agents%20Only-red?style=flat-square)
![HITL-less](https://img.shields.io/badge/HITL--less%20AI--DD-metaproject-yellow?style=flat-square)

> ⚠️ **AI-Agent-Only Repository**
>
> This repo is **planned, maintained, and managed exclusively by AI Agents**.
> Slop issues, rough edges, and AI artifacts are **expected and intentionally
> present** as part of an **HITL-less / minimized AI-DD** metaproject focused
> on learning, refining, and brute-force training both the agents and the
> human operator. Bug reports and contributions are still welcome, but please
> expect AI-generated code, comments, and documentation throughout.
<!-- AI-DD-META:END -->
# phenodag — multi-agent multi-project DAG (Go)

Headless single-binary Go CLI for a fleet work queue. Self-contained SQLite backend (modernc.org/sqlite, pure Go) so it runs offline. Sources: 5-file root (`phenodag.go` ~2.1K LOC + `phenodag_v3.go` ~0.6K + `phenodag_dedup2.go` ~0.1K + `phenodag_extras.go` ~1.5K + `queries.go` ~0.4K = ~4.7K LOC) + `internal/remoteclaim/` (POSIX flock + SQLite store).

> **Status: superset-merge refactor in progress (phenodag#5).** The 5-file root carries a `*_Port` duplicate layer (472 hits across the 5 files, 273 unique function pairs) that the FR is consolidating. Use `make verify-no-port-dupes` to enforce. See `scripts/.port-dupes-allowlist` for the current exception list.

**Why this exists**: 300+ local repos + 65+ GitHub repos, dozens of parallel agents, and constant risk of (a) two agents picking the same work and (b) two agents independently writing semantically-identical tasks with different wording. `phenodag` solves both with atomic SQLite claims + hybrid fuzzy-duplicate detection.

## Quick start

```bash
go build -mod=mod -o phenodag .
./phenodag init    --width 20 --stages 6 --db FLEET_DAG.db
./phenodag seed    --preset v3-180 --db FLEET_DAG.db     # 120 core + 60 side = 180 tasks
./phenodag status  --db FLEET_DAG.db
./phenodag pick    --agent me --db FLEET_DAG.db          # atomic
./phenodag claim   --agent me --repo HexaKit --branch feat/x --task <id> --db FLEET_DAG.db
./phenodag done    --agent me --task <id> --db FLEET_DAG.db
./phenodag dupes   --threshold 0.4 --db FLEET_DAG.db     # fuzzy duplicate groups
./phenodag fill    --agent me --db FLEET_DAG.db          # promote side-DAGs into stage gaps
./phenodag export  --db FLEET_DAG.db --out FLEET_DAG.md
```

## Architecture

```
phenodag (this binary)
  ├── phenodag.go              — single-file CLI (init/seed/status/validate/pick/claim/release/heartbeat/done/fail/fill/scan/dupes/export/seed-yaml)
  ├── internal/preset/         — YAML loader (`seed-yaml --preset <name>`)  ✅ v1 schema frozen
  ├── internal/remoteclaim/    — POSIX flock + SQLite store (PK on (agent, repo, branch, worktree))
  ├── presets/                 — 7 built-in YAML presets (v3-180, melosviz-185, agileplus-50, tracera-50, mcp-fleet-60, mcp-fleet-90, empty)
  ├── scripts/                 — generate_preset.py (canonical YAML generator)
  ├── Makefile                 — build / test / install / release / smoke
  ├── go.mod                   — go 1.26, modernc.org/sqlite, gopkg.in/yaml.v3
  └── README.md                — (this file)
```

## Width × Length

**Width 20 and length 100 are minima, not caps.** `init --width N --stages M` accepts any positive integer. v3-180 is a preset (6 stages × 20 width + 12 side-DAGs × 5), not a hard-coded shape.

To create your own preset, use `seed-yaml` (the externalized loader) or `seed` (the built-in switch). For `seed-yaml`, author a YAML file under `presets/` and pass `--preset <name>`:

```bash
# Use one of the 7 built-in YAML presets
./phenodag seed-yaml --preset v3-180 --db FLEET_DAG.db
./phenodag seed-yaml --list                  # show all available presets

# Or generate a new preset
python scripts/generate_preset.py forge-120 8 15 0 0 --repo forge
./phenodag seed-yaml --preset forge-120 --db FLEET_DAG.db
```

The legacy `seed` subcommand is still available for backwards compatibility (the 6 hard-coded presets: v3-180, melosviz-185, agileplus-50, tracera-50, mcp-fleet-60, mcp-fleet-90).

## Presets

Built-in presets seeded with `phenodag seed --preset <name>`:

| Preset | Core | Side | Total | Use it for |
| --- | ---: | ---: | ---: | --- |
| `v3-180` | 120 | 60 | 180 | V3 execution-log fleet (default; 6 stages × 20 width + 12 side-DAGs × 5) |
| `melosviz-185` | 140 | 45 | 185 | Melosviz fleet (7 stages × 20 width + 9 side-DAGs × 5) |
| `agileplus-50` | 20 | 30 | 50 | AgilePlus fleet (4 stages × 5 width + 6 side-DAGs × 5; use `fill` to fill width-20 slots) |
| `tracera-50` | 20 | 30 | 50 | Tracera fleet (4 stages × 5 width + 6 side-DAGs × 5; use `fill` to fill width-20 slots) |
| `mcp-fleet-60` | 30 | 30 | 60 | MCP polyrepo execution plan (6 stages × 5 width + 6 side-DAGs × 5; includes `sd-dagctl` merge pool) |
| `mcp-fleet-90` | 90 | 60 | 150 | Post-fleet-60 depth wave (6 stages × 15 width + 12 side-DAGs × 5) |

`--stages` on `init` must be `≥ 6` for `v3-180`, `≥ 7` for `melosviz-185`, `≥ 5` for `agileplus-50` / `tracera-50` / `mcp-fleet-60` / `mcp-fleet-90`. `--width` only needs to be large enough to hold the core tasks per stage (or larger — `fill` will pack side-DAGs into the slack).

See [docs/dagctl-merge-status.md](docs/dagctl-merge-status.md) for dagctl → phenodag superset merge progress.

## Multi-agent concurrency

- `pick` is atomic via `BEGIN IMMEDIATE` transaction + `withLock` (POSIX `flock` LOCK_EX|LOCK_NB)
- 5 agents picking simultaneously get 5 distinct tasks (verified)
- `claim` is atomic and enforces unique (agent, repo, branch, worktree) tuples
- `heartbeat` updates `last_seen`; `reclaim` reaps agents whose heartbeats are older than `StaleThresholdMin`

## Fuzzy duplicate detection

- Token-Jaccard (set overlap of normalized tokens)
- Levenshtein distance normalized by max length
- Repo-overlap (does the candidate claim the same repo?)

`score = 0.6 × jaccard + 0.2 × (1 - levenshtein_norm) + 0.2 × repo_overlap`

Groups with score ≥ `--threshold` are persisted to `duplicate_groups` (member IDs + root + score).

15 groups found on v3-180 at threshold 0.4 (Layer-1 audit templates, Layer-2 hygiene variants, etc).

## Headless / stateless

- One management file (`*.db` SQLite)
- One preset file (`*.yaml`)
- No daemon, no broker, no Redis, no server

## Layout

```
phenodag/                           # this repo (Go module)
├── phenodag.go                     # main entrypoint + claim/heartbeat/lifecycle  (~2.1K LOC)
├── phenodag_v3.go                  # sqlite event store + fleet queries          (~0.6K)
├── phenodag_dedup2.go              # simhash/hamming/jaccard primitives          (~0.1K)
├── phenodag_extras.go              # extra cmd_* + output formatters              (~1.5K)
├── queries.go                      # SQL helpers                                  (~0.4K)
├── internal/remoteclaim/           # POSIX flock + SQLite claim store
├── presets/                        # repos.toml (fleet → repo → path assignments)
├── docs/                           # design notes, schema diagrams
└── scripts/
    ├── mod-hygiene.sh              # POSIX go.mod guard (commit-time)
    └── verify-no-port-dupes.sh     # phenodag#5 guardrail (CI + commit-time)
```

> The 5-file root carries a `*_Port` duplicate layer (legacy CLI port to the v2 interface) that phenodag#5 is consolidating. Use `make verify-no-port-dupes` to keep the count from regressing.

## FR #5 Status (2026-06-26)

This is the **scoped, shippable increment** for the `Phase-4b: Complete superset-merge` (size:XXL, kilo-auto-fix insufficient) tracked in issue **#5**.

### What shipped in this increment

- **Baseline metric**: `make verify-no-port-dupes` captures the current `*_Port` duplicate count (472 hits across the 5 root files). The success criterion in #5 is to drop this to <=3% (≤14 hits).
- **Allowlist** (`scripts/.port-dupes-allowlist`): documents the current count so the script passes today and fails loudly if the count regresses during unrelated work.
- **Guardrail** (`scripts/verify-no-port-dupes.sh`): POSIX shell script, zero external deps, mirrors the `mod-hygiene.sh` style. Diff against the allowlist, exit non-zero on regression.
- **CI integration**: `make verify-no-port-dupes` target added alongside `mod-hygiene` and `preset-validate`. Wire into `.github/workflows/ci.yml` as a required check.

### What is NOT in this increment

- Actual consolidation of the 5 root files into one (`phenodag.go` 2088 + `phenodag_v3.go` 553 + `phenodag_dedup2.go` 143 + `phenodag_extras.go` 1453 + `queries.go` 445 = ~4682 LOC) — a true XXL refactor, deferred to subsequent PRs per the FR's milestone structure.
- Output-formatter consolidation (ASCII vs Mermaid Gantt, JSON vs YAML claim payloads).
- 3-claim-system unification (`cmdRemoteClaim` vs `cmdRemoteClaimPort`, etc.).
- Tightening the migration gate or deprecating `phenodag v0.4` binaries.

### Reproducing the baseline

```bash
make verify-no-port-dupes
# Expected: PASS (count matches allowlist of 472)
# To grow the allowlist (after intentional consolidation): edit scripts/.port-dupes-allowlist
```

## Reproduction

```bash
git clone https://github.com/KooshaPari/phenodag.git
cd phenodag
go build -mod=mod -o phenodag .
go test -mod=mod ./...
./phenodag init    --width 20 --stages 6 --db test.db
./phenodag seed    --preset v3-180 --db test.db
./phenodag status  --db test.db
```

## Companion projects

- **dagctl** (`https://github.com/KooshaPari/dagctl`) — predecessor in pure Go, single-file DAG CLI. phenodag is the modular successor.
- **agileplus-spec-harmonizer** — Rust crate that harmonizes gsd/openspec/bmad/kitty spec formats. Both projects share `phenotype-trace-core` (planned) for the libification.
- **Tracera** (`Tracera/crates/tracera-core/`) — long-term Product Knowledge Graph + Autograder + Agent Runtime; consumes `Claim` objects in `phenotype-trace-core` format.

## License

MIT — see [`LICENSE`](./LICENSE).
