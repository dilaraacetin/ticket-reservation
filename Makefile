.PHONY: run docs build test cover cover-html vet lint fmt-check ci tidy clean \
	run-db token promote db-up db-down db-reset db-shell migrate-up migrate-down \
	image up down logs

BINARY := bin/server

# Local settings, if there are any. The leading dash means "carry on when the file
# is not there", and export hands whatever it defines to every command below.
#
# .env is git ignored: it is where a real secret goes. The settings it may hold
# are the ones config.Load reads.
-include .env
export

# The compose database, used by the db targets and by run-db.
#
# Named apart from DATABASE_URL on purpose. Exporting .env exports everything in
# it, and a variable called DATABASE_URL here would then be exported too, turning
# `make run` into a database run for anyone who never asked for one.
COMPOSE_DATABASE_URL ?= postgres://ticket:ticket@localhost:5433/ticket_reservation?sslmode=disable

# The signing secret for local work only.
#
# It lives here rather than in the binary because nobody deploys with make: a
# secret in the source ships with the source, and a server that starts on one is
# a server anyone who has read the repository can forge a token for. The binary
# has no fallback of its own and refuses to start without AUTH_SECRET.
#
# Never assigned to AUTH_SECRET at this level. The bare export above hands every
# make variable to every sub-command, so an AUTH_SECRET defined here would reach
# docker compose as well, and the guard in docker-compose.yml that refuses to
# start without one would never fire. The recipes below fall back to it in the
# shell instead, which keeps it inside the command that needs it.
DEV_AUTH_SECRET ?= dev-only-secret-do-not-deploy-0000

# In-memory stores, no Docker required.
#
# DATABASE_URL is cleared rather than just left unset: exporting .env hands this
# target whatever is in there, so without this the target that promises no
# database quietly needs one, and fails when it is not running.
run:
	DATABASE_URL= AUTH_SECRET="$${AUTH_SECRET:-$(DEV_AUTH_SECRET)}" go run ./cmd/server

# Against the local Postgres.
run-db:
	DATABASE_URL='$(COMPOSE_DATABASE_URL)' AUTH_SECRET="$${AUTH_SECRET:-$(DEV_AUTH_SECRET)}" go run ./cmd/server

# Signs a token for a user id, for trying the API by hand.
# Usage: make token FOR=user-1 [LIFETIME=24h]
#
# Not USER: that is already in the environment as the login name, so a
# target-specific default for it never applies and the token comes out signed
# for whoever is at the keyboard.
FOR ?= user-1

token:
	@AUTH_SECRET="$${AUTH_SECRET:-$(DEV_AUTH_SECRET)}" go run ./cmd/token '$(FOR)' $(LIFETIME)

# Makes an existing account an administrator, against the compose database.
# Usage: make promote EMAIL=you@example.com
promote:
	DATABASE_URL='$(COMPOSE_DATABASE_URL)' go run ./cmd/admin promote '$(EMAIL)'

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

# The version the image reports at startup. Overridden by whatever builds a
# release: make image VERSION=1.4.0
VERSION ?= dev

image:
	docker build --build-arg VERSION='$(VERSION)' -t ticket-reservation:$(VERSION) -t ticket-reservation:dev .

# The whole thing: database, migrations, then the server.
up:
	docker compose up --build -d --wait

down:
	docker compose down

logs:
	docker compose logs -f app

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
	DATABASE_URL='$(COMPOSE_DATABASE_URL)' go run ./cmd/migrate up

migrate-down:
	DATABASE_URL='$(COMPOSE_DATABASE_URL)' go run ./cmd/migrate down
