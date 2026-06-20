// phenodag_v3.go — dagctl v3 20x6 engine + L7 SUSTAIN2 (port).
//
// Ported from C:/Users/koosh/Dev/dagctl/dagctl_v3_seed.go and
// dagctl_v3_extend2.go on 2026-06-16 for the superset merge.
// Preserves phenodag's existing 4 presets (v3-180, melosviz-185,
// agileplus-50, tracera-50) and fuzzy-dedup. New presets stack on top:
// v3-260, v3-extend2-295, v3-extend3-415.
//
// Adds three commands: seed3, extend3-v2, extend3-v3.
package main

import (
	"flag"
	"fmt"
	"sync"
	"time"
)

const v3PortVersion = "v3.2.0-port"

var v3PortRepos = []string{
	"agent-user-status", "agentapi-plusplus", "Agentora", "AgilePlus",
	"HexaKit", "PhenoDevOps", "Pyron", "pheno", "FocalPoint", "HeliosCLI",
	"helioscope", "PhenoProc", "phenokits-commons", "localbase3", "phenoRuntime",
	"Tracera", "phenoShared", "PhenoObservability", "Tracely", "thegent",
	"thegent-fresh", "thegent-jsonl", "AgentMCP", "Agslag",
}

// v3PortSubprojects — 4 subprojects × 5 slots = 20 per layer.
var v3PortSubprojects = []string{"phenotype", "agentora", "phenoX", "phenoCore"}
var v3PortSlots = []int{1, 2, 3, 4, 5}

// v3PortStages — L1:audit, L2:hygiene, L3:test, L4:tooling, L5:governance, L6:sota.
var v3PortStages = []struct {
	Stage int
	Kind  string
	Label string
}{
	{1, "audit", "audit"},
	{2, "hygiene", "hygiene"},
	{3, "test", "test"},
	{4, "tooling", "tooling"},
	{5, "governance", "governance"},
	{6, "sota", "sota"},
}

// v3PortTask — port-local task struct (avoids modifying phenodag.go's
// seedTask; uses all available tasks-table columns).
type v3PortTask struct {
	ID, Description, Subproject, Category, Lane, Branch, Status, Kind, SideDAG string
	Stage, Slot, Priority                                                      int
}

func v3PortHash(s string) string {
	return hashID(0, 0, s) // stable 16-char hex
}

func v3PortBuildCore() []v3PortTask {
	tasks := []v3PortTask{}
	for _, s := range v3PortStages {
		for _, sub := range v3PortSubprojects {
			for _, slot := range v3PortSlots {
				id := fmt.Sprintf("task-%d%02d-%02d", s.Stage, indexOfV3PortSub(sub)+1, slot)
				desc := fmt.Sprintf("L%d %s for %s subproject slot %d: %s work on the %s track.",
					s.Stage, s.Label, sub, slot, s.Label, sub)
				_ = v3PortRepos
				tasks = append(tasks, v3PortTask{
					ID: id, Stage: s.Stage, Slot: slot, Description: desc, Subproject: sub,
					Category: s.Label, Lane: sub, Branch: "main", Status: "ready", Kind: s.Kind, Priority: 5,
				})
			}
		}
	}
	return tasks
}

func indexOfV3PortSub(s string) int {
	for i, x := range v3PortSubprojects {
		if x == s {
			return i
		}
	}
	return 0
}

func v3PortBuildSide() []v3PortTask {
	sideDAGs := []string{
		"sd-ci", "sd-docs", "sd-error", "sd-fuzz", "sd-sota", "sd-type",
		"sd-perf", "sd-libify", "sd-test", "sd-release", "sd-obs", "sd-audit",
	}
	tasks := []v3PortTask{}
	idx := 0
	for _, sd := range sideDAGs {
		for i := 1; i <= 5; i++ {
			repo := v3PortRepos[idx%len(v3PortRepos)]
			id := fmt.Sprintf("%s-%02d", sd, i)
			desc := fmt.Sprintf("%s task %02d for %s: cross-cutting %s work.", sd, i, repo, sd)
			tasks = append(tasks, v3PortTask{
				ID: id, Stage: 1, Slot: i, Description: desc, Subproject: repo,
				Category: sd, Lane: sd, Branch: "main", Status: "ready", Kind: "hygiene", Priority: 5,
				SideDAG: sd,
			})
			idx++
		}
	}
	return tasks
}

