.PHONY: bin

BUILD := $(shell git describe --tags)
bin:
	go build -ldflags="-s -w -X github.com/open-oni/batch-agent/version.Version=$(BUILD)" -o bin/batch-agent github.com/open-oni/batch-agent/cmd/batch-agent
