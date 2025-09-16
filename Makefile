BUILD := $(shell git describe --tags)

.PHONY: bin
bin:
	CGO_ENABLED=0 go build -ldflags="-s -w -X github.com/open-oni/oni-agent/internal/version.Version=$(BUILD)" -o bin/agent github.com/open-oni/oni-agent/cmd/agent
	CGO_ENABLED=0 go build -ldflags="-s -w -X github.com/open-oni/oni-agent/internal/version.Version=$(BUILD)" -o bin/remove-issues github.com/open-oni/oni-agent/cmd/remove-issues

.PHONY: test
test:
	go test ./...

.PHONY: format
format:
	go fmt ./...

.PHONY: lint
lint:
	go tool revive ./...
	go vet ./...

.PHONY: clean
clean:
	rm -f bin/*

.PHONY: audit
audit:
	go tool govulncheck ./...
