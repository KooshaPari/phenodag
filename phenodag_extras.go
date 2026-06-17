// nolint:dupl,funlen,gocyclop,gocognit
// phenodag_extras.go — dagctl meta/viz/test/extras/remote-claim ports.
//
// Ported from C:/Users/koosh/Dev/dagctl/*.go on 2026-06-16 for the
// superset merge. Each cmd* function is a near-verbatim port adapted to
// phenodag's openDB/migrate/gDBPath helpers and the phenodag
// `tasks`/`edges`/`agents`/`repos`/`side_dags` schema.
//
// New commands (alpha-sorted):
//   add, agent-stats, burndown, completion, critical-path, csv, dashboard,
//   dedup-explain, diff, dispatch, doctor, gantt, html, mermaid, next,
//   promote, remote-claim, remote-claims, remote-heartbeat, remote-reap,
//   remote-release, remote-transfer, sweep, thrash, topo, where, worktree-claim.
//
// NOTE: This file is intentionally large (1480 lines) as an interim artifact
// of the superset-merge integration (Phase 4). It will be refactored and split
// into focused modules post-merge when dagctl is retired.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"embed"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/KooshaPari/phenodag/internal/remoteclaim"
)

//go:embed dagctl_dag_template.html
var htmlTemplateFS embed.FS

const defaultRemoteClaimsDB = "FLEET_REMOTE_CLAIMS.db"

