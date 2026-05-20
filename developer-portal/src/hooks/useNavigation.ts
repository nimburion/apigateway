import { useCallback, useEffect, useState } from 'react'
import { SelectedSurface } from '../types'

export type PortalPage = 'posture' | 'groups' | 'metrics' | 'metrics-trend' | 'admin'
export type CatalogSortMode = 'default' | 'owner' | 'risk' | 'surface' | 'traffic' | 'errorRate'

function encodeToken(v: string) {
  if (typeof window === 'undefined') return v
  return window.btoa(v).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}
function decodeToken(v: string) {
  if (!v || typeof window === 'undefined') return ''
  return window.atob(v.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(v.length / 4) * 4, '='))
}

export function buildPath(page: PortalPage, group: string | null, surface?: SelectedSurface | null) {
  if (page === 'groups') return group ? `/portal/groups/${encodeURIComponent(group)}` : '/portal/groups'
  if (page === 'metrics') return group ? `/portal/metrics/${encodeURIComponent(group)}` : '/portal/metrics'
  if (page === 'metrics-trend') return group ? `/portal/metrics/trend/${encodeURIComponent(group)}` : '/portal/metrics/trend'
  if (page === 'admin') return '/portal/admin'
  if (group && surface?.kind === 'route')
    return `/portal/posture/${encodeURIComponent(group)}/route/${encodeURIComponent(surface.method.method)}/${encodeToken(surface.route.path_prefix)}`
  if (group && surface?.kind === 'websocket')
    return `/portal/posture/${encodeURIComponent(group)}/websocket/${encodeToken(surface.websocket.path)}`
  return group ? `/portal/posture/${encodeURIComponent(group)}` : '/portal/posture'
}

function readPath() {
  if (typeof window === 'undefined') return { page: 'groups' as PortalPage, group: '', surfaceKind: '', surfaceMethod: '', surfacePath: '' }
  const segs = window.location.pathname.replace(/\/+$/, '').split('/').filter(Boolean)
  const pi = segs.indexOf('portal')
  if (pi === -1) return { page: 'groups' as PortalPage, group: '', surfaceKind: '', surfaceMethod: '', surfacePath: '' }
  const page = segs[pi + 1] as PortalPage | undefined
  const second = segs[pi + 2] ?? ''
  const third = segs[pi + 3] ?? ''
  const fourth = segs[pi + 4] ?? ''
  const group = second ? decodeURIComponent(second) : ''
  if (page === 'groups') return { page: 'groups' as PortalPage, group, surfaceKind: '', surfaceMethod: '', surfacePath: '' }
  if (page === 'admin') return { page: 'admin' as PortalPage, group: '', surfaceKind: '', surfaceMethod: '', surfacePath: '' }
  if (page === 'metrics' && second === 'trend') return { page: 'metrics-trend' as PortalPage, group: third ? decodeURIComponent(third) : '', surfaceKind: '', surfaceMethod: '', surfacePath: '' }
  if (page === 'metrics') return { page: 'metrics' as PortalPage, group, surfaceKind: '', surfaceMethod: '', surfacePath: '' }
  if (page === 'posture') {
    if (second === 'route') return { page: 'posture' as PortalPage, group, surfaceKind: 'route', surfaceMethod: decodeURIComponent(third), surfacePath: decodeToken(fourth) }
    if (second === 'websocket') return { page: 'posture' as PortalPage, group, surfaceKind: 'websocket', surfaceMethod: '', surfacePath: decodeToken(third) }
    return { page: 'posture' as PortalPage, group, surfaceKind: '', surfaceMethod: '', surfacePath: '' }
  }
  return { page: 'groups' as PortalPage, group: '', surfaceKind: '', surfaceMethod: '', surfacePath: '' }
}

export function buildQuery(params: {
  sort: CatalogSortMode; q: string; team: string; domain: string
  visibility: string; status: string; method: string; scopes: string; protection: string
}) {
  const p = new URLSearchParams()
  if (params.sort !== 'default') p.set('sort', params.sort)
  if (params.q.trim()) p.set('q', params.q.trim())
  if (params.team) p.set('team', params.team)
  if (params.domain) p.set('domain', params.domain)
  if (params.visibility) p.set('visibility', params.visibility)
  if (params.status) p.set('status', params.status)
  if (params.method) p.set('method', params.method)
  if (params.scopes) p.set('scopes', params.scopes)
  if (params.protection) p.set('protection', params.protection)
  return p.toString()
}

