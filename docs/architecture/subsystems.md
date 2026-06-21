# Subsystems — phenodag

ADR-038 cross-link: see [ADR-038: Hexagonal port-adapter L4 policy](https://github.com/KooshaPari/phenotype-apps/blob/main/docs/adr/2026-06-18/ADR-038-hexagonal-port-adapter-l4-policy.md) for the canonical input/output port contract.

> L7 subsystem decomposition. Bounded contexts, ports, owned data, external
> dependencies, and failure modes for the phenodag Go module (DAG engine
> for the Phenotype fleet + GitHub lease / remote-claim subsystem).
> Companion to `README.md`. Initial decomposition 2026-06-21 (v16
> cycle-6 T1).

## Subsystem map

| Subsystem | Path | Responsibility | Owned data | Critical? |
|---|---|---|---|---|
| Core DAG | `phenodag.go`, `phenodag_v3.go`, `phenodag_extras.go`, `phenodag_dedup2.go` | DAG build, validation, cycle detect, topological sort, dedup, query API | DAG nodes, edges, in-memory cache | yes |
| Lease (GitHub) | `gh_repo_lease.go` | Acquire / renew / release GitHub repo leases (idempotent, advisory) | active lease table, expiry timer | yes |
| Remote claim | `internal/remoteclaim/` | Distributed claim acquisition: SQLite + file-lock + GitHub fallback | claim store, file lock, GH fallback state | yes |
| Queries | `queries.go` | Read API: shortest-path, ancestors, descendants, by-tag | query result cache | no |
| Database | `FLEET_DAG.db` (SQLite) | Local persistent store: nodes, edges, leases, claims | SQLite WAL | yes |
| Test surface | `phenodag_test.go`, `gh_repo_lease_test.go`, `internal/remoteclaim/remoteclaim_test.go` | Unit + integration tests; no production code | n/a | n/a |

## Port catalogue

### Input ports (consumed)

- `pheno-config::Config` (via `Configra`) — layered config.
- `pheno-errors::Error` envelope.
- `pheno-tracing` OTLP exporter.
- GitHub REST API (over HTTPS) — for lease acquisition (token from `GITHUB_TOKEN` env).
- `gh` CLI (subprocess, optional) — for lease pre-flight checks.
- SQLite (via `mattn/go-sqlite3`).

### Output ports (produced)

- `phenodag.Dag` (Go) — public DAG handle.
- `gh_repo_lease.Lease` (Go) — public lease handle.
- `remoteclaim.Claim` (Go) — public claim handle.
- JSON export of DAG for downstream consumers (`pheno-registry`, `pheno-vibecoding-guard`).
- Telemetry events on every DAG mutation (via `pheno-tracing`).

## External dependencies

| Dependency | Kind | Used by |
|---|---|---|
| `pheno-config` | Go module (path-dep) | config cascade |
| `pheno-errors` | Go module | error envelope |
| `pheno-tracing` | Go module (path-dep) | OTLP spans |
| GitHub REST API | HTTPS (token) | lease acquisition |
| `mattn/go-sqlite3` | Go module | local storage |
| `google/go-github` | Go module | GitHub client |
| `pelletier/go-toml` | Go module | config parse |
| `mattn/go-colorable` | Go module | log output |
| `spf13/cobra` | Go module | CLI (if exposed) |

## Failure modes

| Subsystem | Failure | Detection | Recovery |
|---|---|---|---|
| Core DAG | cycle detected | topological-sort fail | report cycle path; exit non-zero |
| Core DAG | duplicate node | `UNIQUE` constraint on insert | log warn; skip insert; continue |
| Core DAG | dedup miss | hash compare returns different | log warn; keep both; flag for review |
| Lease (GitHub) | rate limit | 403 + `X-RateLimit-Remaining: 0` | honor `Retry-After`; surface `LeaseRateLimited` |
| Lease (GitHub) | token expired | 401 from API | refresh token; retry once |
| Lease (GitHub) | lease already held by another | 422 from API | emit `LeaseHeld`; backoff |
| Lease (GitHub) | repo not found | 404 from API | surface `RepoMissing`; abort lease |
| Remote claim | local file lock held | flock `EWOULDBLOCK` | wait; max 30s; surface `LockTimeout` |
| Remote claim | SQLite WAL corruption | `database disk image is malformed` | rebuild from event log |
| Remote claim | GitHub fallback also fails | 5xx after retries | emit `ClaimUnacquirable`; do not retry |
| Queries | query timeout | `context.DeadlineExceeded` | partial result + warning |
| Database | disk full | `ENOSPC` on write | surface `DbFull`; refuse new writes |

## Change log

- 2026-06-21 — initial decomposition (v16 cycle-6 T1, L7). 5 production subsystems (1 cross-cutting storage row). ADR-038 cross-link added.