// v3PortL7Sustain2 — L7 SUSTAIN2 (20 tasks, 4 subprojects × 5 slots).
var v3PortL7Sustain2 = []v3PortTask{
	{ID: "task-07-01-01", Stage: 7, Subproject: "phenotype", Slot: 1, Category: "observability", Kind: "sota", Priority: 8, Status: "ready", Description: "SUSTAIN2: OTel + exemplars + 4-SLO windows for phenotype-registry."},
	{ID: "task-07-01-02", Stage: 7, Subproject: "phenotype", Slot: 2, Category: "drift-detect", Kind: "audit", Priority: 7, Status: "ready", Description: "SUSTAIN2: daily drift detector with golden SQLite comparison."},
	{ID: "task-07-01-03", Stage: 7, Subproject: "phenotype", Slot: 3, Category: "runbook", Kind: "governance", Priority: 7, Status: "ready", Description: "SUSTAIN2: 25-error-code runbook with deeplinks to metrics+traces."},
	{ID: "task-07-01-04", Stage: 7, Subproject: "phenotype", Slot: 4, Category: "self-heal", Kind: "governance", Priority: 6, Status: "ready", Description: "SUSTAIN2: self-heal loop with stale-heartbeat reclaim and auto-merge of dups."},
	{ID: "task-07-01-05", Stage: 7, Subproject: "phenotype", Slot: 5, Category: "capacity", Kind: "tooling", Priority: 6, Status: "ready", Description: "SUSTAIN2: capacity plan with 2x peak headroom + load-test replay."},
	{ID: "task-07-02-01", Stage: 7, Subproject: "agentora", Slot: 1, Category: "observability", Kind: "sota", Priority: 8, Status: "ready", Description: "SUSTAIN2: agent runtime OTel with per-agent exemplar linking."},
	{ID: "task-07-02-02", Stage: 7, Subproject: "agentora", Slot: 2, Category: "drift-detect", Kind: "audit", Priority: 7, Status: "ready", Description: "SUSTAIN2: agent prompt-version drift detector with semantic-hash pinning."},
	{ID: "task-07-02-03", Stage: 7, Subproject: "agentora", Slot: 3, Category: "runbook", Kind: "governance", Priority: 7, Status: "ready", Description: "SUSTAIN2: agent failure runbook with 30 patterns + recovery scripts."},
	{ID: "task-07-02-04", Stage: 7, Subproject: "agentora", Slot: 4, Category: "self-heal", Kind: "governance", Priority: 6, Status: "ready", Description: "SUSTAIN2: agent self-heal with idempotent retry + circuit-breaker."},
	{ID: "task-07-02-05", Stage: 7, Subproject: "agentora", Slot: 5, Category: "capacity", Kind: "tooling", Priority: 6, Status: "ready", Description: "SUSTAIN2: agent capacity plan with 3x burst + autoscaler."},
	{ID: "task-07-03-01", Stage: 7, Subproject: "phenoX", Slot: 1, Category: "observability", Kind: "sota", Priority: 8, Status: "ready", Description: "SUSTAIN2: cross-SDK OTel with stable attribute keys and traceparent propagation."},
	{ID: "task-07-03-02", Stage: 7, Subproject: "phenoX", Slot: 2, Category: "drift-detect", Kind: "audit", Priority: 7, Status: "ready", Description: "SUSTAIN2: cross-SDK API drift detector with `phenotype-registry` source-of-truth."},
	{ID: "task-07-03-03", Stage: 7, Subproject: "phenoX", Slot: 3, Category: "runbook", Kind: "governance", Priority: 7, Status: "ready", Description: "SUSTAIN2: cross-SDK runbook for error envelope + retry/backoff canonical usage."},
	{ID: "task-07-03-04", Stage: 7, Subproject: "phenoX", Slot: 4, Category: "self-heal", Kind: "governance", Priority: 6, Status: "ready", Description: "SUSTAIN2: cross-SDK self-heal with idempotency keys + jittered backoff."},
	{ID: "task-07-03-05", Stage: 7, Subproject: "phenoX", Slot: 5, Category: "capacity", Kind: "tooling", Priority: 6, Status: "ready", Description: "SUSTAIN2: cross-SDK capacity plan with per-SDK p99 budget."},
	{ID: "task-07-04-01", Stage: 7, Subproject: "phenoCore", Slot: 1, Category: "observability", Kind: "sota", Priority: 8, Status: "ready", Description: "SUSTAIN2: phenoCore OTel with exemplars + exemplar-to-trace linking."},
	{ID: "task-07-04-02", Stage: 7, Subproject: "phenoCore", Slot: 2, Category: "drift-detect", Kind: "audit", Priority: 7, Status: "ready", Description: "SUSTAIN2: phenoCore semantic_hash drift detector with daily verify."},
	{ID: "task-07-04-03", Stage: 7, Subproject: "phenoCore", Slot: 3, Category: "runbook", Kind: "governance", Priority: 7, Status: "ready", Description: "SUSTAIN2: phenoCore runbook with 50 patterns and SOTA recovery scripts."},
	{ID: "task-07-04-04", Stage: 7, Subproject: "phenoCore", Slot: 4, Category: "self-heal", Kind: "governance", Priority: 6, Status: "ready", Description: "SUSTAIN2: phenoCore self-heal with auto-merge dedup and dead-letter."},
	{ID: "task-07-04-05", Stage: 7, Subproject: "phenoCore", Slot: 5, Category: "capacity", Kind: "tooling", Priority: 6, Status: "ready", Description: "SUSTAIN2: phenoCore capacity with 4x headroom + horizontal autoscaler."},
}

