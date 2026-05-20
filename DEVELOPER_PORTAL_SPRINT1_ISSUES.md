# Developer Portal Sprint 1 Issues

Questo documento spezza lo Sprint 1 del backlog in issue implementabili e ordinabili in sequenza.

Riferimento principale: [DEVELOPER_PORTAL_BACKLOG.md](/Users/giuseppe/Workspace/github.com/nimburion/apigateway/DEVELOPER_PORTAL_BACKLOG.md)

## Obiettivo Dello Sprint 1

Introdurre il nuovo schema config e arricchire il catalogo read-only del portal senza:

- cambiare ancora il source of truth runtime verso database
- introdurre draft, publish, rollback o API admin
- rompere `routes validate`, `routes show`, `openapi generate`

## Sequenza Consigliata

1. DP-S1-01
2. DP-S1-02
3. DP-S1-03
4. DP-S1-04
5. DP-S1-05
6. DP-S1-06
7. DP-S1-07
8. DP-S1-08
9. DP-S1-09
10. DP-S1-10

## DP-S1-01 - Aggiungere Le Nuove Struct Di Config

Scope:

- introdurre le nuove struct senza attivare ancora alcuna logica database runtime

File:

- `internal/config/gateway.go`
- `internal/config/routes.go`

Modifiche:

- aggiungere `PortalConfig`
- aggiungere `PortalAuthConfig`
- aggiungere `PortalCatalogConfig`
- aggiungere `ConfigStoreConfig`
- aggiungere `ResourceMetadata`
- estendere `Gateway` con `Portal` e `ConfigStore`
- estendere `Group`, `Route` e `WebSocket` con `Metadata`
- definire enum costanti per:
  - `portal.mode`
  - `config_store.source_of_truth`
  - `config_store.backend`
  - `metadata.visibility`
  - `metadata.status`

Acceptance criteria:

- il codice compila
- nessun comportamento runtime cambia se le nuove sezioni non sono presenti in config
- i campi nuovi sono tutti opzionali salvo quelli che saranno vincolati in validazione

Dipendenze:

- nessuna

## DP-S1-02 - Validazione Config Nuova In `Gateway.Validate`

Scope:

- applicare le regole nuove senza toccare ancora `LoadRoutes` o il boot runtime

File:

- `internal/config/gateway.go`
- `internal/config/gateway_test.go`

Modifiche:

- aggiornare `Gateway.Validate()` con le regole di coerenza tra `portal`, `config_store`, `management` e `database`
- mantenere compatibile la regola attuale per `routes` o `routes_files` quando `source_of_truth=file`
- introdurre i test per:
  - default compatibili con configurazione attuale
  - `source_of_truth=file`
  - `source_of_truth=database`
  - `portal.mode=managed`
  - errori su combinazioni invalide

Acceptance criteria:

- i test nuovi coprono sia path validi sia invalidi
- una config legacy senza `portal` e `config_store` continua a validare
- una config `database` senza `database.type=postgres` fallisce con errore esplicito

Dipendenze:

- DP-S1-01

## DP-S1-03 - Validazione E Normalizzazione Metadata Di Routing

Scope:

- rendere i metadata usabili dal catalogo senza lasciare valori sporchi o enum incoerenti

File:

- `internal/config/routes.go`
- `internal/config/routes_test.go`

Modifiche:

- introdurre helper di normalizzazione `ResourceMetadata`
- trim dei campi stringa
- validazione enum `visibility` e `status`
- lasciare opzionali `docs_url`, `runbook_url`, `support_channel`
- decidere se validare `docs_url` e `runbook_url` come URL assoluti

Acceptance criteria:

- metadata vuoti restano ammessi
- metadata presenti vengono normalizzati
- enum invalidi falliscono con errori puntuali sul path configurazione

Dipendenze:

- DP-S1-01

## DP-S1-04 - Spostare L'Ownership Dei Vincoli Schema Nel Framework

Scope:

- eliminare il post-processing locale dello schema in `apigateway`
- far vivere i vincoli del gateway nel sistema schema/config del framework o nelle config extensions

File:

- `pkg/config/schema/schema.go` nel modulo `github.com/nimburion/nimburion`
- `pkg/config/schema/schema_test.go` nel modulo `github.com/nimburion/nimburion`
- `internal/config/gateway.go`
- `internal/config/routes.go`
- `internal/tools/configschema/main.go`
- `internal/tools/configschema/main_test.go`

Modifiche:

- introdurre nel framework un meccanismo esplicito per consentire alle config extensions di contribuire vincoli schema oltre alla sola shape strutturale
- spostare in quel meccanismo i vincoli oggi modellati o previsti in `apigateway`, in particolare:
  - enum di `portal.mode`
  - enum di `config_store.source_of_truth`
  - enum di `config_store.backend`
  - enum di `metadata.visibility`
  - enum di `metadata.status`
  - minimi o required sui campi di timing e path quando applicabili
- valutare se i vincoli oggi presenti nel tool locale di `apigateway` debbano diventare:
  - metadata dichiarativi sulle struct/config extensions
  - oppure hook schema nel package `pkg/config/schema`
- ridurre `internal/tools/configschema/main.go` a solo bootstrap temporaneo o rimuoverlo del tutto se non serve piu
- aggiungere test nel framework per verificare che i vincoli arrivino nel JSON Schema senza post-processing locale
- aggiornare i test di `apigateway` per verificare che il builder framework produca lo schema corretto direttamente

Acceptance criteria:

- `apigateway` non aggiunge piu enum, required o minimi con post-processing locale
- il JSON Schema finale contiene i vincoli del gateway gia in uscita da `pkg/config/schema`
- `internal/tools/configschema/main.go` non contiene piu logica di mutazione schema gateway-specific
- i test sul framework e sul repo gateway falliscono se i vincoli spariscono