func envOrPort(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func remoteClaimsDBPort() string {
	return envOrPort("REMOTE_CLAIMS_DB", defaultRemoteClaimsDB)
}

func remoteAgentIDPort(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv("DAGCTL_AGENT"); v != "" {
		return v
	}
	return ""
}

func openRemoteStorePort(dbPath string) (*remoteclaim.SQLiteStore, error) {
	return remoteclaim.OpenSQLite(dbPath)
}

func openRemoteTransportPort(mode, dbPath, coordRepo string, issueNum int) (remoteclaim.Transport, *remoteclaim.SQLiteStore, error) {
	store, err := openRemoteStorePort(dbPath)
	if err != nil {
		return nil, nil, err
	}
	transport, err := remoteclaim.NewTransport(mode, store, coordRepo, issueNum)
	if err != nil {
		store.Close()
		return nil, nil, err
	}
	return transport, store, nil
}

func printClaimJSONPort(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// =====================================================================
// remote-claim family
// =====================================================================

func cmdRemoteClaimPort(args []string) error {
	fs := flag.NewFlagSet("remote-claim", flag.ExitOnError)
	dbPath := fs.String("db", remoteClaimsDBPort(), "remote claims SQLite path")
	agentID := fs.String("agent", "", "agent / chat session ID")
	transport := fs.String("transport", envOrPort("DAGCTL_CLAIM_TRANSPORT", "local"), "local or github")
	coordRepo := fs.String("coord-repo", "", "coordination repo owner/name (github transport)")
	issueNum := fs.Int("issue", 0, "tracking issue number (github transport)")
	ttl := fs.Int64("ttl", remoteclaim.DefaultTTLSeconds, "claim TTL seconds")
	branch := fs.String("branch", "", "branch name (optional)")
	worktree := fs.String("worktree", "", "worktree path (optional)")
	reason := fs.String("reason", "", "reason value (optional)")
	taskID := fs.String("task", "", "linked task id (optional)")
	fs.Parse(args)

	pos := fs.Args()
	if len(pos) < 1 {
		return fmt.Errorf("usage: phenodag remote-claim <owner/repo> -agent ID")
	}
	_, repoName, err := remoteclaim.ParseOwnerRepo(pos[0])
	if err != nil {
		return err
	}
	agent := remoteAgentIDPort(*agentID)
	if agent == "" {
		return fmt.Errorf("-agent or DAGCTL_AGENT required")
	}

	kind := remoteclaim.KindRepo
	if *branch != "" {
		kind = remoteclaim.KindBranch
	}
	if *worktree != "" {
		kind = remoteclaim.KindWorktree
	}

	tr, store, err := openRemoteTransportPort(*transport, *dbPath, *coordRepo, *issueNum)
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	claim := remoteclaim.Claim{
		AgentID:    agent,
		Kind:       kind,
		Repo:       repoName,
		Branch:     *branch,
		Worktree:   *worktree,
		TTLSeconds: *ttl,
		TaskID:     *taskID,
		Reason:     remoteclaim.Reason{Kind: remoteclaim.ReasonManual, Value: *reason},
	}
	claim.Resource = remoteclaim.ResourceKey(kind, repoName, *branch, *worktree)

	issued, err := tr.Claim(ctx, claim)
	if err != nil {
		return err
	}
	printClaimJSONPort(map[string]any{
		"claim_id":  issued.ID,
		"resource":  issued.Resource,
		"repo":      pos[0],
		"agent_id":  issued.AgentID,
		"epoch":     issued.Epoch,
		"status":    issued.Status,
		"transport": *transport,
	})
	return nil
}

func cmdRemoteHeartbeatPort(args []string) error {
	fs := flag.NewFlagSet("remote-heartbeat", flag.ExitOnError)
	dbPath := fs.String("db", remoteClaimsDBPort(), "remote claims SQLite path")
	claimID := fs.String("claim", "", "claim id")
	agentID := fs.String("agent", "", "agent id")
	epoch := fs.Int64("epoch", 0, "fencing epoch seen at last read")
	transport := fs.String("transport", envOrPort("DAGCTL_CLAIM_TRANSPORT", "local"), "local or github")
	coordRepo := fs.String("coord-repo", "", "coordination repo owner/name")
	issueNum := fs.Int("issue", 0, "tracking issue number")
	fs.Parse(args)
	if *claimID == "" {
		return fmt.Errorf("-claim required")
	}
	agent := remoteAgentIDPort(*agentID)
	if agent == "" {
		return fmt.Errorf("-agent or DAGCTL_AGENT required")
	}
	if *epoch == 0 {
		store, err := openRemoteStorePort(*dbPath)
		if err != nil {
			return err
		}
		c, err := store.Get(context.Background(), *claimID)
		store.Close()
		if err != nil || c == nil {
			return fmt.Errorf("claim not found")
		}
		*epoch = c.Epoch
	}
	tr, store, err := openRemoteTransportPort(*transport, *dbPath, *coordRepo, *issueNum)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := tr.Heartbeat(context.Background(), *claimID, agent, *epoch); err != nil {
		return err
	}
	fmt.Printf("heartbeat ok claim=%s epoch=%d\n", *claimID, *epoch)
	return nil
}

func cmdRemoteReleasePort(args []string) error {
	fs := flag.NewFlagSet("remote-release", flag.ExitOnError)
	dbPath := fs.String("db", remoteClaimsDBPort(), "remote claims SQLite path")
	claimID := fs.String("claim", "", "claim id")
	agentID := fs.String("agent", "", "agent id")
	epoch := fs.Int64("epoch", 0, "fencing epoch")
	transport := fs.String("transport", envOrPort("DAGCTL_CLAIM_TRANSPORT", "local"), "local or github")
	coordRepo := fs.String("coord-repo", "", "coordination repo owner/name")
	issueNum := fs.Int("issue", 0, "tracking issue number")
	fs.Parse(args)
	if *claimID == "" {
		return fmt.Errorf("-claim required")
	}
	agent := remoteAgentIDPort(*agentID)
	if agent == "" {
		return fmt.Errorf("-agent or DAGCTL_AGENT required")
	}
	if *epoch == 0 {
		store, err := openRemoteStorePort(*dbPath)
		if err == nil {
			if c, err := store.Get(context.Background(), *claimID); err == nil && c != nil {
				*epoch = c.Epoch
			}
			store.Close()
		}
	}
	tr, store, err := openRemoteTransportPort(*transport, *dbPath, *coordRepo, *issueNum)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := tr.Release(context.Background(), *claimID, agent, *epoch); err != nil {
		return err
	}
	fmt.Printf("released claim=%s\n", *claimID)
	return nil
}

func cmdRemoteClaimsPort(args []string) error {
	fs := flag.NewFlagSet("remote-claims", flag.ExitOnError)
	dbPath := fs.String("db", remoteClaimsDBPort(), "remote claims SQLite path")
	transport := fs.String("transport", envOrPort("DAGCTL_CLAIM_TRANSPORT", "local"), "local or github")
	coordRepo := fs.String("coord-repo", "", "coordination repo owner/name")
	issueNum := fs.Int("issue", 0, "tracking issue number")
	fs.Parse(args)
	tr, store, err := openRemoteTransportPort(*transport, *dbPath, *coordRepo, *issueNum)
	if err != nil {
		return err
	}
	defer store.Close()
	claims, err := tr.List(context.Background())
	if err != nil {
		return err
	}
	if claims == nil {
		claims = []remoteclaim.Claim{}
	}
	printClaimJSONPort(claims)
	return nil
}

func cmdRemoteReapPort(args []string) error {
	fs := flag.NewFlagSet("remote-reap", flag.ExitOnError)
	dbPath := fs.String("db", remoteClaimsDBPort(), "remote claims SQLite path")
	transport := fs.String("transport", envOrPort("DAGCTL_CLAIM_TRANSPORT", "local"), "local or github")
	coordRepo := fs.String("coord-repo", "", "coordination repo owner/name")
	issueNum := fs.Int("issue", 0, "tracking issue number")
	fs.Parse(args)
	tr, store, err := openRemoteTransportPort(*transport, *dbPath, *coordRepo, *issueNum)
	if err != nil {
		return err
	}
	defer store.Close()
	reaped, err := tr.ReapExpired(context.Background(), time.Now().UTC())
	if err != nil {
		return err
	}
	if reaped == nil {
		reaped = []remoteclaim.Claim{}
	}
	printClaimJSONPort(reaped)
	return nil
}

func cmdRemoteTransferPort(args []string) error {
	fs := flag.NewFlagSet("remote-transfer", flag.ExitOnError)
	dbPath := fs.String("db", remoteClaimsDBPort(), "remote claims SQLite path")
	fromID := fs.String("from", "", "source claim id")
	toID := fs.String("to", "", "destination claim id (generated if empty)")
	toAgent := fs.String("to-agent", "", "new agent id")
	transport := fs.String("transport", envOrPort("DAGCTL_CLAIM_TRANSPORT", "local"), "local or github")
	coordRepo := fs.String("coord-repo", "", "coordination repo owner/name")
	issueNum := fs.Int("issue", 0, "tracking issue number")
	fs.Parse(args)
	if *fromID == "" || *toAgent == "" {
		return fmt.Errorf("-from and -to-agent required")
	}
	tr, store, err := openRemoteTransportPort(*transport, *dbPath, *coordRepo, *issueNum)
	if err != nil {
		return err
	}
	defer store.Close()
	if *toID == "" {
		*toID = remoteclaim.NewClaimID()
	}
	if err := tr.Transfer(context.Background(), *fromID, *toID, *toAgent); err != nil {
		return err
	}
	newClaim, err := store.Get(context.Background(), *toID)
	if err != nil {
		return err
	}
	printClaimJSONPort(newClaim)
	return nil
}

// =====================================================================
// Meta commands
// =====================================================================

func detectRepoPort() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := cwd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return filepath.Base(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func runGitPort(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	_ = err
	return strings.TrimSpace(string(out))
}

func cmdWorktreeClaimPort(args []string) error {
	fs := flag.NewFlagSet("worktree-claim", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	repo := fs.String("repo", "", "repo name (defaults to cwd)")
	branch := fs.String("branch", "", "branch name (auto-generated if empty)")
	agentID := fs.String("agent", "", "agent ID")
	fs.Parse(args)
	if *agentID == "" {
		return fmt.Errorf("-agent required")
	}
	if *repo == "" {
		*repo = detectRepoPort()
	}
	if *repo == "" {
		return fmt.Errorf("-repo required (or run from inside a git repo)")
	}
	if *branch == "" {
		*branch = fmt.Sprintf("wt-%s-%d", *agentID, time.Now().Unix())
	}
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		if err := migrate(db); err != nil {
			return err
		}
		// worktrees table may not exist yet; create if missing (idempotent).
		_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS worktrees (
			id TEXT PRIMARY KEY, repo TEXT, branch TEXT, path TEXT,
			created_at TEXT, last_used TEXT, agent TEXT, claimed_by TEXT)`)
		_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS work_claims (
			worktree_id TEXT, agent TEXT, claimed_at TEXT,
			PRIMARY KEY(worktree_id))`)

		var repoPath string
		_ = db.QueryRow(`SELECT path FROM repos WHERE name=?`, *repo).Scan(&repoPath)
		if repoPath == "" {
			repoPath = filepath.Join(".", *repo)
		}
		wtPath := filepath.Join(filepath.Dir(repoPath), *repo+"-wtrees", *branch)
		_ = os.MkdirAll(wtPath, 0o755)
		// Try git worktree add; ignore errors if branch exists.
		_ = runGitPort(repoPath, "worktree", "add", "-b", *branch, wtPath, "HEAD")
		now := nowUTC()
		_, _ = db.Exec(`INSERT OR REPLACE INTO worktrees(id, repo, branch, path, created_at, last_used, agent)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, *branch, *repo, *branch, wtPath, now, now, *agentID)
		_, _ = db.Exec(`INSERT OR REPLACE INTO work_claims(worktree_id, agent, claimed_at) VALUES (?, ?, ?)`,
			*branch, *agentID, now)
		fmt.Printf("Worktree claimed: %s @ %s\n", *branch, wtPath)
		return nil
	})
}

func cmdAgentStatsPort(args []string) error {
	fs := flag.NewFlagSet("agent-stats", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		// Some agent columns may not exist on older phenodag DBs; use safe COALESCE.
		rows, err := db.Query(`SELECT id, COALESCE(status,''), COALESCE(last_seen,'') FROM agents ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		fmt.Printf("%-24s %-10s %-22s %-8s %-8s\n", "AGENT", "STATUS", "LAST_SEEN", "DONE", "FAILED")
		for rows.Next() {
			var id, status, lastSeen string
			_ = rows.Scan(&id, &status, &lastSeen)
			var done, failed int
			_ = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE assigned_agent=? AND status='done'`, id).Scan(&done)
			_ = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE assigned_agent=? AND status='failed'`, id).Scan(&failed)
			fmt.Printf("%-24s %-10s %-22s %-8d %-8d\n", id, status, lastSeen, done, failed)
		}
		return nil
	})
}

func cmdDiffPort(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "current DB")
	other := fs.String("other", "", "other DB to compare against")
	fs.Parse(args)
	if *other == "" {
		return fmt.Errorf("-other required")
	}
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		otherDB, err := openDB(*other)
		if err != nil {
			return err
		}
		defer otherDB.Close()
		cur := map[string]string{}
		rows, _ := db.Query("SELECT id, status FROM tasks")
		for rows.Next() {
			var id, status string
			_ = rows.Scan(&id, &status)
			cur[id] = status
		}
		rows.Close()
		otherMap := map[string]string{}
		rows, _ = otherDB.Query("SELECT id, status FROM tasks")
		for rows.Next() {
			var id, status string
			_ = rows.Scan(&id, &status)
			otherMap[id] = status
		}
		rows.Close()
		var added, removed, changed int
		for id, status := range cur {
			if o, ok := otherMap[id]; !ok {
				added++
				fmt.Printf("+ %s (status=%s)\n", id, status)
			} else if o != status {
				changed++
				fmt.Printf("~ %s: %s -> %s\n", id, o, status)
			}
		}
		for id := range otherMap {
			if _, ok := cur[id]; !ok {
				removed++
				fmt.Printf("- %s\n", id)
			}
		}
		fmt.Printf("\nDiff: +%d added, -%d removed, ~%d changed\n", added, removed, changed)
		return nil
	})
}

