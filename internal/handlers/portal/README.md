# Developer Portal Handler

## Overview

Questo handler serve il Developer Portal React come SPA embedded nel binario Go.

## Come funziona

- Il file `portal.go` usa la direttiva `//go:embed dist` per includere tutti i file della cartella `dist/` nel binario compilato
- La funzione `GetPortalHTML` serve i file statici dal filesystem embedded
- Le API `/api/portal/routes` e `/api/portal/groups` forniscono i dati al frontend React (incluse info OpenAPI e la tabella operazioni se configurate)
- Il routing SPA è gestito servendo `index.html` per tutte le route non trovate

## Route Configuration

Nel file `cmd/server/server.go` il handler va inizializzato e le route configurate così:

```go
portalHandler, err := portal.NewPortalHandler(&routeDefs)
if err != nil {
    return fmt.Errorf("initialize developer portal handler: %w", err)
}
mgmtRouter.GET("/portal", portalHandler.GetPortalHTML)
mgmtRouter.GET(
    "/portal/*path",
    portalHandler.GetPortalHTML,
    portalHandler.AssetCacheMiddleware(),
    portalHandler.StaticMiddleware(),
) // Wildcard per assets e SPA routing
mgmtRouter.GET("/api/portal/routes", portalHandler.GetRoutes)
mgmtRouter.GET("/api/portal/groups", portalHandler.GetGroups)
```

Il gateway usa il layer di autenticazione del server di management (`management.auth_enabled: true`) e richiede che i token validi per il portale includano lo scope `management:portal`. Il middleware di autenticazione e autorizzazione viene riutilizzato con gli stessi validator di `/ready` e `/metrics`.

**IMPORTANTE**: La route wildcard `/portal/*` è essenziale per:
- Servire gli assets (JS, CSS) da `/portal/assets/*` tramite il middleware static e l'header cache permanente
- Gestire il routing client-side della SPA (il middleware static lascia passare la richiesta all'`index.html` quando il file non esiste)

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

- `GET /portal` - Developer Portal UI (React SPA)
- `GET /portal/*` - Assets e SPA routing
- `GET /api/portal/routes` - Lista di tutte le route configurate
- `GET /api/portal/groups` - Lista dei gruppi di route

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
- Verifica che la route wildcard `/portal/*` sia registrata
- Rebuilda il Gateway dopo aver modificato le route

### Portal files not found

Se vedi questo errore:
- La cartella `dist/` non esiste o è vuota
- Esegui il build del developer-portal e copia i file
