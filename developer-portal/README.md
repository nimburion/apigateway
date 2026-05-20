# API Gateway - Developer Portal

Una moderna Single Page Application (SPA) React per visualizzare e documentare le API del gateway.

## 🚀 Features

- **Visualizzazione Routes**: Mostra tutte le routes HTTP e WebSocket organizzate per gruppi
- **Ricerca Avanzata**: Filtra routes per nome, path o gruppo
- **Dettagli Scopes**: Visualizza gli scopes OAuth2 richiesti per ogni endpoint
- **Design Moderno**: UI responsive con Tailwind CSS
- **TypeScript**: Type-safe development

## 📦 Installazione

```bash
npm install
```

## 🛠️ Sviluppo

```bash
npm run dev
```

L'applicazione sarà disponibile su `http://localhost:5173` e farà proxy delle chiamate API e di `/metrics` verso il management server del gateway.

## 🏗️ Build

```bash
npm run build
```

I file di produzione saranno generati nella cartella `dist/`.

## 🔧 Configurazione

Il proxy di sviluppo è configurato in `vite.config.ts`. Per cambiare target puoi usare `VITE_API_PROXY_TARGET` oppure modificare direttamente il file:

```typescript
server: {
  proxy: {
    '/api': {
      target: 'http://localhost:8080',
      changeOrigin: true
    },
    '/metrics': {
      target: 'http://localhost:8080',
      changeOrigin: true
    }
  }
}
```

Se il portal deve leggere `/metrics` da un host differente rispetto alla UI, puoi anche impostare:

```bash
VITE_METRICS_URL=https://gateway-mgmt.example.com/metrics
```

oppure:

```bash
VITE_MANAGEMENT_BASE_URL=https://gateway-mgmt.example.com
```

## 📁 Struttura

```
src/
├── components/       # Componenti React riutilizzabili
│   ├── GroupCard.tsx
│   ├── MethodBadge.tsx
│   ├── RoutesList.tsx
│   ├── ScopeModal.tsx
│   └── SearchBar.tsx
├── types.ts         # TypeScript types
├── App.tsx          # Componente principale
├── main.tsx         # Entry point
└── index.css        # Stili globali
```

## 🎨 Personalizzazione

I colori e gli stili possono essere personalizzati modificando:
- `tailwind.config.js` per i colori del tema
- `src/index.css` per gli stili globali
- I componenti individuali per stili specifici