func cmdCriticalPathPort(args []string) error {
	fs := flag.NewFlagSet("critical-path", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	_ = fs.String("from", "", "start task (default: first ready)")
	fs.Parse(args)
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		adj := map[string][]string{}
		indeg := map[string]int{}
		rows, _ := db.Query("SELECT id FROM tasks")
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			indeg[id] = 0
		}
		rows.Close()
		rows, _ = db.Query("SELECT from_task, to_task FROM edges")
		for rows.Next() {
			var f, t string
			_ = rows.Scan(&f, &t)
			adj[f] = append(adj[f], t)
			indeg[t]++
		}
		rows.Close()
		dist := map[string]int{}
		prev := map[string]string{}
		queue := []string{}
		for id, d := range indeg {
			if d == 0 {
				dist[id] = 0
				queue = append(queue, id)
			}
		}
		head := 0
		for head < len(queue) {
			u := queue[head]
			head++
			for _, v := range adj[u] {
				if dist[v] < dist[u]+1 {
					dist[v] = dist[u] + 1
					prev[v] = u
				}
				indeg[v]--
				if indeg[v] == 0 {
					queue = append(queue, v)
				}
			}
		}
		var end string
		maxD := -1
		for id, d := range dist {
			if d > maxD {
				maxD = d
				end = id
			}
		}
		if end == "" {
			fmt.Println("No path found")
			return nil
		}
		path := []string{end}
		for {
			p, ok := prev[path[len(path)-1]]
			if !ok {
				break
			}
			path = append(path, p)
		}
		for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
			path[i], path[j] = path[j], path[i]
		}
		fmt.Printf("Critical path (length %d):\n", len(path))
		for _, id := range path {
			fmt.Printf("  %s\n", id)
		}
		return nil
	})
}

// =====================================================================
// Test/Ops commands
// =====================================================================

