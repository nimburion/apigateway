# Developer Portal Backlog

## Obiettivo

Portare il developer portal da vista read-only della config caricata a boot a control plane leggero per:

- sviluppatori che devono capire rapidamente route, auth, OpenAPI, ownership e stato
- operatori che devono creare draft, validare, pubblicare, fare rollback e audit
- runtime che deve poter leggere la configurazione da backend stateful senza legarsi ad AWS

Dettaglio operativo Sprint 1:

- [DEVELOPER_PORTAL_SPRINT1_ISSUES.md](/Users/giuseppe/Workspace/github.com/nimburion/apigateway/DEVELOPER_PORTAL_SPRINT1_ISSUES.md)

Scelta architetturale iniziale:

- backend stateful primario: PostgreSQL gestito in HA
- design del config store astratto, non vendor-specific
- endpoint read-only attuali mantenuti per compatibilita durante la migrazione

## Stato Attuale Del Repo

- Il portal e una SPA React embedded nel binario Go: `developer-portal/` e `internal/handlers/portal/portal.go`
- Il backend del portal espone solo:
  - `GET /api/portal/routes`
  - `GET /api/portal/groups`
- La UI legge i dati una volta sola al caricamento: `developer-portal/src/App.tsx`
- La config gateway e file/env/flags based: `cmd/main.go`, `internal/config/gateway.go`, `cmd/routes/routes.go`
- Lo schema viene generato da `internal/tools/configschema/main.go`

## Architettura Target

### Principi

- Il source of truth della routing config deve poter essere `file` o `database`
- Il runtime deve usare solo configurazioni validate e versionate
- Ogni publish deve essere auditabile e rollbackabile
- Il portal developer-facing e quello admin condividono catalogo e read model, ma non permessi
- Le API nuove devono essere versionate (`/api/portal/v1/...`)

### Componenti Da Introdurre

- `internal/config`:
  - estensione della config gateway con `portal` e `config_store`
  - metadata utili al catalogo developer-facing
- `internal/configstore`:
  - contratto di persistenza per draft, versioni, publish e audit
  - implementazione `memory` per test
  - implementazione `postgres` per produzione
- `internal/runtimeconfig`:
  - loader della active config
  - validazione prima dell'attivazione
  - cache last-good
  - reload polling-based
- `internal/handlers/portaladmin`:
  - API admin per draft, validate, publish, rollback, version history e audit
- `developer-portal/src/admin`:
  - UI admin per editing, diff, validate, publish e rollback

## Nuovo Schema Config Proposto

### Modifiche Alla Struct `Gateway`

Estendere `internal/config/gateway.go` e `internal/config/routes.go` con:

- `portal`
- `config_store`
- `metadata` su group, route e websocket

### YAML Proposto

```yaml
app:
  name: api-gateway

management:
  enabled: true
  port: 9001
  auth_enabled: true

portal:
  enabled: true
  mode: managed
  catalog:
    expose_target_urls: false
    expose_openapi_errors: true
  auth:
    read_scopes:
      - management:portal:read
    write_scopes:
      - management:config:write
    publish_scopes:
      - management:config:publish
    rollback_scopes:
      - management:config:rollback

config_store:
  enabled: true
  source_of_truth: database
  backend: postgres
  bootstrap_from_file: true
  auto_reload: true
  poll_interval: 10s
  activation_timeout: 5s
  last_good_cache_path: /var/lib/apigateway/last-good-routes.json
  require_validation_before_publish: true
  require_base_version_match: true

database:
  type: postgres
  url: ${APP_DB_URL}

routes:
  groups:
    default:
      prefix: /api
      metadata:
        owner_team: platform
        domain: gateway
        visibility: internal
        docs_url: https://docs.example.com/gateway
        support_channel: "#api-platform"
      routes:
        - path_prefix: /users
          target_url: http://users:8080
          metadata:
            owner_team: iam
            domain: identity
            status: active
            runbook_url: https://runbooks.example.com/users-api
          endpoints:
            - path: /
              methods:
                GET: {}
```

