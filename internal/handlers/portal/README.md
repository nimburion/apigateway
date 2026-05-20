# Posture Portal Handler

## Overview

Questo handler serve il Posture Portal React come SPA embedded nel binario Go.

## Come funziona

- Il file `portal.go` usa la direttiva `//go:embed dist` per includere tutti i file della cartella `dist/` nel binario compilato
- La funzione `GetPortalHTML` serve i file statici dal filesystem embedded
- Le API legacy read-only `/api/portal/routes`, `/api/portal/groups` e `/api/portal/openapi.yaml` forniscono catalogo e spec runtime
- In `portal.mode=managed`, le API admin versionate `/api/portal/v1/...` gestiscono draft, validate, publish, rollback, history e audit
- Gli asset statici sono serviti con route compatibili con il router `nethttp`, che non supporta wildcard tipo `*path`
- Il catalogo del portal usa due sorgenti:
  - config routes come source of truth principale
  - runtime routes annotate via `internal/portalmeta` per le surface registrate fuori config

## Registry Pattern

Le route registrate direttamente sul router non espongono abbastanza semantica per il catalogo solo tramite introspezione runtime. Per questo il portal adotta un registry esplicito:

- `httpopenapi.Annotate(...)` per metadata OpenAPI
- `portalmeta.Annotate(...)` per metadata catalogo/governance

Esempio:

```go
handler := portalmeta.Annotate(
    httpopenapi.Annotate(myHandler, httpopenapi.EndpointAnnotations{
        Summary: "Authenticated user info",
        Tags:    []string{"auth"},
    }),
    portalmeta.Metadata{
        Resource: gatewaycfg.ResourceMetadata{
            OwnerTeam:      "platform",
            Domain:         "auth",
            Visibility:     gatewaycfg.MetadataVisibilityInternal,
            Status:         gatewaycfg.MetadataStatusActive,
            DocsURL:        "https://docs.example.com/auth",
            RunbookURL:     "https://runbooks.example.com/auth",
            SupportChannel: "#api-platform",
        },
        AuthRequired: false,
        Scopes:       nil,
        HasRateLimit: false,
    },
)
```

Il portal mostra queste surface come `runtime_only` quando non esistono nel file config, ma può comunque esporre metadata catalogo significativi grazie al registry.

Per gli endpoint built-in auth esistono helper dedicati in `internal/portalmeta/auth.go`.

## Route Configuration

Nel file `cmd/server/server.go` il handler va inizializzato e le route configurate così:

```go
portalHandler, err := portal.NewPortalHandler(&routeDefs, &gwCfg.Portal, &portal.RuntimeInfo{
    AuthEnabled:           cfg.Auth.Enabled,
    ManagementEnabled:     cfg.Management.Enabled,
    ManagementAuthEnabled: cfg.Management.AuthEnabled,
    PortalMode:            gwCfg.Portal.Mode,
})
if err != nil {
    return fmt.Errorf("initialize developer portal handler: %w", err)
}
mgmtRouter.GET("/portal", portalHandler.GetPortalHTML, portalSecurity...)
mgmtRouter.GET("/portal/", portalHandler.GetPortalHTML, portalSecurity...)
mgmtRouter.GET("/portal/assets/:file", portalHandler.GetPortalHTML, portalSecurity...)
mgmtRouter.GET("/portal/:file", portalHandler.GetPortalHTML, portalSecurity...)
mgmtRouter.GET("/api/portal/routes", portalHandler.GetRoutes, portalSecurity...)
mgmtRouter.GET("/api/portal/groups", portalHandler.GetGroups, portalSecurity...)
mgmtRouter.GET("/api/portal/openapi.yaml", runtimeOpenAPIHandler, portalSecurity...)
```

Il gateway usa il layer di autenticazione del server di management (`management.auth_enabled: true`). Le API catalog legacy usano lo scope di lettura del portale; le API v1 managed usano `portal.auth.read_scopes`, `write_scopes`, `publish_scopes` e `rollback_scopes` in base all'operazione. Il middleware di autenticazione e autorizzazione viene riutilizzato con gli stessi validator di `/ready` e `/metrics`.