func cmdDoctorPort(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fix := fs.Bool("fix", false, "attempt to repair minor issues")
	fs.Parse(args)
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		checks := []struct {
			name string
			fn   func(*sql.DB) (bool, string)
		}{
			{"schema-ok", checkSchemaPort},
			{"no-dup-ids", checkNoDupIDsPort},
			{"no-dangling-edges", checkNoDanglingPort},
			{"has-tasks", checkHasTasksPort},
		}
		allOK := true
		for _, c := range checks {
			ok, msg := c.fn(db)
			marker := "[ OK ]"
			if !ok {
				marker = "[FAIL]"
				allOK = false
			}
			fmt.Printf("%s %s: %s\n", marker, c.name, msg)
		}
		if allOK {
			fmt.Println("\nDoctor: ALL CHECKS PASSED")
		} else {
			fmt.Println("\nDoctor: ISSUES FOUND")
			if *fix {
				fmt.Println("Attempting repair...")
				_, _ = db.Exec(`DELETE FROM edges WHERE from_task NOT IN (SELECT id FROM tasks) OR to_task NOT IN (SELECT id FROM tasks)`)
				fmt.Println("Dangling edges removed")
			}
		}
		return nil
	})
}

func checkSchemaPort(db *sql.DB) (bool, string) {
	var n int
	_ = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('tasks','edges','agents','claims','repos','side_dags','duplicate_groups','dag_meta')").Scan(&n)
	if n >= 8 {
		return true, "schema present"
	}
	return false, fmt.Sprintf("only %d/8 tables present", n)
}

func checkNoDupIDsPort(db *sql.DB) (bool, string) {
	var n int
	_ = db.QueryRow("SELECT COUNT(*) FROM (SELECT id FROM tasks GROUP BY id HAVING COUNT(*) > 1)").Scan(&n)
	if n == 0 {
		return true, "no duplicate IDs"
	}
	return false, fmt.Sprintf("%d duplicate IDs", n)
}

func checkNoDanglingPort(db *sql.DB) (bool, string) {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM edges e
		LEFT JOIN tasks t1 ON e.from_task = t1.id
		LEFT JOIN tasks t2 ON e.to_task = t2.id
		WHERE t1.id IS NULL OR t2.id IS NULL`).Scan(&n)
	if n == 0 {
		return true, "no dangling edges"
	}
	return false, fmt.Sprintf("%d dangling edges", n)
}

func checkHasTasksPort(db *sql.DB) (bool, string) {
	var n int
	_ = db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&n)
	if n > 0 {
		return true, fmt.Sprintf("%d tasks", n)
	}
	return false, "no tasks seeded"
}

func cmdThrashPort(args []string) error {
	fs := flag.NewFlagSet("thrash", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	agents := fs.Int("agents", 5, "number of concurrent agents")
	duration := fs.Duration("duration", 5*time.Second, "duration to thrash")
	fs.Parse(args)
	end := time.Now().Add(*duration)
	ops := 0
	for time.Now().Before(end) {
		agentID := fmt.Sprintf("thrash-%d", rand.Intn(*agents))
		_ = withLock(gDBPath, func() error {
			db, err := openDB(gDBPath)
			if err != nil {
				return err
			}
			defer db.Close()
			_, _ = db.Exec(`INSERT INTO agents(id, status, last_seen) VALUES (?, 'active', ?)
				ON CONFLICT(id) DO UPDATE SET status='active', last_seen=excluded.last_seen`,
				agentID, nowUTC())
			var taskID string
			err = db.QueryRow(`SELECT id FROM tasks WHERE status='ready' ORDER BY RANDOM() LIMIT 1`).Scan(&taskID)
			if err == nil {
				_, _ = db.Exec("UPDATE tasks SET status='in_progress', assigned_agent=?, updated_at=? WHERE id=?",
					agentID, nowUTC(), taskID)
				ops++
			}
			return nil
		})
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Printf("Thrash complete: %d ops in %s\n", ops, *duration)
	return nil
}

func cmdSweepPort(args []string) error {
	fs := flag.NewFlagSet("sweep", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	dryRun := fs.Bool("dry-run", false, "show what would be removed without acting")
	fs.Parse(args)
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		removed := 0
		// Requeue failed tasks older than 24h (phenodag stores status in tasks; no failures table).
		threshold := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
		rows, _ := db.Query("SELECT id FROM tasks WHERE status='failed' AND updated_at < ?", threshold)
		var requeue []string
		for rows.Next() {
			var t string
			_ = rows.Scan(&t)
			requeue = append(requeue, t)
		}
		rows.Close()
		if !*dryRun {
			for _, t := range requeue {
				_, _ = db.Exec("UPDATE tasks SET status='ready', assigned_agent='', updated_at=? WHERE id=?",
					nowUTC(), t)
			}
		}
		fmt.Printf("Sweep: %d failed tasks requeued\n", removed+len(requeue))
		return nil
	})
}

func cmdDispatchPort(args []string) error {
	fs := flag.NewFlagSet("dispatch", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	agentID := fs.String("agent", "", "agent ID")
	repo := fs.String("repo", "", "prefer tasks for this repo")
	fs.Parse(args)
	if *agentID == "" {
		return fmt.Errorf("-agent required")
	}
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		_, _ = db.Exec(`INSERT INTO agents(id, status, last_seen) VALUES (?, 'active', ?)
			ON CONFLICT(id) DO UPDATE SET status='active', last_seen=excluded.last_seen`,
			*agentID, nowUTC())
		var id, desc, sub, kind string
		var stage, priority int
		q := `SELECT id, description, COALESCE(subproject,''), stage, COALESCE(kind,''), COALESCE(priority, 0)
			FROM tasks WHERE status='ready' AND (side_dag='' OR side_dag IS NULL)`
		if *repo != "" {
			q += ` AND repo=?`
		}
		q += ` ORDER BY priority DESC, stage ASC, id ASC LIMIT 1`
		var qerr error
		if *repo != "" {
			qerr = db.QueryRow(q, *repo).Scan(&id, &desc, &sub, &stage, &kind, &priority)
		} else {
			qerr = db.QueryRow(q).Scan(&id, &desc, &sub, &stage, &kind, &priority)
		}
		if qerr != nil {
			qerr = db.QueryRow(`SELECT id, description, COALESCE(subproject,''), stage, COALESCE(kind,''), COALESCE(priority, 0)
				FROM tasks WHERE status='ready' AND side_dag IS NOT NULL AND side_dag != ''
				ORDER BY priority DESC, id ASC LIMIT 1`).Scan(&id, &desc, &sub, &stage, &kind, &priority)
		}
		if qerr != nil {
			out := map[string]interface{}{"agent": *agentID, "status": "NO_TASK"}
			b, _ := json.Marshal(out)
			fmt.Println(string(b))
			return nil
		}
		_, _ = db.Exec("UPDATE tasks SET status='in_progress', assigned_agent=?, updated_at=? WHERE id=?",
			*agentID, nowUTC(), id)
		out := map[string]interface{}{
			"agent":       *agentID,
			"task_id":     id,
			"description": desc,
			"subproject":  sub,
			"stage":       stage,
			"kind":        kind,
			"priority":    priority,
			"status":      "DISPATCHED",
		}
		b, _ := json.Marshal(out)
		fmt.Println(string(b))
		return nil
	})
}

// =====================================================================
// Viz commands
// =====================================================================

type ganttTaskPort struct {
	id, status string
	stage      int
}

func cmdGanttPort(args []string) error {
	fs := flag.NewFlagSet("gantt", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	ascii := fs.Bool("ascii", false, "emit ASCII gantt instead of Mermaid")
	fs.Parse(args)
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		rows, _ := db.Query(`SELECT id, stage, status FROM tasks WHERE (side_dag='' OR side_dag IS NULL) ORDER BY stage, id`)
		var tasks []ganttTaskPort
		for rows.Next() {
			var t ganttTaskPort
			_ = rows.Scan(&t.id, &t.stage, &t.status)
			tasks = append(tasks, t)
		}
		rows.Close()
		if *ascii {
			printASCIIGanttPort(tasks)
		} else {
			printMermaidGanttPort(tasks)
		}
		return nil
	})
}

func printASCIIGanttPort(tasks []ganttTaskPort) {
	byStage := map[int][]string{}
	for _, t := range tasks {
		byStage[t.stage] = append(byStage[t.stage], t.id)
	}
	stages := []int{}
	for s := range byStage {
		stages = append(stages, s)
	}
	sort.Ints(stages)
	fmt.Println("Gantt (one row per stage):")
	for _, s := range stages {
		progress := 0
		for _, t := range tasks {
			if t.stage == s && t.status == "done" {
				progress++
			}
		}
		pct := 0
		if len(byStage[s]) > 0 {
			pct = (progress * 100) / len(byStage[s])
		}
		bar := strings.Repeat("=", pct/5) + strings.Repeat(" ", 20-pct/5)
		fmt.Printf("L%d |%s| %d%% (%d/%d)\n", s, bar, pct, progress, len(byStage[s]))
	}
}

func printMermaidGanttPort(tasks []ganttTaskPort) {
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	fmt.Fprintln(w, "```mermaid")
	fmt.Fprintln(w, "gantt")
	fmt.Fprintln(w, "    title phenodag Gantt")
	fmt.Fprintln(w, "    dateFormat YYYY-MM-DD")
	_, _ = io.WriteString(w, "    axisFormat %m-%d\n")
	byStage := map[int][]string{}
	for _, t := range tasks {
		byStage[t.stage] = append(byStage[t.stage], t.id)
	}
	stages := []int{}
	for s := range byStage {
		stages = append(stages, s)
	}
	sort.Ints(stages)
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for i, s := range stages {
		fmt.Fprintf(w, "    section L%d\n", s)
		for j, id := range byStage[s] {
			start := base.AddDate(0, 0, i*5+j)
			fmt.Fprintf(w, "        %-22s :a%d, %s, 1d\n", id, i*100+j, start.Format("2006-01-02"))
		}
	}
	fmt.Fprintln(w, "```")
}

