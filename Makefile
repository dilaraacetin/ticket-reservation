.PHONY: run docs build test cover cover-html vet lint fmt-check ci tidy clean \
	run-db db-up db-down db-reset db-shell migrate-up migrate-down

BINARY := bin/server

# The local development database, matching docker-compose.yml. Deliberately not
# exported: only the targets that need it pass it, so `make run` stays runnable
# with no database at all.
DATABASE_URL ?= postgres://ticket:ticket@localhost:5433/ticket_reservation?sslmode=disable

# In-memory stores, no Docker required.
run:
	go run ./cmd/server

# Against the local Postgres.
run-db:
	DATABASE_URL='$(DATABASE_URL)' go run ./cmd/server

docs:
	open http://localhost:8080/docs

build:
	go build -o $(BINARY) ./cmd/server

test:
	go test -race ./...

cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

cover-html: cover
	go tool cover -html=coverage.out

vet:
	go vet ./...

lint:
	golangci-lint run ./...

# Fails when anything is unformatted. gofmt -l alone always exits 0, so the check
# has to look at whether it printed anything.
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "these files are not gofmt'd:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

# What CI runs, minus the tidy check, which only makes sense on a clean checkout.
ci: fmt-check vet lint test

tidy:
	go mod tidy

clean:
	rm -rf bin coverage.out

db-up:
	docker compose up -d --wait

db-down:
	docker compose down

db-reset:
	docker compose down -v
	docker compose up -d --wait
	$(MAKE) migrate-up

db-shell:
	docker compose exec postgres psql -U ticket -d ticket_reservation

migrate-up:
	DATABASE_URL='$(DATABASE_URL)' go run ./cmd/migrate up

migrate-down:
	DATABASE_URL='$(DATABASE_URL)' go run ./cmd/migrate down
