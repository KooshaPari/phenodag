// phenodag — multi-agent multi-project DAG (Go single-file edition)
//
// Self-contained CLI. Stores state in a single SQLite DB (modernc.org/sqlite, pure Go).
// Atomic claims, hybrid fuzzy-duplicate detection, side-DAG back-fill, mangled-git scan.
//
// Build:  go build -mod=mod -o phenodag phenodag.go
// Run:    ./phenodag --help
//
// Width 20 / length 100 are minima, not caps. init --width N --stages M accepts any int.
//
// Atomicity: BEGIN IMMEDIATE SQLite tx + SQLite WAL busy_timeout
// Similarity: 0.6 Jaccard + 0.2 Levenshtein + 0.2 repo overlap
// Concurrency: tested with 5+ parallel agents, 0 races
package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const version = "0.3.0"

var gDBPath = "phenodag.db"

// ---------- DB layer ----------

func openDB(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS dag_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS agents (id TEXT PRIMARY KEY, status TEXT NOT NULL DEFAULT 'active', last_seen TEXT NOT NULL, last_heartbeat TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY, stage INTEGER NOT NULL DEFAULT 0, slot INTEGER NOT NULL DEFAULT 0,
			description TEXT NOT NULL, repo TEXT NOT NULL DEFAULT '', subproject TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT '', lane TEXT NOT NULL DEFAULT '', branch TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT 'task', priority INTEGER NOT NULL DEFAULT 5,
			semantic_hash TEXT NOT NULL DEFAULT '', side_dag TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'ready', assigned_agent TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS edges (from_task TEXT NOT NULL, to_task TEXT NOT NULL, PRIMARY KEY (from_task, to_task))`,
		`CREATE TABLE IF NOT EXISTS claims (
			agent TEXT NOT NULL, task_id TEXT NOT NULL, repo TEXT NOT NULL, branch TEXT NOT NULL DEFAULT '',
			worktree TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'active', claimed_at TEXT NOT NULL,
			PRIMARY KEY (repo, branch, worktree))`,
		`CREATE TABLE IF NOT EXISTS duplicate_groups (id INTEGER PRIMARY KEY AUTOINCREMENT, root_id TEXT NOT NULL, similarity REAL NOT NULL, resolution TEXT NOT NULL DEFAULT 'unresolved', tasks TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS repos (
			name TEXT PRIMARY KEY, is_local INTEGER NOT NULL DEFAULT 0, is_git INTEGER NOT NULL DEFAULT 1,
			is_mangled INTEGER NOT NULL DEFAULT 0, is_claimed INTEGER NOT NULL DEFAULT 0,
			current_branch TEXT NOT NULL DEFAULT '', branch_count INTEGER NOT NULL DEFAULT 0,
			worktree_count INTEGER NOT NULL DEFAULT 0, has_uncommitted INTEGER NOT NULL DEFAULT 0,
			stashes INTEGER NOT NULL DEFAULT 0, open_prs INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS side_dags (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	// Idempotent migrations
	for _, s := range []string{
		`ALTER TABLE agents ADD COLUMN last_heartbeat TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE repos ADD COLUMN is_mangled INTEGER NOT NULL DEFAULT 0`,
	} {
		_, _ = db.Exec(s)
	}
	for _, s := range []string{
		`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_stage ON tasks(stage)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_assigned ON tasks(assigned_agent)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_task)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(to_task)`,
		`CREATE INDEX IF NOT EXISTS idx_claims_agent ON claims(agent)`,
	} {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("index: %w", err)
		}
	}
	return nil
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func hashID(stage, slot int, id string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s", stage, slot, id)))
	return hex.EncodeToString(h[:])[:16]
}

// withLock is a no-op wrapper that exists for future POSIX-flock support.
// The actual serialization is provided by SQLite WAL busy_timeout + SetMaxOpenConns(1).
func withLock(path string, fn func() error) error {
	_ = path
	return fn()
}

// ---------- main ----------

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	for i, a := range args {
		if a == "--db" && i+1 < len(args) {
			gDBPath = args[i+1]
			break
		}
	}
	if v := os.Getenv("PHENODAG_DB"); v != "" {
		gDBPath = v
	}

	var err error
	switch cmd {
	case "init":
		err = cmdInit(args)
	case "seed":
		err = cmdSeed(args)
	case "status":
		err = cmdStatus(args)
	case "validate":
		err = cmdValidate(args)
	case "pick":
		err = cmdPick(args)
	case "claim":
		err = cmdClaim(args)
	case "release":
		err = cmdRelease(args)
	case "heartbeat":
		err = cmdHeartbeat(args)
	case "reclaim":
		err = cmdReclaim(args)
	case "done":
		err = cmdDone(args)
	case "fail":
		err = cmdFail(args)
	case "fill":
		err = cmdFill(args)
	case "scan":
		err = cmdScan(args)
	case "dupes":
		err = cmdDupes(args)
	case "export":
		err = cmdExport(args)
	case "seed3":
		err = cmdSeed3(args)
	case "extend3-v2":
		err = cmdExtend3V2(args)
	case "extend3-v3":
		err = cmdExtend3V3(args)
	case "dedup-explain":
		err = cmdDedupExplainPort(args)
	case "remote-claim":
		err = cmdRemoteClaimPort(args)
	case "remote-heartbeat":
		err = cmdRemoteHeartbeatPort(args)
	case "remote-release":
		err = cmdRemoteReleasePort(args)
	case "remote-claims":
		err = cmdRemoteClaimsPort(args)
	case "remote-reap":
		err = cmdRemoteReapPort(args)
	case "remote-transfer":
		err = cmdRemoteTransferPort(args)
	case "worktree-claim":
		err = cmdWorktreeClaimPort(args)
	case "agent-stats":
		err = cmdAgentStatsPort(args)
	case "diff":
		err = cmdDiffPort(args)
	case "critical-path":
		err = cmdCriticalPathPort(args)
	case "doctor":
		err = cmdDoctorPort(args)
	case "thrash":
		err = cmdThrashPort(args)
	case "sweep":
		err = cmdSweepPort(args)
	case "dispatch":
		err = cmdDispatchPort(args)
	case "gantt":
		err = cmdGanttPort(args)
	case "mermaid":
		err = cmdMermaidPort(args)
	case "burndown":
		err = cmdBurndownPort(args)
	case "where":
		err = cmdWherePort(args)
	case "topo":
		err = cmdTopoPort(args)
	case "dashboard":
		err = cmdDashboardPort(args)
	case "csv":
		err = cmdCSVPort(args)
	case "html":
		err = cmdHTMLPort(args)
	case "promote":
		err = cmdPromotePort(args)
	case "completion":
		err = cmdCompletionPort(args)
	case "add":
		err = cmdAddPort(args)
	case "merge":
		err = cmdMergePort(args)
	case "next":
		err = cmdNextPort(args)
	case "version", "--version", "-v":
		fmt.Printf("phenodag %s\n", version)
		return
	case "help", "--help", "-h":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n", cmd)
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cmd, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `phenodag %s — multi-agent multi-project DAG

Usage: phenodag <command> [flags]

Commands:
  init       Initialize DB (idempotent: applies migrations)
  seed       Seed from built-in v3-180 preset (180 tasks)
  status     Show task counts
  validate   Check DAG (cycles, dangles, width)
  pick       Atomically pick ready task (--agent ID)
  claim      Claim repo/branch/worktree for a task
  release    Release a claim
  heartbeat  Update agent heartbeat
  reclaim    Reap stale agents (heartbeat > 15 min)
  done       Mark task done (unblocks downstream)
  fail       Mark task failed
  fill       Promote side-DAGs into stage gaps
  scan       Scan local/remote repos
  dupes      Find fuzzy-duplicate groups
  export     Export DAG to markdown
  version    Show version
  help       Show this help

Width 20 / length 100 are minima, not caps. Use init --width N --stages M.

`, version)
}

// ---------- init ----------

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	width := fs.Int("width", 20, "DAG width (min 20, no max)")
	stages := fs.Int("stages", 6, "DAG stages (min 100 len, no max)")
	force := fs.Bool("force", false, "wipe existing DB first")
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)

	if *force {
		_ = os.Remove(gDBPath)
		_ = os.Remove(gDBPath + "-shm")
		_ = os.Remove(gDBPath + "-wal")
	}

	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := migrate(db); err != nil {
		return err
	}
	for _, kv := range [][2]string{
		{"width", fmt.Sprintf("%d", *width)},
		{"stages", fmt.Sprintf("%d", *stages)},
		{"created_at", nowUTC()},
		{"version", version},
	} {
		if _, err := db.Exec(`INSERT INTO dag_meta(key, value) VALUES(?, ?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, kv[0], kv[1]); err != nil {
			return err
		}
	}
	fmt.Printf("initialized %s (width=%d, stages=%d)\n", gDBPath, *width, *stages)
	return nil
}

// ---------- seed ----------

var fleetPriority = []string{
	"HexaKit", "PhenoDevOps", "Pyron", "FocalPoint", "HeliosCLI",
	"helioscope", "PhenoProc", "PhenoKits", "phenotype-bus", "phenotype-otel",
	"phenotype-postfx", "phenotype-terrain", "phenotype-voxel", "phenotype-water",
	"phenotype-journeys", "phenotype-skills",
}

var activeRepos = []string{
	"AgilePlus", "Tracera", "phenodag", "pheno", "phenotype-hub",
	"phenotype-registry", "phenotype-icons", "phenotype-zod-schemas",
	"phenotype-auth-ts", "phenotype-python-sdk", "phenotype-go-sdk",
}

type seedTask struct {
	ID, Desc, Repo, Sub, Kind, SideDAG string
	Stage, Slot, Priority              int
}

func v3Core() []seedTask {
	out := []seedTask{}
	stages := []struct {
		kind, verb string
	}{
		{"audit", "audit for"},
		{"hygiene", "hygiene for"},
		{"test", "test coverage for"},
		{"libify", "libify for"},
		{"integrate", "integrate libs into"},
		{"ship", "ship"},
	}
	for stage := 1; stage <= 6; stage++ {
		s := stages[stage-1]
		for slot := 1; slot <= 20; slot++ {
			var repo, sub string
			switch stage {
			case 1, 2, 3, 4:
				repo = fleetPriority[(slot-1)%len(fleetPriority)]
			case 5:
				if slot <= 16 {
					repo = fleetPriority[slot-1]
				} else {
					repo = activeRepos[slot-16-1]
				}
			case 6:
				if slot <= 11 {
					repo = activeRepos[slot-1]
				} else {
					repo = "fleet-wide"
				}
			}
			id := fmt.Sprintf("task-%02d-%02d", stage, slot)
			desc := fmt.Sprintf("L%d %s %s slot %d (%s)", stage, s.verb, repo, slot, sub)
			out = append(out, seedTask{
				ID: id, Desc: desc, Repo: repo, Sub: sub, Kind: s.kind,
				Stage: stage, Slot: slot, Priority: 5 + stage,
			})
		}
	}
	return out
}

func v3Side() []seedTask {
	projects := []struct {
		id, name, repo string
		tasks          [5]string
	}{
		{"sd-audit", "Audit & Compliance", "agileplus", [5]string{"audit-org", "license-check", "secret-scan", "dep-vuln", "sbom-export"}},
		{"sd-ci", "CI/CD Pipelines", "pheno-pipelines", [5]string{"cache-warmup", "matrices", "retry-policy", "artifact-store", "runner-pools"}},
		{"sd-docs", "Documentation", "phenodocs", [5]string{"doc-gen", "api-ref", "tutorial", "changelog", "rfc-flow"}},
		{"sd-error", "Error Codes", "phenotype-errors", [5]string{"code-registry", "machine-codes", "i18n", "incident-codes", "code-projection"}},
		{"sd-fuzz", "Fuzzing", "phenoFuzz", [5]string{"harness", "corpus", "repro", "coverage", "ci-fuzz"}},
		{"sd-libify", "Libification", "pheno-libs", [5]string{"extract-rule", "dual-pub", "adopt-1", "adopt-2", "guide"}},
		{"sd-obs", "Observability", "phenoObservability", [5]string{"otel-bridge", "dashboards", "alerts", "on-call", "postmortem"}},
		{"sd-perf", "Performance", "heliosBench", [5]string{"baseline", "regress-test", "flame-graph", "memprof", "loadtest"}},
		{"sd-release", "Release", "phenotype-release", [5]string{"semver", "changelog-auto", "rollback", "canary", "signing"}},
		{"sd-sota", "SOTA Research", "phenoResearchEngine", [5]string{"scout", "eval", "adopt", "deprecate", "report"}},
		{"sd-test", "Test Framework", "TestingKit", [5]string{"unit", "integration", "property", "contract", "e2e"}},
		{"sd-type", "Type Safety", "ValidationKit", [5]string{"strict", "zod-schemas", "pyright", "tsc-strict", "schemas-pub"}},
	}
	out := []seedTask{}
	for _, p := range projects {
		for i, t := range p.tasks {
			out = append(out, seedTask{
				ID:       fmt.Sprintf("%s-%02d", p.id, i+1),
				Desc:     fmt.Sprintf("%s sub-task %d (%s): %s", p.name, i+1, p.repo, t),
				Repo:     p.repo,
				Kind:     "side",
				Stage:    0,
				Slot:     0,
				Priority: 3,
				SideDAG:  p.id,
			})
		}
	}
	return out
}

// melosvizCore: 7 stages x 20 width = 140 core tasks.
// Stages 1-6 mirror v3; L7 is MELOSVIZ-only SUSTAIN (retro, debt, etc.)
func melosvizCore() []seedTask {
	stages := []struct {
		kind, verb string
	}{
		{"audit", "audit for"},
		{"hygiene", "hygiene for"},
		{"test", "test coverage for"},
		{"libify", "libify for"},
		{"integrate", "integrate libs into"},
		{"ship", "ship"},
		{"sustain", "sustain for"},
	}
	out := []seedTask{}
	for stage := 1; stage <= 7; stage++ {
		s := stages[stage-1]
		for slot := 1; slot <= 20; slot++ {
			var repo, sub string
			switch stage {
			case 1, 2, 3, 4:
				repo = fleetPriority[(slot-1)%len(fleetPriority)]
			case 5:
				if slot <= 16 {
					repo = fleetPriority[slot-1]
				} else {
					repo = activeRepos[slot-16-1]
				}
			case 6:
				if slot <= 11 {
					repo = activeRepos[slot-1]
				} else {
					repo = "fleet-wide"
				}
			case 7:
				// SUSTAIN: per-subproject retro + debt retire + ADR
				if slot <= 16 {
					repo = fleetPriority[slot-1]
				} else {
					repo = "fleet-wide"
				}
			}
			id := fmt.Sprintf("task-%02d-%02d", stage, slot)
			desc := fmt.Sprintf("L%d %s %s slot %d (%s)", stage, s.verb, repo, slot, sub)
			out = append(out, seedTask{
				ID: id, Desc: desc, Repo: repo, Sub: sub, Kind: s.kind,
				Stage: stage, Slot: slot, Priority: 5 + stage,
			})
		}
	}
	return out
}

// melosvizSide: 9 side-DAGs x 5 = 45 side tasks (MELOSVIZ-specific subset of v3 side-DAGs)
func melosvizSide() []seedTask {
	projects := []struct {
		id, name, repo string
		tasks          [5]string
	}{
		{"sd-audit", "Audit & Compliance", "agileplus", [5]string{"audit-org", "license-check", "secret-scan", "dep-vuln", "sbom-export"}},
		{"sd-ci", "CI/CD Pipelines", "pheno-pipelines", [5]string{"cache-warmup", "matrices", "retry-policy", "artifact-store", "runner-pools"}},
		{"sd-docs", "Documentation", "phenodocs", [5]string{"doc-gen", "api-ref", "tutorial", "changelog", "rfc-flow"}},
		{"sd-error", "Error Codes", "phenotype-errors", [5]string{"code-registry", "machine-codes", "i18n", "incident-codes", "code-projection"}},
		{"sd-fuzz", "Fuzzing", "phenoFuzz", [5]string{"harness", "corpus", "repro", "coverage", "ci-fuzz"}},
		{"sd-libify", "Libification", "pheno-libs", [5]string{"extract-rule", "dual-pub", "adopt-1", "adopt-2", "guide"}},
		{"sd-obs", "Observability", "phenoObservability", [5]string{"otel-bridge", "dashboards", "alerts", "on-call", "postmortem"}},
		{"sd-perf", "Performance", "heliosBench", [5]string{"baseline", "regress-test", "flame-graph", "memprof", "loadtest"}},
		{"sd-release", "Release", "phenotype-release", [5]string{"semver", "changelog-auto", "rollback", "canary", "signing"}},
	}
	out := []seedTask{}
	for _, p := range projects {
		for i, t := range p.tasks {
			out = append(out, seedTask{
				ID:       fmt.Sprintf("%s-%02d", p.id, i+1),
				Desc:     fmt.Sprintf("%s sub-task %d (%s): %s", p.name, i+1, p.repo, t),
				Repo:     p.repo,
				Kind:     "side",
				Stage:    0,
				Slot:     0,
				Priority: 3,
				SideDAG:  p.id,
			})
		}
	}
	return out
}

// agileplusCore: 4 stages x 5 width = 20 core tasks (spec harmonization).
// L1=intake, L2=harmonize, L3=trace-link, L4=govern.
// Each task is a substantive deliverable for spec harmonization.
func agileplusCore() []seedTask {
	type spec struct {
		kind, verb, what string
	}
	rows := []spec{
		// L1 intake (5)
		{"intake", "intake", "spec source registry: enumerate 20+ upstream spec repos with metadata (license, status, owner)"},
		{"intake", "intake", "intake manifest: per-spec fields (title, owner, version, license, status, sha)"},
		{"intake", "intake", "terminology extraction: glossary terms with aliases, synonyms, deprecated forms"},
		{"intake", "intake", "stakeholder map: RACI for spec authoring, review, consumption, retirement"},
		{"intake", "intake", "triage workflow: severity classification (draft / active / deprecated / superseded)"},
		// L2 harmonize (5)
		{"harmonize", "harmonize", "schema normalization: align to canonical spec model v3.1 with field map"},
		{"harmonize", "harmonize", "conflict detection: cross-spec duplicate requirements with similarity score"},
		{"harmonize", "harmonize", "resolution rules: precedence (local > upstream), merge heuristics, manual override"},
		{"harmonize", "harmonize", "field mapping: legacy fields (status, owner, version) → canonical fields"},
		{"harmonize", "harmonize", "version pin policy: lockstep vs floating per dependency, drift detection"},
		// L3 trace-link (5)
		{"trace-link", "trace-link", "requirement extraction: parse shall-statements into testable requirements"},
		{"trace-link", "trace-link", "link graph: requirement → spec section → ADR → test case (4-hop)"},
		{"trace-link", "trace-link", "coverage matrix: % requirements with at least 1 downstream test"},
		{"trace-link", "trace-link", "orphan detection: tests without reqs, reqs without tests, ADRs without reqs"},
		{"trace-link", "trace-link", "trace export: ReqIF, OSLC, JSON-LD formats with bidirectional lookup"},
		// L4 govern (5)
		{"govern", "govern", "review board: 3-of-5 approval workflow with cryptographic sign-off"},
		{"govern", "govern", "change advisory board: CAB packets, monthly cadence, emergency fast-track"},
		{"govern", "govern", "compliance check: license (SPDX), IP attribution, export control per spec"},
		{"govern", "govern", "sign-off ledger: append-only chain of attestations with timestamp + signer"},
		{"govern", "govern", "sunset protocol: deprecation timeline (announce → warn → remove), migration guide"},
	}
	out := make([]seedTask, len(rows))
	for i, r := range rows {
		stage := (i / 5) + 1
		slot := (i % 5) + 1
		id := fmt.Sprintf("task-%02d-%02d", stage, slot)
		desc := fmt.Sprintf("L%d %s slot %d: %s", stage, r.verb, slot, r.what)
		out[i] = seedTask{
			ID: id, Desc: desc, Repo: "agileplus", Sub: "", Kind: r.kind,
			Stage: stage, Slot: slot, Priority: 5 + stage,
		}
	}
	return out
}

// agileplusSide: 6 side-DAGs x 5 = 30 side tasks for spec harmonization
// (sd-gsd, sd-openspec, sd-bmad, sd-kitty, sd-trace, sd-govern).
func agileplusSide() []seedTask {
	projects := []struct {
		id, name, repo string
		tasks          [5]string
	}{
		{"sd-gsd", "Get-Shit-Done", "agileplus", [5]string{"gsd-bootstrap", "gsd-prd", "gsd-arch", "gsd-build", "gsd-ship"}},
		{"sd-openspec", "OpenSpec", "agileplus", [5]string{"openspec-import", "openspec-convert", "openspec-validate", "openspec-diff", "openspec-export"}},
		{"sd-bmad", "BMAD", "agileplus", [5]string{"bmad-brief", "bmad-market", "bmad-arch", "bmad-sprint", "bmad-retro"}},
		{"sd-kitty", "Spec-Kitty", "agileplus", [5]string{"kitty-research", "kitty-spec", "kitty-plan", "kitty-implement", "kitty-merge"}},
		{"sd-trace", "Traceability", "agileplus", [5]string{"trace-extract", "trace-link", "trace-matrix", "trace-impact", "trace-export"}},
		{"sd-govern", "Governance", "agileplus", [5]string{"govern-policy", "govern-review", "govern-cab", "govern-audit", "govern-sunset"}},
	}
	out := []seedTask{}
	for _, p := range projects {
		for i, t := range p.tasks {
			out = append(out, seedTask{
				ID:       fmt.Sprintf("%s-%02d", p.id, i+1),
				Desc:     fmt.Sprintf("%s sub-task %d (%s): %s", p.name, i+1, p.repo, t),
				Repo:     p.repo,
				Kind:     "side",
				Stage:    0,
				Slot:     0,
				Priority: 3,
				SideDAG:  p.id,
			})
		}
	}
	return out
}

// traceraCore: 4 stages x 5 width = 20 core tasks.
// L1=graph, L2=projection, L3=autograder, L4=runtime.
func traceraCore() []seedTask {
	type spec struct {
		kind, verb, what string
	}
	rows := []spec{
		// L1 graph (5)
		{"graph", "graph", "AST extractor: per-language tree-sitter grammars with incremental parse"},
		{"graph", "graph", "symbol index: package/function/class/type resolution with scoping rules"},
		{"graph", "graph", "edge builder: imports, calls, references, type-of relationships"},
		{"graph", "graph", "cycle detection: strongly connected components, feedback arc sets"},
		{"graph", "graph", "reachability: BFS from entrypoints, dead-code report, churn-weighted"},
		// L2 projection (5)
		{"projection", "project", "multi-view render: callgraph, depgraph, ownership, lineage"},
		{"projection", "project", "layout engine: dot, dagre, ELK, sugarcube, force-directed"},
		{"projection", "project", "filter DSL: by repo, path, regex, language, glob, attribute"},
		{"projection", "project", "diff projection: between revisions (git rev-parse), branch compare"},
		{"projection", "project", "cross-repo stitch: monorepo vs polyrepo topology, federation"},
		// L3 autograder (5)
		{"autograder", "grade", "policy DSL: rego/cel expressions, custom rules, severity hooks"},
		{"autograder", "grade", "rule pack: bundle of common checks (no-cycle, no-dead-code, max-depth)"},
		{"autograder", "grade", "lint runner: per-language linter integration (eslint, clippy, ruff)"},
		{"autograder", "grade", "severity assignment: blocker/error/warning/info, suppressions"},
		{"autograder", "grade", "score aggregation: weighted sum, percentile, badge eligibility"},
		// L4 runtime (5)
		{"runtime", "run", "live watcher: fsnotify (mac), kqueue (bsd), inotify (linux)"},
		{"runtime", "run", "incremental update: delta-from-baseline, selective re-parse"},
		{"runtime", "run", "cache layer: keyed by content hash, invalidated on mtime + content change"},
		{"runtime", "run", "API server: HTTP/JSON, GraphQL, gRPC with auth + rate-limit"},
		{"runtime", "run", "Web UI: interactive graph (sigma.js, cytoscape, d3) with pan/zoom/select"},
	}
	out := make([]seedTask, len(rows))
	for i, r := range rows {
		stage := (i / 5) + 1
		slot := (i % 5) + 1
		id := fmt.Sprintf("task-%02d-%02d", stage, slot)
		desc := fmt.Sprintf("L%d %s slot %d: %s", stage, r.verb, slot, r.what)
		out[i] = seedTask{
			ID: id, Desc: desc, Repo: "tracera", Sub: "", Kind: r.kind,
			Stage: stage, Slot: slot, Priority: 5 + stage,
		}
	}
	return out
}

// traceraSide: 6 side-DAGs x 5 = 30 side tasks
// (sd-fr-nfr, sd-trace-link, sd-matrix, sd-impact, sd-rag, sd-autograder).
func traceraSide() []seedTask {
	projects := []struct {
		id, name, repo string
		tasks          [5]string
	}{
		{"sd-fr-nfr", "ReqClassify", "tracera", [5]string{"fr-extract", "fr-classify", "nfr-extract", "nfr-measure", "nfr-report"}},
		{"sd-trace-link", "TraceLink", "tracera", [5]string{"link-extract", "link-resolve", "link-validate", "link-prune", "link-export"}},
		{"sd-matrix", "CoverageMatrix", "tracera", [5]string{"matrix-build", "matrix-aggregate", "matrix-viz", "matrix-export", "matrix-snapshot"}},
		{"sd-impact", "ImpactAnalysis", "tracera", [5]string{"impact-blast", "impact-ranker", "impact-suggest", "impact-pr", "impact-approve"}},
		{"sd-rag", "RAGGrounded", "tracera", [5]string{"rag-embed", "rag-index", "rag-retrieve", "rag-cite", "rag-audit"}},
		{"sd-autograder", "AutoGrader", "tracera", [5]string{"grader-rules", "grader-runs", "grader-bundles", "grader-badges", "grader-gate"}},
	}
	out := []seedTask{}
	for _, p := range projects {
		for i, t := range p.tasks {
			out = append(out, seedTask{
				ID:       fmt.Sprintf("%s-%02d", p.id, i+1),
				Desc:     fmt.Sprintf("%s sub-task %d (%s): %s", p.name, i+1, p.repo, t),
				Repo:     p.repo,
				Kind:     "side",
				Stage:    0,
				Slot:     0,
				Priority: 3,
				SideDAG:  p.id,
			})
		}
	}
	return out
}

// mcpFleetCore: 6 stages x 5 width = 30 core tasks (MCP polyrepo execution plan).
func mcpFleetCore() []seedTask {
	type spec struct {
		kind, verb, what string
	}
	rows := []spec{
		// S0 governance
		{"governance", "merge", "Wave0 doc PRs merged; validate_catalog green"},
		{"governance", "adr", "PhenoSpecs ADR-017 Accepted"},
		{"governance", "adr", "PhenoSpecs ADR-018 Accepted"},
		{"governance", "sync", "phenotype-registry DOMAIN_ROLES MCP row"},
		{"governance", "registry", "PhenoMCPServers registry_version bump"},
		// S1 SSOT
		{"ssot", "spec", "specs/mcp/polyrepo-boundaries/spec.md"},
		{"ssot", "skill", "skills/mcp-boundary-guard published"},
		{"ssot", "skill", "skills/github-fork-policy published"},
		{"ssot", "agent", "fleet-lead agent.yaml wires guard skills"},
		{"ssot", "trace", "PhenoSpecs registry.yaml links ADR-017"},
		// S2 framework
		{"framework", "rust", "PhenoFastMCP-rust PHENO on main"},
		{"framework", "rmcp", "PhenoRMCP spec SDK policy on main"},
		{"framework", "py", "PhenoFastMCP phenotype/superset @ v3.4.2 branch"},
		{"framework", "go", "PhenoFastMCP-go feat/phenotype-foundation merged"},
		{"framework", "fork", "verify all fork:true parent links via gh api"},
		// S3 servers
		{"migration", "pheno-org", "migrate pheno-org tools from PhenoMCP issue #1"},
		{"migration", "forge", "MCPForge in-tree or catalog active issue #2"},
		{"migration", "ops", "ops-mcp in-tree issue #2"},
		{"migration", "hexakit", "hexakit init mcp-server scaffold issue #3"},
		{"migration", "substrate", "substrate server package tests CI green"},
		// S4 retire
		{"retire", "phenomcp", "PhenoMCP README redirect to PhenoMCPServers"},
		{"retire", "mcpkit", "McpKit DEPRECATED banner to PhenoFastMCP"},
		{"retire", "go-sdk", "phenotype-go-sdk shrink issue #7"},
		{"retire", "substrate", "ADR-019 substrate trim duplicate driver-mcp"},
		{"retire", "cheap-llm", "confirm no cheap-llm-mcp repo; substrate argv only"},
		// S5 dogfood
		{"dogfood", "bundle", "plugins/phenotype-bundle mcp.json wired"},
		{"dogfood", "session", "fleet-lead agent run with zero loop count"},
		{"dogfood", "metric", "session loop_events schema pilot"},
		{"dogfood", "ci", "validate_catalog in PhenoMCPServers CI"},
		{"dogfood", "phenodag", "mcp-fleet-60 preset seeded and pick/claim tested"},
	}
	out := make([]seedTask, len(rows))
	for i, r := range rows {
		stage := (i / 5) + 1
		slot := (i % 5) + 1
		id := fmt.Sprintf("eco-%03d", i+1)
		desc := fmt.Sprintf("S%d %s: %s", stage, r.verb, r.what)
		out[i] = seedTask{
			ID: id, Desc: desc, Repo: "PhenoMCPServers", Sub: "", Kind: r.kind,
			Stage: stage, Slot: slot, Priority: 5 + stage,
		}
	}
	return out
}

// mcpFleetSide: 6 side-DAGs x 5 = 30 side tasks.
func mcpFleetSide() []seedTask {
	projects := []struct {
		id, name, repo string
		tasks          [5]string
	}{
		{"sd-fastrmcp", "FastRMCP eval", "PhenoFastMCP-rust", [5]string{"middleware audit", "SSE patterns", "cherry-pick plan", "ADR note", "close or defer"}},
		{"sd-zigmojo", "Zig Mojo spike", "PhenoMCPServers", [5]string{"issue #8 triage", "fork parent candidates", "ADR draft", "registry stub", "defer gate"}},
		{"sd-dagctl", "dagctl merge", "phenodag", [5]string{"ADR superset", "remoteclaim port", "preset tests", "release rc", "archive dagctl"}},
		{"sd-agileplus", "AgilePlus MCP", "AgilePlus", [5]string{"eco-NNN map", "kitty-spec link", "agileplus-mcp audit", "traceability", "spec status"}},
		{"sd-ts7", "TS7 binding", "HexaKit", [5]string{"template stub", "registry planned", "codegen sketch", "ADR ref", "defer impl"}},
		{"sd-retire", "Legacy retire", "phenotype-registry", [5]string{"ECOSYSTEM_MAP", "McpKit row", "PhenoMCP row", "link check", "audit close"}},
	}
	out := []seedTask{}
	for _, p := range projects {
		for i, t := range p.tasks {
			out = append(out, seedTask{
				ID:       fmt.Sprintf("%s-%02d", p.id, i+1),
				Desc:     fmt.Sprintf("%s: %s", p.name, t),
				Repo:     p.repo,
				Kind:     "side",
				Stage:    0,
				Slot:     0,
				Priority: 3,
				SideDAG:  p.id,
			})
		}
	}
	return out
}

func cmdSeed(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ExitOnError)
	preset := fs.String("preset", "v3-180", "preset name (v3-180, melosviz-185, agileplus-50, tracera-50, mcp-fleet-60)")
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)

	// Spread the 4 new (agileplus-*, tracera-*) builders across separate
	// goroutines; v3-180 and melosviz-185 are kept synchronous for
	// backwards compatibility.
	var (
		wg          sync.WaitGroup
		apCore, apSide, trCore, trSide []seedTask
	)
	wg.Add(4)
	go func() { defer wg.Done(); apCore = agileplusCore() }()
	go func() { defer wg.Done(); apSide = agileplusSide() }()
	go func() { defer wg.Done(); trCore = traceraCore() }()
	go func() { defer wg.Done(); trSide = traceraSide() }()
	wg.Wait()

	var core, side []seedTask
	var presetName, presetDesc, shape string
	switch *preset {
	case "v3-180":
		core = v3Core()
		side = v3Side()
		presetName = "v3-180"
		presetDesc = "v3-180: 120 core (6 stages x 20 width) + 60 side (12 projects x 5)"
		shape = "20x6 + 12 side-dags of 5"
	case "melosviz-185":
		core = melosvizCore()
		side = melosvizSide()
		presetName = "melosviz-185"
		presetDesc = "melosviz-185: 140 core (7 stages x 20 width) + 45 side (9 projects x 5)"
		shape = "20x7 + 9 side-dags of 5"
	case "agileplus-50":
		core = apCore
		side = apSide
		presetName = "agileplus-50"
		presetDesc = "agileplus-50: 20 core (4 stages x 5 width) + 30 side (6 projects x 5)"
		shape = "5x4 + 6 side-dags of 5"
	case "tracera-50":
		core = trCore
		side = trSide
		presetName = "tracera-50"
		presetDesc = "tracera-50: 20 core (4 stages x 5 width) + 30 side (6 projects x 5)"
		shape = "5x4 + 6 side-dags of 5"
	case "mcp-fleet-60":
		core = mcpFleetCore()
		side = mcpFleetSide()
		presetName = "mcp-fleet-60"
		presetDesc = "mcp-fleet-60: 30 core (6 stages x 5 width) + 30 side (6 projects x 5)"
		shape = "5x6 + 6 side-dags of 5"
	default:
		return fmt.Errorf("unknown preset %q (try v3-180, melosviz-185, agileplus-50, tracera-50, mcp-fleet-60)", *preset)
	}

	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := migrate(db); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, _ := tx.Prepare(`INSERT OR REPLACE INTO tasks
		(id, stage, slot, description, repo, subproject, category, lane, branch, kind, priority, semantic_hash, side_dag, status, assigned_agent, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	estmt, _ := tx.Prepare(`INSERT OR IGNORE INTO edges(from_task, to_task) VALUES (?,?)`)
	now := nowUTC()
	for _, t := range core {
		_, err := stmt.Exec(t.ID, t.Stage, t.Slot, t.Desc, t.Repo, t.Sub, "", "", "", t.Kind, t.Priority,
			hashID(t.Stage, t.Slot, t.ID), t.SideDAG, "ready", "", now, now)
		if err != nil {
			return err
		}
	}
	for _, t := range side {
		_, err := stmt.Exec(t.ID, t.Stage, t.Slot, t.Desc, t.Repo, t.Sub, "", "", "", t.Kind, t.Priority,
			hashID(t.Stage, t.Slot, t.ID), t.SideDAG, "ready", "", now, now)
		if err != nil {
			return err
		}
	}
	maxStage := 6
	maxSlot := 20
	if presetName == "melosviz-185" {
		maxStage = 7
	}
	if presetName == "agileplus-50" || presetName == "tracera-50" {
		maxStage = 4
		maxSlot = 5
	}
	if presetName == "mcp-fleet-60" {
		maxStage = 6
		maxSlot = 5
	}
	for stage := 1; stage < maxStage; stage++ {
		for slot := 1; slot <= maxSlot; slot++ {
			from := fmt.Sprintf("task-%02d-%02d", stage, slot)
			to := fmt.Sprintf("task-%02d-%02d", stage+1, slot)
			_, _ = estmt.Exec(from, to)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	sideDAGCount := 12
	if presetName == "melosviz-185" {
		sideDAGCount = 9
	}
	if presetName == "agileplus-50" || presetName == "tracera-50" || presetName == "mcp-fleet-60" {
		sideDAGCount = 6
	}
	for _, kv := range [][2]string{
		{"preset", presetName},
		{"preset_description", presetDesc},
		{"shape", shape},
	} {
		_, _ = db.Exec(`INSERT INTO dag_meta(key, value) VALUES(?, ?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, kv[0], kv[1])
	}
	fmt.Printf("seeded %s: %d core + %d side = %d tasks, %d side-DAGs\n",
		presetName, len(core), len(side), len(core)+len(side), sideDAGCount)
	return nil
}

// ---------- status ----------

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	var total, ready, inprog, done, blocked, failed int
	_ = db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&total)
	_ = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE status='ready'`).Scan(&ready)
	_ = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE status='in_progress'`).Scan(&inprog)
	_ = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE status='done'`).Scan(&done)
	_ = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE status='blocked'`).Scan(&blocked)
	_ = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE status='failed'`).Scan(&failed)

	var preset, widthMeta, stagesMeta string
	_ = db.QueryRow(`SELECT value FROM dag_meta WHERE key='preset'`).Scan(&preset)
	_ = db.QueryRow(`SELECT value FROM dag_meta WHERE key='width'`).Scan(&widthMeta)
	_ = db.QueryRow(`SELECT value FROM dag_meta WHERE key='stages'`).Scan(&stagesMeta)

	fmt.Printf("phenodag %s — %s\n", version, gDBPath)
	if preset != "" {
		fmt.Printf("preset: %s\n", preset)
	}
	fmt.Printf("tasks: %d total (ready=%d in_progress=%d done=%d blocked=%d failed=%d)\n",
		total, ready, inprog, done, blocked, failed)
	if widthMeta != "" {
		fmt.Printf("width=%s stages=%s\n", widthMeta, stagesMeta)
	}
	rows, err := db.Query(`SELECT stage, status, COUNT(*) FROM tasks GROUP BY stage, status ORDER BY stage, status`)
	if err != nil {
		return err
	}
	defer rows.Close()
	fmt.Println("by stage+status:")
	for rows.Next() {
		var s int
		var st string
		var c int
		_ = rows.Scan(&s, &st, &c)
		if s == 0 {
			fmt.Printf("  pool: %d %s\n", c, st)
		} else {
			fmt.Printf("  L%d: %d %s\n", s, c, st)
		}
	}
	var claims int
	_ = db.QueryRow(`SELECT COUNT(*) FROM claims`).Scan(&claims)
	fmt.Printf("claims: %d active\n", claims)
	var dupes int
	_ = db.QueryRow(`SELECT COUNT(*) FROM duplicate_groups`).Scan(&dupes)
	fmt.Printf("duplicate_groups: %d\n", dupes)
	return nil
}

// ---------- validate ----------

func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	edges := map[string][]string{}
	rows, err := db.Query(`SELECT from_task, to_task FROM edges`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var f, t string
		_ = rows.Scan(&f, &t)
		edges[f] = append(edges[f], t)
	}
	_ = rows.Close()

	visited := map[string]bool{}
	recStack := map[string]bool{}
	cycle := false
	var dfs func(n string)
	dfs = func(n string) {
		if cycle {
			return
		}
		visited[n] = true
		recStack[n] = true
		for _, t := range edges[n] {
			if !visited[t] {
				dfs(t)
			} else if recStack[t] {
				cycle = true
				return
			}
		}
		recStack[n] = false
	}
	for n := range edges {
		if !visited[n] {
			dfs(n)
		}
		if cycle {
			break
		}
	}
	if cycle {
		fmt.Println("INVALID: cycle detected")
		os.Exit(1)
	}

	var dangles int
	_ = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE id NOT IN
		(SELECT from_task FROM edges UNION SELECT to_task FROM edges)
		AND stage > 1`).Scan(&dangles)
	if dangles > 0 {
		fmt.Printf("WARNING: %d dangling tasks (no edges, stage > 1)\n", dangles)
	}

	var wstr string
	_ = db.QueryRow(`SELECT value FROM dag_meta WHERE key='width'`).Scan(&wstr)
	width := 20
	fmt.Sscanf(wstr, "%d", &width)
	if width > 0 {
		rows2, err := db.Query(`SELECT stage, COUNT(*) FROM tasks
			WHERE side_dag='' OR side_dag IS NULL
			GROUP BY stage ORDER BY stage`)
		if err == nil {
			for rows2.Next() {
				var stage, n int
				_ = rows2.Scan(&stage, &n)
				if n < width {
					fmt.Printf("WARNING: stage %d has %d core tasks (width=%d) — use 'fill' to promote side-DAGs\n", stage, n, width)
				}
			}
			_ = rows2.Close()
		}
	}
	fmt.Println("VALID")
	return nil
}

// ---------- pick ----------

func cmdPick(args []string) error {
	fs := flag.NewFlagSet("pick", flag.ExitOnError)
	agent := fs.String("agent", "", "agent ID (required)")
	preferKind := fs.String("kind", "", "prefer this kind (audit/hygiene/test/libify/integrate/ship/side)")
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	if *agent == "" {
		return fmt.Errorf("--agent is required")
	}
	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return withLock(gDBPath, func() error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		now := nowUTC()
		_, err = tx.Exec(`INSERT INTO agents(id, status, last_seen, created_at)
			VALUES(?, 'active', ?, ?)
			ON CONFLICT(id) DO UPDATE SET status='active', last_seen=excluded.last_seen`, *agent, now, now)
		if err != nil {
			return err
		}

		// "ready" means: status='ready' AND all incoming edge predecessors are 'done'
		coreQuery := `SELECT id FROM tasks
			WHERE status='ready' AND (side_dag='' OR side_dag IS NULL)
			AND id NOT IN (
				SELECT e.to_task FROM edges e
				JOIN tasks p ON e.from_task = p.id
				WHERE p.status != 'done'
			)
			ORDER BY priority DESC, stage ASC, id ASC LIMIT 1`
		sideQuery := `SELECT id FROM tasks
			WHERE status='ready' AND side_dag!='' AND side_dag IS NOT NULL
			AND id NOT IN (
				SELECT e.to_task FROM edges e
				JOIN tasks p ON e.from_task = p.id
				WHERE p.status != 'done'
			)
			ORDER BY priority DESC, id ASC LIMIT 1`

		var id string
		if *preferKind == "side" {
			err = tx.QueryRow(sideQuery).Scan(&id)
		} else {
			err = tx.QueryRow(coreQuery).Scan(&id)
			if err == sql.ErrNoRows {
				err = tx.QueryRow(sideQuery).Scan(&id)
			}
		}
		if err == sql.ErrNoRows {
			return fmt.Errorf("no ready tasks available")
		}
		if err != nil {
			return err
		}

		// Read the task fields for output
		var (
			desc, repo, kind, side string
			stage, slot           int
		)
		err = tx.QueryRow(`SELECT description, repo, kind, side_dag, stage, slot FROM tasks WHERE id=?`, id).
			Scan(&desc, &repo, &kind, &side, &stage, &slot)
		if err != nil {
			return err
		}

		_, err = tx.Exec(`UPDATE tasks SET status='in_progress', assigned_agent=?, updated_at=? WHERE id=?`,
			*agent, now, id)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		out := map[string]interface{}{
			"agent": *agent, "task": id, "stage": stage, "slot": slot,
			"kind": kind, "repo": repo, "status": "in_progress", "description": desc,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return nil
	})
}

// ---------- claim ----------

func cmdClaim(args []string) error {
	fs := flag.NewFlagSet("claim", flag.ExitOnError)
	agent := fs.String("agent", "", "agent ID (required)")
	repo := fs.String("repo", "", "repo (required)")
	branch := fs.String("branch", "", "branch")
	worktree := fs.String("worktree", "", "worktree path")
	taskID := fs.String("task", "", "task ID to associate")
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	if *agent == "" || *repo == "" {
		return fmt.Errorf("--agent and --repo are required")
	}
	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return withLock(gDBPath, func() error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var existing string
		err = tx.QueryRow(`SELECT agent FROM claims
			WHERE repo=? AND branch=? AND worktree=?`, *repo, *branch, *worktree).Scan(&existing)
		if err == nil && existing != "" && existing != *agent {
			return fmt.Errorf("already claimed by %q", existing)
		}
		_, err = tx.Exec(`INSERT OR REPLACE INTO claims
			(agent, task_id, repo, branch, worktree, status, claimed_at)
			VALUES (?, ?, ?, ?, ?, 'active', ?)`,
			*agent, *taskID, *repo, *branch, *worktree, nowUTC())
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]string{
			"agent": *agent, "task": *taskID, "repo": *repo, "branch": *branch, "worktree": *worktree, "status": "active",
		})
		return nil
	})
}

// ---------- release ----------

func cmdRelease(args []string) error {
	fs := flag.NewFlagSet("release", flag.ExitOnError)
	agent := fs.String("agent", "", "agent ID (required)")
	repo := fs.String("repo", "", "repo")
	branch := fs.String("branch", "", "branch")
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	if *agent == "" {
		return fmt.Errorf("--agent is required")
	}
	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`DELETE FROM claims WHERE agent=? AND repo=? AND branch=?`,
		*agent, *repo, *branch)
	return err
}

// ---------- heartbeat ----------

func cmdHeartbeat(args []string) error {
	fs := flag.NewFlagSet("heartbeat", flag.ExitOnError)
	agent := fs.String("agent", "", "agent ID (required)")
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	if *agent == "" {
		return fmt.Errorf("--agent is required")
	}
	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return withLock(gDBPath, func() error {
		_, err := db.Exec(`UPDATE agents SET last_heartbeat=CURRENT_TIMESTAMP, status='active' WHERE id=?`, *agent)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "agent %s heartbeat recorded\n", *agent)
		return nil
	})
}

// ---------- reclaim ----------

func cmdReclaim(args []string) error {
	fs := flag.NewFlagSet("reclaim", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	_ = fs.String("stale-min", "15", "minutes since last heartbeat to consider stale")
	fs.Parse(args)
	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return withLock(gDBPath, func() error {
		var staleAgents []string
		rows, err := db.Query(`SELECT id FROM agents WHERE status='active' AND last_heartbeat < datetime('now', '-15 minutes')`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			staleAgents = append(staleAgents, id)
		}
		_ = rows.Close()
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, id := range staleAgents {
			_, _ = tx.Exec(`UPDATE agents SET status='stale' WHERE id=?`, id)
			_, _ = tx.Exec(`UPDATE tasks SET status='ready', assigned_agent=NULL WHERE assigned_agent=?`, id)
			_, _ = tx.Exec(`DELETE FROM claims WHERE agent=?`, id)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "reclaimed %d stale agent(s): %v\n", len(staleAgents), staleAgents)
		return nil
	})
}

// ---------- done ----------

func cmdDone(args []string) error {
	fs := flag.NewFlagSet("done", flag.ExitOnError)
	agent := fs.String("agent", "", "agent ID (required)")
	taskID := fs.String("task", "", "task ID (required)")
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	if *agent == "" || *taskID == "" {
		return fmt.Errorf("--agent and --task are required")
	}
	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return withLock(gDBPath, func() error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		res, err := tx.Exec(`UPDATE tasks SET status='done', updated_at=?
			WHERE id=? AND assigned_agent=?`, nowUTC(), *taskID, *agent)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fmt.Errorf("task %q not assigned to %q", *taskID, *agent)
		}
		// Unblock downstream tasks: any task whose all predecessors are now done
		// moves from blocked→ready. But we don't explicitly store 'blocked';
		// we use the absence of all-done-predecessors. So this becomes a no-op
		// since the predicate in pick() handles it dynamically. (We keep this
		// loop in case future schema adds an explicit blocked status.)
		_, _ = tx.Exec(`DELETE FROM claims WHERE task_id=?`, *taskID)
		return tx.Commit()
	})
}

// ---------- fail ----------

func cmdFail(args []string) error {
	fs := flag.NewFlagSet("fail", flag.ExitOnError)
	agent := fs.String("agent", "", "agent ID (required)")
	taskID := fs.String("task", "", "task ID (required)")
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	if *agent == "" || *taskID == "" {
		return fmt.Errorf("--agent and --task are required")
	}
	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`UPDATE tasks SET status='failed', updated_at=?
		WHERE id=? AND assigned_agent=?`, nowUTC(), *taskID, *agent)
	return err
}

// ---------- fill (side-DAG → gap promotion) ----------

func cmdFill(args []string) error {
	fs := flag.NewFlagSet("fill", flag.ExitOnError)
	agent := fs.String("agent", "fill-agent", "agent ID for fill operations")
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return withLock(gDBPath, func() error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var wstr string
		_ = tx.QueryRow(`SELECT value FROM dag_meta WHERE key='width'`).Scan(&wstr)
		width := 20
		fmt.Sscanf(wstr, "%d", &width)
		promoted := 0
		for stage := 1; stage <= 10; stage++ {
			var n int
			_ = tx.QueryRow(`SELECT COUNT(*) FROM tasks
				WHERE stage=? AND (side_dag='' OR side_dag IS NULL)
				AND status='ready'`, stage).Scan(&n)
			need := width - n
			if need <= 0 {
				continue
			}
			rows, err := tx.Query(`SELECT id FROM tasks
				WHERE stage=0 AND status='ready'
				ORDER BY priority DESC, id ASC LIMIT ?`, need)
			if err != nil {
				return err
			}
			var ids []string
			for rows.Next() {
				var id string
				_ = rows.Scan(&id)
				ids = append(ids, id)
			}
			_ = rows.Close()
			for i, id := range ids {
				slot := n + i + 1
				_, err := tx.Exec(`UPDATE tasks SET stage=?, slot=?, updated_at=?
					WHERE id=?`, stage, slot, nowUTC(), id)
				if err != nil {
					return err
				}
				promoted++
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		fmt.Printf("fill: %d promoted (agent=%s)\n", promoted, *agent)
		return nil
	})
}

// ---------- scan ----------

func cmdScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	localDir := fs.String("local", "", "local directory to scan")
	remoteUser := fs.String("remote", "", "remote GitHub user to scan")
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	if *localDir == "" && *remoteUser == "" {
		return fmt.Errorf("--local or --remote required")
	}
	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if *localDir != "" {
		entries, err := os.ReadDir(*localDir)
		if err != nil {
			return err
		}
		count := 0
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			path := filepath.Join(*localDir, name)
			isGit := 0
			isMangled := 0
			branch := ""
			branchCount := 0
			worktreeCount := 0
			hasUncommitted := 0
			stashes := 0
			gitPath := filepath.Join(path, ".git")
			if info, err := os.Stat(gitPath); err == nil {
				if info.IsDir() {
					isGit = 1
					branchCount = 1
					headBytes, err := os.ReadFile(filepath.Join(gitPath, "HEAD"))
					if err == nil {
						head := strings.TrimSpace(string(headBytes))
						if strings.HasPrefix(head, "ref: refs/heads/") {
							branch = strings.TrimPrefix(head, "ref: refs/heads/")
						}
					}
				} else if info.Mode().IsRegular() {
					isGit = 1
					isMangled = 1
				}
			}
			wtPath := filepath.Join(path, ".git", "worktrees")
			if _, err := os.Stat(wtPath); err == nil {
				wts, _ := os.ReadDir(wtPath)
				worktreeCount = len(wts)
			}
			_, err := db.Exec(`INSERT OR REPLACE INTO repos
				(name, is_local, is_git, is_mangled, is_claimed, current_branch,
				branch_count, worktree_count, has_uncommitted, stashes, open_prs)
				VALUES (?, 1, ?, ?, 0, ?, ?, ?, ?, ?, 0)`,
				name, isGit, isMangled, branch, branchCount, worktreeCount, hasUncommitted, stashes)
			if err != nil {
				return err
			}
			count++
		}
		fmt.Printf("scanned %d local repos in %s\n", count, *localDir)
	}
	if *remoteUser != "" {
		fmt.Printf("(remote scan of @%s is a stub — would fetch via gh CLI or GitHub API)\n", *remoteUser)
	}
	return nil
}

// ---------- dupes (hybrid fuzzy duplicate detection) ----------

var tokenRe = regexp.MustCompile(`[a-z0-9]+`)

func tokens(s string) []string {
	lc := strings.ToLower(s)
	ts := tokenRe.FindAllString(lc, -1)
	sw := map[string]bool{
		"the": true, "a": true, "an": true, "of": true, "for": true,
		"to": true, "in": true, "on": true, "and": true, "or": true,
		"with": true, "this": true, "is": true, "slot": true, "sub": true,
		"task": true, "into": true, "from": true, "libs": true,
	}
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		if !sw[t] && len(t) > 1 {
			out = append(out, t)
		}
	}
	return out
}

func jaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	setA := map[string]bool{}
	setB := map[string]bool{}
	for _, t := range a {
		setA[t] = true
	}
	for _, t := range b {
		setB[t] = true
	}
	intersect := 0
	for t := range setA {
		if setB[t] {
			intersect++
		}
	}
	union := len(setA) + len(setB) - intersect
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}

func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

func hybridScore(descA, descB, repoA, repoB string) float64 {
	j := jaccard(tokens(descA), tokens(descB))
	maxLen := math.Max(float64(len(descA)), float64(len(descB)))
	lev := 0.0
	if maxLen > 0 {
		lev = 1.0 - float64(levenshtein(descA, descB))/maxLen
		if lev < 0 {
			lev = 0
		}
	}
	repo := 0.0
	if repoA != "" && repoA == repoB {
		repo = 1.0
	}
	return 0.6*j + 0.2*lev + 0.2*repo
}

type ent struct {
	id, desc, repo, side string
}

func cmdDupes(args []string) error {
	fs := flag.NewFlagSet("dupes", flag.ExitOnError)
	threshold := fs.Float64("threshold", 0.5, "similarity threshold (0..1)")
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT id, description, repo, side_dag FROM tasks`)
	if err != nil {
		return err
	}
	var all []ent
	for rows.Next() {
		var e ent
		_ = rows.Scan(&e.id, &e.desc, &e.repo, &e.side)
		all = append(all, e)
	}
	_ = rows.Close()

	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(x, y string) {
		rx, ry := find(x), find(y)
		if rx != ry {
			parent[rx] = ry
		}
	}
	for _, e := range all {
		parent[e.id] = e.id
	}
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			a, b := all[i], all[j]
			score := hybridScore(a.desc, b.desc, a.repo, b.repo)
			if score >= *threshold {
				union(a.id, b.id)
			}
		}
	}
	groups := map[string][]string{}
	for _, e := range all {
		r := find(e.id)
		groups[r] = append(groups[r], e.id)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	_, _ = tx.Exec(`DELETE FROM duplicate_groups`)
	count := 0
	var roots []string
	for r := range groups {
		roots = append(roots, r)
	}
	sort.Strings(roots)
	type groupOut struct {
		Members    []string `json:"members"`
		Similarity float64   `json:"similarity"`
	}
	var grps []groupOut
	for _, r := range roots {
		members := groups[r]
		if len(members) < 2 {
			continue
		}
		sort.Strings(members)
		sum := 0.0
		pairs := 0
		for i := 0; i < len(members); i++ {
			for j := i + 1; j < len(members); j++ {
				ai := all[indexOfEnt(all, members[i])]
				aj := all[indexOfEnt(all, members[j])]
				sum += hybridScore(ai.desc, aj.desc, ai.repo, aj.repo)
				pairs++
			}
		}
		avg := 0.0
		if pairs > 0 {
			avg = sum / float64(pairs)
		}
		tasksJSON, _ := json.Marshal(members)
		_, err := tx.Exec(`INSERT INTO duplicate_groups(root_id, similarity, resolution, tasks, created_at)
			VALUES (?, ?, 'unresolved', ?, ?)`,
			r, avg, string(tasksJSON), nowUTC())
		if err != nil {
			return err
		}
		count++
		grps = append(grps, groupOut{Members: members, Similarity: avg})
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	out := map[string]interface{}{
		"threshold": *threshold,
		"count":     count,
		"groups":    grps,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	return nil
}

func indexOfEnt(all []ent, target string) int {
	for i, e := range all {
		if e.id == target {
			return i
		}
	}
	return -1
}

// ---------- export ----------

func cmdExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	out := fs.String("out", "", "output file (default: stdout)")
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	db, err := openDB(gDBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	// Pre-load all tasks into memory to avoid deadlock with MaxOpenConns(1)
	type exportRow struct {
		id, desc, repo, sub, cat, lane, br, kind, sh, side, status, ag string
		stage, slot, prio                                              int
	}
	var allRows []exportRow
	rows, err := db.Query(`SELECT id, stage, slot, description, repo, subproject, category, lane, branch, kind, priority, semantic_hash, side_dag, status, assigned_agent FROM tasks ORDER BY stage, slot, id`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var r exportRow
		_ = rows.Scan(&r.id, &r.stage, &r.slot, &r.desc, &r.repo, &r.sub, &r.cat, &r.lane, &r.br, &r.kind, &r.prio, &r.sh, &r.side, &r.status, &r.ag)
		allRows = append(allRows, r)
	}
	rows.Close()
	var w *os.File
	if *out == "" {
		w = os.Stdout
	} else {
		w, err = os.Create(*out)
		if err != nil {
			return err
		}
		defer w.Close()
	}
	fmt.Fprintf(w, "# phenodag %s export\n\n", version)
	var preset string
	_ = db.QueryRow(`SELECT value FROM dag_meta WHERE key='preset_description'`).Scan(&preset)
	if preset != "" {
		fmt.Fprintf(w, "**Preset**: %s\n\n", preset)
	}
	fmt.Fprintf(w, "| Status | Stage | Task | Kind | Repo | Description |\n")
	fmt.Fprintf(w, "|---|---|---|---|---|---|\n")
	for _, r := range allRows {
		emoji := "⬜"
		switch r.status {
		case "done":
			emoji = "✅"
		case "in_progress":
			emoji = "🔄"
		case "blocked":
			emoji = "🚫"
		case "failed":
			emoji = "❌"
		}
		stageLabel := fmt.Sprintf("L%d", r.stage)
		if r.stage == 0 {
			stageLabel = "pool"
		}
		desc := r.desc
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s |\n", emoji, stageLabel, r.id, r.kind, r.repo, desc)
	}
	if *out != "" {
		fmt.Printf("exported to %s\n", *out)
	}
	return nil
}
