#!/usr/bin/env sh
set -eu

VERSION="${VERSION:-v0.1.0}"
CONFIG_FILE="${CONFIG_FILE:-config/config.example.yaml}"
OPENAPI_OUTPUT="${OPENAPI_OUTPUT:-/tmp/nimbgtw-openapi.yml}"

go generate ./internal/handlers/portal
go generate ./internal/config
go test ./...
npm --prefix developer-portal run build
make build VERSION="${VERSION}"
./nimbgtw routes validate --config-file "${CONFIG_FILE}"
./nimbgtw openapi generate --config-file "${CONFIG_FILE}" --output "${OPENAPI_OUTPUT}"
./nimbgtw version | grep "${VERSION}"
