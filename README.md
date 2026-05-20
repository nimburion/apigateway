# API Gateway

HTTP/WebSocket API Gateway built on `nimburion`.

## Overview
This service exposes public endpoints and proxies them to internal backends with:
- authentication and authorization
- support for public and protected APIs in the same gateway
- hierarchical rate limiting
- per-route OpenAPI request validation
- WebSocket proxy support
- Posture Portal on the management server

## Quick Start
```bash
nimbgtw config generate --output /tmp/apigateway.minimal.yaml
nimbgtw routes validate --config-file config/config.example.yaml
nimbgtw openapi generate --config-file config/config.example.yaml --output /tmp/openapi.yml
go run cmd/main.go --config-file config/config.example.yaml
```

`config/config.example.yaml` is intentionally runnable as a local read-only setup:
- `portal.mode=read-only`
- `config_store.source_of_truth=file`
- public management server (`management.auth_enabled=false`)
- a single public `/healthz` proxy route

To run protected routes or a protected management server, start from `config/config.example.yaml` and add auth settings (`auth.enabled=true`, `issuer`, `jwks_url`, `audience`).
Examples in this document assume `nimbgtw` is available in your `PATH`.

`auth.enabled=true` is required only when the effective routing configuration uses auth-aware middleware or scopes, or when `management.auth_enabled=true`.
Purely public proxy routes can run with `auth.enabled=false`.

## Routing Model
- `Authenticate`, `ClaimsGuardFromConfig`, and `ForwardIdentityHeaders` are resolved per group, route, endpoint, and method.
- `disable_middlewares` is applied at the same levels, so a public route can live inside a protected group.
- `scopes` require auth on the effective route.
- `/health` on the management server stays public.
- `/portal`, `/portal/`, `/portal/assets/:file`, `/portal/:file`, `/api/portal/routes`, and `/api/portal/groups` follow `management.auth_enabled`.

## Posture Portal Config
- `portal.enabled` turns the Posture Portal surface on or off.
- `portal.mode` supports `read-only` and `managed`.
- `managed` mode is GA for routing configuration. Boot/process settings such as auth, database, HTTP listeners, and management server settings remain file/env driven.
- `portal.catalog.expose_target_urls` controls whether the catalog is allowed to expose backend target URLs.
- `portal.catalog.expose_openapi_errors` controls whether OpenAPI loader/parse errors are exposed in the catalog payload.
- `portal.auth.read_scopes`, `write_scopes`, `publish_scopes`, and `rollback_scopes` protect the managed API v1 read, draft, publish, and rollback operations.
- The React portal supports shareable deep links for group and surface posture:
  - `/portal/groups/:group`
  - `/portal/posture/:group`
  - `/portal/posture/:group/route/:method/:path`
  - `/portal/posture/:group/websocket/:path`
- The posture page exposes visible breadcrumb context and `Copy link` actions for the current group or selected surface.

## Config Store
- `config_store.source_of_truth` supports `file` and `database`.
- `database` source of truth is GA with the `postgres` backend for routing config.
- `EnsureSchema` runs at boot and idempotently creates or updates the config version and audit tables.
- Managed config uses numeric `version` and `base_version` values. When `require_base_version_match=true`, publish requests must include the active `base_version` or they fail with `409` on stale input.
- When `require_validation_before_publish=true`, only `validated` drafts can be published.
- `auto_reload` polls in the background after startup. `last_good_cache_path` lets the gateway keep serving the last activated config when the store is temporarily unavailable.
- Activation failures mark the published version `failed`, restore the previous active row, and leave the runtime on the last-good routes.
- Managed API v1 lives under `/api/portal/v1/...`, including catalog aliases under `/api/portal/v1/catalog/...`; legacy `/api/portal/routes` and `/api/portal/groups` remain read-only catalog endpoints.

## Route Metadata
Route groups, routes, and websockets can expose catalog metadata:
- `owner_team`
- `domain`
- `visibility` with `public|partner|internal`
- `status` with `active|deprecated|experimental`
- `docs_url`
- `runbook_url`
- `support_channel`

Minimal example:
```yaml
routes:
  groups:
    default:
      metadata:
        owner_team: platform
        domain: gateway
        visibility: internal
        status: active
      routes:
        - path_prefix: /healthz
          target_url: http://localhost:8081
          metadata:
            owner_team: platform
            domain: status
            status: active
```

## Main Commands
```bash
nimbgtw config generate --output /tmp/apigateway.minimal.yaml
nimbgtw routes show --config-file config/config.example.yaml
nimbgtw routes validate --config-file config/config.example.yaml
nimbgtw routes compare --config-file config/staging.yaml --other-config-file config/production.yaml --fail-on-drift
nimbgtw openapi generate --config-file config/config.example.yaml --output config/openapi/openapi.yml
go test ./...
```

## Production Smoke
Use the targeted smoke script for `VT1.2` style checks:
```bash
PUBLIC_BASE_URL=https://api.example.com \
PUBLIC_ROUTE=/public/ping \
PROTECTED_ROUTE=/private/users \
PROTECTED_BEARER_TOKEN="$PROD_USER_TOKEN" \
MANAGEMENT_BASE_URL=https://mgmt.example.com \
MANAGEMENT_AUTH_ENABLED=true \
MANAGEMENT_ROUTE=/ready \
MANAGEMENT_BEARER_TOKEN="$PROD_MANAGEMENT_READ_TOKEN" \
make smoke-prod
```

If management auth is disabled, omit `MANAGEMENT_ROUTE` and `MANAGEMENT_BEARER_TOKEN`.

## Observability Checks
For rollout validation, inspect startup and runtime logs for:
- `registered route`
- `registered websocket`
- `registered developer portal on management server`
- `security request denied`

`security request denied` logs only request metadata (`surface`, `method`, `path`, `status`, `authorization_header_present`) and intentionally avoids logging bearer token values.

## Rollback Readiness
Before production promotion, record the previous known-good artifact and the previous known-good config revision as a pair.
Use [docs/rollback-checklist.md](docs/rollback-checklist.md) to confirm rollback can restore both together.

## Documentation
- [API Gateway Documentation](https://nimburion.github.io/documentation/apigateway/)
- [Overview](https://nimburion.github.io/documentation/apigateway/overview/)
- [Operations](https://nimburion.github.io/documentation/apigateway/operations/)
- [Routing and Policies](https://nimburion.github.io/documentation/apigateway/routing-and-policies/)

## Environment
Environment variables use the `APP_` prefix. Common examples:
- `APP_APP_NAME`
- `APP_AUTH_ISSUER`
- `APP_AUTH_JWKS_URL`
- `APP_AUTH_AUDIENCE`
- `APP_MGMT_AUTH_ENABLED`
- `APP_CORS_ALLOWED_ORIGINS`
