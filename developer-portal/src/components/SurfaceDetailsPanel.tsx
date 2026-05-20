import * as Dialog from '@radix-ui/react-dialog'
import { ReactNode, RefObject, useEffect, useId, useState } from 'react'
import MethodBadge from './MethodBadge'
import StatusBadge, { StatusBadgeStatus } from './StatusBadge'
import { PortalSurfaceMetricSummary, RateLimitInfo, SelectedSurface } from '../types'
import { t } from '../i18n'
import { runtimeSignals } from '../runtimeSignals'
import { getGroupDisplayName } from '../groupDisplay'

interface Props {
  surface: SelectedSurface | null
  surfaceMetric: PortalSurfaceMetricSummary | null
  frameworkMiddlewares?: string[]
  groupRateLimit?: RateLimitInfo | null
  runtimeMetricsAvailable: boolean
  onRefreshMetrics?: () => void
  onClose?: () => void
  mode?: 'panel' | 'drawer'
  titleId?: string
  initialFocusRef?: RefObject<HTMLButtonElement>
  errorFocusRef?: RefObject<HTMLDivElement>
}

function Section({ title, children, open = false }: { title: string; children: ReactNode; open?: boolean }) {
  return (
    <details open={open} style={{ borderTop: '1px solid var(--color-border)', padding: '12px 0' }}>
      <summary style={{ cursor: 'pointer', listStyle: 'none', display: 'flex', justifyContent: 'space-between', alignItems: 'center', userSelect: 'none' }}>
        <span style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--color-text-muted)' }}>{title}</span>
        <span style={{ fontSize: 12, color: 'var(--color-text-faint)' }}>▾</span>
      </summary>
      <div style={{ marginTop: 12 }}>{children}</div>
    </details>
  )
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '120px 1fr', gap: 8, padding: '6px 0', borderBottom: '1px solid var(--color-border)', alignItems: 'baseline' }}>
      <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-text-muted)' }}>{label}</span>
      <span style={{ fontSize: 13, color: 'var(--color-text)' }}>{children}</span>
    </div>
  )
}

function CompactRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '108px minmax(0, 1fr)', gap: 8, padding: '5px 0', alignItems: 'baseline' }}>
      <span style={{ fontSize: 11, fontWeight: 600, color: 'var(--color-text-muted)', textTransform: 'uppercase', letterSpacing: '0.04em' }}>{label}</span>
      <span style={{ fontSize: 13, color: 'var(--color-text)', minWidth: 0 }}>{children}</span>
    </div>
  )
}

function CompactSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="card" style={{ padding: 10 }}>
      <div style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--color-text-muted)', marginBottom: 8 }}>
        {title}
      </div>
      {children}
    </div>
  )
}

function MetricPill({ label, value, tone }: { label: string; value: string; tone?: 'ok' | 'warn' | 'danger' }) {
  const cls = tone === 'ok' ? 'metric-ok' : tone === 'warn' ? 'metric-warn' : tone === 'danger' ? 'metric-danger' : 'metric-neutral'
  return (
    <div className="stat-card" style={{ padding: '8px 12px', minHeight: 0 }}>
      <div className="stat-card-label">{label}</div>
      <div className={`stat-card-value ${cls}`} style={{ fontSize: 17, marginTop: 4 }}>{value}</div>
    </div>
  )
}

function PipelineStage({ label, values, removed = [], isEffective = false, onClick }: {
  label: string; values: string[]; removed?: string[]; isEffective?: boolean; onClick?: () => void; meta?: string | null
}) {
  const empty = values.length === 0 && removed.length === 0
  return (
    <div className={`pipeline-stage${isEffective ? ' is-effective' : ''}${empty ? ' is-empty' : ''}`}>
      {onClick ? (
        <button
          type="button"
          onClick={onClick}
          className="pipeline-label"
          aria-haspopup="dialog"
          style={{
            all: 'unset',
            display: 'inline-flex',
            alignItems: 'center',
            fontSize: 11,
            fontWeight: 700,
            textTransform: 'uppercase',
            letterSpacing: '0.06em',
            color: 'var(--color-text-muted)',
            cursor: 'pointer'
          }}
        >
          {label}
        </button>
      ) : (
        <div className="pipeline-label">{label}</div>
      )}
      {values.length > 0 ? values.map(v => <span key={v} className="pipeline-tag">{v}</span>) : <span className="pipeline-tag" style={{ opacity: 0.4 }}>—</span>}
      {removed.map(v => <span key={v} className="pipeline-tag pipeline-tag-removed">{v}</span>)}
      {empty ? null : undefined}
    </div>
  )
}

