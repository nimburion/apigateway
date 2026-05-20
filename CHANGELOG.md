# Changelog

## v0.1.0

- Added GA managed routing config with PostgreSQL-backed versions, drafts, validation, publish, rollback, last-good cache, and audit events.
- Added versioned admin API under `/api/portal/v1/...`; legacy `/api/portal/routes` and `/api/portal/groups` remain read-only catalog endpoints.
- Added Posture Portal Config Admin UI for active config, draft creation, validation, publish, rollback, version history, and audit review.
- Made `make build VERSION=v0.1.0` stamp `nimbgtw version` with app version, git commit, and build time.
- Removed local `github.com/nimburion/nimburion` module replacement so release builds use the tagged dependency.
- Added `scripts/release-check.sh` for generate, test, portal build, binary build, route validation, OpenAPI generation, and version verification.
