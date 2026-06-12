# phenodag — multi-agent multi-project DAG (Go)

Headless single-binary Go CLI for a fleet work queue. Wraps [beads (`bd`)](https://github.com/gastownhall/beads) for storage semantics, but ships with a self-contained SQLite backend (modernc.org/sqlite, pure Go) so it runs offline.

**Why this exists**: 300+ local repos + 65+ GitHub repos, dozens of parallel agents, and constant risk of (a) two agents picking the same work and (b) two agents independently writing semantically-identical tasks with different wording. `phenodag` solves both with atomic SQLite claims + hybrid fuzzy-duplicate detection.

## Quick start

```bash
go build -mod=mod -o phenodag ./cmd/phenodag
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
  ├── internal/store     — SQLite (modernc.org/sqlite, pure Go) + POSIX flock
  ├── internal/similarity — hybrid: token-Jaccard × 0.6 + Levenshtein × 0.2 + repo × 0.2
  ├── internal/claim     — repo + branch + worktree claim store, PK on resource
  ├── internal/preset    — YAML loader (presets/v3-180.yaml, presets/empty.yaml)
  ├── internal/scan      — repo scanner (mangled-git + no-git tolerant)
  ├── internal/backfill  — side-DAG → gap promotion
  ├── internal/bd        — `bd` (beads) CLI wrapper, JSON over stdio (optional)
  ├── internal/graph     — DAG ops (ready/blocked/unblock, ingest, export)
  └── cmd/phenodag       — CLI router (init/seed/status/validate/pick/claim/release/heartbeat/done/fail/fill/scan/dupes/export)
```

## Width × Length

**Width 20 and length 100 are minima, not caps.** `init --width N --stages M` accepts any positive integer. v3-180 is a preset (6 stages × 20 width + 12 side-DAGs × 5), not a hard-coded shape.

To create your own preset, copy `presets/empty.yaml` and add tasks. Or write a Go file in `cmd/phenodag/preset_*.go` (see `preset_v3_180.go` for the pattern).

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
phenodag/
├── cmd/phenodag/        CLI entry + 14 subcommand files
├── internal/            8 packages (store, similarity, claim, graph, scan, backfill, bd, preset)
├── presets/             v3-180.yaml (180 tasks) + empty.yaml
├── docs/                AGENT_PROTOCOLS.md, PLACEMENT.md
├── examples/            agent_loop.sh
├── scripts/             generate_v3_preset.py
├── Makefile             build / test / install
├── go.mod               go 1.26, modernc.org/sqlite, gopkg.in/yaml.v3
└── README.md            (this file)
```

## Reproduction

```bash
git clone https://github.com/KooshaPari/phenodag.git
cd phenodag
go build -mod=mod -o phenodag ./cmd/phenodag
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

MIT
