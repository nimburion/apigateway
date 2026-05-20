.PHONY: generate portal schema openapi build build-all smoke-prod

CONFIG_FILE ?= config/config.yaml
OPENAPI_OUTPUT ?= config/openapi/openapi.yml
VERSION ?= dev
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/nimburion/nimburion/pkg/version.AppVersion=$(VERSION) -X github.com/nimburion/nimburion/pkg/version.GitCommit=$(GIT_COMMIT) -X github.com/nimburion/nimburion/pkg/version.BuildTime=$(BUILD_TIME)

all: generate build

generate: portal schema

portal:
	go generate ./internal/handlers/portal

schema:
	go generate ./internal/config

openapi:
	go run ./cmd openapi generate --config-file $(CONFIG_FILE) --output $(OPENAPI_OUTPUT)

build:
	go build -ldflags "$(LDFLAGS)" -o nimbgtw ./cmd

smoke-prod:
	./scripts/smoke-production.sh