### Nuove Struct Proposte

Da aggiungere in `internal/config/`:

- `type PortalConfig struct`
- `type PortalAuthConfig struct`
- `type PortalCatalogConfig struct`
- `type ConfigStoreConfig struct`
- `type ResourceMetadata struct`

Campi minimi:

- `portal.enabled bool`
- `portal.mode string` con enum `read-only|managed`
- `portal.auth.read_scopes []string`
- `portal.auth.write_scopes []string`
- `portal.auth.publish_scopes []string`
- `portal.auth.rollback_scopes []string`
- `config_store.enabled bool`
- `config_store.source_of_truth string` con enum `file|database`
- `config_store.backend string` con enum `postgres`
- `config_store.bootstrap_from_file bool`
- `config_store.auto_reload bool`
- `config_store.poll_interval time.Duration`
- `config_store.activation_timeout time.Duration`
- `config_store.last_good_cache_path string`
- `config_store.require_validation_before_publish bool`
- `config_store.require_base_version_match bool`

Metadata minimi:

- `owner_team`
- `domain`
- `visibility` con enum `public|partner|internal`
- `status` con enum `active|deprecated|experimental`
- `docs_url`
- `runbook_url`
- `support_channel`

### Regole Di Validazione

Da implementare in `Gateway.Validate()` e testare in `internal/config/*_test.go`:

- se `config_store.source_of_truth=file`, deve restare valida la regola attuale `routes` o `routes_files`
- se `config_store.source_of_truth=database`, `config_store.enabled` deve essere `true`
- se `config_store.source_of_truth=database`, `database.type` deve essere `postgres`
- se `portal.mode=managed`, `management.enabled` deve essere `true`
- se `portal.mode=managed`, `management.auth_enabled` deve essere `true`
- se `portal.mode=managed`, `portal.auth.write_scopes`, `publish_scopes` e `rollback_scopes` non devono essere vuoti
- `poll_interval` e `activation_timeout` devono essere `> 0`
- `last_good_cache_path` obbligatorio quando `auto_reload=true`

### File Da Toccare Per Lo Schema

- `internal/config/gateway.go`
- `internal/config/routes.go`
- `internal/tools/configschema/main.go`
- `config/schema.json`
- `config/config.example.yaml`
- `README.md`

## Modello Dati Del Config Store

Prima implementazione: PostgreSQL con snapshot immutabili in `jsonb`.

### Tabelle

- `portal_config_versions`
  - `id uuid primary key`
  - `kind text not null` con valori `draft|version`
  - `status text not null` con valori `draft|validated|active|superseded|failed`
  - `base_version_id uuid null`
  - `config_json jsonb not null`
  - `config_hash text not null`
  - `validation_report_json jsonb null`
  - `created_by_sub text not null`
  - `created_by_email text null`
  - `change_note text null`
  - `created_at timestamptz not null`
  - `published_at timestamptz null`

- `portal_config_pointers`
  - `pointer_key text primary key`
  - `version_id uuid not null`
  - `updated_by_sub text not null`
  - `updated_at timestamptz not null`

- `portal_publish_jobs`
  - `id uuid primary key`
  - `draft_id uuid not null`
  - `target_version_id uuid null`
  - `status text not null` con valori `pending|running|succeeded|failed`
  - `error_message text null`
  - `started_at timestamptz null`
  - `finished_at timestamptz null`

- `portal_audit_events`
  - `id uuid primary key`
  - `event_type text not null`
  - `entity_type text not null`
  - `entity_id text not null`
  - `actor_sub text not null`
  - `actor_email text null`
  - `payload_json jsonb not null`
  - `created_at timestamptz not null`

Nota:

- draft e versioni condividono la stessa tabella per semplificare diff, optimistic concurrency e rollback

## API Da Aggiungere

### Compatibilita

Mantenere:

- `GET /api/portal/routes`
- `GET /api/portal/groups`

Nuova superficie versionata:

