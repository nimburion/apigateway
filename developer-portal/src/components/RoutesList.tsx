import type { KeyboardEvent } from 'react'
import { GroupData, MethodInfo, PortalSurfaceMetricSummary, RouteInfo, SelectedSurface } from '../types'
import MethodBadge from './MethodBadge'
import StatusBadge, { StatusBadgeStatus } from './StatusBadge'
import { t } from '../i18n'
import { buildRouteMetricKey } from '../metrics'
import { runtimeSignals } from '../runtimeSignals'
import { getGroupDisplayName } from '../groupDisplay'
import { surfacePriorityScore } from '../priority'

interface Props {
  group: GroupData
  sortMode?: 'default' | 'owner' | 'risk' | 'surface' | 'traffic' | 'errorRate'
  runtimeMetricsAvailable?: boolean
  groupMetric?: PortalSurfaceMetricSummary | null
  routeMetrics?: Record<string, PortalSurfaceMetricSummary>
  selectedSurfaceId?: string
  onSelectSurface: (surface: SelectedSurface) => void
  searchTerm?: string
}

const METHOD_ORDER = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']
function sortMethods(ms: MethodInfo[]) {
  return [...ms].sort((a, b) => {
    const ai = METHOD_ORDER.indexOf(a.method.toUpperCase())
    const bi = METHOD_ORDER.indexOf(b.method.toUpperCase())
    return (ai === -1 ? 99 : ai) - (bi === -1 ? 99 : bi) || a.method.localeCompare(b.method)
  })
}

function fmt(n: number) { return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(n) }
function fmtMs(v: number) {
  if (v <= 0) return '—'
  if (v >= 1000) return `${(v / 1000).toFixed(2)}s`
  return `${v.toFixed(0)}ms`
}

function highlight(text: string, term: string) {
  if (!term) return text
  const idx = text.toLowerCase().indexOf(term.toLowerCase())
  if (idx === -1) return text
  return <>{text.slice(0, idx)}<mark style={{ background: '#fef08a', borderRadius: 2, padding: '0 1px' }}>{text.slice(idx, idx + term.length)}</mark>{text.slice(idx + term.length)}</>
}

function latClass(ms: number) { return ms >= 1000 ? 'text-danger' : ms >= 500 ? 'text-warning' : 'metric-neutral' }
function errClass(rate: number) { return rate >= 0.05 ? 'text-danger' : rate >= 0.01 ? 'text-warning' : 'text-success' }

function resolveStatus(status?: string, deprecated?: boolean): StatusBadgeStatus {
  if (deprecated || status === 'deprecated') return 'deprecated'
  if (status === 'experimental') return 'experimental'
  if (status === 'disabled') return 'disabled'
  return 'active'
}

function visibilityBadge(v?: string) {
  if (v === 'public') return <span className="badge badge-green" style={{ fontSize: 10 }}>{v}</span>
  if (v === 'partner') return <span className="badge badge-amber" style={{ fontSize: 10 }}>{v}</span>
  if (v === 'internal') return <span className="badge badge-gray" style={{ fontSize: 10 }}>{v}</span>
  return null
}

function contextBadge(route: RouteInfo) {
  if (route.surface_context === 'management') {
    return <span className="badge badge-sky" style={{ fontSize: 10 }}>{t('badge.management')}</span>
  }
  if (route.runtime_only) {
    return <span className="badge badge-amber" style={{ fontSize: 10 }}>{t('badge.runtimeOnly')}</span>
  }
  return null
}

