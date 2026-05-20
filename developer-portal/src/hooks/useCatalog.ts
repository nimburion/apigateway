import { useCallback, useEffect, useState } from 'react'
import {
  GroupData,
  GroupInfo,
  MethodInfo,
  OpenAPIInfo,
  RateLimitInfo,
  ResourceMetadata,
  RouteInfo,
  WebSocketInfo,
} from '../types'

export interface CatalogState {
  groups: GroupInfo[]
  routes: GroupData[]
  loading: boolean
  error: string
  reload: () => void
}

function ensureString(v: unknown): string { return typeof v === 'string' ? v : '' }
function ensureBoolean(v: unknown): boolean { return typeof v === 'boolean' ? v : false }
function ensureStringArray(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
}
function ensureNumber(v: unknown): number { return typeof v === 'number' && Number.isFinite(v) ? v : 0 }

function normalizeMeta(raw: unknown): ResourceMetadata {
  const m = (raw ?? {}) as Partial<ResourceMetadata>
  return {
    owner_team: ensureString(m.owner_team),
    domain: ensureString(m.domain),
    visibility: ensureString(m.visibility),
    status: ensureString(m.status),
    docs_url: ensureString(m.docs_url),
    runbook_url: ensureString(m.runbook_url),
    support_channel: ensureString(m.support_channel),
  }
}

function normalizeOpenAPI(raw: unknown): OpenAPIInfo | null {
  if (!raw || typeof raw !== 'object') return null
  const o = raw as Partial<OpenAPIInfo>
  return {
    file: ensureString(o.file),
    mode: ensureString(o.mode),
    title: ensureString(o.title),
    version: ensureString(o.version),
    description: ensureString(o.description),
    operations: Array.isArray(o.operations) ? o.operations : [],
    error: ensureString(o.error),
  }
}

function normalizeRateLimit(raw: unknown): RateLimitInfo | null {
  if (!raw || typeof raw !== 'object') return null
  const rl = raw as Partial<RateLimitInfo>
  const requestsPerSecond = ensureNumber(rl.requests_per_second)
  const burst = ensureNumber(rl.burst)
  const source = ensureString(rl.source)
  if (requestsPerSecond <= 0 || burst <= 0) return null
  return {
    requests_per_second: requestsPerSecond,
    burst,
    source,
  }
}

function normalizeMethod(raw: unknown): MethodInfo {
  const m = (raw ?? {}) as Partial<MethodInfo>
  return {
    method: ensureString(m.method),
    scopes: ensureStringArray(m.scopes),
    middlewares: ensureStringArray(m.middlewares),
    declared_middlewares: ensureStringArray(m.declared_middlewares),
    disabled_middlewares: ensureStringArray(m.disabled_middlewares),
    auth_required: ensureBoolean(m.auth_required),
    has_rate_limit: ensureBoolean(m.has_rate_limit),
    rate_limit: normalizeRateLimit(m.rate_limit),
  }
}

function normalizeRoute(raw: unknown): RouteInfo {
  const r = (raw ?? {}) as Partial<RouteInfo>
  return {
    path_prefix: ensureString(r.path_prefix),
    target_url: ensureString(r.target_url),
    methods: Array.isArray(r.methods) ? r.methods.map(normalizeMethod) : [],
    openapi: normalizeOpenAPI(r.openapi),
    metadata: normalizeMeta(r.metadata),
    middlewares: ensureStringArray(r.middlewares),
    declared_middlewares: ensureStringArray(r.declared_middlewares),
    disabled_middlewares: ensureStringArray(r.disabled_middlewares),
    endpoint_middlewares: ensureStringArray(r.endpoint_middlewares),
    endpoint_disabled_middlewares: ensureStringArray(r.endpoint_disabled_middlewares),
    auth_required: ensureBoolean(r.auth_required),
    has_openapi: ensureBoolean(r.has_openapi),
    has_rate_limit: ensureBoolean(r.has_rate_limit),
    rate_limit: normalizeRateLimit(r.rate_limit),
    deprecated: ensureBoolean(r.deprecated),
    has_openapi_errors: ensureBoolean(r.has_openapi_errors),
    exposes_target_url: ensureBoolean(r.exposes_target_url),
    exposes_openapi_errors: ensureBoolean(r.exposes_openapi_errors),
    runtime_only: ensureBoolean(r.runtime_only),
    surface_context: ensureString(r.surface_context) || 'public',
  }
}

