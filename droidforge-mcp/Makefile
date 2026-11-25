.PHONY: build test lint doctor clean

BINARY := droidforge-mcp
GOFLAGS := -trimpath -ldflags="-s -w"

build:
	go build $(GOFLAGS) -o $(BINARY) ./cmd/droidforge-mcp/

test:
	go test -race -count=1 ./...

lint:
	go vet ./...

doctor: build
	./$(BINARY) doctor

clean:
	rm -f $(BINARY)
	go clean -cache

fmt:
	gofmt -w -s .

all: fmt lint test build