// cmdSeed3 — equivalent of dagctl's seed3: 120 core (L1-L6) + 60 side + L7 SUSTAIN2 (20).
// New preset name: v3-260.
func cmdSeed3(args []string) error {
	fs := flag.NewFlagSet("seed3", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)

	// Build core, side, L7 in parallel (3 goroutines; phenodag's pattern).
	var (
		core, side, l7 []v3PortTask
		wg             sync.WaitGroup
	)
	wg.Add(3)
	go func() { defer wg.Done(); core = v3PortBuildCore() }()
	go func() { defer wg.Done(); side = v3PortBuildSide() }()
	go func() { defer wg.Done(); l7 = v3PortL7Sustain2 }()
	wg.Wait()

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
	defer tx.Rollback()
	stmt, _ := tx.Prepare(`INSERT OR IGNORE INTO tasks
		(id, stage, slot, description, repo, subproject, category, lane, branch, kind, priority, semantic_hash, side_dag, status, assigned_agent, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	defer stmt.Close()
	edge, _ := tx.Prepare(`INSERT OR IGNORE INTO edges(from_task, to_task) VALUES (?, ?)`)
	defer edge.Close()
	sideMeta, _ := tx.Prepare(`INSERT OR IGNORE INTO side_dags(id, name, description) VALUES (?, ?, ?)`)
	defer sideMeta.Close()

	sideDAGDescs := map[string]string{
		"sd-ci": "CI hardening", "sd-docs": "Documentation", "sd-error": "Error envelope",
		"sd-fuzz": "Fuzzing", "sd-sota": "SOTA", "sd-type": "Type strict",
		"sd-perf": "Performance", "sd-libify": "Libification", "sd-test": "Test rigor",
		"sd-release": "Release", "sd-obs": "Observability", "sd-audit": "Audit",
	}
	for id, desc := range sideDAGDescs {
		_, _ = sideMeta.Exec(id, id, desc)
	}

	now := nowUTC()
	insertV3PortTask := func(t v3PortTask) error {
		_, err := stmt.Exec(
			t.ID, t.Stage, t.Slot, t.Description, "", t.Subproject, t.Category, t.Lane, t.Branch,
			t.Kind, t.Priority, v3PortHash(t.Description), t.SideDAG, t.Status, "", now, now,
		)
		return err
	}

	for _, t := range core {
		if err := insertV3PortTask(t); err != nil {
			return err
		}
	}
	for _, t := range side {
		if err := insertV3PortTask(t); err != nil {
			return err
		}
	}
	for _, t := range l7 {
		if err := insertV3PortTask(t); err != nil {
			return err
		}
	}

	// Wire L1->L2->L3->L4->L5->L6 per (subproject, slot)
	for _, sub := range v3PortSubprojects {
		for _, slot := range v3PortSlots {
			for stage := 1; stage < 6; stage++ {
				from := fmt.Sprintf("task-%d%02d-%02d", stage, indexOfV3PortSub(sub)+1, slot)
				to := fmt.Sprintf("task-%d%02d-%02d", stage+1, indexOfV3PortSub(sub)+1, slot)
				_, _ = edge.Exec(from, to)
			}
		}
	}
	// Wire L6 -> L7 per (subproject, slot)
	for _, sub := range v3PortSubprojects {
		for _, slot := range v3PortSlots {
			from := fmt.Sprintf("task-6%02d-%02d", indexOfV3PortSub(sub)+1, slot)
			to := fmt.Sprintf("task-07-%02d-%02d", indexOfV3PortSub(sub)+1, slot)
			_, _ = edge.Exec(from, to)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Update meta
	for _, kv := range [][2]string{
		{"preset", "v3-260"},
		{"preset_description", "v3-260: 120 core (L1-L6) + 60 side + 20 L7 SUSTAIN2 = 200+60"},
		{"shape", "20x7"},
		{"stages", "7"},
		{"version", v3PortVersion},
		{"last_updated", now},
	} {
		_, _ = db.Exec(`INSERT INTO dag_meta(key, value) VALUES(?, ?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, kv[0], kv[1])
	}
	fmt.Printf("seeded v3-260: %d core (L1-L6) + %d L7 + %d side = %d total (shape 20x7)\n",
		len(core), len(l7), len(side), len(core)+len(side)+len(l7))
	return nil
}

// v3PortL8 — L8 SOTA (20 tasks, 1:1 with L7).
var v3PortL8 = []v3PortTask{
	{ID: "task-08-01", Stage: 8, Subproject: "agent-user-status", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "Async-actor model with per-agent mailbox + OTel exemplars + 60fps pprof samples."},
	{ID: "task-08-02", Stage: 8, Subproject: "agentapi-plusplus", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "Streaming JSONL over SSE + backpressure + idempotency keys + dedup-via-semantic-hash."},
	{ID: "task-08-03", Stage: 8, Subproject: "Agentora", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "Agent-orchestrator: plan/review/execute loop with SOTA self-consistency + verifier."},
	{ID: "task-08-04", Stage: 8, Subproject: "AgilePlus", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "Backlog with SOTA deps graph + critical-path analysis + cycle-detection on insertion."},
	{ID: "task-08-05", Stage: 8, Subproject: "HexaKit", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "Hexagonal layering: domain->ports->adapters; OTel spans per port call; feature-flagged adapters."},
	{ID: "task-08-06", Stage: 8, Subproject: "PhenoDevOps", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "OIDC workload identity + cosign keyless + SBOM SLSA L3 + build provenance attestation."},
	{ID: "task-08-07", Stage: 8, Subproject: "Pyron", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "SOTA async Rust runtime with work-stealing scheduler and structured cancellation."},
	{ID: "task-08-08", Stage: 8, Subproject: "pheno", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "Type-stable graph IR + diff/merge + provenance hashes; SOTA type system for fleet tasks."},
	{ID: "task-08-09", Stage: 8, Subproject: "FocalPoint", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "Window-focus tracking via CGSPrivate + OTel metrics + Spotlight deeplinks."},
	{ID: "task-08-10", Stage: 8, Subproject: "HeliosCLI", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "TUI with Bubble Tea, multi-pane, key-binds, JSONL output for downstream agents."},
	{ID: "task-08-11", Stage: 8, Subproject: "helioscope", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "Distributed tracing explorer with exemplar-to-trace linking + flame graph diff."},
	{ID: "task-08-12", Stage: 8, Subproject: "PhenoProc", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "Process supervisor with restart-on-panic, OTel span around child, sigchld race-free."},
	{ID: "task-08-13", Stage: 8, Subproject: "phenokits-commons", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "Common scaffold and kit substrate: parsing, linting, scaffolding with AI-on-failure."},
	{ID: "task-08-14", Stage: 8, Subproject: "localbase3", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "Local-first store with CRDT + SQLite WAL + OTel for every mutation."},
	{ID: "task-08-15", Stage: 8, Subproject: "phenoRuntime", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "WASM host with fuel metering, capability-based security, OTel per-invocation."},
	{ID: "task-08-16", Stage: 8, Subproject: "Tracera", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "Trace-aware retry/backoff + jitter + circuit-breaker with OTel error events."},
	{ID: "task-08-17", Stage: 8, Subproject: "phenoShared", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "Cross-SDK abstractions: Result, Error, RetryPolicy, all with stable OTel attribute keys."},
	{ID: "task-08-18", Stage: 8, Subproject: "PhenoObservability", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "OTel collector pipeline with tail-sampling, exemplars, exemplar->trace linking."},
	{ID: "task-08-19", Stage: 8, Subproject: "Tracely", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "End-to-end trace browser: HTTP, gRPC, SQL, OTel exemplar correlation."},
	{ID: "task-08-20", Stage: 8, Subproject: "thegent", Category: "sota", Kind: "sota", Priority: 9, Status: "ready", Description: "SOTA agent protocol: task envelope, claim/release, dedup, audit, all OTel-aware."},
}

