// phenodag_obs.go — Observability integration for phenodag.
//
// Adds:
//   - Global flags --log-level / --log-format (parsed before subcommand routing)
//   - Subcommands: metrics, health, ready
//   - Correlation ID plumbing
//   - Metrics hook wired into the global metrics.Store singleton
//   - All additive; existing behaviour is unchanged.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/KooshaPari/phenodag/internal/logging"
	"github.com/KooshaPari/phenodag/internal/metrics"
)

// Global metrics store — accessible from every command.
var Metrics = metrics.New()

// Global variables for log-level / log-format; parsed at the very top of main().
var (
	gLogLevel  = "info"
	gLogFormat = "text"
)

// defaultCorrelationID is set once per process invocation.
var defaultCorrelationID string

// initLogger parses global env/args for --log-level / --log-format and
// initialises slog.  Must be called before any subcommand runs.
//
// Environment variable fallbacks:
//
//	PHENODAG_LOG_LEVEL  (debug|info|warn|error)
//	PHENODAG_LOG_FORMAT (text|json)
func initLogger() {
	if v := os.Getenv("PHENODAG_LOG_LEVEL"); v != "" {
		gLogLevel = v
	}
	if v := os.Getenv("PHENODAG_LOG_FORMAT"); v != "" {
		gLogFormat = v
	}
	logging.Init(gLogLevel, gLogFormat)
	defaultCorrelationID = logging.NewCorrelationID()
}

// ctx returns a background context decorated with the process-wide
// correlation ID.  Commands that do not receive an external request
// should use this as their root context.
func ctx() context.Context {
	return logging.WithCorrelationID(context.Background(), defaultCorrelationID)
}

// ---------- health ----------

func cmdHealth(args []string) error {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)

	db, err := openDB(gDBPath)
	if err != nil {
		// Failure to open DB is considered unhealthy.
		fmt.Fprintf(os.Stderr, "UNHEALTHY: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Ping the SQLite database to confirm the WAL file is reachable.
	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "UNHEALTHY: db ping: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("OK")
	return nil
}

// ---------- ready ----------

func cmdReady(args []string) error {
	fs := flag.NewFlagSet("ready", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)

	db, err := openDB(gDBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NOT READY: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "NOT READY: db ping: %v\n", err)
		os.Exit(1)
	}

	// Confirm the schema is migrated by reading at least one meta key.
	var val string
	err = db.QueryRow(`SELECT value FROM dag_meta WHERE key='version'`).Scan(&val)
	if err != nil || val == "" {
		fmt.Fprintf(os.Stderr, "NOT READY: schema not initialised (run init first)\n")
		os.Exit(1)
	}

	fmt.Printf("OK %s\n", val)
	return nil
}

// ---------- metrics ----------

func cmdMetrics(args []string) error {
	fs := flag.NewFlagSet("metrics", flag.ExitOnError)
	_ = fs.String("db", gDBPath, "path to SQLite DB")
	fs.Parse(args)

	// Attach DB-level metrics before dumping.
	if db, err := openDB(gDBPath); err == nil {
		defer db.Close()
		var total int64
		_ = db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&total)
		Metrics.SetGauge("tasks_total", total)

		var ready, inprog, done int64
		_ = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE status='ready'`).Scan(&ready)
		_ = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE status='in_progress'`).Scan(&inprog)
		_ = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE status='done'`).Scan(&done)
		Metrics.SetGauge("tasks_ready", ready)
		Metrics.SetGauge("tasks_in_progress", inprog)
		Metrics.SetGauge("tasks_done", done)

		var agents int64
		_ = db.QueryRow(`SELECT COUNT(*) FROM agents`).Scan(&agents)
		Metrics.SetGauge("agents_total", agents)

		var claims int64
		_ = db.QueryRow(`SELECT COUNT(*) FROM claims`).Scan(&claims)
		Metrics.SetGauge("claims_active", claims)
	} else {
		logging.Info(ctx(), "metrics: cannot open db to collect gauge values", "error", err)
	}

	Metrics.Incr("metrics_scrapes_total")

	// Expose process-level gauges
	Metrics.SetGauge("process_start_time_seconds", time.Now().Unix())

	Metrics.WritePrometheusText(os.Stdout)
	return nil
}

// ---------- timing helper ----------

// instrumentCmd wraps a subcommand function with a latency observation.
// The returned function can be assigned in the main switch.
func instrumentCmd(name string, fn func([]string) error) func([]string) error {
	return func(args []string) error {
		start := time.Now()
		err := fn(args)
		Metrics.ObserveLatency("cmd_"+name, time.Since(start))
		Metrics.Incr("cmd_" + name + "_total")
		if err != nil {
			Metrics.Incr("cmd_" + name + "_errors")
		}
		return err
	}
}
