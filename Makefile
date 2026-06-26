.PHONY: build test test-cli-smoke run clean install release lint lint-install smoke doc mod-hygiene preset-validate tidy

BIN     ?= phenodag
LDFLAGS ?= -s -w

# Tier-3 #3 polish (DAG-013).  Runs the binary through a canned init/seed/
# validate/pick/status lifecycle against a fresh temp DB, asserts each
# subcommand exits 0, and asserts the resulting DB is non-empty + has
# the expected schema.  Pure stdlib; no Go toolchain needed at runtime
# (the binary is built once via `make build`).
test-cli-smoke: build
	@python scripts/smoke_cli.py --bin $(BIN) --db /tmp/phenodag-smoke.db

# Tier-3 #3 polish (DAG-013).  Print the CLI's --help as a quick reference
# and validate the README links are reachable (file:// only).
doc:
	@./$(BIN) --help
	@echo "---"
	@python scripts/check_readme_links.py README.md

# Hygiene gardening (DAG-T9). Run after `make lint-install` once.
lint-install:
	@command -v pre-commit >/dev/null 2>&1 || { echo "install pre-commit: pip install pre-commit"; exit 1; }
	@command -v staticcheck >/dev/null 2>&1 || { echo "install staticcheck: go install honnef.co/go/tools/cmd/staticcheck@latest"; exit 1; }
	pre-commit install

# Fast local lint pass (no commit hook).
lint:
	@gofmt -l -s . | grep -v vendor/ | grep -v "\.git/" | tee /tmp/gofmt.out
	@test ! -s /tmp/gofmt.out || { echo "gofmt: reformat with 'gofmt -w .'"; exit 1; }
	go vet ./...
	staticcheck ./...

build:
	GOFLAGS="-mod=mod" go build -mod=mod -ldflags "$(LDFLAGS)" -o $(BIN) .

test:
	GOFLAGS="-mod=mod" go test -mod=mod ./...

run: build
	./$(BIN) --help

# Quick smoke test against an ephemeral DB.
# Uses the externalized preset loader (seed-yaml) so the smoke also
# exercises the new presets/*.yaml path end-to-end.
smoke: build
	@rm -f /tmp/phenodag-smoke.db /tmp/phenodag-smoke.db-shm /tmp/phenodag-smoke.db-wal
	./$(BIN) init     --width 20 --stages 6 --db /tmp/phenodag-smoke.db
	./$(BIN) seed-yaml --preset v3-180 --db /tmp/phenodag-smoke.db
	./$(BIN) validate --db /tmp/phenodag-smoke.db
	./$(BIN) pick     --agent smoke-agent --db /tmp/phenodag-smoke.db
	./$(BIN) status   --db /tmp/phenodag-smoke.db

# Run go mod tidy to align go.mod and go.sum with the actual import graph.
# P23 hardening: keeps the require block minimal so the build is reproducible.
tidy:
	@go mod tidy -compat=1.21
	@echo "== go mod tidy OK =="

# Hygiene sweep: drops unused direct deps and enforces the P23 invariant
# (no mattn/go-sqlite3 alongside modernc.org/sqlite). The grep is the
# authoritative guard; the import-graph check via `go list -m all` is a
# secondary confirmation.
mod-hygiene: tidy
	@echo "== mod-hygiene: scanning require block =="
	@if grep -E 'github.com/mattn/go-sqlite3' go.mod >/dev/null 2>&1; then \
	  echo "  FAIL: mattn/go-sqlite3 still in go.mod (violates P23 pure-Go SQLite)"; \
	  exit 1; \
	else \
	  echo "  OK: no mattn/go-sqlite3 (pure-Go SQLite invariant holds)"; \
	fi
	@for dep in $$(go list -m -f '{{if not .Indirect}}{{.Path}}{{end}}' all 2>/dev/null | sort -u); do \
	  if [ "$$dep" != "github.com/google/uuid" ] && \
	     [ "$$dep" != "gopkg.in/yaml.v3" ] && \
	     [ "$$dep" != "modernc.org/sqlite" ] && \
	     [ "$$dep" != "phenodag" ]; then \
	    echo "  suspect direct dep: $$dep"; \
	  fi; \
	done