function StageStateBadge({ state }: { state: 'configured' | 'inherited' | 'missing' | 'mixed' }) {
  const cls = state === 'configured' ? 'badge-blue' : state === 'inherited' ? 'badge-amber' : state === 'missing' ? 'badge-red' : 'badge-gray'
  const label = state === 'configured' ? 'Configured' : state === 'inherited' ? 'Inherited' : state === 'missing' ? 'Missing' : 'Mixed'
  return <span className={`badge ${cls}`} style={{ fontSize: 10 }}>{label}</span>
}

function MiddlewareListDialog({
  open,
  title,
  description,
  items,
  emptyMessage,
  onOpenChange
}: {
  open: boolean
  title: string
  description: string
  items: string[]
  emptyMessage: string
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="drawer-overlay" />
        <Dialog.Content
          onEscapeKeyDown={(event) => {
            event.stopPropagation()
          }}
          style={{
            position: 'fixed',
            inset: 0,
            zIndex: 50,
            display: 'grid',
            placeItems: 'center',
            padding: 16
          }}
        >
          <div className="card" style={{ width: 'min(760px, 100%)', maxHeight: '80vh', overflow: 'auto', padding: 20 }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, marginBottom: 16 }}>
              <div>
                <Dialog.Title style={{ margin: 0, fontSize: 20, fontWeight: 700 }}>
                  {title}
                </Dialog.Title>
                <div style={{ marginTop: 6, fontSize: 13, color: 'var(--color-text-muted)' }}>
                  {description}
                </div>
              </div>
              <Dialog.Close asChild>
                <button type="button" className="btn btn-ghost" aria-label={t('detail.close')} style={{ minWidth: 44, minHeight: 44 }}>
                  ×
                </button>
              </Dialog.Close>
            </div>

            {items.length > 0 ? (
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                {items.map((item) => (
                  <span key={item} className="badge badge-blue" style={{ fontFamily: 'var(--font-mono)' }}>
                    {item}
                  </span>
                ))}
              </div>
            ) : (
              <div className="alert alert-info" style={{ marginBottom: 0 }}>
                <span>{emptyMessage}</span>
              </div>
            )}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function fmt(n: number) { return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(n) }
function fmtMs(v: number) {
  if (v <= 0) return '0 ms'
  if (v >= 1000) return `${(v / 1000).toFixed(2)} s`
  return `${v.toFixed(0)} ms`
}
function fmtPct(r: number) { return `${(r * 100).toFixed(1)}%` }

function riskToneFromScore(score: number): 'ok' | 'warn' | 'danger' {
  if (score >= 75) return 'danger'
  if (score >= 45) return 'warn'
  return 'ok'
}

export default function SurfaceDetailsPanel({
  surface, surfaceMetric, frameworkMiddlewares = [], groupRateLimit = null, runtimeMetricsAvailable,
  onRefreshMetrics, onClose, mode = 'panel',
  titleId: externalTitleId, initialFocusRef, errorFocusRef
}: Props) {
  const genId = useId()
  const titleId = externalTitleId ?? genId
  const [pipelineDialog, setPipelineDialog] = useState<null | {
    title: string
    description: string
    items: string[]
  }>(null)

  useEffect(() => {
    if (!onClose || typeof window === 'undefined') {
      return
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') {
        return
      }
      if (pipelineDialog !== null) {
        return
      }
      event.preventDefault()
      onClose()
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [onClose, pipelineDialog])

  if (!surface) {
    if (mode === 'drawer') return null
    return (
      <div className="card" style={{ padding: 24 }}>
        <p className="text-muted">{t('detail.emptyBody')}</p>
      </div>
    )
  }

  const isRoute = surface.kind === 'route'
  const meta = isRoute ? surface.route.metadata : surface.websocket.metadata
  const targetUrl = isRoute ? surface.route.target_url : surface.websocket.target_url
  const exposesTarget = isRoute ? surface.route.exposes_target_url : surface.websocket.exposes_target_url
  const openapi = isRoute ? surface.route.openapi ?? null : null
  const scopes = isRoute ? surface.method.scopes : surface.websocket.scopes
  const frameworkMW = Array.from(new Set(frameworkMiddlewares)).filter(Boolean)
  const groupMW = Array.from(new Set(surface.group_middlewares)).filter(Boolean)
  const routeRuntimeMW = isRoute ? Array.from(new Set(surface.route.middlewares)).filter(Boolean) : []
  const methodRuntimeMW = isRoute ? Array.from(new Set(surface.method.middlewares)).filter(Boolean) : []
  const wsRuntimeMW = !isRoute ? Array.from(new Set(surface.websocket.middlewares)).filter(Boolean) : []
  const effectiveMW = isRoute
    ? Array.from(new Set([
      ...frameworkMW,
      ...groupMW,
      ...routeRuntimeMW,
      ...surface.route.endpoint_middlewares,
      ...methodRuntimeMW
    ])).filter(Boolean)
    : Array.from(new Set([
      ...frameworkMW,
      ...groupMW,
      ...wsRuntimeMW
    ])).filter(Boolean)
  const routeDeclMW = isRoute ? Array.from(new Set(surface.route.declared_middlewares)).filter(Boolean) : []
  const routeDisMW = isRoute ? Array.from(new Set(surface.route.disabled_middlewares)).filter(Boolean) : []
  const epDeclMW = isRoute ? Array.from(new Set(surface.route.endpoint_middlewares)).filter(Boolean) : []
  const epDisMW = isRoute ? Array.from(new Set(surface.route.endpoint_disabled_middlewares)).filter(Boolean) : []
  const methDeclMW = isRoute ? Array.from(new Set(surface.method.declared_middlewares)).filter(Boolean) : []
  const methDisMW = isRoute ? Array.from(new Set(surface.method.disabled_middlewares)).filter(Boolean) : []
  const wsDeclMW = !isRoute ? Array.from(new Set(surface.websocket.declared_middlewares)).filter(Boolean) : []
  const wsDisMW = !isRoute ? Array.from(new Set(surface.websocket.disabled_middlewares)).filter(Boolean) : []
  const authRequired = isRoute ? surface.method.auth_required : surface.websocket.auth_required
  const status = isRoute
    ? (surface.route.deprecated ? 'deprecated' : surface.route.metadata.status || 'active')
    : (surface.websocket.deprecated ? 'deprecated' : surface.websocket.metadata.status || 'active')
  const resolvedStatus: StatusBadgeStatus = ['deprecated', 'experimental', 'disabled'].includes(status) ? status as StatusBadgeStatus : 'active'

  const errorCount = surfaceMetric ? surfaceMetric.client_errors + surfaceMetric.server_errors : 0
  const rateLimitedCount = surfaceMetric?.rate_limited_responses ?? 0
  const errorRate = surfaceMetric && surfaceMetric.requests > 0 ? errorCount / surfaceMetric.requests : 0
  const throttlingShare = errorCount > 0 ? rateLimitedCount / errorCount : 0
  const latTone = !surfaceMetric ? undefined : surfaceMetric.average_latency_ms >= 1000 ? 'danger' as const : surfaceMetric.average_latency_ms >= 500 ? 'warn' as const : 'ok' as const
  const errTone = !surfaceMetric ? undefined : errorRate >= 0.05 ? 'danger' as const : errorRate >= 0.01 ? 'warn' as const : 'ok' as const

  const signals = runtimeSignals(surfaceMetric)
  const localRateLimit = isRoute ? surface.method.rate_limit : surface.websocket.rate_limit
  const effectiveRateLimit = localRateLimit ?? groupRateLimit
  const rateLimitSourceLabel = isRoute
    ? (surface.method.rate_limit ? 'Configured on method' : groupRateLimit ? 'Inherited from group' : null)
    : (surface.websocket.rate_limit ? 'Configured on websocket' : groupRateLimit ? 'Inherited from group' : null)
  const isPublic = !authRequired
  const hasAuthControl = authRequired
  const hasRateLimitControl = Boolean(effectiveRateLimit)
  const postureScore = Math.max(0, 100 - (
    (isPublic ? 35 : 0) +
    (!hasAuthControl ? 25 : 0) +
    (!hasRateLimitControl ? 20 : 0) +
    (errorRate >= 0.25 ? 25 : errorRate >= 0.1 ? 15 : errorRate >= 0.05 ? 8 : 0) +
    (rateLimitedCount > 0 ? 12 : 0)
  ))
  const riskTone = riskToneFromScore(postureScore)
  const riskLabel = riskTone === 'danger' ? 'High risk' : riskTone === 'warn' ? 'Medium risk' : 'Low risk'
  const topFinding = isPublic && !hasAuthControl && !hasRateLimitControl
    ? 'Public endpoint without authentication or throttling'
    : !hasAuthControl
      ? 'Authentication is not enforced'
      : !hasRateLimitControl
        ? 'No rate limit is configured on this surface'
        : rateLimitedCount > 0
          ? 'Runtime throttling observed in the selected range'
          : errorRate >= 0.05
            ? 'Elevated error rate in the selected range'
            : 'No critical findings in the selected range'
  const nextAction = !hasAuthControl
    ? 'Require auth or document an approved public exception.'
    : !hasRateLimitControl
      ? 'Configure explicit throttling for this surface.'
      : rateLimitedCount > 0
        ? 'Investigate the 429 source and tune the limit or clients.'
        : 'Keep monitoring the current posture.'
  const riskSummary = `${isPublic ? 'No auth' : 'Auth required'} · ${hasRateLimitControl ? 'Rate limit present' : 'No rate limit'}${rateLimitedCount > 0 ? ` · ${fmtPct(throttlingShare)} 429` : ''}`
  const routeLabel = isRoute ? surface.route.path_prefix : surface.websocket.path
  const groupLabel = getGroupDisplayName(surface.group_name)
  const basePathLabel = surface.group_prefix
  const pipelineDialogBody = t('detail.pipelineDialogBody')
  const pipelineDialogEmpty = t('detail.pipelineDialogEmpty')

  const describeDiff = (runtime: string[], declared: string[], removed: string[]) => {
    const runtimeSet = Array.from(new Set(runtime)).filter(Boolean).sort()
    const declaredSet = Array.from(new Set(declared)).filter(Boolean).sort()
    const removedSet = Array.from(new Set(removed)).filter(Boolean).sort()
    const runtimeText = runtimeSet.length > 0 ? runtimeSet.join(', ') : '—'
    const declaredDiffers = declaredSet.length > 0 && runtimeSet.join('\u0000') !== declaredSet.join('\u0000')
    const parts = [runtimeText]
    if (declaredDiffers) {
      parts.push(`declared: ${declaredSet.join(', ')}`)
    }
    if (removedSet.length > 0) {
      parts.push(`removed: ${removedSet.join(', ')}`)
    }
    return parts.join(' · ')
  }

  const openPipelineDialog = (title: string, items: string[]) => {
    setPipelineDialog({ title, description: pipelineDialogBody, items })
  }

  const pipelineSummary = isRoute
    ? [
        !hasAuthControl ? 'auth is missing at method stage' : 'auth is enforced at method stage',
        !hasRateLimitControl ? 'rate limit is not configured on the surface' : 'rate limiting is present',
        groupMW.length > 0 ? 'group middleware is inherited' : 'group middleware is empty'
      ].join(' · ')
    : [
        !hasAuthControl ? 'websocket auth is missing' : 'websocket auth is enforced',
        !hasRateLimitControl ? 'rate limit is not configured on the websocket' : 'rate limiting is present'
      ].join(' · ')
  const pipelineStatePills = isRoute
    ? [
        {
          label: t('detail.pipelineFramework'),
          state: frameworkMW.length > 0 ? 'inherited' as const : 'missing' as const,
          value: `${frameworkMW.length} middleware(s)`,
          items: frameworkMW
        },
        {
          label: t('detail.pipelineGroup'),
          state: groupMW.length > 0 ? 'inherited' as const : 'missing' as const,
          value: `${groupMW.length} middleware(s)`,
          items: groupMW
        },
        {
          label: t('detail.pipelineRoute'),
          state: routeDisMW.length > 0 ? 'mixed' as const : routeRuntimeMW.length > 0 || routeDeclMW.length > 0 ? 'configured' as const : 'missing' as const,
          value: describeDiff(routeRuntimeMW, routeDeclMW, routeDisMW),
          items: routeRuntimeMW.length > 0 ? routeRuntimeMW : routeDeclMW
        },
        {
          label: t('detail.pipelineEndpoint'),
          state: epDisMW.length > 0 ? 'mixed' as const : epDeclMW.length > 0 ? 'configured' as const : 'missing' as const,
          value: describeDiff(epDeclMW, epDeclMW, epDisMW),
          items: epDeclMW
        },
        {
          label: t('detail.pipelineMethod'),
          state: methDisMW.length > 0 ? 'mixed' as const : methodRuntimeMW.length > 0 || methDeclMW.length > 0 ? 'configured' as const : 'missing' as const,
          value: describeDiff(methodRuntimeMW, methDeclMW, methDisMW),
          items: methodRuntimeMW.length > 0 ? methodRuntimeMW : methDeclMW
        }
      ]
    : [
        {
          label: t('detail.pipelineFramework'),
          state: frameworkMW.length > 0 ? 'inherited' as const : 'missing' as const,
          value: `${frameworkMW.length} middleware(s)`,
          items: frameworkMW
        },
        {
          label: t('detail.pipelineWebsocket'),
          state: wsDisMW.length > 0 ? 'mixed' as const : wsRuntimeMW.length > 0 || wsDeclMW.length > 0 ? 'configured' as const : 'missing' as const,
          value: describeDiff(wsRuntimeMW, wsDeclMW, wsDisMW),
          items: wsRuntimeMW.length > 0 ? wsRuntimeMW : wsDeclMW
        }
      ]

  const shellClassName = mode === 'drawer' ? 'drawer-panel' : 'surface-page'

  return (
    <div className={shellClassName}>
      {/* Header */}
      <div className="drawer-header">
        <div style={{ minWidth: 0, flex: 1 }}>
          <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--color-text-muted)', marginBottom: 4 }}>
            {t('detail.title')}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}>
            {isRoute ? <MethodBadge method={surface.method.method} /> : <span className="badge badge-gray">WS</span>}
            <h3 id={titleId} style={{ margin: 0, fontSize: 15, fontWeight: 600, fontFamily: 'var(--font-mono)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {routeLabel}
            </h3>
            <span className={`badge ${authRequired ? 'badge-red' : 'badge-green'}`}>{authRequired ? 'Auth' : 'Public'}</span>
            <span className="badge badge-outline" style={{ fontSize: 10 }}>{t('metrics.apiGroup')}: {groupLabel}</span>
            <span className="badge badge-outline" style={{ fontSize: 10 }}>{t('detail.basePath')}: {basePathLabel}</span>
          </div>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
          {isRoute && surface.route.has_openapi && (
            <a
              href="/api/portal/openapi.yaml"
              download="openapi.yaml"
              className="btn btn-secondary btn-sm"
              style={{ flexShrink: 0 }}
            >
              {t('routes.downloadRuntimeSpec')}
            </a>
          )}
          {onClose && (
            <button
              ref={initialFocusRef}
              type="button"
              className="btn btn-ghost drawer-close-btn"
              onClick={onClose}
              aria-label={t('detail.close')}
              style={{ flexShrink: 0 }}
            >
              <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false" width="18" height="18">
                <path d="M6 6l12 12M18 6L6 18" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" />
              </svg>
            </button>
          )}
        </div>
      </div>

      {/* Body */}
      <div className="drawer-body">
        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1.45fr) minmax(300px, 0.55fr)', gap: 12, alignItems: 'start', marginBottom: 12 }}>
          <div style={{ minWidth: 0, display: 'grid', gap: 12 }}>
            <div className={`card ${riskTone === 'danger' ? 'card-danger' : riskTone === 'warn' ? 'card-warning' : 'card-success'}`} style={{ padding: 10 }}>
              <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1.3fr) minmax(220px, 0.7fr)', gap: 12, alignItems: 'center' }}>
                <div style={{ minWidth: 0, display: 'grid', gap: 4 }}>
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>
                    <span className={`badge ${riskTone === 'danger' ? 'badge-red' : riskTone === 'warn' ? 'badge-amber' : 'badge-green'}`}>{riskLabel}</span>
                    <span className="badge badge-gray">Posture score {postureScore}/100</span>
                  </div>
                  <div style={{ fontSize: 14, fontWeight: 700, color: 'var(--color-text)', lineHeight: 1.3 }}>{topFinding}</div>
                  <div style={{ fontSize: 12, color: 'var(--color-text-muted)', lineHeight: 1.4 }}>{riskSummary}</div>
                </div>
                <div style={{ display: 'grid', gap: 4, justifyItems: 'end', textAlign: 'right' }}>
                  <span className="badge badge-gray">Next action</span>
                  <div style={{ fontSize: 12.5, color: 'var(--color-text)', lineHeight: 1.35, maxWidth: 260 }}>{nextAction}</div>
                </div>
              </div>
            </div>

            <CompactSection title="Findings">
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr>
                    <th style={{ textAlign: 'left', fontSize: 11, letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--color-text-muted)', paddingBottom: 8 }}>Severity</th>
                    <th style={{ textAlign: 'left', fontSize: 11, letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--color-text-muted)', paddingBottom: 8 }}>Finding</th>
                    <th style={{ textAlign: 'left', fontSize: 11, letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--color-text-muted)', paddingBottom: 8 }}>Evidence</th>
                    <th style={{ textAlign: 'left', fontSize: 11, letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--color-text-muted)', paddingBottom: 8 }}>Action</th>
                  </tr>
                </thead>
                <tbody>
                  <tr style={{ borderTop: '1px solid var(--color-border)' }}>
                    <td style={{ padding: '10px 8px 10px 0', verticalAlign: 'top' }}>
                      <span className={`badge ${isPublic ? 'badge-red' : 'badge-green'}`}>{isPublic ? 'Critical' : 'Info'}</span>
                    </td>
                    <td style={{ padding: '10px 8px 10px 0', verticalAlign: 'top', fontSize: 13, color: 'var(--color-text)', fontWeight: 600 }}>Public exposure</td>
                    <td style={{ padding: '10px 8px 10px 0', verticalAlign: 'top', fontSize: 13, color: 'var(--color-text-muted)' }}>{isPublic ? 'Reachable without authentication.' : 'Authentication is enforced.'}</td>
                    <td style={{ padding: '10px 0', verticalAlign: 'top', fontSize: 13, color: 'var(--color-text)' }}>{isPublic ? 'Require auth or record an approved exception.' : 'Keep auth enforced.'}</td>
                  </tr>
                  <tr style={{ borderTop: '1px solid var(--color-border)' }}>
                    <td style={{ padding: '10px 8px 10px 0', verticalAlign: 'top' }}>
                      <span className={`badge ${!hasRateLimitControl ? 'badge-amber' : 'badge-blue'}`}>{!hasRateLimitControl ? 'High' : 'Info'}</span>
                    </td>
                    <td style={{ padding: '10px 8px 10px 0', verticalAlign: 'top', fontSize: 13, color: 'var(--color-text)', fontWeight: 600 }}>Rate limiting</td>
                    <td style={{ padding: '10px 8px 10px 0', verticalAlign: 'top', fontSize: 13, color: 'var(--color-text-muted)' }}>{!hasRateLimitControl ? 'No explicit throttling policy.' : effectiveRateLimit?.source === 'group' ? 'Inherited from group.' : 'Configured locally.'}</td>
                    <td style={{ padding: '10px 0', verticalAlign: 'top', fontSize: 13, color: 'var(--color-text)' }}>{!hasRateLimitControl ? 'Configure explicit throttling.' : 'Keep the current limit or tune it.'}</td>
                  </tr>
                  {rateLimitedCount > 0 && (
                    <tr style={{ borderTop: '1px solid var(--color-border)' }}>
                      <td style={{ padding: '10px 8px 10px 0', verticalAlign: 'top' }}>
                        <span className="badge badge-amber">Medium</span>
                      </td>
                      <td style={{ padding: '10px 8px 10px 0', verticalAlign: 'top', fontSize: 13, color: 'var(--color-text)', fontWeight: 600 }}>Runtime throttling</td>
                      <td style={{ padding: '10px 8px 10px 0', verticalAlign: 'top', fontSize: 13, color: 'var(--color-text-muted)' }}>{errorCount > 0 ? `${fmtPct(throttlingShare)} of failures are 429s.` : '429s observed in the selected window.'}</td>
                      <td style={{ padding: '10px 0', verticalAlign: 'top', fontSize: 13, color: 'var(--color-text)' }}>Investigate clients or adjust the limit.</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </CompactSection>

            {isRoute && (
              <CompactSection title="Runtime">
                {runtimeMetricsAvailable ? (
                  surfaceMetric ? (
                    <>
                      {signals.length > 0 && (
                        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 8 }}>
                          {signals.map(s => <span key={s.id} className={`badge ${s.className}`}>{s.label}</span>)}
                        </div>
                      )}
                      <div style={{ display: 'grid', gridTemplateColumns: rateLimitedCount > 0 ? 'repeat(4, 1fr)' : 'repeat(3, 1fr)', gap: 8 }}>
                        <MetricPill label={t('metrics.requests')} value={fmt(surfaceMetric.requests)} tone="ok" />
                        <MetricPill label={t('metrics.avgLatency')} value={fmtMs(surfaceMetric.average_latency_ms)} tone={latTone} />
                        <div ref={errorFocusRef} tabIndex={-1} style={{ outline: 'none' }}>
                          <MetricPill label={t('metrics.errors')} value={`${fmt(errorCount)} (${fmtPct(errorRate)})`} tone={errTone} />
                        </div>
                        {rateLimitedCount > 0 && <MetricPill label={t('metrics.rateLimited')} value={fmt(rateLimitedCount)} tone="warn" />}
                      </div>
                      {onRefreshMetrics && (
                        <button type="button" className="btn btn-ghost btn-sm" onClick={onRefreshMetrics} style={{ marginTop: 8 }}>
                          ↻ {t('metrics.refresh')}
                        </button>
                      )}
                    </>
                  ) : (
                    <div className="alert alert-info">
                      <span>No traffic observed for this surface in the current window.</span>
                      {onRefreshMetrics && <button type="button" className="btn btn-ghost btn-sm" onClick={onRefreshMetrics} style={{ marginLeft: 8 }}>Refresh</button>}
                    </div>
                  )
                ) : (
                  <p className="text-muted" style={{ fontSize: 12, marginBottom: 0 }}>{t('detail.metricsUnavailable')}</p>
                )}
              </CompactSection>
            )}
          </div>

          <div style={{ minWidth: 0, display: 'grid', gap: 12 }}>
            <CompactSection title="Identity">
              <CompactRow label={t('detail.group')}>{groupLabel}</CompactRow>
              <CompactRow label={t('detail.basePath')}>{basePathLabel}</CompactRow>
              <CompactRow label={t('routes.owner')}>{meta.owner_team || '—'}</CompactRow>
              <CompactRow label={t('routes.domain')}>{meta.domain || '—'}</CompactRow>
              <CompactRow label={t('routes.lifecycle')}><StatusBadge status={resolvedStatus} /></CompactRow>
              {meta.visibility && <CompactRow label={t('routes.visibility')}><span className="badge badge-gray">{meta.visibility}</span></CompactRow>}
              <CompactRow label={t('routes.target')}><span className="path-text-muted">{exposesTarget && targetUrl ? targetUrl : t('routes.targetHidden')}</span></CompactRow>
              {meta.support_channel && <CompactRow label={t('group.support')}>{meta.support_channel}</CompactRow>}
            </CompactSection>

            <CompactSection title={t('detail.security')}>
              <CompactRow label="Authentication">
                <span className={`badge ${authRequired ? 'badge-green' : 'badge-red'}`}>{authRequired ? 'Required' : 'Not required'}</span>
              </CompactRow>
              <CompactRow label={isRoute ? 'Rate limiting' : 'WebSocket rate limit'}>
                {effectiveRateLimit ? (
                  <div style={{ display: 'grid', gap: 6 }}>
                    {groupRateLimit && (
                      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center' }}>
                        <span className="badge badge-amber">Inherited</span>
                        <span className="badge badge-gray" style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{groupRateLimit.requests_per_second} rps</span>
                        <span className="badge badge-gray" style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>burst {groupRateLimit.burst}</span>
                      </div>
                    )}
                    {localRateLimit && (
                      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center' }}>
                        <span className="badge badge-blue">{rateLimitSourceLabel ?? 'Configured locally'}</span>
                        <span className="badge badge-gray" style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{localRateLimit.requests_per_second} rps</span>
                        <span className="badge badge-gray" style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>burst {localRateLimit.burst}</span>
                      </div>
                    )}
                  </div>
                ) : (
                  <span className="badge badge-gray">Not configured</span>
                )}
              </CompactRow>
              <CompactRow label="Scopes">
                {scopes.length > 0 ? (
                  <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                    {scopes.map(s => <span key={s} className="badge badge-purple" style={{ fontFamily: 'var(--font-mono)', fontSize: 11 }}>{s}</span>)}
                  </div>
                ) : <span className="text-faint">None</span>}
              </CompactRow>
            </CompactSection>
          </div>
        </div>

        <Section title={t('detail.pipeline')} open={false}>
          <p style={{ fontSize: 12, color: 'var(--color-text-muted)', marginBottom: 10 }}>
            {isRoute
              ? 'Effective chain protects observability and headers, but does not enforce authentication or throttling at route level.'
              : t('detail.pipelineBody')}
          </p>
          <div className="card" style={{ padding: 12, marginBottom: 12, background: 'var(--color-bg-muted)' }}>
            <div style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--color-text-muted)' }}>
              Pipeline diagnosis
            </div>
            <div style={{ marginTop: 6, fontSize: 13, color: 'var(--color-text)' }}>
              {pipelineSummary}
            </div>
          </div>
            <div style={{ display: 'grid', gap: 10 }}>
                      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: 8 }}>
                        {pipelineStatePills.map((item) => (
                          <button
                            key={item.label}
                            type="button"
                            className="card"
                            style={{ width: '100%', padding: 10, textAlign: 'left', cursor: 'pointer', appearance: 'none' }}
                            onClick={() => openPipelineDialog(item.label, item.items)}
                          >
                            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, alignItems: 'center', marginBottom: 8 }}>
                              <span style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--color-text-muted)' }}>{item.label}</span>
                              <StageStateBadge state={item.state} />
                            </div>
                            <div style={{ fontSize: 13, color: 'var(--color-text)' }}>{item.value}</div>
                          </button>
                        ))}
                    </div>
            <details>
              <summary style={{ cursor: 'pointer', listStyle: 'none', fontSize: 12, fontWeight: 600, color: 'var(--color-text-muted)' }}>
                Show detailed chain
              </summary>
              <div style={{ marginTop: 10 }}>
                <div className="pipeline-stack">
                  <div className="pipeline">
                    <div>
                      <PipelineStage label={t('detail.pipelineFramework')} values={frameworkMW} onClick={() => openPipelineDialog(t('detail.pipelineFramework'), frameworkMW)} />
                    </div>
                    <div className="pipeline-arrow">→</div>
                    {isRoute ? (
                      <>
                        <div>
                          <PipelineStage label={t('detail.pipelineGroup')} values={groupMW} onClick={() => openPipelineDialog(t('detail.pipelineGroup'), groupMW)} />
                        </div>
                        <div className="pipeline-arrow">→</div>
                        <div>
                          <PipelineStage
                            label={t('detail.pipelineRoute')}
                            values={routeRuntimeMW.length > 0 ? routeRuntimeMW : routeDeclMW}
                            removed={routeDisMW}
                            onClick={() => openPipelineDialog(t('detail.pipelineRoute'), routeRuntimeMW.length > 0 ? routeRuntimeMW : routeDeclMW)}
                          />
                        </div>
                        <div className="pipeline-arrow">→</div>
                        <div>
                          <PipelineStage label={t('detail.pipelineEndpoint')} values={epDeclMW} removed={epDisMW} onClick={() => openPipelineDialog(t('detail.pipelineEndpoint'), epDeclMW)} />
                        </div>
                        <div className="pipeline-arrow">→</div>
                        <div>
                          <PipelineStage
                            label={t('detail.pipelineMethod')}
                            values={methodRuntimeMW.length > 0 ? methodRuntimeMW : methDeclMW}
                            removed={methDisMW}
                            onClick={() => openPipelineDialog(t('detail.pipelineMethod'), methodRuntimeMW.length > 0 ? methodRuntimeMW : methDeclMW)}
                          />
                        </div>
                      </>
                    ) : (
                      <div>
                        <PipelineStage
                          label={t('detail.pipelineWebsocket')}
                          values={wsRuntimeMW.length > 0 ? wsRuntimeMW : wsDeclMW}
                          removed={wsDisMW}
                          onClick={() => openPipelineDialog(t('detail.pipelineWebsocket'), wsRuntimeMW.length > 0 ? wsRuntimeMW : wsDeclMW)}
                        />
                      </div>
                    )}
                  </div>
                  <div className="pipeline-result">
                    <div className="pipeline-result-arrow">↓</div>
                    <PipelineStage label={t('detail.pipelineEffective')} values={effectiveMW} isEffective onClick={() => openPipelineDialog(t('detail.pipelineEffective'), effectiveMW)} />
                  </div>
                </div>
              </div>
            </details>
            <div style={{ marginTop: 2, fontSize: 12, color: 'var(--color-text-muted)' }}>
              Effective chain: {effectiveMW.length > 0 ? `${effectiveMW.length} middleware(s)` : 'none'}
            </div>
          </div>
        </Section>

        <Section title={isRoute ? 'HTTP & Contract' : 'WebSocket'} open={false}>
          {isRoute && (
            <Row label="OpenAPI">
              {surface.route.has_openapi ? <span className="badge badge-blue">{t('badge.openapi')}</span> : <span className="text-faint">{t('routes.noOpenAPIShort')}</span>}
            </Row>
          )}
          {isRoute && openapi?.mode && <Row label="Validation mode"><span className="badge badge-gray" style={{ fontFamily: 'var(--font-mono)' }}>{openapi.mode}</span></Row>}
          {isRoute && openapi?.error && (
            <div className="alert alert-danger" style={{ marginTop: 8 }}>{openapi.error}</div>
          )}
        </Section>

      </div>
      <MiddlewareListDialog
        open={pipelineDialog !== null}
        onOpenChange={(open) => {
          if (!open) setPipelineDialog(null)
        }}
        title={pipelineDialog?.title ?? ''}
        description={pipelineDialog?.description ?? ''}
        items={pipelineDialog?.items ?? []}
        emptyMessage={pipelineDialogEmpty}
      />
    </div>
  )
}
