import { useMemo } from 'react'
import { GroupData, GroupInfo, PortalSurfaceMetricSummary } from '../types'
import { CatalogSortMode } from './useNavigation'

interface FilterParams {
  routes: GroupData[]
  groups: GroupInfo[]
  selectedGroup: string | null
  searchTerm: string
  selectedTeam: string
  selectedDomain: string
  selectedVisibility: string
  selectedStatus: string
  selectedMethod: string
  selectedScopesMode: string
  selectedProtection: string
  sortMode: CatalogSortMode
  groupMetricsIndex: Record<string, PortalSurfaceMetricSummary>
}

function isFrameworkGroup(group: GroupData) {
  if (group.name === '__management__') return true
  const hasRoutes = group.routes.length > 0
  const runtimeRoutesOnly = hasRoutes && group.routes.every((route) => route.runtime_only || route.surface_context === 'management')
  return runtimeRoutesOnly && group.websockets.length === 0
}

export function useFilters(params: FilterParams) {
  const {
    routes, groups, selectedGroup, searchTerm,
    selectedTeam, selectedDomain, selectedVisibility,
    selectedStatus, selectedMethod, selectedScopesMode,
    selectedProtection, sortMode, groupMetricsIndex,
  } = params

  const routeTeams = useMemo(() =>
    Array.from(new Set(routes.flatMap(g => [
      g.metadata.owner_team,
      ...g.routes.map(r => r.metadata.owner_team),
      ...g.websockets.map(w => w.metadata.owner_team),
    ].filter(Boolean)))).sort((a, b) => a.localeCompare(b)),
  [routes])

  const routeDomains = useMemo(() =>
    Array.from(new Set(routes.flatMap(g => [
      g.metadata.domain,
      ...g.routes.map(r => r.metadata.domain),
      ...g.websockets.map(w => w.metadata.domain),
    ].filter(Boolean)))).sort((a, b) => a.localeCompare(b)),
  [routes])

  const matchesMeta = (m: GroupData['metadata'], deprecated = false) => {
    if (selectedTeam && m.owner_team !== selectedTeam) return false
    if (selectedDomain && m.domain !== selectedDomain) return false
    if (selectedVisibility && m.visibility !== selectedVisibility) return false
    const status = deprecated ? 'deprecated' : m.status
    if (selectedStatus && status !== selectedStatus) return false
    return true
  }

  const matchesProtection = (auth: boolean) => {
    if (selectedProtection === 'public') return !auth
    if (selectedProtection === 'protected') return auth
    return true
  }

  const matchesScopes = (scopes: string[]) => {
    if (selectedScopesMode === 'with') return scopes.length > 0
    if (selectedScopesMode === 'without') return scopes.length === 0
    return true
  }

  const trafficOf = (name: string) => groupMetricsIndex[name]?.requests ?? 0
  const errorRateOf = (name: string) => {
    const m = groupMetricsIndex[name]
    if (!m || m.requests <= 0) return 0
    return (m.client_errors + m.server_errors) / m.requests
  }
  const riskOf = (item: { auth_required?: boolean; has_rate_limit?: boolean; deprecated?: boolean; metadata?: { status?: string } }) => {
    let s = 0
    if (item.auth_required) s += 2
    if (item.has_rate_limit) s += 1
    if (item.deprecated || item.metadata?.status === 'deprecated') s += 3
    if (item.metadata?.status === 'experimental') s += 2
    return s
  }

  const compareByMode = <T extends {
    name: string; metadata?: { owner_team?: string; status?: string }
    route_count?: number; websocket_count?: number; routes?: unknown[]; websockets?: unknown[]
    auth_required?: boolean; has_rate_limit?: boolean; deprecated?: boolean
  }>(a: T, b: T): number => {
    if (sortMode === 'owner') {
      const d = (a.metadata?.owner_team ?? '').localeCompare(b.metadata?.owner_team ?? '')
      return d !== 0 ? d : a.name.localeCompare(b.name)
    }
    if (sortMode === 'traffic') {
      const d = trafficOf(b.name) - trafficOf(a.name)
      return d !== 0 ? d : a.name.localeCompare(b.name)
    }
    if (sortMode === 'errorRate') {
      const d = errorRateOf(b.name) - errorRateOf(a.name)
      return d !== 0 ? d : trafficOf(b.name) - trafficOf(a.name) || a.name.localeCompare(b.name)
    }
    if (sortMode === 'risk') {
      const d = riskOf(b) - riskOf(a)
      return d !== 0 ? d : a.name.localeCompare(b.name)
    }
    if (sortMode === 'surface') {
      const as_ = (a.route_count ?? a.routes?.length ?? 0) + (a.websocket_count ?? a.websockets?.length ?? 0)
      const bs_ = (b.route_count ?? b.routes?.length ?? 0) + (b.websocket_count ?? b.websockets?.length ?? 0)
      return (bs_ - as_) || a.name.localeCompare(b.name)
    }
    const ad = a.name.toLowerCase() === 'default', bd = b.name.toLowerCase() === 'default'
    if (ad && !bd) return -1; if (!ad && bd) return 1
    return a.name.localeCompare(b.name)
  }

  const filteredRoutes = useMemo(() => {
    const term = searchTerm.toLowerCase()
    return routes.map(group => {
      const visible = selectedGroup ? group.name === selectedGroup : !isFrameworkGroup(group)
      if (!visible) return null

      const routeMatches = group.routes.filter(route => {
        const methods = route.methods.filter(m => {
          if (selectedMethod && m.method !== selectedMethod) return false
          if (!matchesProtection(m.auth_required)) return false
          if (!matchesScopes(m.scopes)) return false
          return true
        })
        if (!methods.length) return false
        const matchSearch = !searchTerm ||
          group.name.toLowerCase().includes(term) ||
          group.prefix.toLowerCase().includes(term) ||
          route.path_prefix.toLowerCase().includes(term) ||
          methods.some(m => m.method.toLowerCase().includes(term) || m.scopes.some(s => s.toLowerCase().includes(term))) ||
          route.metadata.owner_team.toLowerCase().includes(term) ||
          route.metadata.domain.toLowerCase().includes(term)
        return matchSearch && matchesMeta(route.metadata, route.deprecated)
      })

      const wsMatches = group.websockets.filter(ws => {
        if (selectedMethod) return false
        if (!matchesProtection(ws.auth_required)) return false
        if (!matchesScopes(ws.scopes)) return false
        const matchSearch = !searchTerm ||
          group.name.toLowerCase().includes(term) ||
          ws.path.toLowerCase().includes(term) ||
          ws.scopes.some(s => s.toLowerCase().includes(term))
        return matchSearch && matchesMeta(ws.metadata, ws.deprecated)
      })

      const groupMatchSearch = !searchTerm ||
        group.name.toLowerCase().includes(term) ||
        group.prefix.toLowerCase().includes(term)
      const groupMatchMeta = matchesMeta(group.metadata, group.deprecated)

      if (!groupMatchSearch && !routeMatches.length && !wsMatches.length) return null
      if (!groupMatchMeta && !routeMatches.length && !wsMatches.length) return null

      return {
        ...group,
        routes: routeMatches.map(r => ({
          ...r,
          methods: r.methods.filter(m => {
            if (selectedMethod && m.method !== selectedMethod) return false
            if (!matchesProtection(m.auth_required)) return false
            if (!matchesScopes(m.scopes)) return false
            return true
          }),
        })),
        websockets: wsMatches,
      }
    }).filter((g): g is GroupData =>
      g !== null && (g.routes.length > 0 || g.websockets.length > 0 || matchesMeta(g.metadata, g.deprecated))
    )
  }, [routes, selectedGroup, searchTerm, selectedTeam, selectedDomain, selectedVisibility, selectedStatus, selectedMethod, selectedScopesMode, selectedProtection])

  const sortedFilteredRoutes = useMemo(() => [...filteredRoutes].sort(compareByMode), [filteredRoutes, sortMode, groupMetricsIndex])

  const filteredGroups = useMemo(() => {
    const term = searchTerm.toLowerCase()
    return groups.filter(g => {
      if (selectedGroup && g.name !== selectedGroup) return false
      if (!selectedGroup) {
        const routeGroup = routes.find((route) => route.name === g.name)
        if (routeGroup && isFrameworkGroup(routeGroup)) return false
      }
      const matchSearch = !searchTerm ||
        g.name.toLowerCase().includes(term) ||
        g.prefix.toLowerCase().includes(term) ||
        g.metadata.owner_team.toLowerCase().includes(term) ||
        g.metadata.domain.toLowerCase().includes(term)
      const hasVisible = filteredRoutes.some(r => r.name === g.name)
      return matchSearch && matchesMeta(g.metadata, g.deprecated) && hasVisible
    })
  }, [groups, routes, selectedGroup, searchTerm, filteredRoutes])

  const sortedFilteredGroups = useMemo(() => [...filteredGroups].sort(compareByMode), [filteredGroups, sortMode, groupMetricsIndex])
  const sortedGroupOptions = useMemo(() => [...groups].sort(compareByMode), [groups, sortMode, groupMetricsIndex])

  const totals = useMemo(() => {
    const protected_ = filteredRoutes.reduce((n, g) =>
      n + g.routes.filter(r => r.auth_required).length + g.websockets.filter(w => w.auth_required).length, 0)
    const publicHTTP = filteredRoutes.reduce((n, g) => n + g.routes.filter(r => !r.auth_required).length, 0)
    const scopes = new Set(filteredRoutes.flatMap(g => g.routes.flatMap(r => r.methods.flatMap(m => m.scopes)))).size
    const rateLimited = filteredRoutes.reduce((n, g) =>
      n + g.routes.filter(r => r.has_rate_limit || r.methods.some(m => m.has_rate_limit)).length +
          g.websockets.filter(w => w.has_rate_limit).length, 0)
    const openapi = filteredRoutes.reduce((n, g) => n + g.routes.filter(r => r.has_openapi).length, 0)
    const deprecated = filteredRoutes.reduce((n, g) => n + g.routes.filter(r => r.deprecated).length, 0)
    const http = filteredRoutes.reduce((n, g) => n + g.routes.length, 0)
    const ws = filteredRoutes.reduce((n, g) => n + g.websockets.length, 0)
    const experimental = filteredRoutes.reduce((n, g) =>
      n + g.routes.filter(r => r.metadata.status === 'experimental').length +
          g.websockets.filter(w => w.metadata.status === 'experimental').length, 0)
    return { protected: protected_, publicHTTP, scopes, rateLimited, openapi, deprecated, http, ws, experimental }
  }, [filteredRoutes])

  const activeFilterCount = [
    searchTerm, selectedTeam, selectedDomain, selectedVisibility,
    selectedStatus, selectedMethod, selectedScopesMode, selectedProtection, selectedGroup,
  ].filter(Boolean).length

  const nonGroupFilterCount = [
    searchTerm, selectedTeam, selectedDomain, selectedVisibility,
    selectedStatus, selectedMethod, selectedScopesMode, selectedProtection,
  ].filter(Boolean).length

  return {
    filteredRoutes,
    sortedFilteredRoutes,
    sortedFilteredGroups,
    sortedGroupOptions,
    totals,
    activeFilterCount,
    nonGroupFilterCount,
    routeTeams,
    routeDomains,
  }
}
