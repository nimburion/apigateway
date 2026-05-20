import { useEffect, useMemo, useState } from 'react'
import { GroupData, GroupInfo, PortalSurfaceMetricSummary, SelectedSurface } from '../types'
import RoutesList from '../components/RoutesList'
import SearchBar from '../components/SearchBar'
import SurfaceDetailsPanel from '../components/SurfaceDetailsPanel'
import { t } from '../i18n'
import { CatalogSortMode } from '../hooks/useNavigation'
import { buildRouteMetricKey } from '../metrics'
import { surfacePriorityLevel, surfacePriorityReasonCodes, surfacePriorityScore, PriorityReasonCode } from '../priority'

interface Props {
  sortedFilteredRoutes: GroupData[]
  selectedSurface: SelectedSurface | null
  runtimeInfo: GroupInfo['runtime_info']
  isManagementGroup: boolean
  runtimeMetricsAvailable: boolean
  groupMetricsIndex: Record<string, PortalSurfaceMetricSummary>
  routeMetricsIndex: Record<string, PortalSurfaceMetricSummary>
  selectedGroupRateLimit?: { requests_per_second: number; burst: number; source: string } | null
  totals: { protected: number; publicHTTP: number; scopes: number; rateLimited: number; openapi: number; deprecated: number; http: number; ws: number }
  selectedWindowHasTraffic: boolean
  activeFilterCount: number
  searchTerm: string; onSearchTermChange: (v: string) => void
  sortMode: CatalogSortMode; onSortChange: (v: string) => void
  team: string; onTeamChange: (v: string) => void
  domain: string; onDomainChange: (v: string) => void
  visibility: string; onVisibilityChange: (v: string) => void
  status: string; onStatusChange: (v: string) => void
  method: string; onMethodChange: (v: string) => void
  scopes: string; onScopesChange: (v: string) => void
  protection: string; onProtectionChange: (v: string) => void
  routeTeams: string[]
  routeDomains: string[]
  onSelectSurface: (s: SelectedSurface) => void
  onCloseSurface: () => void
  onResetFilters: () => void
  onRefreshMetrics: () => void
  onCopySurfaceLink: () => void
  copiedLinkState: 'idle' | 'group' | 'surface'
  surfaceMetric: PortalSurfaceMetricSummary | null
}