func cmdMermaidPort(args []string) error {
	fs := flag.NewFlagSet("mermaid", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		fmt.Println("```mermaid")
		fmt.Println("flowchart LR")
		rows, _ := db.Query("SELECT id, COALESCE(subproject,'') FROM tasks WHERE side_dag='' OR side_dag IS NULL ORDER BY id")
		for rows.Next() {
			var id, sub string
			_ = rows.Scan(&id, &sub)
			fmt.Printf("    %s[\"%s<br/>%s\"]\n", sanitizeIDPort(id), id, sub)
		}
		rows.Close()
		rows, _ = db.Query("SELECT from_task, to_task FROM edges")
		for rows.Next() {
			var f, t string
			_ = rows.Scan(&f, &t)
			fmt.Printf("    %s --> %s\n", sanitizeIDPort(f), sanitizeIDPort(t))
		}
		rows.Close()
		fmt.Println("```")
		return nil
	})
}

func sanitizeIDPort(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

func cmdBurndownPort(args []string) error {
	fs := flag.NewFlagSet("burndown", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		var total, done int
		_ = db.QueryRow("SELECT COUNT(*) FROM tasks WHERE side_dag='' OR side_dag IS NULL").Scan(&total)
		_ = db.QueryRow("SELECT COUNT(*) FROM tasks WHERE status='done' AND (side_dag='' OR side_dag IS NULL)").Scan(&done)
		pending := total - done
		fmt.Printf("Burndown (total=%d, done=%d, pending=%d):\n", total, done, pending)
		const width = 50
		bars := width
		if total > 0 {
			bars = (pending * width) / total
		}
		fmt.Println("[" + strings.Repeat("#", bars) + strings.Repeat(" ", width-bars) + "]")
		pct := 0
		if total > 0 {
			pct = (done * 100) / total
		}
		fmt.Printf("Progress: %d%%\n", pct)
		return nil
	})
}

// =====================================================================
// Extras
// =====================================================================