- `GET /api/portal/v1/catalog/groups`
- `GET /api/portal/v1/catalog/routes`
- `GET /api/portal/v1/catalog/summary`
- `GET /api/portal/v1/config/active`
- `GET /api/portal/v1/config/versions`
- `GET /api/portal/v1/config/versions/:id`
- `GET /api/portal/v1/config/drafts`
- `POST /api/portal/v1/config/drafts`
- `GET /api/portal/v1/config/drafts/:id`
- `PUT /api/portal/v1/config/drafts/:id`
- `POST /api/portal/v1/config/drafts/:id/validate`
- `POST /api/portal/v1/config/drafts/:id/publish`
- `POST /api/portal/v1/config/versions/:id/rollback`
- `GET /api/portal/v1/publish-jobs/:id`
- `GET /api/portal/v1/audit-events`

### Contratti Minimi

`POST /api/portal/v1/config/drafts`

```json
{
  "base_version_id": "uuid",
  "change_note": "add users route",
  "config": {
    "groups": {}
  }
}
```

`POST /api/portal/v1/config/drafts/:id/validate`

```json
{
  "valid": true,
  "errors": [],
  "warnings": [],
  "normalized_config": {
    "groups": {}
  }
}
```

`POST /api/portal/v1/config/drafts/:id/publish`

```json
{
  "job_id": "uuid",
  "status": "pending"
}
```

`GET /api/portal/v1/config/active`

```json
{
  "version_id": "uuid",
  "config_hash": "sha256",
  "published_at": "2026-03-08T12:00:00Z",
  "config": {
    "groups": {}
  }
}
```

### Scopes

Applicare in `cmd/server/server.go` middleware separati:

- read catalog: `management:portal:read`
- create/edit draft: `management:config:write`
- publish: `management:config:publish`
- rollback: `management:config:rollback`

## CLI Da Aggiungere

Nuovo command group consigliato: `portal-config`

- `portal-config export --source file|database --output <path>`
- `portal-config import --input <path> --as-draft`
- `portal-config validate --input <path>`
- `portal-config publish --draft-id <id>`
- `portal-config rollback --version-id <id>`

File da toccare:

- `cmd/main.go`
- nuovo package `cmd/portalconfig`

## Backlog Per Sprint

## Sprint 1 - Fondazioni E Schema

Obiettivo:

- introdurre il nuovo schema config senza rompere il runtime file-based

Task:

- estendere `internal/config/gateway.go` con `PortalConfig` e `ConfigStoreConfig`
- aggiungere `ResourceMetadata` in `internal/config/routes.go`
- aggiornare `Gateway.Validate()` per supportare `source_of_truth=file|database`
- spostare i vincoli schema gateway-specific nel sistema schema/config del framework invece di post-processare lo schema in `apigateway`
- rigenerare `config/schema.json`
- aggiornare `config/config.example.yaml`
- aggiornare `internal/handlers/portal/portal.go` e `developer-portal/src/types.ts` per esporre metadata e informazioni runtime effettive
- mantenere gli endpoint attuali compatibili

Test:

- `internal/config/gateway_test.go`
- `internal/config/routes_test.go`
- `internal/handlers/portal/portal_test.go`

Definition of done:

- la config valida sia in modalita file che database
- il catalogo mostra metadata e non solo route/scopes
- `apigateway` non muta piu il JSON Schema con logica locale gateway-specific
- nessuna regressione su `routes validate`, `routes show`, `openapi generate`

## Sprint 2 - Config Store E API Admin Backend

Obiettivo:

- introdurre backend stateful e CRUD dei draft senza ancora fare activation live

Task:

- creare `internal/configstore/contract.go`
- creare `internal/configstore/memory_store.go`
- creare `internal/configstore/postgres_store.go`
- creare SQL migrations in nuovo package `internal/configstore/migrations`
- aggiungere `internal/handlers/portaladmin`
- aggiungere gli endpoint `/api/portal/v1/config/*`, `/api/portal/v1/publish-jobs/*`, `/api/portal/v1/audit-events`
- validare i draft usando la stessa pipeline di `gwCfg.LoadRoutes(...)`
- registrare le nuove route in `cmd/server/server.go`

