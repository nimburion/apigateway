# API Gateway

HTTP/WebSocket API Gateway built on `nimburion`.

## Overview
This service exposes public endpoints and proxies them to internal backends with:
- authentication and authorization
- hierarchical rate limiting
- per-route OpenAPI request validation
- WebSocket proxy support
- developer portal on the management server

## Quick Start
```bash
go run ./cmd --config-file config/config.yaml
```

## Main Commands
```bash
go run ./cmd routes show --config-file config/config.yaml
go run ./cmd routes validate --config-file config/config.yaml
go run ./cmd openapi generate --config-file config/config.yaml --output config/openapi/openapi.yml
go test ./...
```

## Documentation
- Wiki base: [https://nimburion.github.io/wiki/apigateway](https://nimburion.github.io/wiki/apigateway)
- Overview: [https://nimburion.github.io/wiki/apigateway/overview](https://nimburion.github.io/wiki/apigateway/overview)
- Operations: [https://nimburion.github.io/wiki/apigateway/operations](https://nimburion.github.io/wiki/apigateway/operations)
- Routing and Policies: [https://nimburion.github.io/wiki/apigateway/routing-and-policies](https://nimburion.github.io/wiki/apigateway/routing-and-policies)

## Environment
Environment variables use the `APP_` prefix. Common examples:
- `APP_AUTH_ISSUER`
- `APP_AUTH_JWKS_URL`
- `APP_AUTH_AUDIENCE`
- `APP_CORS_ALLOWED_ORIGINS`
