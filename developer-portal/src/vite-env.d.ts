/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_PROXY_TARGET?: string
  readonly VITE_METRICS_URL?: string
  readonly VITE_MANAGEMENT_BASE_URL?: string
}

declare module '*.svg' {
  const src: string
  export default src
}
