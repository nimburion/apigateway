import { GroupData, PortalSurfaceMetricSummary } from '../types'
import StatusBadge, { StatusBadgeStatus } from '../components/StatusBadge'
import { t } from '../i18n'
import { getGroupDisplayName } from '../groupDisplay'

interface Props {
  sortedFilteredRoutes: GroupData[]
  groupMetricsIndex: Record<string, PortalSurfaceMetricSummary>
  groupWindowErrorIndex: Record<string, number>
  runtimeMetricsAvailable: boolean
  selectedWindowHasTraffic: boolean
  activeFilterCount: number
  onOpenPosture: (groupName: string) => void
  onOpenMetrics: (groupName: string) => void
  onResetFilters: () => void
}

function resolveStatus(status?: string, deprecated?: boolean): StatusBadgeStatus {
  if (deprecated || status === 'deprecated') return 'deprecated'
  if (status === 'experimental') return 'experimental'
  if (status === 'disabled') return 'disabled'
  return 'active'
}

function fmt(n: number) { return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(n) }
function fmtMs(v: number) {
  if (v <= 0) return '—'
  if (v >= 1000) return `${(v / 1000).toFixed(2)}s`
  return `${v.toFixed(0)}ms`
}

export default function GroupsPage({
  sortedFilteredRoutes, groupMetricsIndex, groupWindowErrorIndex, runtimeMetricsAvailable,
  selectedWindowHasTraffic,
  activeFilterCount, onOpenPosture, onOpenMetrics, onResetFilters
}: Props) {
  if (sortedFilteredRoutes.length === 0) {
    return (
      <div className="empty-state">
        <div className="empty-state-title">{t('app.emptyTitle')}</div>
        <p className="empty-state-body">{t('app.emptyBody')}</p>
        {activeFilterCount > 0 && (
          <button type="button" className="btn btn-primary" style={{ marginTop: 16 }} onClick={onResetFilters}>
            {t('app.clearFilters')}
          </button>
        )}
      </div>
    )
  }

  const isFrameworkGroup = (group: GroupData) => {
    if (group.name === '__management__') {
      return true
    }
    const hasRoutes = group.routes.length > 0
    const runtimeRoutesOnly = hasRoutes && group.routes.every((route) => route.runtime_only || route.surface_context === 'management')
    return runtimeRoutesOnly && group.websockets.length === 0
  }

  const declaredGroups = sortedFilteredRoutes.filter((group) => !isFrameworkGroup(group))
  const frameworkGroups = sortedFilteredRoutes.filter((group) => isFrameworkGroup(group))

  const renderTable = (groups: GroupData[]) => (
    <table className="data-table">
      <thead>
        <tr>
          <th>Group</th>
          <th>Prefix</th>
          <th style={{ width: 60, textAlign: 'right' }}>HTTP</th>
          <th style={{ width: 60, textAlign: 'right' }}>WS</th>
          <th style={{ width: 80 }}>Security</th>
          <th style={{ width: 80 }}>Status</th>
          <th>Owner · Domain</th>
          {runtimeMetricsAvailable && <th style={{ width: 70, textAlign: 'right' }}>Req</th>}
          {runtimeMetricsAvailable && <th style={{ width: 70, textAlign: 'right' }}>Avg</th>}
          <th style={{ width: 130 }}></th>
        </tr>
      </thead>
      <tbody>
        {groups.map(group => {
          const metric = groupMetricsIndex[group.name]
          const windowErrorCount = groupWindowErrorIndex[group.name] ?? 0
          const hasMetadata = [group.metadata.owner_team, group.metadata.domain, group.metadata.visibility, group.metadata.status].some(Boolean)

          return (
            <tr key={group.name}>
              <td>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
                  <span style={{ fontWeight: 600, fontSize: 13 }}>{getGroupDisplayName(group.name)}</span>
                  {group.auth_required && <span className="badge badge-red" style={{ fontSize: 10 }}>Auth</span>}
                  {group.has_rate_limit && <span className="badge badge-amber" style={{ fontSize: 10 }}>Group RL</span>}
                  {!group.has_rate_limit && group.has_rate_limited_surfaces && <span className="badge badge-amber" style={{ fontSize: 10 }}>Inherited RL</span>}
                  {group.routes.some(r => r.has_openapi) && <span className="badge badge-blue" style={{ fontSize: 10 }}>{t('badge.openapi')}</span>}
                  {runtimeMetricsAvailable && windowErrorCount > 0 && (
                    <span className="badge badge-red" style={{ fontSize: 10 }}>⚠ err</span>
                  )}
                </div>
                {group.middlewares.length > 0 && (
                  <div style={{ fontSize: 11, color: 'var(--color-text-faint)', marginTop: 2 }}>MW: {group.middlewares.join(', ')}</div>
                )}
              </td>
              <td>
                <code className="path-text-muted" style={{ fontSize: 12 }}>{group.prefix || '/'}</code>
              </td>
              <td style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{group.routes.length}</td>
              <td style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{group.websockets.length}</td>
              <td>
                {group.auth_required
                  ? <span className="badge badge-red" style={{ fontSize: 10 }}>Protected</span>
                  : <span className="badge badge-green" style={{ fontSize: 10 }}>Public</span>
                }
              </td>
              <td><StatusBadge status={resolveStatus(group.metadata.status, group.deprecated)} /></td>
              <td>
                {hasMetadata ? (
                  <div style={{ fontSize: 12 }}>
                    <span className="text-muted">{group.metadata.owner_team || '—'}</span>
                    {group.metadata.domain && <span className="text-faint"> · {group.metadata.domain}</span>}
                  </div>
                ) : <span className="text-faint" style={{ fontSize: 12 }}>—</span>}
              </td>
              {runtimeMetricsAvailable && (
                <td style={{ textAlign: 'right', fontVariantNumeric: 'tabular-nums', fontSize: 12 }}>
                  {metric ? fmt(metric.requests) : '—'}
                </td>
              )}
              {runtimeMetricsAvailable && (
                <td style={{ textAlign: 'right', fontSize: 12, color: !metric ? 'var(--color-text-faint)' : metric.average_latency_ms >= 1000 ? 'var(--color-danger)' : metric.average_latency_ms >= 500 ? 'var(--color-warning)' : 'inherit' }}>
                  {metric ? fmtMs(metric.average_latency_ms) : '—'}
                </td>
              )}
              <td>
                <div style={{ display: 'flex', gap: 6 }}>
                  <button type="button" className="btn btn-primary btn-sm" onClick={() => onOpenPosture(group.name)} title={t('groupsInfo.openPosture')}>
                    {t('nav.posture')}
                  </button>
                  <button type="button" className="btn btn-secondary btn-sm" onClick={() => onOpenMetrics(group.name)} title={t('nav.metrics')}>
                    {t('nav.metrics')}
                  </button>
                </div>
              </td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )

  return (
    <div style={{ display: 'grid', gap: 18 }}>
      {!selectedWindowHasTraffic && (
        <div className="alert alert-warning" style={{ display: 'flex', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
          <div>
            <strong>{t('metrics.emptyWindowTitle')}</strong>
            <div style={{ marginTop: 4 }}>{t('metrics.emptyWindowBody')}</div>
          </div>
        </div>
      )}
      {declaredGroups.length > 0 && (
        <section>
          <div style={{ marginBottom: 10, display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
            <div>
              <div className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">{t('nav.groupsInfo')}</div>
              <h2 style={{ marginTop: 6, fontSize: 20, fontWeight: 700 }}>{t('groupsInfo.declaredTitle')}</h2>
              <p style={{ marginTop: 6, fontSize: 13, color: 'var(--color-text-muted)' }}>
                {t('groupsInfo.pageBody')}
              </p>
            </div>
            <a
              href="/api/portal/openapi.yaml"
              download="openapi.yaml"
              className="btn btn-secondary btn-sm"
            >
              {t('routes.downloadRuntimeSpec')}
            </a>
          </div>
          <div style={{ overflowX: 'auto' }}>
            {renderTable(declaredGroups)}
          </div>
        </section>
      )}

      {frameworkGroups.length > 0 && (
        <section>
          <div style={{ marginBottom: 10 }}>
            <div className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">Runtime</div>
            <h2 style={{ marginTop: 6, fontSize: 20, fontWeight: 700 }}>{t('groupsInfo.frameworkTitle')}</h2>
            <p style={{ marginTop: 6, fontSize: 13, color: 'var(--color-text-muted)' }}>
              {t('groupsInfo.frameworkBody')}
            </p>
          </div>
          <div style={{ overflowX: 'auto' }}>
            {renderTable(frameworkGroups)}
          </div>
        </section>
      )}
    </div>
  )
}