function normalizeWebSocket(raw: unknown): WebSocketInfo {
  const w = (raw ?? {}) as Partial<WebSocketInfo>
  return {
    path: ensureString(w.path),
    target_url: ensureString(w.target_url),
    scopes: ensureStringArray(w.scopes),
    metadata: normalizeMeta(w.metadata),
    middlewares: ensureStringArray(w.middlewares),
    declared_middlewares: ensureStringArray(w.declared_middlewares),
    disabled_middlewares: ensureStringArray(w.disabled_middlewares),
    auth_required: ensureBoolean(w.auth_required),
    has_rate_limit: ensureBoolean(w.has_rate_limit),
    rate_limit: normalizeRateLimit(w.rate_limit),
    deprecated: ensureBoolean(w.deprecated),
    exposes_target_url: ensureBoolean(w.exposes_target_url),
  }
}

function normalizeGroupData(raw: unknown): GroupData {
  const g = (raw ?? {}) as Partial<GroupData>
  return {
    name: ensureString(g.name),
    prefix: ensureString(g.prefix),
    metadata: normalizeMeta(g.metadata),
    middlewares: ensureStringArray(g.middlewares),
    auth_required: ensureBoolean(g.auth_required),
    has_rate_limit: ensureBoolean(g.has_rate_limit),
    has_rate_limited_surfaces: ensureBoolean((g as { has_rate_limited_surfaces?: unknown }).has_rate_limited_surfaces),
    rate_limit: normalizeRateLimit(g.rate_limit),
    deprecated: ensureBoolean(g.deprecated),
    routes: Array.isArray(g.routes) ? g.routes.map(normalizeRoute) : [],
    websockets: Array.isArray(g.websockets) ? g.websockets.map(normalizeWebSocket) : [],
  }
}

function normalizeGroupInfo(raw: unknown): GroupInfo {
  const g = (raw ?? {}) as Partial<GroupInfo>
  const ri = (g as { runtime_info?: Partial<GroupInfo['runtime_info']> }).runtime_info ?? {}
  return {
    name: ensureString(g.name),
    prefix: ensureString(g.prefix),
    metadata: normalizeMeta(g.metadata),
    middlewares: ensureStringArray(g.middlewares),
    has_oauth2: ensureBoolean(g.has_oauth2),
    has_me_api: ensureBoolean(g.has_me_api),
    route_count: typeof g.route_count === 'number' ? g.route_count : 0,
    websocket_count: typeof g.websocket_count === 'number' ? g.websocket_count : 0,
    auth_required: ensureBoolean(g.auth_required),
    has_openapi: ensureBoolean(g.has_openapi),
    has_rate_limit: ensureBoolean(g.has_rate_limit),
    deprecated: ensureBoolean(g.deprecated),
    runtime_info: {
      auth_enabled: ensureBoolean(ri.auth_enabled),
      management_enabled: ensureBoolean(ri.management_enabled),
      management_auth_enabled: ensureBoolean(ri.management_auth_enabled),
      portal_mode: ensureString(ri.portal_mode),
      framework_middlewares: ensureStringArray(ri.framework_middlewares),
    },
  }
}

export function useCatalog(): CatalogState {
  const [groups, setGroups] = useState<GroupInfo[]>([])
  const [routes, setRoutes] = useState<GroupData[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [key, setKey] = useState(0)

  useEffect(() => {
    let cancelled = false
    setLoading(true)

    Promise.all([
      fetch('/api/portal/groups').then(r => r.json()),
      fetch('/api/portal/routes').then(r => r.json()),
    ])
      .then(([groupsData, routesData]) => {
        if (cancelled) return
        setGroups(Array.isArray(groupsData?.groups) ? groupsData.groups.map(normalizeGroupInfo) : [])
        setRoutes(Array.isArray(routesData?.groups) ? routesData.groups.map(normalizeGroupData) : [])
        setError('')
      })
      .catch((err) => {
        if (cancelled) return
        setError(err instanceof Error ? err.message : 'Failed to fetch catalog')
        setGroups([])
        setRoutes([])
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => { cancelled = true }
  }, [key])

  const reload = useCallback(() => setKey(k => k + 1), [])
  return { groups, routes, loading, error, reload }
}