# DAG-T9.0 / FR #5 increment: fail CI if any *_Port function is added or
# reintroduced without first being removed from scripts/.port-dupes-allowlist.
# Tracks the XXL superset-merge refactor: as the FR completes, the
# allowlist shrinks toward zero. Pure POSIX + awk + grep.
verify-no-port-dupes:
	@echo "== verify-no-port-dupes =="
	@fail=0; \
	known_total=$$(awk '!/^[[:space:]]*#/ && NF {n++} END {print n+0}' scripts/.port-dupes-allowlist); \
	actual_total=$$(grep -hoE '^func [A-Za-z0-9_]+Port\b' *.go 2>/dev/null | sort -u | wc -l | tr -d ' '); \
	listed=$$(awk '!/^[[:space:]]*#/ && NF {print $$1}' scripts/.port-dupes-allowlist | sort -u); \
	present=$$(grep -hoE '^func [A-Za-z0-9_]+Port\b' *.go 2>/dev/null | sed 's/^func //; s/Port$$//' | sort -u); \
	extra=$$(comm -23 <(echo "$$present") <(echo "$$listed")); \
	missing=$$(comm -13 <(echo "$$present") <(echo "$$listed")); \
	echo "  *_Port funcs on disk: $$actual_total"; \
	echo "  allowlist entries:     $$known_total"; \
	if [ -n "$$extra" ]; then \
	  echo "  FAIL: new *_Port funcs not in allowlist:"; \
	  echo "$$extra" | sed 's/^/    /'; \
	  fail=1; \
	fi; \
	if [ -n "$$missing" ]; then \
	  echo "  INFO: allowlist entries no longer on disk (FR progress — remove from allowlist):"; \
	  echo "$$missing" | sed 's/^/    /'; \
	fi; \
	exit $$fail

# Round-trip every built-in preset YAML through the loader and assert
# that the seeded task count equals the YAML-declared task count
# (core.stages * core.width + sum of side_dags[].size). Pure POSIX + awk.
# Exits non-zero if any preset fails validation or seeding.
preset-validate:
	@echo "== preset-validate =="
	@$(MAKE) build
	@fail=0; \
	for p in v3-180 melosviz-185 agileplus-50 tracera-50 mcp-fleet-60 mcp-fleet-150 empty; do \
	  db=/tmp/phenodag-validate-$$p.db; \
	  ./$(BIN) seed-yaml --preset $$p --db $$db >/dev/null 2>&1; \
	  actual=$$(./$(BIN) status --db $$db | tr -d '\r' | awk '/^tasks: [0-9]+ total/{print $$2; exit}'); \
	  expected=$$(awk -f scripts/count-expected.awk presets/$$p.yaml); \
	  if [ -n "$$actual" ] && [ "$$actual" = "$$expected" ]; then \
	    echo "  $$p: $$actual tasks OK"; \
	  else \
	    echo "  $$p: MISMATCH (actual=$$actual, expected=$$expected)"; \
	    fail=1; \
	  fi; \
	  rm -f $$db $$db-shm $$db-wal; \
	done; \
	exit $$fail

clean:
	rm -f $(BIN) /tmp/phenodag-*.db* /tmp/phenodag-*.md

install: build
	cp $(BIN) $(GOPATH)/bin/$(BIN) 2>/dev/null || cp $(BIN) /usr/local/bin/$(BIN)

release: build
	@echo "Building release with stripped symbols"
	GOFLAGS="-mod=mod" CGO_ENABLED=0 go build -mod=mod -ldflags "$(LDFLAGS)" -trimpath -o $(BIN) .
	@ls -la $(BIN)
