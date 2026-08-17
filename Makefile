.PHONY: run docs build test cover cover-html vet lint tidy clean

BINARY := bin/server

run:
	go run ./cmd/server

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

tidy:
	go mod tidy

clean:
	rm -rf bin coverage.out