// v3PortSideExtend2 — 3 new side-DAGs (sd-sota-research, sd-supply-chain, sd-cross-cutting).
var v3PortSideExtend2 = map[string][]v3PortTask{
	"sd-sota-research": {
		{ID: "sd-sota-research-01", Stage: 8, Subproject: "cross-cutting", Category: "sota", Kind: "sota", Priority: 8, Status: "ready", SideDAG: "sd-sota-research", Description: "Weekly SOTA scan: arxiv-sanity + HuggingFace papers + AlphaSignal; track frontier agents, code-gen, retrieval."},
		{ID: "sd-sota-research-02", Stage: 8, Subproject: "cross-cutting", Category: "sota", Kind: "sota", Priority: 8, Status: "ready", SideDAG: "sd-sota-research", Description: "Compile a SOTA leaderboard for fleet tasks: accuracy, latency, cost, energy, determinism."},
		{ID: "sd-sota-research-03", Stage: 8, Subproject: "cross-cutting", Category: "sota", Kind: "sota", Priority: 8, Status: "ready", SideDAG: "sd-sota-research", Description: "Implement 1 frontier technique per sprint: RAG, agentic loops, code-as-actions, structured outputs."},
		{ID: "sd-sota-research-04", Stage: 8, Subproject: "cross-cutting", Category: "sota", Kind: "sota", Priority: 8, Status: "ready", SideDAG: "sd-sota-research", Description: "Cross-SDK eval harness: bench every SDK against the same eval set; report regressions."},
		{ID: "sd-sota-research-05", Stage: 8, Subproject: "cross-cutting", Category: "sota", Kind: "sota", Priority: 8, Status: "ready", SideDAG: "sd-sota-research", Description: "Open-source the eval set and the runner; subscribe to upstream changes and re-bench."},
	},
	"sd-supply-chain": {
		{ID: "sd-supply-chain-01", Stage: 8, Subproject: "cross-cutting", Category: "supply", Kind: "governance", Priority: 7, Status: "ready", SideDAG: "sd-supply-chain", Description: "SLSA L3 build provenance: cosign sign + in-toto attestations; verify in CI."},
		{ID: "sd-supply-chain-02", Stage: 8, Subproject: "cross-cutting", Category: "supply", Kind: "governance", Priority: 7, Status: "ready", SideDAG: "sd-supply-chain", Description: "SBOM in SPDX 2.3 for every release; CVE feed subscription; auto-PR for known CVEs."},
		{ID: "sd-supply-chain-03", Stage: 8, Subproject: "cross-cutting", Category: "supply", Kind: "governance", Priority: 7, Status: "ready", SideDAG: "sd-supply-chain", Description: "VEX (Vulnerability Exploitability eXchange) for false-positive CVEs in transitive deps."},
		{ID: "sd-supply-chain-04", Stage: 8, Subproject: "cross-cutting", Category: "supply", Kind: "governance", Priority: 7, Status: "ready", SideDAG: "sd-supply-chain", Description: "Sigstore + keyless signing; rotate keys; reject unsigned artifacts at the registry."},
		{ID: "sd-supply-chain-05", Stage: 8, Subproject: "cross-cutting", Category: "supply", Kind: "governance", Priority: 7, Status: "ready", SideDAG: "sd-supply-chain", Description: "Internal registry mirror with rate-limiting, license drift detection, and dep audit."},
	},
	"sd-cross-cutting": {
		{ID: "sd-cross-cutting-01", Stage: 8, Subproject: "cross-cutting", Category: "cross", Kind: "governance", Priority: 8, Status: "ready", SideDAG: "sd-cross-cutting", Description: "Cross-SDK error envelope with stable JSON shape, OTel exception event, dedup-via-hash."},
		{ID: "sd-cross-cutting-02", Stage: 8, Subproject: "cross-cutting", Category: "cross", Kind: "governance", Priority: 8, Status: "ready", SideDAG: "sd-cross-cutting", Description: "Cross-SDK tracing context propagation: W3C traceparent in HTTP, gRPC, JSONL, file headers."},
		{ID: "sd-cross-cutting-03", Stage: 8, Subproject: "cross-cutting", Category: "cross", Kind: "governance", Priority: 8, Status: "ready", SideDAG: "sd-cross-cutting", Description: "Cross-SDK feature parity matrix; auto-detect drift, file issues, track in `phenotype-registry`."},
		{ID: "sd-cross-cutting-04", Stage: 8, Subproject: "cross-cutting", Category: "cross", Kind: "governance", Priority: 8, Status: "ready", SideDAG: "sd-cross-cutting", Description: "Cross-SDK retry/backoff: jittered exponential, idempotency keys, OTel retry counter."},
		{ID: "sd-cross-cutting-05", Stage: 8, Subproject: "cross-cutting", Category: "cross", Kind: "governance", Priority: 8, Status: "ready", SideDAG: "sd-cross-cutting", Description: "Cross-SDK config: TOML+env+flags with precedence; dotenv auto-load; `pheno-config` schema."},
	},
}

