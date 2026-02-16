import { useState } from 'react'
import { GroupData, OpenAPIOperation } from '../types'
import MethodBadge from './MethodBadge'

interface Props {
  group: GroupData;
}

export default function RoutesList({ group }: Props) {
  const [expandedMethods, setExpandedMethods] = useState<Record<string, boolean>>({})
  const [expandedPaths, setExpandedPaths] = useState<Record<string, boolean>>({})

  const sortMethods = (methods: { method: string; scopes: string[] }[]) => {
    const methodOrder = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']

    const getIndex = (method: string) => {
      const normalized = method.toUpperCase()
      const idx = methodOrder.indexOf(normalized)
      return idx !== -1 ? idx : methodOrder.length
    }

    return [...methods].sort((a, b) => {
      const orderDiff = getIndex(a.method) - getIndex(b.method)
      if (orderDiff !== 0) {
        return orderDiff
      }
      return a.method.localeCompare(b.method)
    })
  }

  const normalizePath = (path: string) => {
    if (!path) {
      return ''
    }
    let normalized = path.replace(/\/:([A-Za-z0-9_]+)/g, '/{$1}')
    normalized = normalized.replace(/\/{2,}/g, '/')
    if (normalized.length > 1 && normalized.endsWith('/')) {
      normalized = normalized.slice(0, -1)
    }
    return normalized
  }

  const getOpenAPIOperation = (routePath: string, method: string, operations?: OpenAPIOperation[]) => {
    if (!operations || operations.length === 0) {
      return null
    }

    const normalizedMethod = method.toUpperCase()
    const normalizedRoutePath = normalizePath(routePath)
    return operations.find((op) => (
      op.method.toUpperCase() === normalizedMethod && normalizePath(op.path) === normalizedRoutePath
    )) || null
  }

  const toggleExpanded = (key: string) => {
    setExpandedMethods((prev) => ({
      ...prev,
      [key]: !prev[key]
    }))
  }

  const togglePathExpanded = (key: string) => {
    setExpandedPaths((prev) => ({
      ...prev,
      [key]: !prev[key]
    }))
  }

  const groupKeyForRoute = (routePath: string) => {
    const normalized = normalizePath(routePath)
    const paramIndex = normalized.indexOf('/{')
    if (paramIndex > 0) {
      return normalized.slice(0, paramIndex)
    }
    return normalized
  }

  const getEndpointSuffix = (routePath: string, basePath: string) => {
    if (!routePath.startsWith(basePath)) {
      return routePath
    }
    const suffix = routePath.slice(basePath.length)
    if (!suffix) {
      return '/'
    }
    return suffix.startsWith('/') ? suffix : `/${suffix}`
  }

  const groupedRoutes = group.routes.reduce<Record<string, typeof group.routes>>((acc, route) => {
    const key = groupKeyForRoute(route.path_prefix)
    if (!acc[key]) {
      acc[key] = []
    }
    acc[key].push(route)
    return acc
  }, {})

  const groupedRouteEntries = Object.entries(groupedRoutes).sort(([a], [b]) => a.localeCompare(b))

  const openapiModeBadgeClass = (mode?: string) => {
    const normalized = (mode || 'strict').toLowerCase()
    if (normalized === 'warn-only' || normalized === 'warn_only' || normalized === 'warnonly') {
      return 'bg-amber-100 text-amber-800'
    }
    return 'bg-emerald-100 text-emerald-800'
  }

  return (
    <>
      <div className="bg-white rounded-lg shadow-sm border border-gray-200 overflow-hidden">
        <div className="bg-gradient-to-r from-blue-50 to-indigo-50 px-6 py-4 border-b border-gray-200">
          <div className="flex items-center justify-between">
            <h3 className="text-xl font-semibold text-gray-900">{group.name}</h3>
            <span className="px-3 py-1 bg-blue-600 text-white text-sm font-medium rounded-full">
              {group.prefix}
            </span>
          </div>
        </div>

        {group.routes.length > 0 && (
          <div className="p-6">
            <h4 className="text-sm font-semibold text-gray-700 uppercase tracking-wide mb-4">
              HTTP Routes
            </h4>
            <div className="space-y-4">
              {groupedRouteEntries.map(([basePath, routes]) => (
                <div key={basePath} className="rounded-lg border border-gray-200 bg-gray-50/40 p-4">
                  <button
                    type="button"
                    onClick={() => togglePathExpanded(basePath)}
                    className="flex w-full flex-wrap items-center justify-between gap-2 text-left"
                    aria-expanded={Boolean(expandedPaths[basePath])}
                  >
                    <div className="flex flex-wrap items-center gap-2">
                      <code className="text-sm font-mono text-gray-800 bg-gray-100 px-3 py-1 rounded">
                        {basePath}
                      </code>
                      <span className="text-xs text-gray-500">
                        {routes.length} endpoint{routes.length > 1 ? 's' : ''}
                      </span>
                    </div>
                    <span className="text-xs font-medium text-blue-700">
                      {expandedPaths[basePath] ? 'Hide endpoints' : 'Show endpoints'}
                    </span>
                  </button>
                  {expandedPaths[basePath] && (
                    <div className="mt-3 space-y-3">
                      {routes.map((route, idx) => {
                        const displayPath = getEndpointSuffix(normalizePath(route.path_prefix), basePath)

                        return (
                          <div
                            key={`${route.path_prefix}-${idx}`}
                            className="border border-gray-200 rounded-lg p-4 hover:border-blue-300 hover:bg-blue-50/30 transition-all bg-white"
                          >
                            <code className="text-sm font-mono text-gray-800 bg-gray-100 px-3 py-1 rounded">
                              {displayPath}
                            </code>
                            <div className="mt-2 text-xs text-gray-500">
                              Target: <span className="font-mono">{route.target_url}</span>
                            </div>
                            <div className="mt-4 space-y-2">
                              {sortMethods(route.methods).map((m, midx) => {
                                const key = `${route.path_prefix}::${m.method}`
                                const expanded = Boolean(expandedMethods[key])
                                const operation = getOpenAPIOperation(
                                  route.path_prefix,
                                  m.method,
                                  route.openapi?.operations
                                )

                                return (
                                  <div key={midx} className="rounded-md border border-gray-200 bg-white">
                                    <div className="flex items-center justify-between gap-3 px-3 py-2">
                                      <div className="flex items-center gap-2">
                                        <MethodBadge
                                          method={m.method}
                                          onClick={() => toggleExpanded(key)}
                                          expanded={expanded}
                                        />
                                        <button
                                          type="button"
                                          onClick={() => toggleExpanded(key)}
                                          className="text-xs font-medium text-blue-700 hover:text-blue-800"
                                          aria-expanded={expanded}
                                        >
                                          {expanded ? 'Hide details' : 'Show details'}
                                        </button>
                                      </div>
                                      <div className="text-[11px] text-gray-500">
                                        {m.scopes.length > 0 ? `${m.scopes.length} scope${m.scopes.length > 1 ? 's' : ''}` : 'No scopes'}
                                      </div>
                                    </div>
                                    {expanded && (
                                      <div className="border-t border-gray-200 bg-gray-50 px-3 py-3 text-xs text-gray-700">
                                        <div className="mt-2">
                                          <span className="font-semibold text-gray-800">Scopes:</span>{' '}
                                          {m.scopes.length > 0 ? (
                                            <span className="font-mono">{m.scopes.join(', ')}</span>
                                          ) : (
                                            <span className="text-gray-500">none</span>
                                          )}
                                        </div>
                                        {route.openapi ? (
                                          <>
                                            <div className="mt-2 flex flex-wrap items-center gap-2">
                                            <span className="rounded bg-blue-600 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-white">
                                              OpenAPI
                                            </span>
                                              <span className={`rounded px-2 py-0.5 font-mono text-[10px] ${openapiModeBadgeClass(route.openapi.mode)}`}>
                                                {route.openapi.mode || 'strict'}
                                              </span>
                                              <span className="rounded bg-gray-200 px-2 py-0.5 font-mono text-[10px] text-gray-700">
                                                {route.openapi.file || 'n/a'}
                                              </span>
                                            </div>
                                            <div className="mt-2">
                                              <span className="font-semibold text-gray-800">Path:</span>{' '}
                                              <span className="font-mono">{normalizePath(route.path_prefix)}</span>
                                            </div>
                                            <div className="mt-2 grid gap-1">
                                              <div>
                                                <span className="font-semibold text-gray-800">Summary:</span>{' '}
                                                {operation?.summary || '-'}
                                              </div>
                                              <div>
                                                <span className="font-semibold text-gray-800">Operation ID:</span>{' '}
                                                <span className="font-mono">{operation?.operation_id || '-'}</span>
                                              </div>
                                              <div>
                                                <span className="font-semibold text-gray-800">Deprecated:</span>{' '}
                                                {operation?.deprecated ? 'yes' : 'no'}
                                              </div>
                                              {!operation && (
                                                <div className="text-gray-500">
                                                  No OpenAPI operation matched this method and path.
                                                </div>
                                              )}
                                            </div>
                                            {route.openapi.error && (
                                              <div className="mt-2 rounded bg-red-50 px-2 py-1 text-[11px] text-red-700">
                                                {route.openapi.error}
                                              </div>
                                            )}
                                          </>
                                        ) : (
                                          <div className="mt-2 text-gray-500">
                                            OpenAPI not configured for this route.
                                          </div>
                                        )}
                                      </div>
                                    )}
                                  </div>
                                )
                              })}
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {group.websockets.length > 0 && (
          <div className="p-6 border-t border-gray-200 bg-purple-50/30">
            <h4 className="text-sm font-semibold text-gray-700 uppercase tracking-wide mb-4">
              WebSocket Routes
            </h4>
            <div className="space-y-3">
              {group.websockets.map((ws, idx) => (
                <div
                  key={idx}
                  className="border border-purple-200 rounded-lg p-4 bg-white"
                >
                  <div className="flex items-center gap-2 mb-2">
                    <code className="text-sm font-mono text-gray-800 bg-gray-100 px-3 py-1 rounded">
                      {ws.path}
                    </code>
                    <span className="px-2 py-1 bg-purple-600 text-white text-xs font-semibold rounded">
                      WS
                    </span>
                  </div>
                  <div className="flex flex-wrap gap-2 mb-2">
                    {ws.scopes.map((scope, sidx) => (
                      <span
                        key={sidx}
                        className="px-2 py-1 bg-gray-100 text-gray-700 text-xs rounded"
                      >
                        {scope}
                      </span>
                    ))}
                  </div>
                  <div className="text-xs text-gray-500">
                    Target: <span className="font-mono">{ws.target_url}</span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

    </>
  )
}
