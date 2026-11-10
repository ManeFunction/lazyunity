BINARY  := lazyunity
VERSION ?= 0.1.0
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build run test vet fmt lint check

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/lazyunity

run:
	go run ./cmd/lazyunity

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

lint: vet
	@test -z "$$(gofmt -s -l .)" || (echo "gofmt needed:"; gofmt -s -l .; exit 1)

check: lint test
