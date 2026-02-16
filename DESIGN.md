# API Gateway Design

Questo documento descrive i dettagli tecnici e di implementazione del gateway.

## Obiettivi tecnici
- Routing configurabile verso backend HTTP e WebSocket.
- Enforcement centralizzato di auth/scopes/rate limiting.
- Supporto OpenAPI sia in validazione configurazione sia a runtime.
- Portale sviluppatori embedded.
- Generazione automatica artefatti (`schema.json`, `openapi.yml`, `portal dist`).

## Struttura del progetto
- `cmd/main.go`: entrypoint CLI, wiring comandi e boot del server.
- `cmd/server/server.go`: bootstrap runtime del gateway.
- `cmd/routes/routes.go`: comandi `routes show|validate`.
- `cmd/openapi/openapi.go`: comando `openapi generate`.
- `internal/config/*`: modello config gateway + validazione/normalizzazione/merge route.
- `internal/routing/register.go`: registrazione route proxy e method-not-allowed handlers.
- `internal/middleware/*`: key function rate limit + validazione richieste OpenAPI.
- `internal/proxy/websocket.go`: proxy WebSocket (upgrade + forwarding).
- `internal/handlers/portal/*`: handler portale e API portale, static embedded.
- `internal/tools/configschema/main.go`: generatore `config/schema.json`.

## Bootstrap applicazione
Flusso in `cmd/main.go`:
1. Inizializza estensione config gateway (`Gateway`).
2. Costruisce `cli.ServiceCommandOptions` (nimburion).
3. Registra comandi custom:
   - `routes`
   - `openapi`
4. Esegue comando root nimburion.

Durante il bootstrap viene anche risolta `ConfigDir` per caricare route files relativi al file di config.

## Modello configurazione gateway
`internal/config/gateway.go`:
- `Gateway`:
  - `Routes` (`routes` inline)
  - `RoutesFiles` (`routes_files`)
  - `ConfigDir` (interno, non esposto)
- `Validate()`:
  - richiede almeno una fonte route (inline o files)
  - trim dei path in `routes_files`
- `LoadRoutes()`:
  - merge inline + files
  - supporta overlay group (uso tipico: secret OAuth2)
  - validazione semantica route
  - resolve path OpenAPI
  - validazione file OpenAPI
  - validazione allineamento OpenAPI vs route

## Config route e regole
`internal/config/routes.go` definisce:
- `Routing`, `Group`, `Route`, `Endpoint`, `Method`, `WebSocket`, `RateLimit`, `OpenAPI`.

Validazioni principali:
- gruppi consentiti: `default`, `central`
- `prefix`, `path_prefix`, `path` con slash iniziale
- `target_url` HTTP per route, WS/WSS per websocket
- metodi supportati: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`
- `rate_limit.requests_per_second` e `burst` > 0
- `openapi.file` obbligatorio quando `openapi` è impostato
- `openapi.mode` enum: `strict`, `warn-only`
- almeno una route o websocket per gruppo
- rilevazione duplicati path/method

Normalizzazioni principali:
- trim slash finali (tranne root)
- metodi HTTP uppercase
- path params da stile route (`:id`) a stile OpenAPI (`{id}`) nei confronti spec

## Pipeline runtime server
`cmd/server/server.go`:
1. Verifica config non nulla.
2. Richiede `auth.enabled=true`.
3. Costruisce validator JWT via JWKS.
4. Carica e valida route (`gwCfg.LoadRoutes`).
5. Costruisce server public/management via nimburion.
6. Crea route groups e applica middlewares di gruppo.
7. Registra endpoint auth (`/auth/me`, OAuth2 flow) quando configurati.
8. Registra proxy HTTP e WebSocket.
9. Registra portale su management.
10. Avvia server con signal handling nimburion.

## Routing HTTP
`internal/routing/register.go`:
- Per ogni route costruisce `httputil.NewSingleHostReverseProxy`.
- Applica eventuale `strip_prefix`.
- Compone middleware chain per metodo:
  - rate limit gruppo/route/metodo (priorità crescente)
  - `authz.RequireScopes` (se scopes presenti)
  - validazione OpenAPI request (se configurata)
- Registra handler 405 per metodi non consentiti con header `Allow`.

## Routing WebSocket
`internal/proxy/websocket.go`:
- Verifica/gestisce upgrade websocket.
- Connessione backend con dial TCP.
- Forward header upgrade + identity headers.
- Copia bidirezionale stream client/backend con cancel context.

## Middleware OpenAPI request validation
`nimburion/pkg/middleware/openapivalidation` (usato da `internal/routing/register.go`):
- Carica e valida spec da file (`ResolvedFile` o `File`).
- Costruisce router OpenAPI (`legacyrouter.NewRouter`).
- Per ogni request:
  - snapshot e restore body
  - clone request con rewrite strip prefix
  - `ValidateRequest`
- Modalità:
  - `strict`: errore -> risposta 4xx
  - `warn-only`: log warning e continua

## OpenAPI alignment (config-time)
In `LoadRoutes()`:
- Carica doc OpenAPI associato alla route.
- Estrae operazioni per prefisso route.
- Costruisce set operazioni da config (`METHOD + PATH`).
- Fallisce se:
  - spec manca operazioni configurate
  - spec ha operazioni extra non presenti nella route config

## Developer Portal
`internal/handlers/portal/portal.go`:
- Serve SPA React embedded con fallback a `index.html`.
- Espone API:
  - `GET /api/portal/routes`
  - `GET /api/portal/groups`
- Arricchisce route con metadata OpenAPI (titolo/versione/operazioni/errori di load).

Build embedding:
- `//go:generate` in `internal/handlers/portal/portal.go`:
  - build frontend (`developer-portal`)
  - copia `dist` in `internal/handlers/portal/dist`
