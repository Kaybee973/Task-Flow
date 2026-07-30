# TaskFlow Makefile
#
# Common targets for development, testing, and CI.
# Use `make <target>` to run a specific command.

GO       ?= go
GOFLAGS  ?=
BINARY   ?= taskflow
PORT     ?= 8080

.PHONY: all build vet test race lint sign-payload run clean

all: build vet test

# ── Build ──────────────────────────────────────────────────────

build:
	$(GO) build $(GOFLAGS) -o $(BINARY) .

build-race:
	$(GO) build $(GOFLAGS) -race -o $(BINARY) .

# ── Static analysis ────────────────────────────────────────────

vet:
	$(GO) vet $(GOFLAGS) ./...

fmt:
	$(GO) fmt ./...

# fmt-check verifies formatting without modifying files.
# Exits non-zero if any file needs formatting.
fmt-check:
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "Files need formatting:"; \
		gofmt -l .; \
		exit 1; \
	fi

# lint runs vet and checks formatting (without modifying).
# Use `make fmt` to auto-format before committing.
lint: vet fmt-check

# ── Tests ──────────────────────────────────────────────────────

test:
	$(GO) test $(GOFLAGS) -v -count=1 ./...

test-short:
	$(GO) test $(GOFLAGS) -short -count=1 ./...

race:
	$(GO) test $(GOFLAGS) -race -v -count=1 ./...

test-all: build-race vet race
	@echo "All checks passed ✓"

# ── Database ────────────────────────────────────────────────────

# DATABASE_URL must be set, e.g.:
#   export DATABASE_URL="postgres://user:pass@localhost:5432/taskflow?sslmode=disable"

# migrate applies all pending SQL migrations.
migrate:
	@if [ -z "$$DATABASE_URL" ]; then \
		echo "ERROR: DATABASE_URL is not set"; \
		exit 1; \
	fi
	@for f in migrations/*.sql; do \
		echo "Running $$f..."; \
		psql "$$DATABASE_URL" -f "$$f" -q 2>&1 | grep -v "NOTICE:\|already exists"; \
	done
	@echo "Migrations complete ✓"

# db-status checks if PostgreSQL is reachable and tables exist.
db-status:
	@if [ -z "$$DATABASE_URL" ]; then \
		echo "DATABASE_URL not set — using in-memory storage"; \
		exit 0; \
	fi
	@echo "Testing PostgreSQL connection..."
	@psql "$$DATABASE_URL" -c "SELECT COUNT(*) AS task_count FROM tasks" -q 2>&1 || \
		echo "⚠️  Could not connect (run 'make migrate' first)"

# ── Tools ──────────────────────────────────────────────────────

# sign-payload generates a valid x402 PAYMENT-SIGNATURE header
# using a freshly generated ECDSA key. Output includes the sender
# address, the base64-encoded header value, and ready-to-use curl
# commands.
sign-payload:
	$(GO) run $(GOFLAGS) scripts/sign_payload.go

# ── Run ────────────────────────────────────────────────────────

run:
	PORT=$(PORT) $(GO) run $(GOFLAGS) .

run-pg:
	DATABASE_URL=$(DATABASE_URL) PORT=$(PORT) $(GO) run $(GOFLAGS) .

run-binary:
	PORT=$(PORT) ./$(BINARY)

# ── Docker ─────────────────────────────────────────────────────

# Start PostgreSQL + the app with hot-reload.
dc-up:
	docker compose up -d

# Start only PostgreSQL (for running the app locally with make run-pg).
dc-db:
	docker compose up -d db

# View logs.
dc-logs:
	docker compose logs -f

# Stop and remove containers.
dc-down:
	docker compose down

# Stop and remove containers + volumes (wipes the database).
dc-down-clean:
	docker compose down -v

# Build the production image.
dc-build:
	docker compose build app

# Run the production stack.
dc-prod:
	docker compose up -d app

# ── Clean ──────────────────────────────────────────────────────

clean:
	rm -f $(BINARY)
	rm -rf dist/