// cmdExtend3V2 — equivalent of dagctl's extend3-v2: 20 L8 + 15 side (3 new DAGs).
// New preset name: v3-extend2-295.
func cmdExtend3V2(args []string) error {
	fs := flag.NewFlagSet("extend3-v2", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)

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
	defer tx.Rollback()
	stmt, _ := tx.Prepare(`INSERT OR IGNORE INTO tasks
		(id, stage, slot, description, repo, subproject, category, lane, branch, kind, priority, semantic_hash, side_dag, status, assigned_agent, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	defer stmt.Close()
	edge, _ := tx.Prepare(`INSERT OR IGNORE INTO edges(from_task, to_task) VALUES (?, ?)`)
	defer edge.Close()
	sideMeta, _ := tx.Prepare(`INSERT OR IGNORE INTO side_dags(id, name, description) VALUES (?, ?, ?)`)
	defer sideMeta.Close()

	_, _ = sideMeta.Exec("sd-sota-research", "sd-sota-research", "SOTA research: arxiv scan, leaderboard, eval harness.")
	_, _ = sideMeta.Exec("sd-supply-chain", "sd-supply-chain", "Supply chain: SLSA, SBOM, VEX, cosign, registry mirror.")
	_, _ = sideMeta.Exec("sd-cross-cutting", "sd-cross-cutting", "Cross-SDK: error envelope, tracing context, feature parity.")

	now := nowUTC()
	insertV3PortTask := func(t v3PortTask) error {
		_, err := stmt.Exec(
			t.ID, t.Stage, t.Slot, t.Description, t.Subproject, t.Subproject, t.Category, t.Subproject, t.Branch,
			t.Kind, t.Priority, v3PortHash(t.Description), t.SideDAG, t.Status, "", now, now,
		)
		return err
	}

	for _, t := range v3PortL8 {
		if err := insertV3PortTask(t); err != nil {
			return err
		}
	}
	for _, sideTasks := range v3PortSideExtend2 {
		for _, t := range sideTasks {
			// For side tasks, repo/subproject is "cross-cutting" not the subproject field
			t2 := t
			t2.Subproject = "cross-cutting"
			if err := insertV3PortTask(t2); err != nil {
				return err
			}
		}
	}

	// Wire L7 -> L8 (1:1, indexed cat*5+slot)
	for cat := 1; cat <= 4; cat++ {
		for slot := 1; slot <= 5; slot++ {
			from := fmt.Sprintf("task-07-%02d-%02d", cat, slot)
			to := fmt.Sprintf("task-08-%02d", (cat-1)*5+slot)
			_, _ = edge.Exec(from, to)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, kv := range [][2]string{
		{"preset", "v3-extend2-295"},
		{"preset_description", "v3-extend2-295: 120 core + 60 side + 20 L7 + 20 L8 + 15 side-extend2 = 235+60"},
		{"last_updated", now},
	} {
		_, _ = db.Exec(`INSERT INTO dag_meta(key, value) VALUES(?, ?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, kv[0], kv[1])
	}
	totalSide := 0
	for _, sideTasks := range v3PortSideExtend2 {
		totalSide += len(sideTasks)
	}
	fmt.Printf("extended v3: %d L8 + %d side-extend2 tasks\n", len(v3PortL8), totalSide)
	_ = time.Now // silence unused import on Windows-only path
	return nil
}

