.PHONY: build test run clean install release lint lint-install smoke

BIN     ?= phenodag
LDFLAGS ?= -s -w

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

# Quick smoke test against an ephemeral DB
smoke: build
	@rm -f /tmp/phenodag-smoke.db /tmp/phenodag-smoke.db-shm /tmp/phenodag-smoke.db-wal
	./$(BIN) init   --width 20 --stages 6 --db /tmp/phenodag-smoke.db
	./$(BIN) seed   --preset v3-180 --db /tmp/phenodag-smoke.db
	./$(BIN) validate --db /tmp/phenodag-smoke.db
	./$(BIN) pick   --agent smoke-agent --db /tmp/phenodag-smoke.db
	./$(BIN) status --db /tmp/phenodag-smoke.db

clean:
	rm -f $(BIN) /tmp/phenodag-*.db* /tmp/phenodag-*.md

install: build
	cp $(BIN) $(GOPATH)/bin/$(BIN) 2>/dev/null || cp $(BIN) /usr/local/bin/$(BIN)

release: build
	@echo "Building release with stripped symbols"
	GOFLAGS="-mod=mod" CGO_ENABLED=0 go build -mod=mod -ldflags "$(LDFLAGS)" -trimpath -o $(BIN) .
	@ls -la $(BIN)
