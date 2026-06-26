# OBSERVABILITY — Audit Area G: Remediation & Future Work

**Status:** Partially addressed in `fix/phenodag-obs-ops`.  
**Scope:** Structured logging, correlation ID, health/ready CLI commands, metrics export.

---

## What was added (this PR)

### 1. Structured logging (`internal/logging/log.go`)

- Wraps Go 1.26 `log/slog` behind a thin convenience package.
- Supports **log levels**: `debug`, `info`, `warn`, `error`.
- Supports **output formats**: `text` (default) and `json`.
- Configuration via:
  - CLI global flags: `--log-level`, `--log-format`
  - Environment variables: `PHENODAG_LOG_LEVEL`, `PHENODAG_LOG_FORMAT`

### 2. Correlation ID (`internal/logging/log.go`)

- Each process invocation generates a 16-hex-char correlation ID.
- Propagated through a `context.Context` key via `logging.WithCorrelationID`.
- The context-aware `logging.Logger(ctx)` attaches the correlation ID as
  `"correlation_id"` to every log line.
- All top-level command errors now use `logging.Error(ctx(), ...)`.

### 3. Health / Readiness CLI commands

```
phenodag health   # exits 0 if the SQLite DB is reachable
phenodag ready    # exits 0 if the DB is both reachable AND initialised
phenodag metrics  # dumps Prometheus-format metrics to stdout
```

- `health` — pings the DB. Suitable for Docker HEALTHCHECK.
- `ready` — pings the DB + verifies the `dag_meta` table has a `version` row.
  Use this for orchestration liveness probes.
- `metrics` — gathers task/agent/claim counts from SQLite and writes
  Prometheus exposition format to stdout. Exposes:
  - `phenodag_uptime_seconds` (gauge)
  - `phenodag_tasks_total` / `_ready` / `_in_progress` / `_done` (gauges)
  - `phenodag_agents_total` / `phenodag_claims_active` (gauges)
  - `phenodag_cmd_*_total` / `_errors` / latency summaries (counters)

### 4. Metrics hook (`internal/metrics/metrics.go`)

- Thread-safe counters, gauges, and latency buckets.
- Prometheus-text exporter (no client lib dependency).
- `Metrics.Incr()` / `Metrics.ObserveLatency()` usable from any command.
- Available as the global `main.Metrics` variable.

---

## What was NOT applied (remediation guidance)

The following are outside the scope of this additive PR but should be
addressed in follow-up work:

### OpenTelemetry (OTel) tracing

The current codebase is a single-binary CLI with no HTTP/gRPC surface.
Distributed tracing (trace propagation, span creation) would add value
only if:

1. The remote-claim subsystem (`internal/remoteclaim/`) grows an actual
   HTTP server (today it reads a local SQLite file).
2. The `dashboard` subcommand spawns a long-lived web server.

When either condition is met, inject an OTel Go SDK tracer provider and
wrap request handlers with `otelhttp.NewHandler`. The `correlation_id`
in the context can be mapped to the OTel `trace_id` via a custom
`idgen` implementation.

**Suggested approach:**

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
    "go.opentelemetry.io/otel/sdk/resource"
    "go.opentelemetry.io/otel/sdk/trace"
)

func initOTel() {
    exp, _ := otlptracehttp.New(context.Background())
    tp := trace.NewTracerProvider(trace.WithBatcher(exp))
    otel.SetTracerProvider(tp)
}
```

### Request-scoped structured context in `phenodag_extras.go`

The ported commands (`cmdDashboardPort`, `cmdGanttPort`, etc.) use
`context.Background()` or no context at all.  These should be migrated
to use the `ctx()` helper from `phenodag_obs.go` so that their logs
carry a correlation ID.

**Diff for one representative function:**

```diff
 func cmdDashboardPort(args []string) error {
+    log := logging.Logger(ctx())
+    log.Info("dashboard started")
     // ... existing body ...
 }
```

Repeat for every `cmd*Port` function in `phenodag_extras.go` that emits
output to stderr or has side-effects worth tracing.

### Structured error types

Errors are currently plain strings / `fmt.Errorf`.  For production
diagnostics, define a sentinel or structured error type that carries
a `correlation_id` and a machine-readable error code:

```go
type PhenoError struct {
    Code    string // e.g. "E_DB_OPEN"
    Message string
    Ctx     context.Context
}

func (e *PhenoError) Error() string {
    cid := logging.GetCorrelationID(e.Ctx)
    return fmt.Sprintf("[%s] %s (correlation_id=%s)", e.Code, e.Message, cid)
}
```

### Structured logging in `queries.go`

`queries.go` currently uses `log.Printf` (stdlib logger, unstructured).
Replace with the context-aware logger:

```diff
- log.Printf("queryTasksIDStatus: %v", err)
+ slog.Default().Warn("queryTasksIDStatus", "error", err)
```

### Log rotation

The binary itself writes to stderr.  For long-running agent scenarios
where `phenodag` is invoked repeatedly, configure external log rotation
(e.g. `logrotate` on Linux, or pipe stderr through `multilog`).
Alternatively, add a `--log-file` flag that writes to a rotating file
via an io.Writer wrapper.

---

## Verification

```bash
# Quick smoke
make smoke-obs

# Manual check
./phenodag --log-level debug --log-format json status

# Prometheus output
./phenodag metrics
```