func cmdWherePort(args []string) error {
	fs := flag.NewFlagSet("where", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	repo := fs.String("repo", "", "override repo detection")
	fs.Parse(args)
	detected := *repo
	if detected == "" {
		detected = detectRepoPort()
	}
	if detected == "" {
		return fmt.Errorf("not in a git repo (use -repo)")
	}
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		fmt.Printf("Repo: %s\n", detected)
		rows, _ := db.Query(`SELECT id, stage, status, description FROM tasks
			WHERE repo = ? AND status IN ('ready','in_progress')
			ORDER BY stage, id`, detected)
		count := 0
		for rows.Next() {
			var id, status, desc string
			var stage int
			_ = rows.Scan(&id, &stage, &status, &desc)
			marker := "O"
			if status == "in_progress" {
				marker = ">"
			}
			fmt.Printf("  [%s] %-12s L%d %s\n", marker, id, stage, desc)
			count++
		}
		rows.Close()
		if count == 0 {
			fmt.Println("  (no available tasks for this repo)")
		}
		return nil
	})
}

func cmdTopoPort(args []string) error {
	fs := flag.NewFlagSet("topo", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	format := fs.String("format", "ascii", "ascii|dot|json")
	fs.Parse(args)
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		tasks := map[string]map[string]string{}
		rows, _ := db.Query("SELECT id, stage, COALESCE(status,''), COALESCE(subproject,'') FROM tasks")
		for rows.Next() {
			var id, st, sub, status string
			_ = rows.Scan(&id, &st, &status, &sub)
			tasks[id] = map[string]string{"stage": st, "status": status, "sub": sub}
		}
		rows.Close()
		edges := [][]string{}
		rows, _ = db.Query("SELECT from_task, to_task FROM edges")
		for rows.Next() {
			var f, t string
			_ = rows.Scan(&f, &t)
			edges = append(edges, []string{f, t})
		}
		rows.Close()
		switch *format {
		case "dot":
			fmt.Println("digraph phenodag {")
			fmt.Println("  rankdir=LR;")
			keys := make([]string, 0, len(tasks))
			for k := range tasks {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Printf("  \"%s\" [label=\"%s\\nL%s\\n%s\"];\n", k, k, tasks[k]["stage"], tasks[k]["status"])
			}
			for _, e := range edges {
				fmt.Printf("  \"%s\" -> \"%s\";\n", e[0], e[1])
			}
			fmt.Println("}")
		case "json":
			out := map[string]interface{}{"tasks": tasks, "edges": edges}
			b, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(b))
		default:
			byStage := map[string][]string{}
			for id, t := range tasks {
				stage := t["stage"]
				byStage[stage] = append(byStage[stage], id)
			}
			stages := []string{}
			for s := range byStage {
				stages = append(stages, s)
			}
			sort.Strings(stages)
			for _, s := range stages {
				sort.Strings(byStage[s])
				fmt.Printf("L%s: %s\n", s, strings.Join(byStage[s], " "))
			}
			fmt.Printf("\n%d edges\n", len(edges))
		}
		return nil
	})
}

func cmdDashboardPort(args []string) error {
	fs := flag.NewFlagSet("dashboard", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	interval := fs.Duration("interval", 2*time.Second, "refresh interval")
	fs.Parse(args)
	tick := time.NewTicker(*interval)
	defer tick.Stop()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	for {
		select {
		case <-sigCh:
			fmt.Println("\nDashboard stopped")
			return nil
		case <-tick.C:
			renderDashboardPort(gDBPath)
		}
	}
}

func renderDashboardPort(dbPath string) {
	fmt.Print("\033[2J\033[H")
	db, err := openDB(dbPath)
	if err != nil {
		fmt.Printf("Dashboard error: %v\n", err)
		return
	}
	defer db.Close()
	var total, done, inprog, failed, ready, blocked int
	_ = db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&total)
	_ = db.QueryRow("SELECT COUNT(*) FROM tasks WHERE status='done'").Scan(&done)
	_ = db.QueryRow("SELECT COUNT(*) FROM tasks WHERE status='in_progress'").Scan(&inprog)
	_ = db.QueryRow("SELECT COUNT(*) FROM tasks WHERE status='failed'").Scan(&failed)
	_ = db.QueryRow("SELECT COUNT(*) FROM tasks WHERE status='ready'").Scan(&ready)
	_ = db.QueryRow("SELECT COUNT(*) FROM tasks WHERE status='blocked'").Scan(&blocked)
	now := time.Now().Format("15:04:05")
	fmt.Printf("phenodag Dashboard [%s]\n", now)
	fmt.Println(strings.Repeat("-", 60))
	pct := 0
	if total > 0 {
		pct = (done * 100) / total
	}
	bar := "[" + strings.Repeat("=", pct/2) + strings.Repeat(" ", 50-pct/2) + "]"
	fmt.Printf("Progress: %d%%  %s\n", pct, bar)
	fmt.Printf("Total: %d  Done: %d  Ready: %d  InProgress: %d  Blocked: %d  Failed: %d\n\n",
		total, done, ready, inprog, blocked, failed)
	rows, _ := db.Query(`SELECT stage, COUNT(*), SUM(CASE WHEN status='done' THEN 1 ELSE 0 END) FROM tasks WHERE side_dag='' OR side_dag IS NULL GROUP BY stage ORDER BY stage`)
	for rows.Next() {
		var s, t, d int
		_ = rows.Scan(&s, &t, &d)
		p := 0
		if t > 0 {
			p = (d * 100) / t
		}
		b := "[" + strings.Repeat("=", p/5) + strings.Repeat(" ", 20-p/5) + "]"
		fmt.Printf("  L%d %s %3d%% (%d/%d)\n", s, b, p, d, t)
	}
	rows.Close()
}

func cmdCSVPort(args []string) error {
	fs := flag.NewFlagSet("csv", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	out := fs.String("out", "phenodag.csv", "output CSV file")
	fs.Parse(args)
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w := csv.NewWriter(f)
		defer w.Flush()
		_ = w.Write([]string{"id", "stage", "slot", "status", "subproject", "category", "kind", "priority", "description"})
		rows, _ := db.Query(`SELECT id, stage, slot, status, COALESCE(subproject,''), COALESCE(category,''), COALESCE(kind,''), COALESCE(priority,0), description FROM tasks ORDER BY stage, id`)
		for rows.Next() {
			var id, status, sub, cat, kind, desc string
			var stage, slot, prio int
			_ = rows.Scan(&id, &stage, &slot, &status, &sub, &cat, &kind, &prio, &desc)
			_ = w.Write([]string{id, fmt.Sprintf("%d", stage), fmt.Sprintf("%d", slot), status, sub, cat, kind, fmt.Sprintf("%d", prio), desc})
		}
		rows.Close()
		fmt.Printf("Exported CSV: %s\n", *out)
		return nil
	})
}