interface NavState {
  page: PortalPage
  group: string | null
  surface: SelectedSurface | null
  returnTo: {
    page: PortalPage
    group: string | null
    surface: SelectedSurface | null
  } | null
  sortMode: CatalogSortMode
  searchTerm: string
  team: string; domain: string; visibility: string
  status: string; method: string; scopes: string; protection: string
  pathSurface: { kind: string; path: string; method: string }
}

export function useNavigation() {
  function readInitial(): NavState {
    if (typeof window === 'undefined') return {
      page: 'groups', group: null, surface: null, returnTo: null, sortMode: 'default',
      searchTerm: '', team: '', domain: '', visibility: '', status: '',
      method: '', scopes: '', protection: '', pathSurface: { kind: '', path: '', method: '' }
    }
    const ps = readPath()
    const p = new URLSearchParams(window.location.search)
    const sort = p.get('sort')
    return {
      page: ps.page,
      group: ps.group || null,
      surface: null,
      returnTo: null,
      sortMode: ['owner','risk','surface','traffic','errorRate'].includes(sort ?? '') ? sort as CatalogSortMode : 'default',
      searchTerm: p.get('q') ?? '',
      team: p.get('team') ?? '',
      domain: p.get('domain') ?? '',
      visibility: p.get('visibility') ?? '',
      status: p.get('status') ?? '',
      method: p.get('method') ?? '',
      scopes: p.get('scopes') ?? '',
      protection: p.get('protection') ?? '',
      pathSurface: { kind: ps.surfaceKind, path: ps.surfacePath, method: ps.surfaceMethod },
    }
  }

  const [state, setState] = useState<NavState>(readInitial)

  useEffect(() => {
    if (typeof window === 'undefined') return
    const handler = () => setState(readInitial())
    window.addEventListener('popstate', handler)
    return () => window.removeEventListener('popstate', handler)
  }, [])

  // Sync URL whenever filter state changes
  useEffect(() => {
    if (typeof window === 'undefined') return
    const path = buildPath(state.page, state.group, state.surface)
    const qs = buildQuery({
      sort: state.sortMode, q: state.searchTerm, team: state.team,
      domain: state.domain, visibility: state.visibility, status: state.status,
      method: state.method, scopes: state.scopes, protection: state.protection,
    })
    const next = `${path}${qs ? `?${qs}` : ''}`
    const cur = `${window.location.pathname}${window.location.search}`
    if (cur !== next) window.history.replaceState(window.history.state ?? {}, '', next)
  }, [state.page, state.group, state.surface, state.sortMode, state.searchTerm,
      state.team, state.domain, state.visibility, state.status,
      state.method, state.scopes, state.protection])

  const navigate = useCallback((
    page: PortalPage,
    group: string | null,
    surface: SelectedSurface | null = null,
    replace = false
  ) => {
    const path = buildPath(page, group, surface)
    const current = typeof window !== 'undefined' ? `${window.location.pathname}${window.location.search}` : ''
    setState(prev => ({
      ...prev,
      page, group, surface,
      returnTo: surface ? { page: prev.page, group: prev.group, surface: prev.surface } : null,
      searchTerm: prev.group !== group ? '' : prev.searchTerm,
      pathSurface: {
        kind: surface?.kind ?? '',
        path: surface?.kind === 'route' ? surface.route.path_prefix : surface?.kind === 'websocket' ? surface.websocket.path : '',
        method: surface?.kind === 'route' ? surface.method.method : '',
      }
    }))
    if (typeof window !== 'undefined') {
      const cur = window.location.pathname.replace(/\/+$/, '') || '/'
      const tgt = path.replace(/\/+$/, '') || '/'
      if (cur !== tgt) {
        const historyState = surface ? { returnTo: current } : {}
        replace ? window.history.replaceState(historyState, '', path) : window.history.pushState(historyState, '', path)
      }
    }
  }, [])

  const setFilter = useCallback(<K extends keyof NavState>(key: K, value: NavState[K]) => {
    setState(prev => ({ ...prev, [key]: value }))
  }, [])

  const resetFilters = useCallback(() => {
    setState(prev => ({
      ...prev,
      searchTerm: '', team: '', domain: '', visibility: '',
      status: '', method: '', scopes: '', protection: '',
      group: null, surface: null,
    }))
    if (typeof window !== 'undefined') window.history.replaceState({}, '', buildPath('groups', null))
  }, [])

  return { state, navigate, setFilter, resetFilters }
}
