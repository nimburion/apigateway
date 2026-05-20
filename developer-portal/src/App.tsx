import * as Dialog from '@radix-ui/react-dialog'
import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { PortalPathMetric, SelectedSurface } from './types'
import Sidebar from './components/Sidebar'
import PosturePage from './pages/PosturePage'
import GroupsPage from './pages/GroupsPage'
import AdminPage from './pages/AdminPage'
import { useCatalog } from './hooks/useCatalog'
import { useMetrics } from './hooks/useMetrics'
import { useNavigation, PortalPage } from './hooks/useNavigation'
import { useFilters } from './hooks/useFilters'
import { getLocale, setLocale, t } from './i18n'
import {
  buildGroupMetricsIndex, buildRouteMetricKey,
  buildRouteMetricsIndex,
  filterMetricsByGroup, filterMetricsBySurface,
} from './metrics'
import {
  buildMetricsTrendPoints,
  buildGroupWindowErrorIndex,
  buildWindowMetricsData,
} from './metricsHistory'
import MetricsDashboard from './components/MetricsDashboard'
import { getGroupDisplayName } from './groupDisplay'
import { useMetricsHistory } from './hooks/useMetricsHistory'
import GroupSelectorList from './components/GroupSelectorList'

type PortalTheme = 'light' | 'dark'
const defaultTrendWindowMinutes = 15
const presetTrendWindowMinutes = [5, 10, 15, 30, 45, 60, 180, 360, 720, 1440, 4320]
const primaryTrendWindowMinutes = [15, 30, 60, 360, 1440]
const comparisonWindowStorageKey = 'nimburion.portal.metrics.comparisonWindowMinutes'

function buildTrendWindowOptions(retentionMs: number): number[] {
  const retentionMinutes = Math.max(defaultTrendWindowMinutes, Math.floor(retentionMs / 60000))
  return presetTrendWindowMinutes.filter((minutes) => minutes <= retentionMinutes)
}

function buildAvailableTrendWindowOptions(history: { captured_at: string }[], retentionMs: number): number[] {
  if (history.length === 0) {
    return buildTrendWindowOptions(retentionMs)
  }

  const currentTimestamp = Date.parse(history[history.length - 1].captured_at)
  if (!Number.isFinite(currentTimestamp)) {
    return buildTrendWindowOptions(retentionMs)
  }

  const retentionMinutes = Math.max(defaultTrendWindowMinutes, Math.floor(retentionMs / 60000))
  const maxWindowMinutes = history.reduce((max, snapshot) => {
    const capturedAt = Date.parse(snapshot.captured_at)
    if (!Number.isFinite(capturedAt)) {
      return max
    }
    const ageMinutes = Math.max(1, Math.floor((currentTimestamp - capturedAt) / 60000))
    return Math.max(max, ageMinutes)
  }, defaultTrendWindowMinutes)

  return presetTrendWindowMinutes.filter((minutes) => minutes <= retentionMinutes && minutes <= maxWindowMinutes)
}

function formatTrendWindowLabel(minutes: number): string {
  if (minutes < 60) {
    return `${minutes}m`
  }
  if (minutes % 60 === 0) {
    return `${minutes / 60}h`
  }
  const hours = Math.floor(minutes / 60)
  return `${hours}h ${minutes % 60}m`
}

function readTheme(): PortalTheme {
  if (typeof window === 'undefined') return 'light'
  return window.localStorage.getItem('nimburion.portal.theme') === 'dark' ? 'dark' : 'light'
}

function readComparisonWindowMinutes(): number {
  if (typeof window === 'undefined') {
    return defaultTrendWindowMinutes
  }

  const stored = Number(window.localStorage.getItem(comparisonWindowStorageKey))
  return Number.isFinite(stored) && primaryTrendWindowMinutes.includes(stored)
    ? stored
    : defaultTrendWindowMinutes
}