func cmdHTMLPort(args []string) error {
	fs := flag.NewFlagSet("html", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	out := fs.String("out", "phenodag.html", "output HTML file")
	fs.Parse(args)
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		tmplBytes, err := htmlTemplateFS.ReadFile("dagctl_dag_template.html")
		if err != nil {
			return err
		}
		tmpl := string(tmplBytes)
		type node struct{ ID, Status, Sub, Stage string }
		var nodes []node
		rows, _ := db.Query("SELECT id, COALESCE(status,''), COALESCE(subproject,''), stage FROM tasks")
		for rows.Next() {
			var n node
			var s int
			_ = rows.Scan(&n.ID, &n.Status, &n.Sub, &s)
			n.Stage = fmt.Sprintf("%d", s)
			nodes = append(nodes, n)
		}
		rows.Close()
		type edge struct{ From, To string }
		var edges []edge
		rows, _ = db.Query("SELECT from_task, to_task FROM edges")
		for rows.Next() {
			var e edge
			_ = rows.Scan(&e.From, &e.To)
			edges = append(edges, e)
		}
		rows.Close()
		nodesJSON, _ := json.Marshal(nodes)
		edgesJSON, _ := json.Marshal(edges)
		tmpl = strings.Replace(tmpl, "{{NODES}}", string(nodesJSON), 1)
		tmpl = strings.Replace(tmpl, "{{EDGES}}", string(edgesJSON), 1)
		if err := os.WriteFile(*out, []byte(tmpl), 0644); err != nil {
			return err
		}
		fmt.Printf("Exported HTML: %s\n", *out)
		return nil
	})
}

func cmdPromotePort(args []string) error {
	fs := flag.NewFlagSet("promote", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	taskID := fs.String("task", "", "task ID to promote")
	stage := fs.Int("stage", 0, "target stage")
	fs.Parse(args)
	if *taskID == "" {
		return fmt.Errorf("-task required")
	}
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		if *stage > 0 {
			_, _ = db.Exec("UPDATE tasks SET side_dag='', stage=?, updated_at=? WHERE id=?", *stage, nowUTC(), *taskID)
		} else {
			_, _ = db.Exec("UPDATE tasks SET side_dag='', updated_at=? WHERE id=?", nowUTC(), *taskID)
		}
		fmt.Printf("Promoted %s to main DAG\n", *taskID)
		return nil
	})
}

func cmdCompletionPort(args []string) error {
	fs := flag.NewFlagSet("completion", flag.ExitOnError)
	_ = fs.String("shell", "bash", "bash|zsh|fish")
	fs.Parse(args)
	commands := []string{
		"init", "seed", "status", "validate", "pick", "claim", "release",
		"heartbeat", "reclaim", "done", "fail", "fill", "scan", "dupes", "export",
		"seed3", "extend3-v2", "extend3-v3", "dedup-explain",
		"remote-claim", "remote-heartbeat", "remote-release", "remote-claims", "remote-reap", "remote-transfer",
		"worktree-claim", "agent-stats", "diff", "critical-path",
		"doctor", "thrash", "sweep", "dispatch",
		"gantt", "mermaid", "burndown",
		"where", "topo", "dashboard", "csv", "html", "promote", "completion",
		"add", "merge", "next",
	}
	switch *fs.String("shell", "bash", "") {
	case "bash":
		fmt.Println("# bash completion for phenodag")
		fmt.Println("_phenodag() {")
		fmt.Println("  local cur cmds")
		fmt.Println("  cur=\"${COMP_WORDS[COMP_CWORD]}\"")
		fmt.Println("  cmds=\"" + strings.Join(commands, " ") + "\"")
		fmt.Println("  if [ \"${COMP_CWORD}\" = \"1\" ]; then")
		fmt.Println("    COMPREPLY=($(compgen -W \"${cmds}\" -- \"${cur}\"))")
		fmt.Println("  fi")
		fmt.Println("}")
		fmt.Println("complete -F _phenodag phenodag")
	case "zsh":
		fmt.Println("# zsh completion for phenodag")
		fmt.Println("#compdef phenodag")
		fmt.Println("_phenodag() {")
		fmt.Println("  local -a commands")
		fmt.Println("  commands=(")
		for _, c := range commands {
			fmt.Printf("    '%s:%s command'\n", c, c)
		}
		fmt.Println("  )")
		fmt.Println("  _describe 'command' commands")
		fmt.Println("}")
		fmt.Println("compdef _phenodag phenodag")
	case "fish":
		for _, c := range commands {
			fmt.Printf("complete -c phenodag -n \"__fish_use_subcommand\" -a %s\n", c)
		}
	default:
		return fmt.Errorf("unknown shell: %s", *fs.String("shell", "bash", ""))
	}
	return nil
}

// =====================================================================
// add / merge / next (dagctl v1 commands not in phenodag)
// =====================================================================