Dipendenze:

- DP-S1-01
- DP-S1-02
- DP-S1-03

## DP-S1-05 - Rigenerare Gli Artifact Di Config

Scope:

- aggiornare gli artifact generati e l'esempio di configurazione

File:

- `config/schema.json`
- `config/config.example.yaml`

Modifiche:

- rigenerare `config/schema.json` usando il builder framework senza post-processing gateway-specific
- aggiornare `config/config.example.yaml` con:
  - sezione `portal`
  - sezione `config_store`
  - esempio metadata su group e route
- mantenere l'esempio in modalita `source_of_truth=file` per non introdurre dipendenze esterne nel sample base

Acceptance criteria:

- `config/schema.json` riflette lo schema aggiornato
- `config/config.example.yaml` resta utilizzabile come config locale minima

Dipendenze:

- DP-S1-04

## DP-S1-06 - Documentare La Nuova Superficie Config

Scope:

- allineare la documentazione principale al nuovo modello

File:

- `README.md`
- opzionale: `internal/handlers/portal/README.md`

Modifiche:

- documentare `portal.mode`
- documentare `config_store.source_of_truth`
- spiegare che in Sprint 1 il backend database e solo predisposto a livello schema/config
- aggiungere un esempio minimo di metadata

Acceptance criteria:

- README coerente con `config/config.example.yaml`
- nessun riferimento ambiguo a funzionalita non ancora implementate in Sprint 1

Dipendenze:

- DP-S1-05

## DP-S1-07 - Arricchire Il Read Model Backend Del Portal

Scope:

- migliorare gli endpoint read-only esistenti senza introdurre nuove API admin

File:

- `internal/handlers/portal/portal.go`
- `internal/routing/middleware_resolution.go`

Modifiche:

- estendere i DTO JSON restituiti da `GetRoutes` e `GetGroups`
- includere metadata di group, route e websocket
- esporre middleware effettivi risolti per gruppo e route usando `ApplyMiddlewareDirectives`
- aggiungere flag utili al catalogo:
  - `auth_required`
  - `has_openapi`
  - `has_rate_limit`
  - `deprecated`
- evitare di esporre sempre `target_url` se `portal.catalog.expose_target_urls=false`
- evitare di esporre dettagli OpenAPI sensibili se `portal.catalog.expose_openapi_errors=false`

Nota tecnica:

- `NewPortalHandler` dovra probabilmente ricevere anche `PortalConfig`, non solo `Routing`

Acceptance criteria:

- gli endpoint legacy continuano a rispondere
- il payload contiene abbastanza contesto per una UI catalogo utile
- la logica di visibilita dei campi e guidata da config, non hardcoded

Dipendenze:

- DP-S1-01
- DP-S1-03

## DP-S1-08 - Copertura Test Del Read Model Portal

Scope:

- bloccare regressioni sui nuovi campi esposti dal backend

File:

- `internal/handlers/portal/portal_test.go`
- opzionale: `cmd/server/server_test.go`

Modifiche:

- aggiungere fixture con metadata, OpenAPI e middlewares
- verificare serializzazione dei nuovi campi
- verificare rispetto delle opzioni `expose_target_urls` e `expose_openapi_errors`
- verificare che la route `/portal` continui a servire la SPA come prima

Acceptance criteria:

- test coverage sui payload principali
- test espliciti per i due toggle di catalog visibility

Dipendenze:

- DP-S1-07

## DP-S1-09 - Allineare I Contratti Frontend

Scope:

- aggiornare i tipi TS senza ancora fare redesign sostanziale

File:

- `developer-portal/src/types.ts`
- opzionale: `developer-portal/src/App.tsx`

Modifiche:

- estendere i tipi con metadata e flag runtime nuovi
- adattare il parsing difensivo in `App.tsx`
- preservare la compatibilita con campi eventualmente assenti

Acceptance criteria:

- build TypeScript pulita
- il frontend tollera sia payload vecchi sia payload arricchiti

Dipendenze:

- DP-S1-07

## DP-S1-10 - Rendere Visibili Metadata E Stato Nella UI Esistente

Scope:

- far diventare il catalogo immediatamente piu utile senza introdurre ancora l'area admin

File:

- `developer-portal/src/components/GroupCard.tsx`
- `developer-portal/src/components/RoutesList.tsx`
- opzionale: `developer-portal/src/components/SearchBar.tsx`
- opzionale: `developer-portal/src/index.css`

Modifiche:

- mostrare `owner_team`, `domain`, `visibility`, `status`
- mostrare se una route richiede auth
- mostrare middleware effettivi
- mostrare se OpenAPI e rate limit sono presenti
- aggiungere filtri base almeno per:
  - team
  - domain
  - visibility
  - status
- mantenere il layout attuale semplice e leggibile

Acceptance criteria:

- la UI continua a funzionare embedded sotto `/portal`
- un developer riesce a capire owner, visibilita e requisiti auth senza aprire il YAML

Dipendenze:

- DP-S1-09

## Comandi Di Verifica Sprint 1

Da eseguire alla fine dello sprint:

```bash
go test ./...
nimbgtw routes validate --config-file config/config.example.yaml
nimbgtw routes show --config-file config/config.example.yaml
nimbgtw openapi generate --config-file config/config.example.yaml --output /tmp/openapi.yml
```

Se si tocca il frontend:

```bash
cd developer-portal
npm run build
```

## Out Of Scope Esplicito

- `internal/configstore`
- nuove API `/api/portal/v1/config/*`
- publish job
- rollback
- activation runtime da database
- hot reload
- locking cross-instance