Test:

- `internal/configstore/*_test.go`
- `internal/handlers/portaladmin/*_test.go`
- `cmd/server/server_test.go`

Definition of done:

- un admin puo creare un draft, aggiornarlo, validarlo e vedere la lista versioni
- il source of truth runtime puo restare `file` mentre il database accumula draft/versioni

## Sprint 3 - Activation Runtime E HA Safety

Obiettivo:

- permettere al gateway di usare la active version dal database in modo sicuro su piu istanze

Task:

- creare `internal/runtimeconfig/manager.go`
- creare `internal/runtimeconfig/loader.go`
- creare `internal/runtimeconfig/cache.go`
- all'avvio, se `source_of_truth=database`, caricare la active config dal config store
- aggiungere polling di `portal_config_pointers.active`
- introdurre last-good snapshot locale
- introdurre optimistic concurrency sul publish
- usare `pkg/coordination/postgres` per serializzare publish e rollback cross-instance
- aggiungere metriche e log strutturati per reload, failure e rollback

Test:

- `internal/runtimeconfig/*_test.go`
- `cmd/server/server_test.go`
- test end-to-end con 2 istanze contro lo stesso Postgres

Definition of done:

- due istanze leggono la stessa active version
- publish valido attiva la nuova config senza restart manuale
- publish invalido o reload fallito non interrompe il traffico e mantiene la last-good

## Sprint 4 - UI Admin E Workflow Operativo

Obiettivo:

- rendere il portal utile anche a chi gestisce

Task:

- aggiungere area admin in `developer-portal/src/`
- introdurre schermate:
  - active version
  - drafts list
  - editor JSON/YAML con validazione
  - diff tra draft e active
  - publish dialog con change note
  - rollback dialog
  - audit log
- introdurre badge chiari per auth, scopes, OpenAPI mode, metadata, status e deprecation
- introdurre filtri per team, domain, visibility, status

Test:

- test componenti principali React
- smoke test manuale su portal embedded

Definition of done:

- un operatore puo fare draft, validate, publish e rollback dalla UI
- uno sviluppatore puo filtrare il catalogo per owner/domain/visibility e capire rapidamente cosa usare

## Sprint 5 - Hardening E OpenAPI Portabile

Obiettivo:

- eliminare le dipendenze residue dal filesystem locale e chiudere i gap operativi

Task:

- estendere `OpenAPI` con sorgente esplicita:
  - `source_type=file|artifact`
  - `artifact_ref`
- introdurre persistenza degli artefatti OpenAPI nel config store o in object storage astratto
- aggiornare validazione OpenAPI per leggere `file` o `artifact`
- aggiungere export/import completo di config + OpenAPI
- aggiungere rate limit e pagination per audit/version history
- deprecare gradualmente gli endpoint legacy `/api/portal/routes` e `/api/portal/groups`

Test:

- `internal/config/gateway_test.go`
- `internal/handlers/portal/portal_test.go`
- `cmd/openapi/openapi_test.go`

Definition of done:

- il source of truth database non dipende piu da path locali non condivisi
- il portal e operativo anche in deploy multi-instance senza shared filesystem

## Ordine Di Implementazione Consigliato

1. Sprint 1
2. Sprint 2
3. Sprint 3
4. Sprint 4
5. Sprint 5

Motivo:

- prima si rende il modello configurabile e compatibile
- poi si aggiunge il backend stateful
- poi si collega il runtime alla active version
- infine si mette sopra la UI operativa

## Scelte Esplicite

- non AWS-only
- PostgreSQL come prima implementazione stateful perche e cross-cloud e adatto a versioning, diff, audit e locking
- interfaccia `configstore` astratta per poter aggiungere altri backend dopo il primo rilascio
- endpoints nuovi versionati sotto `/api/portal/v1`
- nessun hot reload senza last-good e safety rails