Nota:
- con l'adapter `nethttp` il path matcher supporta segmenti `:param`, non wildcard `*path`
- per questo gli asset del portal vengono registrati come `/portal/assets/:file`
- il frontend usa deep link condivisibili lato browser per gruppo e posture (`/portal/groups/:group`, `/portal/posture/:group`, `/portal/posture/:group/route/:method/:path`, `/portal/posture/:group/websocket/:path`), ma il bootstrap SPA continua a passare da `/portal` e dagli asset top-level, quindi questo mapping resta sufficiente

## Struttura Files

```
portal/
├── dist/                    # Build del React app (embedded)
│   ├── index.html
│   └── assets/
│       ├── index-[hash].js
│       └── index-[hash].css
└── portal.go               # Handler Go
```

## Endpoints

- `GET /portal` - Posture Portal UI (React SPA)
- `GET /portal/` - Alias della root UI
- `GET /portal/assets/:file` - Assets statici del frontend
- `GET /portal/:file` - File top-level del frontend
- `GET /api/portal/routes` - Lista di tutte le route configurate
- `GET /api/portal/groups` - Lista dei gruppi di route
- `GET /api/portal/v1/catalog/routes` - Alias v1 della lista route
- `GET /api/portal/v1/catalog/groups` - Alias v1 della lista gruppi
- `GET /api/portal/v1/catalog/summary` - Conteggi catalogo e runtime info
- `GET /api/portal/openapi.yaml` - Spec OpenAPI generata a runtime dalle route registrate dal gateway
- `GET /api/portal/v1/config/active` - Config routing attiva
- `GET /api/portal/v1/config/versions` - Storico versioni
- `GET /api/portal/v1/config/drafts` - Draft aperti o validati
- `POST /api/portal/v1/config/drafts` - Crea draft
- `PUT /api/portal/v1/config/drafts/:version` - Aggiorna draft
- `POST /api/portal/v1/config/drafts/:version/validate` - Valida draft
- `POST /api/portal/v1/config/drafts/:version/publish` - Pubblica draft validato
- `POST /api/portal/v1/config/versions/:version/rollback` - Rollback verso una versione precedente
- `GET /api/portal/v1/audit-events` - Audit trail delle operazioni managed
- `GET /metrics` - Metriche Prometheus del management server, lette direttamente dal frontend del portal senza endpoint duplicato

## Deep Links

Il frontend supporta URL condivisibili per:
- gruppo: `/portal/groups/:group`
- postura gruppo: `/portal/posture/:group`
- postura route HTTP: `/portal/posture/:group/route/:method/:path`
- postura websocket: `/portal/posture/:group/websocket/:path`

La pagina posture mostra breadcrumb visibile e azioni `Copy link` per il gruppo corrente e per la surface selezionata.

### Response Example (OpenAPI)

```json
{
  "groups": [
    {
      "name": "default",
      "prefix": "/",
      "routes": [
        {
          "path_prefix": "/api/v1/payments",
          "target_url": "http://payments-service:8080",
          "methods": [
            {
              "method": "GET",
              "scopes": ["payment:read"]
            }
          ],
          "openapi": {
            "file": "./config/openapi/payments.yaml",
            "mode": "strict",
            "title": "Payments API",
            "version": "1.0.0",
            "operations": [
              {
                "path": "/api/v1/payments",
                "method": "GET",
                "summary": "List payments",
                "operation_id": "listPayments"
              }
            ]
          }
        }
      ],
      "websockets": []
    }
  ]
}
```

## Build

Per aggiornare il portal:

1. Build del React app:
```bash
cd developer-portal
npm run build
```

2. Copia i file:
```bash
cp -r developer-portal/dist microservices/api-gateway/internal/handlers/portal/
```

3. Rebuild del Gateway:
```bash
cd microservices/api-gateway
go build
```

Oppure usa il Makefile:
```bash
make build-all
```

## Cache Headers

- Assets (JS, CSS, immagini): `Cache-Control: public, max-age=31536000, immutable`
- index.html: `Cache-Control: no-cache, no-store, must-revalidate`

## Troubleshooting

### 404 su assets

Se ottieni 404 su `/portal/assets/*`:
- Verifica che siano registrate le route `/portal/assets/:file` e `/portal/:file`
- Verifica che il bundle referenziato in `dist/index.html` esista davvero in `dist/assets/`
- Rebuilda il Gateway dopo aver modificato le route

### Portal files not found

Se vedi questo errore:
- La cartella `dist/` non esiste o è vuota
- Esegui il build del developer-portal e copia i file