export default function RoutesList({
  group, sortMode = 'default', runtimeMetricsAvailable = false,
  groupMetric = null, routeMetrics = {},
  selectedSurfaceId, onSelectSurface, searchTerm = ''
}: Props) {

  function normPath(p: string) {
    return p.replace(/\/:([A-Za-z0-9_]+)/g, '/{$1}').replace(/\/{2,}/g, '/')
  }

  function joinPath(prefix: string, path: string) {
    const parts = [prefix, path].map(s => s.trim()).filter(s => s && s !== '/').map(s => s.replace(/\/+$/, ''))
    return parts.length === 0 ? '/' : normPath(parts.join('/'))
  }

  function getMetric(route: RouteInfo) {
    return routeMetrics[buildRouteMetricKey(group.name, joinPath(group.prefix, route.path_prefix))]
  }

  function routeTraffic(route: RouteInfo) { return getMetric(route)?.requests ?? 0 }
  function routeErrRate(route: RouteInfo) {
    const m = getMetric(route)
    return m && m.requests > 0 ? (m.client_errors + m.server_errors) / m.requests : 0
  }
  function riskScore(r: RouteInfo) {
    return surfacePriorityScore({
      authRequired: r.auth_required,
      hasRateLimit: r.has_rate_limit,
      deprecated: r.deprecated || r.metadata.status === 'deprecated',
      experimental: r.metadata.status === 'experimental',
      hasOpenApi: r.has_openapi,
      managementSurface: r.surface_context === 'management',
      ownerTeam: r.metadata.owner_team,
      docsUrl: r.metadata.docs_url,
      runbookUrl: r.metadata.runbook_url,
      supportChannel: r.metadata.support_channel,
      requests: routeTraffic(r),
      errorRate: routeErrRate(r),
    })
  }

  function compareRoutes(a: RouteInfo, b: RouteInfo) {
    if (sortMode === 'traffic') return routeTraffic(b) - routeTraffic(a) || a.path_prefix.localeCompare(b.path_prefix)
    if (sortMode === 'errorRate') {
      const d = routeErrRate(b) - routeErrRate(a)
      return d !== 0 ? d : routeTraffic(b) - routeTraffic(a)
    }
    if (sortMode === 'risk') return riskScore(b) - riskScore(a) || a.path_prefix.localeCompare(b.path_prefix)
    if (sortMode === 'owner') return (a.metadata.owner_team || '').localeCompare(b.metadata.owner_team || '') || a.path_prefix.localeCompare(b.path_prefix)
    return a.path_prefix.localeCompare(b.path_prefix)
  }

  const groupSignals = runtimeSignals(groupMetric)
  const wsPublic = group.websockets.filter(w => !w.auth_required).length
  const wsProtected = group.websockets.filter(w => w.auth_required).length

  return (
    <div className="card" style={{ marginBottom: 16, overflow: 'hidden' }}>
      {/* Group header */}
      <div style={{ padding: '12px 16px', borderBottom: '1px solid var(--color-border)', background: 'var(--color-bg)', display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div style={{ minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
            <span style={{ fontSize: 14, fontWeight: 600 }}>{getGroupDisplayName(group.name)}</span>
            {group.routes.length > 0 && (
              <span className="badge badge-gray" style={{ fontSize: 10 }}>
                HTTP <span style={{ opacity: 0.55, margin: '0 4px' }}>·</span> {group.routes.length}
              </span>
            )}
            {group.websockets.length > 0 && (
              <span className="badge badge-gray" style={{ fontSize: 10 }}>
                WS <span style={{ opacity: 0.55, margin: '0 4px' }}>·</span> {group.websockets.length}
              </span>
            )}
            <span className="badge badge-outline" style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{group.prefix || '/'}</span>
            {group.auth_required && <span className="badge badge-red">Auth</span>}
            {group.has_rate_limit && <span className="badge badge-amber">Group RL</span>}
            {group.middlewares.length > 0 && <span className="badge badge-gray">MW: {group.middlewares.join(', ')}</span>}
            {groupSignals.map(s => <span key={s.id} className={`badge ${s.className}`} title={s.label}>{s.label}</span>)}
          </div>
          {(group.metadata.owner_team || group.metadata.domain) && (
            <div style={{ fontSize: 12, color: 'var(--color-text-muted)', marginTop: 3 }}>
              {[group.metadata.owner_team, group.metadata.domain].filter(Boolean).join(' · ')}
            </div>
          )}
        </div>
        {/* Group-level metrics */}
        {runtimeMetricsAvailable && groupMetric && (
          <div style={{ display: 'flex', gap: 16, fontSize: 12, flexShrink: 0 }}>
            <div><span className="text-muted">Req </span><span className="inline-metric-value">{fmt(groupMetric.requests)}</span></div>
            <div><span className="text-muted">Avg </span><span className={`inline-metric-value ${latClass(groupMetric.average_latency_ms)}`}>{fmtMs(groupMetric.average_latency_ms)}</span></div>
          </div>
        )}
      </div>

      {/* HTTP routes */}
      {group.routes.length > 0 && (
        <>
          <div style={{ padding: '6px 16px', background: 'var(--color-bg)', borderBottom: '1px solid var(--color-border)' }}>
            <span style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--color-text-faint)' }}>
              HTTP
            </span>
          </div>
          <table className="data-table" style={{ borderRadius: 0, border: 'none' }}>
            <thead>
              <tr>
                <th style={{ width: 90 }}>Method</th>
                <th>Path</th>
                <th style={{ width: 120 }}>Owner</th>
                <th style={{ width: 120 }}>Security</th>
                <th style={{ width: 80 }}>Status</th>
                {runtimeMetricsAvailable && <th style={{ width: 80, textAlign: 'right' }}>Req</th>}
                {runtimeMetricsAvailable && <th style={{ width: 80, textAlign: 'right' }}>Avg</th>}
                {runtimeMetricsAvailable && <th style={{ width: 72, textAlign: 'right' }}>Err%</th>}
              </tr>
            </thead>
            <tbody>
              {[...group.routes].sort(compareRoutes).flatMap((route) =>
                sortMethods(route.methods).map((method) => {
                  const key = `${route.path_prefix}::${method.method}`
                  const isSelected = selectedSurfaceId === key
                  const metric = getMetric(route)
                  const routeRequests = metric?.requests ?? 0
                  const signals = runtimeSignals(metric)
                  const errRate = metric && metric.requests > 0 ? (metric.client_errors + metric.server_errors) / metric.requests : 0

                  return (
                    <tr
                      key={key}
                      className={isSelected ? 'selected' : undefined}
                      style={{ cursor: 'pointer' }}
                      onClick={() => onSelectSurface({
                        kind: 'route', id: key,
                        group_name: group.name, group_prefix: group.prefix,
                        group_middlewares: group.middlewares, route, method
                      })}
                      tabIndex={0}
                      role="button"
                      aria-label={`${method.method} ${normPath(route.path_prefix)} - open details`}
                      onKeyDown={(event: KeyboardEvent<HTMLTableRowElement>) => {
                        if (event.key === 'Enter' || event.key === ' ') {
                          event.preventDefault()
                          onSelectSurface({
                            kind: 'route', id: key,
                            group_name: group.name, group_prefix: group.prefix,
                            group_middlewares: group.middlewares, route, method
                          })
                        }
                      }}
                    >
                      <td>
                        <MethodBadge
                          method={method.method}
                          expanded={isSelected}
                          authRequired={method.auth_required}
                          primaryScope={method.scopes[0] ?? ''}
                        />
                      </td>
                      <td>
                        <div style={{ minWidth: 0 }}>
                          <code className="path-text" style={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: 320, fontSize: 13, lineHeight: 1.45 }}>
                            {highlight(normPath(route.path_prefix), searchTerm)}
                          </code>
                          <div style={{ marginTop: 6, display: 'flex', flexWrap: 'wrap', gap: 6, alignItems: 'center' }}>
                            {contextBadge(route)}
                            {group.has_rate_limit && !method.has_rate_limit && <span className="badge badge-amber" style={{ fontSize: 10 }}>Inherited RL</span>}
                            {!group.has_rate_limit && method.has_rate_limit && <span className="badge badge-amber" style={{ fontSize: 10 }}>Method RL</span>}
                            {group.has_rate_limit && method.has_rate_limit && <span className="badge badge-blue" style={{ fontSize: 10 }}>Method override</span>}
                            {route.has_openapi && <span className="badge badge-blue" style={{ fontSize: 10 }}>{t('badge.openapi')}</span>}
                            {signals.map(s => <span key={s.id} className={`badge ${s.className}`} style={{ fontSize: 10 }} title={s.label}>{s.label}</span>)}
                          </div>
                        </div>
                      </td>
                      <td>
                        <div style={{ fontSize: 12 }}>
                          <span className="text-muted">{route.metadata.owner_team || '—'}</span>
                          {route.metadata.domain && <span className="text-faint"> · {route.metadata.domain}</span>}
                        </div>
                      </td>
                      <td>
                        <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                          {method.auth_required ? <span className="badge badge-red" style={{ fontSize: 10 }}>Auth</span> : null}
                          {visibilityBadge(route.metadata.visibility)}
                        </div>
                      </td>
                      <td>
                        <StatusBadge status={resolveStatus(route.metadata.status, route.deprecated)} />
                      </td>
                      {runtimeMetricsAvailable && <td style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums', fontSize: 12, whiteSpace: 'nowrap' }}>{fmt(routeRequests)}</td>}
                      {runtimeMetricsAvailable && <td style={{ textAlign: 'right', fontSize: 12, whiteSpace: 'nowrap' }} className={metric ? latClass(metric.average_latency_ms) : 'text-faint'}>{metric ? fmtMs(metric.average_latency_ms) : '—'}</td>}
                      {runtimeMetricsAvailable && <td style={{ textAlign: 'right', fontSize: 12, whiteSpace: 'nowrap' }} className={metric && metric.requests > 0 ? errClass(errRate) : 'text-faint'}>{metric && metric.requests > 0 ? `${(errRate * 100).toFixed(1)}%` : '—'}</td>}
                    </tr>
                  )
                })
              )}
            </tbody>
          </table>
        </>
      )}

      {/* WebSocket routes */}
      {group.websockets.length > 0 && (
        <>
          <div style={{ padding: '6px 16px', background: 'var(--color-bg)', borderTop: group.routes.length > 0 ? '1px solid var(--color-border)' : undefined, borderBottom: '1px solid var(--color-border)' }}>
            <span style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--color-text-faint)' }}>
              WebSocket — {group.websockets.length} · {wsPublic} public · {wsProtected} protected
            </span>
          </div>
          <table className="data-table" style={{ borderRadius: 0, border: 'none' }}>
            <thead>
              <tr>
                <th style={{ width: 50 }}>Type</th>
                <th>Path</th>
                <th style={{ width: 120 }}>Owner</th>
                <th style={{ width: 80 }}>Security</th>
                <th style={{ width: 80 }}>Status</th>
              </tr>
            </thead>
            <tbody>
              {[...group.websockets].sort((a, b) => a.path.localeCompare(b.path)).map((ws, idx) => {
                const key = `ws::${ws.path}::${idx}`
                const isSelected = selectedSurfaceId === key
                return (
                  <tr
                    key={key}
                    className={isSelected ? 'selected' : undefined}
                    style={{ cursor: 'pointer' }}
                    onClick={() => onSelectSurface({
                      kind: 'websocket', id: key,
                      group_name: group.name, group_prefix: group.prefix,
                      group_middlewares: group.middlewares, websocket: ws
                    })}
                  >
                    <td><span className="badge badge-gray" style={{ fontSize: 10 }}>WS</span></td>
                    <td>
                      <code className="path-text" style={{ fontSize: 12 }}>{highlight(normPath(ws.path), searchTerm)}</code>
                      {!group.has_rate_limit && ws.has_rate_limit && <span className="badge badge-amber" style={{ fontSize: 10, marginLeft: 8 }}>WS RL</span>}
                    </td>
                    <td><span className="text-muted" style={{ fontSize: 12 }}>{ws.metadata.owner_team || '—'}</span></td>
                    <td>
                      <div style={{ display: 'grid', gap: 4 }}>
                        <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                          {ws.auth_required ? <span className="badge badge-red" style={{ fontSize: 10 }}>Auth</span> : <span className="badge badge-green" style={{ fontSize: 10 }}>Public</span>}
                          {visibilityBadge(ws.metadata.visibility)}
                        </div>
                        {ws.scopes.length > 0 && (
                          <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                            {ws.scopes.map(s => <span key={s} className="badge badge-purple" style={{ fontSize: 10, fontFamily: 'var(--font-mono)' }}>{s}</span>)}
                          </div>
                        )}
                      </div>
                    </td>
                    <td><StatusBadge status={resolveStatus(ws.metadata.status, ws.deprecated)} /></td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </>
      )}
    </div>
  )
}
