.PHONY: generate portal schema openapi build build-all

CONFIG_FILE ?= config/config.yaml
OPENAPI_OUTPUT ?= config/openapi/openapi.yml

generate: portal schema

portal:
	go generate ./internal/handlers/portal

schema:
	go generate ./internal/config

openapi:
	go run ./cmd openapi generate --config-file $(CONFIG_FILE) --output $(OPENAPI_OUTPUT)

build:
	go build ./cmd

build-all: generate build