function formatRelativeUpdated(value: string | null, nowMs: number): string {
  if (!value) {
    return t('metrics.notAvailable')
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return t('metrics.notAvailable')
  }
  const diffSeconds = Math.max(0, Math.round((nowMs - date.getTime()) / 1000))
  if (diffSeconds < 60) {
    return `${diffSeconds}s ago`
  }
  const diffMinutes = Math.round(diffSeconds / 60)
  if (diffMinutes < 60) {
    return `${diffMinutes}m ago`
  }
  const diffHours = Math.round(diffMinutes / 60)
  if (diffHours < 24) {
    return `${diffHours}h ago`
  }
  const diffDays = Math.round(diffHours / 24)
  return `${diffDays}d ago`
}

function LoadingSkeleton() {
  return (
    <div className="app-shell">
      <div className="app-sidebar" style={{ padding: 16 }}>
        <div className="skeleton" style={{ height: 28, width: 140, marginBottom: 24 }} />
        {[1, 2, 3, 4].map(i => <div key={i} className="skeleton" style={{ height: 32, marginBottom: 8 }} />)}
      </div>
      <div className="app-content">
        <div className="page-header"><div className="skeleton" style={{ height: 20, width: 160 }} /></div>
        <div className="page-body">
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 10, marginBottom: 20 }}>
            {[1, 2, 3, 4].map(i => <div key={i} className="skeleton" style={{ height: 72 }} />)}
          </div>
          {[1, 2, 3].map(i => <div key={i} className="skeleton" style={{ height: 48, marginBottom: 8 }} />)}
        </div>
      </div>
    </div>
  )
}

