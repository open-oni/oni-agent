.PHONY: bin test clean

BUILD := $(shell git describe --tags)
bin:
	CGO_ENABLED=0 go build -ldflags="-s -w -X github.com/open-oni/oni-agent/internal/version.Version=$(BUILD)" -o bin/agent github.com/open-oni/oni-agent/cmd/agent

test:
	go test ./...

clean:
	rm -f bin/*
