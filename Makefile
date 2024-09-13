.PHONY: bin

BUILD := $(shell git describe --tags)
bin:
	go build -ldflags="-s -w -X github.com/open-oni/oni-agent/version.Version=$(BUILD)" -o bin/agent github.com/open-oni/oni-agent/cmd/agent