func cmdAddPort(args []string) error {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	desc := fs.String("desc", "", "task description")
	sub := fs.String("subproject", "root", "subproject")
	stage := fs.Int("stage", 0, "stage")
	kind := fs.String("kind", "hygiene", "kind")
	priority := fs.Int("priority", 5, "priority")
	fs.Parse(args)
	if *desc == "" {
		return fmt.Errorf("-desc required")
	}
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		if err := migrate(db); err != nil {
			return err
		}
		var count int
		_ = db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&count)
		id := fmt.Sprintf("add-%03d", count+1)
		// Use port's semanticHash (simhash-based; back-compat with dagctl).
		hash := semanticHashPort(*desc)
		bestID, bestSim := "", 0.0
		rows, _ := db.Query("SELECT id, description, COALESCE(subproject,'') FROM tasks WHERE status NOT IN ('done','failed')")
		for rows.Next() {
			var existingID, existingDesc, existingSub string
			_ = rows.Scan(&existingID, &existingDesc, &existingSub)
			sim := hybridSimilarityPort(*desc, *sub, existingDesc, existingSub)
			if sim > bestSim {
				bestSim = sim
				bestID = existingID
			}
		}
		rows.Close()
		if bestSim >= 0.85 {
			fmt.Fprintf(os.Stderr, "DUPLICATE_DETECTED: %.0f%% similar to %s\n", bestSim*100, bestID)
		}
		_, err = db.Exec(`INSERT INTO tasks
			(id, stage, slot, description, repo, subproject, category, lane, branch, kind, priority, semantic_hash, side_dag, status, assigned_agent, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			id, *stage, 0, *desc, "", *sub, "", *sub, "main", *kind, *priority, hash, "", "pending", "", nowUTC(), nowUTC())
		if err != nil {
			return err
		}
		fmt.Printf("Added %s\n", id)
		return nil
	})
}

func cmdMergePort(args []string) error {
	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	taskID := fs.String("task", "", "task to merge")
	intoID := fs.String("into", "", "target task")
	fs.Parse(args)
	if *taskID == "" || *intoID == "" {
		return fmt.Errorf("-task and -into required")
	}
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		_, _ = db.Exec("DELETE FROM tasks WHERE id=?", *taskID)
		_, _ = db.Exec("DELETE FROM edges WHERE from_task=? OR to_task=?", *taskID, *taskID)
		_, _ = db.Exec("UPDATE edges SET to_task=? WHERE to_task=? AND from_task!=?", *intoID, *taskID, *intoID)
		_, _ = db.Exec("UPDATE edges SET from_task=? WHERE from_task=? AND to_task!=?", *intoID, *taskID, *intoID)
		fmt.Printf("Merged %s into %s\n", *taskID, *intoID)
		return nil
	})
}

func cmdNextPort(args []string) error {
	fs := flag.NewFlagSet("next", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	agentID := fs.String("agent", "", "agent ID")
	n := fs.Int("n", 10, "limit")
	fs.Parse(args)
	if *agentID == "" {
		return fmt.Errorf("-agent required")
	}
	return withLock(gDBPath, func() error {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		rows, _ := db.Query(`SELECT id, stage, COALESCE(subproject,''), description FROM tasks
			WHERE status='ready' ORDER BY priority DESC, stage ASC, id ASC LIMIT ?`, *n)
		count := 0
		for rows.Next() {
			var id, sub, desc string
			var stage int
			_ = rows.Scan(&id, &stage, &sub, &desc)
			fmt.Printf("%s stage=%d sub=%s %s\n", id, stage, sub, desc)
			count++
		}
		rows.Close()
		if count == 0 {
			fmt.Println("NO_READY_TASKS")
		}
		return nil
	})
}

// =====================================================================
// dedup-explain
// =====================================================================

func cmdDedupExplainPort(args []string) error {
	fs := flag.NewFlagSet("dedup-explain", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	a := fs.String("a", "", "first task ID (or use -descA)")
	b := fs.String("b", "", "second task ID (or use -descB)")
	descA := fs.String("descA", "", "first task description")
	descB := fs.String("descB", "", "second task description")
	threshold := fs.Float64("threshold", 0.85, "near-duplicate threshold")
	fs.Parse(args)

	if (*a != "" || *b != "") && (*descA == "" || *descB == "") {
		db, err := openDB(gDBPath)
		if err != nil {
			return err
		}
		if *a != "" && *descA == "" {
			_ = db.QueryRow("SELECT description FROM tasks WHERE id=?", *a).Scan(descA)
		}
		if *b != "" && *descB == "" {
			_ = db.QueryRow("SELECT description FROM tasks WHERE id=?", *b).Scan(descB)
		}
		db.Close()
	}
	if *descA == "" || *descB == "" {
		return fmt.Errorf("provide -a and -b (or -descA and -descB)")
	}
	ha, hb := simhash64Port(*descA), simhash64Port(*descB)
	hd := hammingDistancePort(ha, hb)
	simhashSim := 1.0 - float64(hd)/64.0
	jac := jaccardNgramPort(*descA, *descB)
	combined := hybridSimilarityPort(*descA, "", *descB, "")

	fmt.Println("Dedup Report")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("A: %s\n", truncatePort(*descA, 80))
	fmt.Printf("B: %s\n", truncatePort(*descB, 80))
	fmt.Println()
	fmt.Printf("SimHash A: %016x\n", ha)
	fmt.Printf("SimHash B: %016x\n", hb)
	fmt.Printf("Hamming distance: %d / 64 bits (sim=%.4f)\n", hd, simhashSim)
	fmt.Println()
	fmt.Printf("Jaccard n-gram (n=3): %.4f\n", jac)
	fmt.Println()
	fmt.Printf("Hybrid similarity (0.3*simhash + 0.7*jaccard): %.4f\n", combined)
	fmt.Println()
	if combined >= *threshold {
		fmt.Printf("VERDICT: NEAR-DUPLICATE (>= %.2f)\n", *threshold)
	} else if combined >= 0.6 {
		fmt.Println("VERDICT: RELATED (>= 0.60)")
	} else {
		fmt.Println("VERDICT: DISTINCT (< 0.60)")
	}
	A := ngramShinglesPort(*descA, 3)
	B := ngramShinglesPort(*descB, 3)
	shared := []string{}
	for k := range A {
		if _, ok := B[k]; ok {
			shared = append(shared, k)
		}
	}
	sort.Strings(shared)
	if len(shared) > 0 {
		fmt.Println("\nTop shared n-grams:")
		max := 10
		if len(shared) < max {
			max = len(shared)
		}
		for i := 0; i < max; i++ {
			fmt.Printf("  %s\n", shared[i])
		}
	}
	return nil
}

func truncatePort(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