export default function App() {
  const [theme, setTheme] = useState<PortalTheme>(readTheme)
  const [locale, setLocaleState] = useState(getLocale())

  const catalog = useCatalog()
  const metrics = useMetrics()
  const nav = useNavigation()
  const { state, navigate, setFilter, resetFilters } = nav

  const [copiedLinkState, setCopied] = useState<'idle' | 'group' | 'surface'>('idle')
  const [groupSelectorOpen, setGroupSelectorOpen] = useState(false)
  const [groupQuery, setGroupQuery] = useState('')
  const [nowMs, setNowMs] = useState(() => Date.now())
  const groupDialogTitleId = useId()
  const groupTriggerRef = useRef<HTMLButtonElement | null>(null)
  const groupSearchInputRef = useRef<HTMLInputElement | null>(null)

  // Theme
  useEffect(() => {
    document.documentElement.dataset.theme = theme
    window.localStorage.setItem('nimburion.portal.theme', theme)
  }, [theme])

  // Locale
  useEffect(() => { document.documentElement.lang = locale }, [locale])

  // / shortcut
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement)?.tagName?.toLowerCase()
      if (e.key === '/' && tag !== 'input' && tag !== 'textarea') {
        e.preventDefault()
        ;(document.getElementById('portal-search-input') as HTMLInputElement | null)?.focus()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])

  useEffect(() => {
    const timer = window.setInterval(() => setNowMs(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])

  // Redirect non-existent group
  useEffect(() => {
    if (catalog.loading || !state.group) return
    if (!catalog.groups.some(g => g.name === state.group)) {
      navigate(state.page, null, null, true)
    }
  }, [catalog.groups, catalog.loading, state.group])

  const metricsHistoryState = useMetricsHistory(metrics.lastUpdated)
  const hasRuntimeMetrics = metricsHistoryState.history.length > 0
  const [comparisonWindowMinutes, setComparisonWindowMinutes] = useState(readComparisonWindowMinutes)
  const comparisonWindowOptions = useMemo(
    () => buildAvailableTrendWindowOptions(metricsHistoryState.history, metricsHistoryState.retentionMs),
    [metricsHistoryState.history, metricsHistoryState.retentionMs]
  )
  useEffect(() => {
    if (comparisonWindowOptions.length === 0) {
      return
    }
    const nextWindow = comparisonWindowOptions.includes(comparisonWindowMinutes)
      ? comparisonWindowMinutes
      : comparisonWindowOptions[comparisonWindowOptions.length - 1]
    if (nextWindow !== comparisonWindowMinutes) {
      setComparisonWindowMinutes(nextWindow)
    }
  }, [comparisonWindowMinutes, comparisonWindowOptions])
  useEffect(() => {
    if (typeof window === 'undefined') {
      return
    }
    window.localStorage.setItem(comparisonWindowStorageKey, String(comparisonWindowMinutes))
  }, [comparisonWindowMinutes])
  // Derived metrics
  const windowMetricsData = useMemo(
    () => buildWindowMetricsData(metricsHistoryState.history, catalog.routes, comparisonWindowMinutes),
    [catalog.routes, comparisonWindowMinutes, metricsHistoryState.history]
  )
  const correlated = windowMetricsData
  const defaultRoutes = catalog.routes.filter(g => g.name !== '__management__')
  const groupInfoRoutes = catalog.routes
  const effectiveCatalog = state.group ? catalog.routes : defaultRoutes
  const focusSurface = state.surface
    ? state.surface.kind === 'route' ? state.surface.route.path_prefix : state.surface.websocket.path
    : null
  const focusMethods = state.surface?.kind === 'route' ? [state.surface.method.method] : []
  const scopedMetrics = correlated
    ? filterMetricsBySurface(filterMetricsByGroup(correlated, state.group), focusSurface, focusMethods)
    : null

  const groupMetricsIndex = correlated ? buildGroupMetricsIndex(correlated) : {}
  const routeMetricsIndex = correlated ? buildRouteMetricsIndex(correlated) : {}
  const trendPoints = correlated
    ? buildMetricsTrendPoints(
      metricsHistoryState.history,
      effectiveCatalog,
      state.group,
      focusSurface,
      focusMethods,
      comparisonWindowMinutes
    )
    : []
  const groupWindowErrorIndex = useMemo(
    () => buildGroupWindowErrorIndex(metricsHistoryState.history, catalog.routes, comparisonWindowMinutes),
    [comparisonWindowMinutes, catalog.routes, metricsHistoryState.history]
  )
  const selectedWindowHasTraffic = Boolean(correlated && correlated.summary.total_requests > 0)
  const lastUpdatedLabel = formatRelativeUpdated(metrics.lastUpdated, nowMs)
  const lastUpdatedTitle = metrics.lastUpdated ?? t('metrics.notAvailable')
  const frameworkGroupNames = useMemo(() => {
    const names = new Set<string>()
    for (const group of catalog.routes) {
      if (group.name === '__management__') {
        names.add(group.name)
        continue
      }
      const hasRoutes = group.routes.length > 0
      const runtimeRoutesOnly = hasRoutes && group.routes.every((route) => route.runtime_only || route.surface_context === 'management')
      if (runtimeRoutesOnly && group.websockets.length === 0) {
        names.add(group.name)
      }
    }
    return names
  }, [catalog.routes])

  const filters = useFilters({
    routes: catalog.routes, groups: catalog.groups,
    selectedGroup: state.group, searchTerm: state.searchTerm,
    selectedTeam: state.team, selectedDomain: state.domain,
    selectedVisibility: state.visibility, selectedStatus: state.status,
    selectedMethod: state.method, selectedScopesMode: state.scopes,
    selectedProtection: state.protection, sortMode: state.sortMode,
    groupMetricsIndex,
  })

  // Surface metric
  const normPath = (p: string) => { const t = p.trim(); return t.length > 1 && t.endsWith('/') ? t.slice(0, -1) : (t.startsWith('/') ? t : '/' + t) }
  const joinPath = (a: string, b: string) => {
    const parts = [a, b].map(s => s.trim()).filter(s => s && s !== '/').map(s => s.replace(/\/+$/, ''))
    return parts.length === 0 ? '/' : normPath(parts.join('/'))
  }
  const surfaceMetric = state.surface?.kind === 'route'
    ? routeMetricsIndex[buildRouteMetricKey(state.surface.group_name, normPath(joinPath(state.surface.group_prefix, state.surface.route.path_prefix)))] ?? null
    : null

  const selectedGroupInfo = catalog.groups.find(g => g.name === state.group) ?? null
  const selectedCatalogGroup = catalog.routes.find(g => g.name === state.group) ?? null
  const selectedGroupRateLimit = selectedCatalogGroup?.has_rate_limit ? selectedCatalogGroup.rate_limit ?? null : null
  const runtimeInfo = selectedGroupInfo?.runtime_info ?? catalog.groups[0]?.runtime_info ?? {
    auth_enabled: false, management_enabled: false, management_auth_enabled: false, portal_mode: ''
  }
  const isManagementGroup = state.group === '__management__'
  const handleGroupSelect = (group: string | null) => {
    setGroupSelectorOpen(false)
    setGroupQuery('')
    navigate(state.page, group, null, true)
  }

  const openSurface = (surface: SelectedSurface) => {
    navigate('posture', surface.group_name, surface)
  }
  const closeSurface = () => {
    if (state.returnTo) {
      navigate(state.returnTo.page, state.returnTo.group, state.returnTo.surface, true)
      return
    }
    navigate('posture', state.group, null, true)
  }

  const openGroupFromMetrics = (pm: PortalPathMetric) => {
    const match = pm.primary_match
    if (!match) return
    const matchedPath = normPath(match.path_pattern)
    const observedPath = normPath(pm.path)
    const matchedMethod = (match.matched_methods[0] || '').toUpperCase()

    const routeCandidate = catalog.routes
      .flatMap((group) => group.routes.map((route) => ({ group, route })))
      .map(({ group, route }) => {
        const routeFullPath = normPath(joinPath(group.prefix, route.path_prefix))
        const routeLocalPath = normPath(route.path_prefix)
        const method = route.methods.find((candidate) =>
          matchedMethod ? candidate.method.toUpperCase() === matchedMethod : true
        ) || route.methods[0]
        const score =
          (group.name === match.group_name ? 8 : 0) +
          (routeFullPath === matchedPath ? 4 : 0) +
          (routeFullPath === observedPath ? 3 : 0) +
          (routeLocalPath === matchedPath ? 2 : 0) +
          (routeLocalPath === observedPath ? 1 : 0) +
          (method && matchedMethod && method.method.toUpperCase() === matchedMethod ? 2 : 0)

        return { group, route, method, score }
      })
      .filter((candidate) => candidate.score > 0 && candidate.method)
      .sort((left, right) => right.score - left.score)[0]

    if (!routeCandidate || !routeCandidate.method) {
      navigate('posture', match.group_name, null)
      return
    }

    openSurface({
      kind: 'route',
      id: `${routeCandidate.route.path_prefix}::${routeCandidate.method.method}`,
      group_name: routeCandidate.group.name,
      group_prefix: routeCandidate.group.prefix,
      group_middlewares: routeCandidate.group.middlewares,
      route: routeCandidate.route,
      method: routeCandidate.method
    })
  }

  const copyLink = async (path: string, kind: 'group' | 'surface') => {
    const url = `${window.location.origin}${path}`
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(url)
      setCopied(kind)
      setTimeout(() => setCopied('idle'), 1500)
    }
  }

  if (catalog.loading) return <LoadingSkeleton />

  const pageTitle = state.page === 'groups'
    ? t('nav.groupsInfo')
    : state.page === 'admin'
      ? 'Config Admin'
      : state.page === 'metrics-trend'
        ? t('metrics.trendTitle')
        : state.page === 'metrics'
          ? t('nav.metrics')
          : t('nav.posture')

  return (
    <>
      <div className="app-shell">
        <Sidebar
          groups={filters.sortedGroupOptions}
          visibleSurfaceCount={filters.sortedFilteredRoutes.reduce((total, group) => total + group.routes.length + group.websockets.length, 0)}
          selectedGroup={state.group}
          activePage={state.page}
          theme={theme}
          locale={locale}
          onNavigate={navigate}
          onThemeToggle={() => setTheme(t => t === 'dark' ? 'light' : 'dark')}
          onLocaleChange={l => { setLocale(l as 'en' | 'it'); setLocaleState(l as 'en' | 'it') }}
        />

        <div className="app-content">
          <div className="page-header">
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, minWidth: 0 }}>
              <h1 className="page-title">{pageTitle}</h1>
              {isManagementGroup && (
                <span className="badge badge-amber" style={{ letterSpacing: '0.04em' }}>
                  {t('badge.management')}
                </span>
              )}
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap', justifyContent: 'flex-end' }}>
              <div
                role="tablist"
                aria-label={t('metrics.timeWindowSelector')}
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 2,
                  padding: 4,
                  border: '1px solid var(--color-border)',
                  borderRadius: 999,
                  background: 'var(--color-surface)',
                  minHeight: 40
                }}
              >
                {primaryTrendWindowMinutes.map((minutes) => {
                  const available = comparisonWindowOptions.includes(minutes)
                  const selected = minutes === comparisonWindowMinutes
                  return (
                    <button
                      key={minutes}
                      type="button"
                      role="tab"
                      aria-selected={selected}
                      disabled={!available}
                      onClick={() => setComparisonWindowMinutes(minutes)}
                      className="btn btn-ghost btn-sm"
                      title={formatTrendWindowLabel(minutes)}
                      style={{
                        minWidth: 54,
                        minHeight: 32,
                        padding: '0 10px',
                        borderRadius: 999,
                        background: selected ? 'var(--color-accent-subtle)' : 'transparent',
                        color: selected ? 'var(--color-accent)' : 'var(--color-text-muted)',
                        opacity: available ? 1 : 0.35,
                        fontFamily: 'var(--font-mono)',
                        fontSize: 12,
                        fontWeight: selected ? 700 : 600
                      }}
                    >
                      {formatTrendWindowLabel(minutes)}
                    </button>
                  )
                })}
              </div>
              <span
                className="badge"
                title={lastUpdatedTitle}
                style={{
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: 8,
                  minHeight: 40,
                  padding: '0 12px',
                  background: 'var(--color-surface)',
                  border: '1px solid var(--color-border)',
                  borderRadius: 14,
                  color: 'var(--color-text-muted)',
                  fontFamily: 'var(--font-mono)',
                  fontSize: 12,
                  fontWeight: 600,
                  letterSpacing: '0.04em',
                  lineHeight: 1
                }}
              >
                {t('metrics.lastUpdated')} {lastUpdatedLabel}
              </span>
              <button
                type="button"
                onClick={metrics.refresh}
                aria-label={t('metrics.refresh')}
                title={t('metrics.refresh')}
                className="inline-flex h-10 items-center justify-center gap-2 rounded-full border border-slate-200 bg-white px-4 text-sm font-semibold text-slate-700 transition hover:border-slate-300 hover:text-slate-900"
              >
                <span aria-hidden="true" className={metrics.refreshing ? 'animate-spin' : ''}>↻</span>
                <span>{metrics.refreshing ? t('metrics.refreshing') : t('metrics.refresh')}</span>
              </button>
              <button
                type="button"
                className={`btn btn-secondary btn-sm${isManagementGroup ? ' group-selector-management' : ''}`}
                onClick={() => setGroupSelectorOpen(true)}
                title={t('posture.changeGroup')}
                aria-label={t('posture.changeGroup')}
                style={{ fontFamily: 'var(--font-mono)', fontSize: 12, display: 'inline-flex', alignItems: 'center', gap: 8, minHeight: 40, height: 40 }}
              >
                <span style={{ fontSize: 10, fontWeight: 700, letterSpacing: '0.06em', textTransform: 'uppercase', color: 'var(--color-text-muted)' }}>
                  {t('nav.groups')}
                </span>
                <span style={{ maxWidth: 220, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {state.group ? getGroupDisplayName(state.group) : t('groupsMenu.all')}
                </span>
                <span aria-hidden="true" style={{ fontSize: 11, color: 'var(--color-text-muted)' }}>▾</span>
                </button>
              {filters.nonGroupFilterCount > 0 && (
                <span className="badge badge-blue">{filters.nonGroupFilterCount} filters</span>
              )}
              {catalog.error && (
                <button type="button" className="btn btn-danger btn-sm" onClick={catalog.reload}>Retry</button>
              )}
            </div>
          </div>

          <div className={`page-body${isManagementGroup ? ' page-body-management' : ''}`}>
            {catalog.error && (
              <div className="alert alert-danger">
                <span>Failed to load catalog: {catalog.error}</span>
              </div>
            )}

            {state.page === 'groups' && (
              <GroupsPage
                sortedFilteredRoutes={[...groupInfoRoutes].sort((a, b) => {
                  const ad = a.name.toLowerCase() === 'default'
                  const bd = b.name.toLowerCase() === 'default'
                  if (ad && !bd) return -1
                  if (!ad && bd) return 1
                  return a.name.localeCompare(b.name)
                })}
                groupMetricsIndex={groupMetricsIndex}
                groupWindowErrorIndex={groupWindowErrorIndex}
                runtimeMetricsAvailable={hasRuntimeMetrics}
                selectedWindowHasTraffic={selectedWindowHasTraffic}
                activeFilterCount={filters.activeFilterCount}
                onOpenPosture={g => navigate('posture', g)}
                onOpenMetrics={g => navigate('metrics', g)}
                onResetFilters={resetFilters}
              />
            )}

            {state.page === 'posture' && (
              <PosturePage
                sortedFilteredRoutes={filters.sortedFilteredRoutes}
                selectedSurface={state.surface}
                runtimeInfo={runtimeInfo}
                isManagementGroup={isManagementGroup}
                runtimeMetricsAvailable={hasRuntimeMetrics}
                groupMetricsIndex={groupMetricsIndex}
                routeMetricsIndex={routeMetricsIndex}
                totals={filters.totals}
                selectedWindowHasTraffic={selectedWindowHasTraffic}
                activeFilterCount={filters.activeFilterCount}
                searchTerm={state.searchTerm} onSearchTermChange={v => setFilter('searchTerm', v)}
                sortMode={state.sortMode} onSortChange={v => setFilter('sortMode', v as any)}
                team={state.team} onTeamChange={v => setFilter('team', v)}
                domain={state.domain} onDomainChange={v => setFilter('domain', v)}
                visibility={state.visibility} onVisibilityChange={v => setFilter('visibility', v)}
                status={state.status} onStatusChange={v => setFilter('status', v)}
                method={state.method} onMethodChange={v => setFilter('method', v)}
                scopes={state.scopes} onScopesChange={v => setFilter('scopes', v)}
                protection={state.protection} onProtectionChange={v => setFilter('protection', v)}
                routeTeams={filters.routeTeams}
                routeDomains={filters.routeDomains}
                onSelectSurface={openSurface}
                onCloseSurface={closeSurface}
                onResetFilters={resetFilters}
                onRefreshMetrics={metrics.refresh}
                onCopySurfaceLink={() => { /* build surface path */ void copyLink(window.location.pathname, 'surface') }}
                copiedLinkState={copiedLinkState}
                surfaceMetric={surfaceMetric}
                selectedGroupRateLimit={selectedGroupRateLimit}
              />
            )}

            {(state.page === 'metrics' || state.page === 'metrics-trend') && (
              <MetricsDashboard
                data={scopedMetrics}
                history={metricsHistoryState.history}
                comparisonWindowMinutes={comparisonWindowMinutes}
                trendPoints={trendPoints}
                loading={metrics.loading}
                refreshing={metrics.refreshing}
                error={metrics.error}
                errorStatus={metrics.errorStatus}
                errorKind={metrics.errorKind}
                sourceUrl={metrics.sourceUrl}
                onRefresh={metrics.refresh}
                onOpenCatalogSurface={openGroupFromMetrics}
                view={state.page === 'metrics-trend' ? 'trend' : 'overview'}
              />
            )}

            {state.page === 'admin' && (
              <AdminPage />
            )}
          </div>
        </div>
      </div>

      {/* Mobile bottom nav */}
      <nav className="mobile-nav">
        {(['posture', 'groups', 'metrics', 'metrics-trend'] as PortalPage[]).map(p => (
          <button
            key={p}
            type="button"
            onClick={() => navigate(p, state.group)}
            aria-current={state.page === p ? 'page' : undefined}
            style={{
              flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center',
              gap: 2, padding: '6px 4px', background: 'none', border: 'none', cursor: 'pointer',
              color: state.page === p ? 'var(--color-accent)' : 'var(--color-text-muted)',
              fontSize: 10, fontWeight: state.page === p ? 700 : 500,
            }}
          >
            <span style={{ fontSize: 16 }}>{p === 'posture' ? '🛡' : p === 'groups' ? '📋' : p === 'metrics' ? '📊' : '📈'}</span>
            {p === 'posture'
              ? t('nav.posture')
              : p === 'groups'
                ? t('nav.groupsInfo')
                : p === 'metrics'
                  ? t('nav.metrics')
                  : t('metrics.trendTitle')}
          </button>
        ))}
        <button
          type="button"
          ref={groupTriggerRef}
          onClick={() => setGroupSelectorOpen(true)}
          style={{
            flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center',
            gap: 2, padding: '6px 4px', background: 'none', border: 'none', cursor: 'pointer',
            color: 'var(--color-text-muted)', fontSize: 10, fontWeight: 500,
          }}
        >
          <span style={{ fontSize: 16 }}>≡</span>
          {state.group ? getGroupDisplayName(state.group) : t('groupsMenu.all')}
        </button>
      </nav>

      <Dialog.Root open={groupSelectorOpen} onOpenChange={setGroupSelectorOpen}>
        <Dialog.Portal>
          <Dialog.Overlay className="drawer-overlay" />
          <Dialog.Content
            aria-labelledby={groupDialogTitleId}
            onCloseAutoFocus={(e) => {
              e.preventDefault()
              groupTriggerRef.current?.focus()
            }}
            onOpenAutoFocus={(e) => {
              e.preventDefault()
              groupSearchInputRef.current?.focus()
            }}
            style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'grid', placeItems: 'center', padding: 16 }}
          >
            <div className="card" style={{ width: 'min(620px, 100%)', maxHeight: '85vh', overflow: 'auto', padding: 20 }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, marginBottom: 16 }}>
                <div>
                  <Dialog.Title id={groupDialogTitleId} style={{ margin: 0, fontSize: 20, fontWeight: 700 }}>
                    {t('posture.changeGroup')}
                  </Dialog.Title>
                  <div style={{ marginTop: 6, fontSize: 13, color: 'var(--color-text-muted)' }}>
                    {t('posture.changeGroupBody')}
                  </div>
                </div>
                <Dialog.Close asChild>
                  <button type="button" className="btn btn-ghost" aria-label={t('detail.close')} style={{ minWidth: 44, minHeight: 44 }}>
                    ×
                  </button>
                </Dialog.Close>
              </div>

              <GroupSelectorList
                variant="dialog"
                groups={filters.sortedGroupOptions}
                selectedGroup={state.group}
                query={groupQuery}
                onQueryChange={setGroupQuery}
                onSelectGroup={handleGroupSelect}
                searchInputRef={groupSearchInputRef}
                frameworkGroupNames={frameworkGroupNames}
              />
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </>
  )
}
