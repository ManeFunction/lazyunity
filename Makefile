BINARY  := lazyunity
LDFLAGS := -s -w # the version is in cmd/lazyunity/main.go

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