export default function PosturePage({
  sortedFilteredRoutes, selectedSurface,
  runtimeInfo, isManagementGroup, runtimeMetricsAvailable,
  groupMetricsIndex, routeMetricsIndex,
  selectedGroupRateLimit = null,
  totals, selectedWindowHasTraffic, activeFilterCount,
  searchTerm, onSearchTermChange, sortMode, onSortChange,
  team, onTeamChange, domain, onDomainChange,
  visibility, onVisibilityChange, status, onStatusChange,
  method, onMethodChange, scopes, onScopesChange,
  protection, onProtectionChange,
  routeTeams, routeDomains,
  onSelectSurface, onCloseSurface, onResetFilters, onRefreshMetrics,
  onCopySurfaceLink, copiedLinkState, surfaceMetric
}: Props) {
  const warnAuth = !runtimeInfo.auth_enabled
  const warnMgmt = !runtimeInfo.management_auth_enabled
  const warningStateKey = (warnAuth || warnMgmt)
    ? `nimburion.portal.warnings.dismissed.${warnAuth ? 'auth' : 'none'}-${warnMgmt ? 'mgmt' : 'none'}`
    : null
  const [warningsDismissed, setWarningsDismissed] = useState(false)

  type PrioritySurfaceItem = {
    id: string
    surface: SelectedSurface
    title: string
    owner: string
    group: string
    reasonCode: PriorityReasonCode | null
    level: ReturnType<typeof surfacePriorityLevel>
    score: number
    traffic: number
    errorRate: number
  }

  useEffect(() => {
    if (typeof window === 'undefined' || !warningStateKey) return
    setWarningsDismissed(window.localStorage.getItem(warningStateKey) === 'true')
  }, [warningStateKey])

  const dismissWarnings = () => {
    setWarningsDismissed(true)
    if (typeof window !== 'undefined' && warningStateKey) {
      window.localStorage.setItem(warningStateKey, 'true')
    }
  }

  const priorityItems = useMemo<PrioritySurfaceItem[]>(() => {
    const items: PrioritySurfaceItem[] = []

    const joinPath = (prefix: string, path: string) => {
      const parts = [prefix, path].map(s => s.trim()).filter(s => s && s !== '/').map(s => s.replace(/\/+$/, ''))
      return parts.length === 0 ? '/' : parts.join('/')
    }

    const normPath = (p: string) => {
      const trimmed = p.trim()
      return trimmed.length > 1 && trimmed.endsWith('/') ? trimmed.slice(0, -1) : (trimmed.startsWith('/') ? trimmed : `/${trimmed}`)
    }

    for (const group of sortedFilteredRoutes) {
      for (const route of group.routes) {
        const routeMetric = routeMetricsIndex[buildRouteMetricKey(group.name, normPath(joinPath(group.prefix, route.path_prefix)))] ?? null
        const routeRequests = routeMetric?.requests ?? 0
        const routeErrors = routeMetric ? routeMetric.client_errors + routeMetric.server_errors : 0
        const routeErrorRate = routeRequests > 0 ? routeErrors / routeRequests : 0
        for (const method of route.methods) {
          const score = surfacePriorityScore({
            authRequired: method.auth_required,
            hasRateLimit: method.has_rate_limit || route.has_rate_limit,
            deprecated: route.deprecated || route.metadata.status === 'deprecated',
            experimental: route.metadata.status === 'experimental',
            hasOpenApi: route.has_openapi,
            managementSurface: route.surface_context === 'management',
            ownerTeam: route.metadata.owner_team,
            docsUrl: route.metadata.docs_url,
            runbookUrl: route.metadata.runbook_url,
            supportChannel: route.metadata.support_channel,
            requests: routeRequests,
            errorRate: routeErrorRate,
            rateLimitedResponses: routeMetric?.rate_limited_responses ?? 0,
          })
          const reasons = surfacePriorityReasonCodes({
            authRequired: method.auth_required,
            hasRateLimit: method.has_rate_limit || route.has_rate_limit,
            deprecated: route.deprecated || route.metadata.status === 'deprecated',
            experimental: route.metadata.status === 'experimental',
            hasOpenApi: route.has_openapi,
            managementSurface: route.surface_context === 'management',
            ownerTeam: route.metadata.owner_team,
            docsUrl: route.metadata.docs_url,
            runbookUrl: route.metadata.runbook_url,
            supportChannel: route.metadata.support_channel,
            requests: routeRequests,
            errorRate: routeErrorRate,
            rateLimitedResponses: routeMetric?.rate_limited_responses ?? 0,
          })

          items.push({
            id: `${group.name}:${route.path_prefix}:${method.method}`,
            surface: {
              kind: 'route',
              id: `${route.path_prefix}::${method.method}`,
              group_name: group.name,
              group_prefix: group.prefix,
              group_middlewares: group.middlewares,
              route,
              method
            },
            title: `${method.method} ${route.path_prefix}`,
            owner: route.metadata.owner_team || '—',
            group: group.name,
            reasonCode: reasons[0] ?? null,
            level: surfacePriorityLevel(score),
            score,
            traffic: routeRequests,
            errorRate: routeErrorRate,
          })
        }
      }

      for (const websocket of group.websockets) {
        const score = surfacePriorityScore({
          authRequired: websocket.auth_required,
          hasRateLimit: websocket.has_rate_limit,
          deprecated: websocket.deprecated || websocket.metadata.status === 'deprecated',
          experimental: websocket.metadata.status === 'experimental',
          hasOpenApi: false,
          managementSurface: false,
          ownerTeam: websocket.metadata.owner_team,
          docsUrl: websocket.metadata.docs_url,
          runbookUrl: websocket.metadata.runbook_url,
          supportChannel: websocket.metadata.support_channel,
        })
        const reasons = surfacePriorityReasonCodes({
          authRequired: websocket.auth_required,
          hasRateLimit: websocket.has_rate_limit,
          deprecated: websocket.deprecated || websocket.metadata.status === 'deprecated',
          experimental: websocket.metadata.status === 'experimental',
          hasOpenApi: false,
          managementSurface: false,
          ownerTeam: websocket.metadata.owner_team,
          docsUrl: websocket.metadata.docs_url,
          runbookUrl: websocket.metadata.runbook_url,
          supportChannel: websocket.metadata.support_channel,
        })

        items.push({
          id: `${group.name}:ws:${websocket.path}`,
          surface: {
            kind: 'websocket',
            id: `ws::${websocket.path}`,
            group_name: group.name,
            group_prefix: group.prefix,
            group_middlewares: group.middlewares,
            websocket
          },
          title: `WS ${websocket.path}`,
          owner: websocket.metadata.owner_team || '—',
          group: group.name,
          reasonCode: reasons[0] ?? null,
          level: surfacePriorityLevel(score),
          score,
          traffic: 0,
          errorRate: 0,
        })
      }
    }

    return items.sort((a, b) => b.score - a.score || b.traffic - a.traffic || a.title.localeCompare(b.title)).slice(0, 5)
  }, [routeMetricsIndex, sortedFilteredRoutes])

  const priorityCounts = useMemo(() => {
    return priorityItems.reduce((acc, item) => {
      acc[item.level] += 1
      return acc
    }, { critical: 0, review: 0, controlled: 0 })
  }, [priorityItems])

  const priorityReasonLabel = (code: PriorityReasonCode | null) => {
    if (!code) return ''
    return t(
      code === 'managementPublic' ? 'posture.reasonManagementPublic'
        : code === 'public' ? 'posture.reasonPublic'
          : code === 'noRateLimit' ? 'posture.reasonNoRateLimit'
            : code === 'deprecated' ? 'posture.reasonDeprecated'
              : code === 'experimental' ? 'posture.reasonExperimental'
                : code === 'missingOwner' ? 'posture.reasonMissingOwner'
                  : code === 'missingDocs' ? 'posture.reasonMissingDocs'
                    : code === 'missingRunbook' ? 'posture.reasonMissingRunbook'
                      : code === 'missingSupport' ? 'posture.reasonMissingSupport'
                        : code === 'highTraffic' ? 'posture.reasonHighTraffic'
                          : code === 'highErrors' ? 'posture.reasonHighErrors'
                            : code === 'rateLimited' ? 'posture.reasonRateLimited'
                              : 'posture.reasonOpenApiMissing'
    )
  }

  return (
    <>
      {isManagementGroup && (
        <div className="alert management-banner" style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
          <div style={{ minWidth: 0, display: 'grid', gap: 8 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
              <span style={{ fontSize: 16, flexShrink: 0 }}>▣</span>
              <strong>{t('posture.managementContextTitle')}</strong>
              <span className="badge badge-amber">{t('badge.management')}</span>
            </div>
            <div style={{ color: 'var(--color-text-muted)' }}>
              {t('posture.managementContextBody')}
            </div>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
            <span className={`badge ${runtimeInfo.management_auth_enabled ? 'badge-blue' : 'badge-amber'}`}>
              {runtimeInfo.management_auth_enabled ? t('authContext.managementProtected') : t('authContext.managementPublic')}
            </span>
          </div>
        </div>
      )}

      {(warnAuth || warnMgmt) && !warningsDismissed && (
        <div className="alert alert-warning" style={{ position: 'relative', display: 'block', paddingRight: 52 }}>
          <div style={{ minWidth: 0, display: 'grid', gap: 8 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, paddingRight: 8 }}>
              <span style={{ fontSize: 16, flexShrink: 0 }}>⚠</span>
              <strong>{t('warnings.title')}</strong>
            </div>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, paddingRight: 8 }}>
              <span className={`badge ${runtimeInfo.auth_enabled ? 'badge-red' : 'badge-green'}`}>
                {runtimeInfo.auth_enabled ? t('authContext.gatewayProtected') : t('authContext.gatewayPublic')}
              </span>
              <span className={`badge ${runtimeInfo.management_auth_enabled ? 'badge-blue' : 'badge-amber'}`}>
                {runtimeInfo.management_auth_enabled ? t('authContext.managementProtected') : t('authContext.managementPublic')}
              </span>
              {runtimeInfo.portal_mode && (
                <span className="badge badge-gray">{t('authContext.portalMode')}: {runtimeInfo.portal_mode}</span>
              )}
            </div>
            <ul style={{ margin: 0, paddingLeft: 16 }}>
              {warnAuth && <li>{t('warnings.gatewayAuthDisabled')}</li>}
              {warnMgmt && <li>{t('warnings.managementAuthDisabled')}</li>}
            </ul>
          </div>
          <button
            type="button"
            className="btn btn-secondary btn-sm drawer-close-btn"
            style={{ position: 'absolute', top: 12, right: 12 }}
            onClick={dismissWarnings}
            aria-label={t('warnings.dismiss')}
            title={t('warnings.dismiss')}
          >
            <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false" width="18" height="18">
              <path d="M6 6l12 12M18 6L6 18" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" />
            </svg>
          </button>
        </div>
      )}

      {/* KPI row */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, marginBottom: 16, flexWrap: 'wrap' }}>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 sm:gap-3" style={{ flex: '1 1 560px', minWidth: 0 }}>
          <div className="stat-card">
            <div className="stat-card-label">{t('posture.publicHttp')}</div>
            <div className="stat-card-value">{totals.publicHTTP}</div>
          </div>
          <div className="stat-card">
            <div className="stat-card-label">{t('posture.protectedHttp')}</div>
            <div className="stat-card-value">{totals.protected}</div>
          </div>
          <div className="stat-card">
            <div className="stat-card-label">{t('posture.uniqueScopes')}</div>
            <div className="stat-card-value">{totals.scopes}</div>
          </div>
          <div className="stat-card">
            <div className="stat-card-label">{t('posture.rateLimited')}</div>
            <div className="stat-card-value">{totals.rateLimited}</div>
          </div>
        </div>
        <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', flexShrink: 0 }}>
          {selectedSurface && (
            <button type="button" className="btn btn-secondary btn-sm" onClick={onCopySurfaceLink}>
              {copiedLinkState === 'surface' ? t('posture.linkCopied') : t('posture.copySurfaceLink')}
            </button>
          )}
        </div>
      </div>

      {!selectedWindowHasTraffic && (
        <div className="alert alert-warning" style={{ display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap', marginBottom: 16 }}>
          <div>
            <strong>{t('metrics.emptyWindowTitle')}</strong>
            <div style={{ marginTop: 4 }}>{t('metrics.emptyWindowBody')}</div>
          </div>
        </div>
      )}

      {!selectedSurface && priorityItems.length > 0 && (
        <div className="card" style={{ marginBottom: 16, padding: 16 }}>
          <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
            <div>
              <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', color: 'var(--color-text-muted)' }}>
                {t('posture.priorityTitle')}
              </div>
              <div style={{ marginTop: 4, fontSize: 14, fontWeight: 600, color: 'var(--color-text)' }}>
                {t('posture.priorityBody')}
              </div>
            </div>
            <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
              <span className="badge badge-red">{t('posture.priorityCritical')}: {priorityCounts.critical}</span>
              <span className="badge badge-amber">{t('posture.priorityNeedsReview')}: {priorityCounts.review}</span>
              <span className="badge badge-green">{t('posture.priorityControlled')}: {priorityCounts.controlled}</span>
            </div>
          </div>

          <div style={{ display: 'grid', gap: 8, marginTop: 14 }}>
            {priorityItems.map((item) => {
              const badgeClass = item.level === 'critical'
                ? 'badge-red'
                : item.level === 'review'
                  ? 'badge-amber'
                  : 'badge-green'
              return (
                <button
                  key={item.id}
                  type="button"
                  onClick={() => onSelectSurface(item.surface)}
                  className="card"
                  style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1fr) auto', gap: 12, padding: 12, textAlign: 'left', cursor: 'pointer', alignItems: 'center' }}
                >
                  <div style={{ minWidth: 0, display: 'grid', gap: 6 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                      <span className={`badge ${badgeClass}`}>{t(item.level === 'critical' ? 'posture.priorityCritical' : item.level === 'review' ? 'posture.priorityNeedsReview' : 'posture.priorityControlled')}</span>
                      <strong style={{ fontSize: 13, color: 'var(--color-text)' }}>{item.title}</strong>
                      {item.reasonCode && <span className="badge badge-gray">{priorityReasonLabel(item.reasonCode)}</span>}
                    </div>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, alignItems: 'center', fontSize: 12, color: 'var(--color-text-muted)' }}>
                      <span>{t('routes.owner')}: {item.owner}</span>
                      <span>·</span>
                      <span>{t('metrics.requests')}: {item.traffic}</span>
                      <span>·</span>
                      <span>{t('metrics.errorRate')}: {(item.errorRate * 100).toFixed(item.errorRate >= 0.1 ? 0 : 1)}%</span>
                    </div>
                  </div>
                  <span className="btn btn-secondary btn-sm" style={{ minHeight: 36, padding: '0 14px' }}>
                    {t('posture.priorityReview')}
                  </span>
                </button>
              )
            })}
          </div>
        </div>
      )}

      {selectedSurface && (
        <div style={{ marginBottom: 20 }}>
          <SurfaceDetailsPanel
            surface={selectedSurface}
            surfaceMetric={surfaceMetric}
            frameworkMiddlewares={runtimeInfo.framework_middlewares}
            groupRateLimit={selectedGroupRateLimit}
            runtimeMetricsAvailable={runtimeMetricsAvailable}
            onRefreshMetrics={onRefreshMetrics}
            onClose={onCloseSurface}
            mode="panel"
          />
        </div>
      )}

      {!selectedSurface && (
        <>
          {/* Search & filter toolbar */}
          <SearchBar
            label={t('search.searchLabel')}
            searchTerm={searchTerm} onSearchTermChange={onSearchTermChange}
            sort={sortMode} onSortChange={onSortChange}
            team={team} onTeamChange={onTeamChange}
            domain={domain} onDomainChange={onDomainChange}
            visibility={visibility} onVisibilityChange={onVisibilityChange}
            status={status} onStatusChange={onStatusChange}
            method={method} onMethodChange={onMethodChange}
            scopes={scopes} onScopesChange={onScopesChange}
            protection={protection} onProtectionChange={onProtectionChange}
            teams={routeTeams} domains={routeDomains}
            onReset={onResetFilters}
          />

          {/* Route list */}
          {sortedFilteredRoutes.length > 0 ? (
            sortedFilteredRoutes.map(group => (
              <RoutesList
                key={group.name}
                group={group}
                sortMode={sortMode}
                runtimeMetricsAvailable={runtimeMetricsAvailable}
                groupMetric={groupMetricsIndex[group.name] ?? null}
                routeMetrics={routeMetricsIndex}
                selectedSurfaceId={undefined}
                onSelectSurface={onSelectSurface}
                searchTerm={searchTerm}
              />
            ))
          ) : (
            <div className="empty-state">
              <div className="empty-state-title">{t('app.emptyTitle')}</div>
              <p className="empty-state-body">{t('app.emptyBody')}</p>
              {activeFilterCount > 0 && (
                <button type="button" className="btn btn-primary" style={{ marginTop: 16 }} onClick={onResetFilters}>
                  {t('app.clearFilters')}
                </button>
              )}
            </div>
          )}
        </>
      )}
    </>
  )
}
