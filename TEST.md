# Test Logic Overview

Questo documento descrive la logica coperta dai test presenti nel progetto.

## `cmd/main_test.go`
- Verifica la costruzione del comando CLI principale.
- Controlla che l'inizializzazione con comandi custom (`routes`, `openapi`) produca un comando non nullo.

## `cmd/openapi/openapi_test.go`
- `normalizeOpenAPIPath`: converte path con `:param` o `*param` in formato OpenAPI (`{param}`), gestisce slash finali e input vuoti.
- `pathParameters`: estrae i parametri presenti nei path OpenAPI.
- `joinPaths`: unisce segmenti di path in modo consistente.
- `buildSpec`: verifica la generazione spec con titolo servizio, servers, route proxate, websocket e route management.
- `sortPaths`: verifica ordinamento deterministico dei path nella spec.
- `openapi generate` end-to-end: esegue il comando CLI, genera file YAML e verifica contenuto base della spec.

## `cmd/routes/routes_test.go`
- `DetermineRoutesBaseDir`: risolve correttamente la directory base a partire da un config file.
- `redactedRoutes`: maschera i secret OAuth2 (`ClientSecret`) in output.
- `flagValue`: legge il valore da flag o usa fallback quando assente.

## `cmd/server/server_test.go`
- `serviceName`: usa fallback `api-gateway` quando non configurato e usa `cfg.Service.Name` quando presente.
- `managementSecurityMiddleware`: ritorna `nil` se auth management è disabilitata; costruisce chain attesa quando abilitata.

## `internal/authn/oauth2_test.go`
- Copre il flusso OAuth2 del gateway (registrazione endpoint e comportamento correlato).
- Verifica casi validi e di errore legati a configurazione/gestione autenticazione OAuth2.

## `internal/config/gateway_test.go`
- `Gateway.Validate`: fallisce se mancano sia `routes_files` sia routes inline; normalizza/trima i path.
- `mergeOverlayGroup`: valida il merge tra gruppo base e overlay (es. secret OAuth2).
- `normalizeOpenAPIPath`: normalizzazione path in formato OpenAPI.
- `diffOperations`: calcolo differenze tra operazioni attese e reali.
- `LoadRoutes` merge: unisce route inline e route da file.
- `LoadRoutes` alignment: errore quando OpenAPI non è allineata alle route configurate.

## `internal/config/routes_test.go`
- `validateOpenAPI`: richiede `openapi.file`, applica default mode `strict`, rifiuta mode non validi.
- `ResolvePathWithBaseDir`: risolve path relativi con base dir.
- `validateAndNormalize`: valida struttura routes e normalizza i metodi HTTP (es. `get` -> `GET`).

## `internal/handlers/health_test.go`
- Verifica che `HealthHandler` ritorni `200` con payload JSON atteso (`status`, `service`).

## `internal/handlers/auth/me_test.go`
- Caso non autenticato: ritorna `401`.
- Caso autenticato: ritorna `200` e include i campi claims principali (es. subject).

## `internal/handlers/portal/portal_test.go`
- `NewPortalHandler`: errore su config nil.
- `collectOpenAPIOperations`: estrae operazioni da documento OpenAPI.
- `filterOpenAPIInfo`: filtra operazioni OpenAPI per path normalizzato.
- Test helper path (`normalizeOpenAPIPath`, `joinRoutePath`).
- Verifica inizializzazione handler con route config valida.
- `GetGroups` e `GetRoutes`: verifica risposta HTTP `200` sugli endpoint API del portale.

## `internal/middleware/main_test.go`
- `RateLimitKeyByTenantAndSubject`: usa fallback IP se claims assenti; usa `tenant:subject` quando claims presenti.

## Validazione OpenAPI runtime
- Implementazione condivisa in `nimburion/pkg/middleware/openapivalidation`.
- Nel gateway è agganciata per-route da `internal/routing/register.go`.
- Copertura test della libreria validatore risiede in `nimburion`.

## `internal/proxy/websocket_test.go`
- `isWebSocketRequest`: riconosce correttamente richieste di upgrade websocket valide e invalide.

## `internal/routing/register_test.go`
- `buildAllowHeaderValue`: costruisce header `Allow` con metodi consentiti + `OPTIONS`.
- `registerMethodNotAllowedRoutes`: registra handler 405 per metodi non consentiti.
- `joinRoutePath`: unione corretta di prefix/suffix per route.

## `internal/tools/configschema/main_test.go`
- `schemaVersionHash`: hash schema stabile a parità di input.
- `removeProperty`: rimuove proprietà e pulisce `required`/`propertyOrder`.
- `addRoutingConstraints`: aggiunge vincoli `anyOf`/`not` per routes.
- `applyGatewaySchemaConstraints`: applica enum e vincoli minimi su `openapi` e `rate_limit`.
