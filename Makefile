.PHONY: run build test test-race cover vet lint tidy clean

BINARY := bin/server

run:
	go run ./cmd/server

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

tidy:
	go mod tidy

clean:
	rm -rf bin coverage.out
