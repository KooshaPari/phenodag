.PHONY: build test run clean install release

BIN     ?= phenodag
LDFLAGS ?= -s -w

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