- embedding con `//go:embed dist`.

## CLI commands
### `routes`
- `routes show`: stampa route finali mergeate (opzionalmente con secret).
- `routes validate`: valida merge + regole route + openapi alignment senza avvio server.

### `openapi generate`
`cmd/openapi/openapi.go`:
- Carica config (o default se config file assente).
- Carica route mergeate.
- Genera documento OpenAPI 3.0.3 aggregato:
  - management endpoints
  - auth endpoints per gruppo
  - route proxate + websocket
  - path params auto da `{param}`
- metadati:
  - `info.title`: `cfg.Service.Name` fallback `"API Gateway"`
  - `info.version`: `nimburion/pkg/version.AppVersion`
  - extension `x-generated-by`: service name fallback `"api-gateway"`
- crea automaticamente directory output.

## Schema configurazione (`config/schema.json`)
Generazione in `internal/tools/configschema/main.go` usando `nimburion/pkg/configschema`.

Caratteristiche:
- base schema nimburion + estensione gateway default.
- `$schema` centralizzato da nimburion.
- `$id` calcolato da module path (`go.mod`) + `/config/schema.json`.
- `schema.version` in `$comment` come hash SHA-256 del contenuto schema.
- rimozione campi interni (`config_dir`, campi runtime-only `group`, `resolved_file`).
- vincoli gateway:
  - `anyOf`/`not` su `routes` e `routes_files` (almeno una fonte non vuota)
  - enum `openapi.mode`
  - required + `minLength` per `openapi.file`
  - minimum per rate limits

Trigger:
- `//go:generate go run ../tools/configschema` in `internal/config/gateway.go`.
- `make schema` per generazione manuale.

## Build/generate targets
`Makefile`:
- `make generate`: esegue generazioni artefatti.
- `make portal`: build portale embedded.
- `make schema`: genera JSON schema config.
- `make openapi`: genera OpenAPI aggregato.

## Testing
Suite presenti in tutti i package principali:
- helper/path normalization
- config validation/normalization/merge
- openapi generation helpers
- middleware e proxy helpers
- handler principali
- tool config schema helpers

Coperture comportamentali aggiunte:
- `openapi generate` end-to-end (lettura config, creazione file output).
- `Gateway.LoadRoutes` su merge inline + `routes_files`.
- errore di OpenAPI alignment quando spec e route divergono.
- middleware OpenAPI runtime in `strict` vs `warn-only`.
- handler portale `GetGroups` e `GetRoutes` con risposta HTTP 200.

Dettaglio logica test: vedi `TEST.md`.

## Decisioni implementative
- Validazione route rimane in Go (deterministica, errori contestuali) e schema JSON è supporto per tooling.
- OpenAPI request validation è per-route, non globale.
- `warn-only` consente rollout graduale OpenAPI enforcement.
- Portale è embedded per evitare dipendenze runtime dal frontend build system.
