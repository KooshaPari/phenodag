# OPS / DEPLOY — Audit Area K: Remediation & Future Work

**Status:** Partially addressed in `fix/phenodag-obs-ops`.  
**Scope:** Dockerfile (multi-stage, healthcheck), `.env.example`, deploy guidance, graceful shutdown.

---

## What was added (this PR)

### 1. Dockerfile (`Dockerfile`)

- **Multi-stage build** using `golang:1.26-alpine` → `alpine:3.21`.
- CGO_ENABLED=0 static binary (pure Go, no libc dependency at runtime).
- Runs as non-root `phenodag` user.
- **HEALTHCHECK** via `phenodag health --db ...` every 30s.
- VOLUME for `/home/phenodag/data` (persistent SQLite storage).
- Default `ENTRYPOINT ["phenodag"]` with `CMD ["--help"]`.

Build & test:

```bash
make docker-build
make docker-run    # init + health + ready on ephemeral volume
```

### 2. `.env.example`

- Documents `PHENODAG_DB`, `PHENODAG_LOG_LEVEL`, `PHENODAG_LOG_FORMAT`,
  `PHENODAG_REMOTE_CLAIMS_DB`.
- No secrets committed (obviously). Copy to `.env` and adjust.

### 3. Makefile targets

| Target | Purpose |
|---|---|
| `docker-build` | Build the Docker image |
| `docker-run` | Quick smoke: init, health, ready |
| `smoke-obs` | CLI-level smoke test of observability commands |

### 4. Graceful shutdown (already present)

Signal handling was already wired in `phenodag_extras.go:1061-1062`
for the `thrash` subcommand:

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
```

No additional changes needed — the existing pattern is correct.

---

## What was NOT applied (remediation guidance)

### CI/CD pipeline config

The repo already has `.sonarcloud.yaml` and `.pre-commit-config.yaml`.
Consider adding a CI workflow that:

- Builds the Docker image and runs `make docker-run`.
- Runs `make test` and `make preset-validate` on every PR.
- Publishes the Docker image to a registry on tagged releases.

**Suggested `.github/workflows/ci.yaml`:**

```yaml
name: CI
on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.26" }
      - run: make build
      - run: make test
      - run: make smoke-obs
  docker:
    needs: build
    if: startsWith(github.ref, 'refs/tags/v')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: make docker-build
      - run: make docker-run
```

### Terraform / Pulumi / Nomad deploy modules

For production deployment, consider one of:

- **Nomad**: a single `job` block that runs the Docker container with a
  `host_volume` for the SQLite DB.
- **Kubernetes**: a `Deployment` + `PersistentVolumeClaim` + `ConfigMap`
  for CLI flags that is seldom cycled.
- **systemd unit** (non-container):

```ini
[Unit]
Description=phenodag fleet DAG worker
After=network.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/phenodag pick --agent $(hostname) --db /var/lib/phenodag/dag.db
User=phenodag
Group=phenodag
Environment=PHENODAG_LOG_LEVEL=info
Environment=PHENODAG_LOG_FORMAT=json

[Install]
WantedBy=multi-user.target
```

> **Note**: because `phenodag` is a CLI tool (not a long-lived daemon),
> the systemd unit uses `Type=oneshot` (or `Type=simple` if you wrap it
> in a loop).  The Docker HEALTHCHECK and `health`/`ready` commands are
> designed for container orchestrators that expect TCP or process probes.

### Secrets management

`.env.example` intentionally contains no secrets.  For production:

- Use a vault / secret store (Hashicorp Vault, AWS Secrets Manager, etc.)
  to inject `PHENODAG_DB`, API tokens, etc.
- Never mount `.env` into production containers without reviewing it
  first.

### Log aggregation

Once the binary emits JSON logs (`--log-format json`), pipe to:

- **Vector** / **Fluent Bit** → OpenSearch / Loki
- **CloudWatch agent** (AWS)
- **Stackdriver logging** (GCP)

The `correlation_id` field is the join key for correlating across
agent invocations.

### Backup / DR for SQLite

The single `phenodag.db` file is the source of truth.  For production:

1. Use `litestream` (https://litestream.io/) to continuously replicate
   to S3-compatible storage.
2. Schedule `sqlite3 phenodag.db .backup` via cron.
3. Ensure the `data/` volume in Docker is on a persistent (non-ephemeral)
   mount.

### Monitoring / alerting

With the `metrics` subcommand, wire Prometheus to scrape:

```
scrape_configs:
  - job_name: 'phenodag'
    static_configs:
      - targets: ['host:9090']
```

Then create alerts for:

- `phenodag_tasks_failed > 0` for > 5 minutes
- `phenodag_uptime_seconds < 60` (container restart loop)
- `phenodag_claims_active == 0` during business hours (no agents working)