// v3PortL9 — L9 DEPLOY & OPERATE (20 tasks).
var v3PortL9 = []v3PortTask{
	{ID: "task-09-01", Stage: 9, Subproject: "agent-user-status", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Progressive rollout: shadow traffic 1% -> 10% -> 50% -> 100% with OTel-driven kill switch."},
	{ID: "task-09-02", Stage: 9, Subproject: "agentapi-plusplus", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Blue/green with smoke-tests + auto-rollback on SLO breach within 5 min."},
	{ID: "task-09-03", Stage: 9, Subproject: "Agentora", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Canary with counterfactual replay; promote on Pareto improvement on cost+latency."},
	{ID: "task-09-04", Stage: 9, Subproject: "AgilePlus", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Feature-flag-driven rollout; per-flag SLO dashboards; auto-disable on regression."},
	{ID: "task-09-05", Stage: 9, Subproject: "HexaKit", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Hexagonal adapter: env-isolated integration tests in CI; canary via OTel exemplars."},
	{ID: "task-09-06", Stage: 9, Subproject: "PhenoDevOps", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "GitOps with ArgoCD app-of-apps; drift detection; sync-window per env."},
	{ID: "task-09-07", Stage: 9, Subproject: "Pyron", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Hot-patch via cargo-dylib-link; restart-free config reload; SRE-friendly signal handling."},
	{ID: "task-09-08", Stage: 9, Subproject: "pheno", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Type-stable graph IR deploy: forward+backward compat; semver-pinned contract tests."},
	{ID: "task-09-09", Stage: 9, Subproject: "FocalPoint", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Notarized + stapled .pkg; Sparkle-based auto-update; launchd-friendly crash reporting."},
	{ID: "task-09-10", Stage: 9, Subproject: "HeliosCLI", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Homebrew tap + cosign-signed bottles; semver with deprecation warnings; auto-migrate aliases."},
	{ID: "task-09-11", Stage: 9, Subproject: "helioscope", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Edge deploy via Cloudflare Workers; R2-backed trace archive; per-tenant throttling."},
	{ID: "task-09-12", Stage: 9, Subproject: "PhenoProc", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Daemon-style deploy via launchd/systemd; graceful shutdown with in-flight drain."},
	{ID: "task-09-13", Stage: 9, Subproject: "phenokits-commons", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Common kit release via OIDC; provenance attestations; staged rollout via registry API."},
	{ID: "task-09-14", Stage: 9, Subproject: "localbase3", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Embedded WASM module with deterministic startup; embedded migrations + safety check."},
	{ID: "task-09-15", Stage: 9, Subproject: "phenoRuntime", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "WASM module deploy via wadm/crane; multi-region replication; canary release channels."},
	{ID: "task-09-16", Stage: 9, Subproject: "Tracera", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Schema-migration with online (no-downtime) and offline (downtime) modes; feature-flagged rollout."},
	{ID: "task-09-17", Stage: 9, Subproject: "phenoShared", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Single-source-of-truth SDK bundle; multi-arch build matrix; auto-bumps downstream."},
	{ID: "task-09-18", Stage: 9, Subproject: "PhenoObservability", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Collector config: dynamic per-tenant pipelines; SLO-driven sampling; cost budgets per team."},
	{ID: "task-09-19", Stage: 9, Subproject: "Tracely", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Trace-archival: hot (7d) + warm (30d) + cold (1y) tiers; S3 + Glacier; restore SLA <1h."},
	{ID: "task-09-20", Stage: 9, Subproject: "thegent", Category: "deploy", Kind: "release", Priority: 8, Status: "ready", Description: "Agent runtime deploy: per-tenant isolation; rate limit; audit log; kill switch."},
}

// v3PortL10 — L10 SUSTAIN3 (20 tasks).
var v3PortL10 = []v3PortTask{
	{ID: "task-10-01", Stage: 10, Subproject: "agent-user-status", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Weekly retro: incidents + SLO breaches + agent failures; file follow-up tasks in this DAG."},
	{ID: "task-10-02", Stage: 10, Subproject: "agentapi-plusplus", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Quarterly debt-retire: pick top 3 pain-points; refactor + delete redundant code paths."},
	{ID: "task-10-03", Stage: 10, Subproject: "Agentora", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Knowledge base: convert high-quality agent outputs into reusable skills; tag + version."},
	{ID: "task-10-04", Stage: 10, Subproject: "AgilePlus", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Process retro: backlog hygiene review; cycle-time trend; DORA metrics dashboard."},
	{ID: "task-10-05", Stage: 10, Subproject: "HexaKit", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Architecture decision records (ADRs): one per non-trivial change; review quarterly."},
	{ID: "task-10-06", Stage: 10, Subproject: "PhenoDevOps", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Post-mortem template: blameless, with action items tracked in this DAG; follow-up SLA."},
	{ID: "task-10-07", Stage: 10, Subproject: "Pyron", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Dependency-update automation: auto-merge patch updates, manual review minors; weekly digest."},
	{ID: "task-10-08", Stage: 10, Subproject: "pheno", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Deprecation cycle: announce in CHANGELOG, soft-remove, hard-remove after 2 minor versions."},
	{ID: "task-10-09", Stage: 10, Subproject: "FocalPoint", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "macOS-version support matrix: keep last 3 majors; quarterly test on betas; phase-out policy."},
	{ID: "task-10-10", Stage: 10, Subproject: "HeliosCLI", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "User-feedback loop: in-CLI `feedback` command; auto-routes to maintainer; weekly review."},
	{ID: "task-10-11", Stage: 10, Subproject: "helioscope", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Telemetry opt-in with anonymized sample; quarterly review of what we collect; auto-purge policy."},
	{ID: "task-10-12", Stage: 10, Subproject: "PhenoProc", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Process-compose: monthly revision; sunset un-used recipes; tag stable/release-1.x."},
	{ID: "task-10-13", Stage: 10, Subproject: "phenokits-commons", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Common template deprecation: auto-detect unused templates; archive with 30-day notice; sunset."},
	{ID: "task-10-14", Stage: 10, Subproject: "localbase3", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Storage migration: format-versioned; forward-migrate on read; deprecate old paths after 2 majors."},
	{ID: "task-10-15", Stage: 10, Subproject: "phenoRuntime", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "WASM spec-versioning: pin targets, lint for new features; warn on deprecated ABIs."},
	{ID: "task-10-16", Stage: 10, Subproject: "Tracera", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Data-retention: 90d hot, 1y warm, 7y cold; quarterly audit; GDPR right-to-erasure flow."},
	{ID: "task-10-17", Stage: 10, Subproject: "phenoShared", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "SDK stability tiers: alpha/beta/ga; clear contract on what's frozen; semver-2.0.0 lock."},
	{ID: "task-10-18", Stage: 10, Subproject: "PhenoObservability", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Cost dashboard: per-tenant OTel ingestion; alert on budget; right-size sampling weekly."},
	{ID: "task-10-19", Stage: 10, Subproject: "Tracely", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "User-feedback on trace UI: in-app `?` button; auto-routes to design+eng; monthly review."},
	{ID: "task-10-20", Stage: 10, Subproject: "thegent", Category: "sustain", Kind: "governance", Priority: 7, Status: "ready", Description: "Agent protocol evolution: spec + deprecation matrix; 1-yr migration window; clear migration guide."},
}

// v3PortSideExtend3 — 4 new side-DAGs: sd-evals, sd-observability, sd-monorepo, sd-agent-mesh.
var v3PortSideExtend3 = map[string][]v3PortTask{
	"sd-evals": {
		{ID: "sd-evals-01", Stage: 9, Subproject: "cross-cutting", Category: "evals", Kind: "test", Priority: 7, Status: "ready", SideDAG: "sd-evals", Description: "Build shared eval-harness: golden prompts, expected outputs, scoring rubric, regression detection."},
		{ID: "sd-evals-02", Stage: 9, Subproject: "cross-cutting", Category: "evals", Kind: "test", Priority: 7, Status: "ready", SideDAG: "sd-evals", Description: "Per-SDK eval-runs on every PR: bench against frozen eval set; report deltas in PR comment."},
		{ID: "sd-evals-03", Stage: 9, Subproject: "cross-cutting", Category: "evals", Kind: "test", Priority: 7, Status: "ready", SideDAG: "sd-evals", Description: "Cost-corrected leaderboard: cost + accuracy + latency normalized to a single Pareto front."},
		{ID: "sd-evals-04", Stage: 9, Subproject: "cross-cutting", Category: "evals", Kind: "test", Priority: 7, Status: "ready", SideDAG: "sd-evals", Description: "Determinism tests: rerun N times, check identical outputs; fail on >1% divergence."},
		{ID: "sd-evals-05", Stage: 9, Subproject: "cross-cutting", Category: "evals", Kind: "test", Priority: 7, Status: "ready", SideDAG: "sd-evals", Description: "Long-running regression: 24h soak test, check for memory leaks, deadlocks, state corruption."},
	},
	"sd-observability": {
		{ID: "sd-observability-01", Stage: 9, Subproject: "cross-cutting", Category: "obs", Kind: "release", Priority: 8, Status: "ready", SideDAG: "sd-observability", Description: "Per-service SLI/SLO: availability + latency + error budget; auto-track + dashboard."},
		{ID: "sd-observability-02", Stage: 9, Subproject: "cross-cutting", Category: "obs", Kind: "release", Priority: 8, Status: "ready", SideDAG: "sd-observability", Description: "Multi-window burn-rate alerts: 1h/6h/24h/3d; reduce noise; auto-page on budget exhaustion."},
		{ID: "sd-observability-03", Stage: 9, Subproject: "cross-cutting", Category: "obs", Kind: "release", Priority: 8, Status: "ready", SideDAG: "sd-observability", Description: "On-call rotation with escalation; alert routing per service; shadow weeks for new joiners."},
		{ID: "sd-observability-04", Stage: 9, Subproject: "cross-cutting", Category: "obs", Kind: "release", Priority: 8, Status: "ready", SideDAG: "sd-observability", Description: "Auto-generated runbooks: span -> log -> dashboard -> code; clickable from alert."},
		{ID: "sd-observability-05", Stage: 9, Subproject: "cross-cutting", Category: "obs", Kind: "release", Priority: 8, Status: "ready", SideDAG: "sd-observability", Description: "Error-budget governance: feature-freeze when budget <50%; auto-resume when recovered."},
	},
	"sd-monorepo": {
		{ID: "sd-monorepo-01", Stage: 10, Subproject: "cross-cutting", Category: "monorepo", Kind: "tooling", Priority: 7, Status: "ready", SideDAG: "sd-monorepo", Description: "Decide mono-vs-poly: 21+ crates suggest monorepo; Cargo workspace + selective CI."},
		{ID: "sd-monorepo-02", Stage: 10, Subproject: "cross-cutting", Category: "monorepo", Kind: "tooling", Priority: 7, Status: "ready", SideDAG: "sd-monorepo", Description: "Shared CI cache: sccache + cargo-chef + Docker layer cache; cold-build <10min."},
		{ID: "sd-monorepo-03", Stage: 10, Subproject: "cross-cutting", Category: "monorepo", Kind: "tooling", Priority: 7, Status: "ready", SideDAG: "sd-monorepo", Description: "Cross-crate impact analysis: which consumers break on change; auto-label in PR."},
		{ID: "sd-monorepo-04", Stage: 10, Subproject: "cross-cutting", Category: "monorepo", Kind: "tooling", Priority: 7, Status: "ready", SideDAG: "sd-monorepo", Description: "Bump-depends cascade: auto-PR all dependents on shared-lib bump; tests on each."},
		{ID: "sd-monorepo-05", Stage: 10, Subproject: "cross-cutting", Category: "monorepo", Kind: "tooling", Priority: 7, Status: "ready", SideDAG: "sd-monorepo", Description: "Release train: weekly snapshot tag; cherry-pick to release branch; auto-publish."},
	},
	"sd-agent-mesh": {
		{ID: "sd-agent-mesh-01", Stage: 10, Subproject: "cross-cutting", Category: "mesh", Kind: "governance", Priority: 8, Status: "ready", SideDAG: "sd-agent-mesh", Description: "Agent discovery: mDNS + DNS-SD + DHT for cross-host; spec version negotiation."},
		{ID: "sd-agent-mesh-02", Stage: 10, Subproject: "cross-cutting", Category: "mesh", Kind: "governance", Priority: 8, Status: "ready", SideDAG: "sd-agent-mesh", Description: "Trust: signed capability tokens; per-pair rate limit; reputation score from completed tasks."},
		{ID: "sd-agent-mesh-03", Stage: 10, Subproject: "cross-cutting", Category: "mesh", Kind: "governance", Priority: 8, Status: "ready", SideDAG: "sd-agent-mesh", Description: "Mesh routing: shortest-path on capability graph; load-balance via in-flight task count."},
		{ID: "sd-agent-mesh-04", Stage: 10, Subproject: "cross-cutting", Category: "mesh", Kind: "governance", Priority: 8, Status: "ready", SideDAG: "sd-agent-mesh", Description: "Work-stealing: idle agents claim from busy neighbors; per-task provenance in claim log."},
		{ID: "sd-agent-mesh-05", Stage: 10, Subproject: "cross-cutting", Category: "mesh", Kind: "governance", Priority: 8, Status: "ready", SideDAG: "sd-agent-mesh", Description: "Mesh health: pings, gossip-based membership, dead-node eviction within 30s."},
	},
}

// cmdExtend3V3 — equivalent of dagctl's extend3-v3: 20 L9 + 20 L10 + 20 side (4 new DAGs).
// New preset name: v3-extend3-415.
func cmdExtend3V3(args []string) error {
	fs := flag.NewFlagSet("extend3-v3", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)

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
	defer tx.Rollback()
	stmt, _ := tx.Prepare(`INSERT OR IGNORE INTO tasks
		(id, stage, slot, description, repo, subproject, category, lane, branch, kind, priority, semantic_hash, side_dag, status, assigned_agent, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	defer stmt.Close()
	edge, _ := tx.Prepare(`INSERT OR IGNORE INTO edges(from_task, to_task) VALUES (?, ?)`)
	defer edge.Close()
	sideMeta, _ := tx.Prepare(`INSERT OR IGNORE INTO side_dags(id, name, description) VALUES (?, ?, ?)`)
	defer sideMeta.Close()

	_, _ = sideMeta.Exec("sd-evals", "sd-evals", "Eval harness, regression, determinism, cost-corrected leaderboard.")
	_, _ = sideMeta.Exec("sd-observability", "sd-observability", "SLI/SLO, burn-rate alerts, on-call, error-budget governance.")
	_, _ = sideMeta.Exec("sd-monorepo", "sd-monorepo", "Monorepo decisions, shared CI, cross-crate impact, release train.")
	_, _ = sideMeta.Exec("sd-agent-mesh", "sd-agent-mesh", "Agent discovery, trust, mesh routing, work-stealing, mesh health.")

	now := nowUTC()
	insertV3PortTask := func(t v3PortTask) error {
		_, err := stmt.Exec(
			t.ID, t.Stage, t.Slot, t.Description, t.Subproject, t.Subproject, t.Category, t.Subproject, t.Branch,
			t.Kind, t.Priority, v3PortHash(t.Description), t.SideDAG, t.Status, "", now, now,
		)
		return err
	}

	for _, t := range v3PortL9 {
		if err := insertV3PortTask(t); err != nil {
			return err
		}
	}
	for _, t := range v3PortL10 {
		if err := insertV3PortTask(t); err != nil {
			return err
		}
	}
	for _, sideTasks := range v3PortSideExtend3 {
		for _, t := range sideTasks {
			t2 := t
			t2.Subproject = "cross-cutting"
			if err := insertV3PortTask(t2); err != nil {
				return err
			}
		}
	}

	// Wire L8 -> L9 -> L10 (1:1).
	for i := 1; i <= 20; i++ {
		from8 := fmt.Sprintf("task-08-%02d", i)
		to9 := fmt.Sprintf("task-09-%02d", i)
		_, _ = edge.Exec(from8, to9)
		to10 := fmt.Sprintf("task-10-%02d", i)
		_, _ = edge.Exec(to9, to10)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, kv := range [][2]string{
		{"preset", "v3-extend3-415"},
		{"preset_description", "v3-extend3-415: full L1-L10 + 7+3+4 side-DAGs = ~415 tasks"},
		{"stages", "10"},
		{"last_updated", now},
	} {
		_, _ = db.Exec(`INSERT INTO dag_meta(key, value) VALUES(?, ?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, kv[0], kv[1])
	}
	totalSide := 0
	for _, sideTasks := range v3PortSideExtend3 {
		totalSide += len(sideTasks)
	}
	fmt.Printf("extended v3: %d L9 + %d L10 + %d side-extend3 tasks\n",
		len(v3PortL9), len(v3PortL10), totalSide)
	return nil
}
