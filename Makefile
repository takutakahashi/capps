.PHONY: build test lint clean run

BINARY := bin/capps
CMD := ./cmd/capps

build:
	go build -o $(BINARY) $(CMD)

test:
	go test ./...

lint:
	@which golangci-lint > /dev/null 2>&1 || (go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

clean:
	rm -rf bin/

run:
	go run $(CMD)

.DEFAULT_GOAL := build
