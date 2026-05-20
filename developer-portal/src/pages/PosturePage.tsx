import { useEffect, useState } from 'react'
import { GroupData, GroupInfo, PortalSurfaceMetricSummary, SelectedSurface } from '../types'
import RoutesList from '../components/RoutesList'
import SearchBar from '../components/SearchBar'
import SurfaceDetailsPanel from '../components/SurfaceDetailsPanel'
import { t } from '../i18n'
import { CatalogSortMode } from '../hooks/useNavigation'

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
