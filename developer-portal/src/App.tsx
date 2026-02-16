import { useState, useEffect } from 'react'
import { GroupInfo, GroupData } from './types'
import GroupCard from './components/GroupCard'
import RoutesList from './components/RoutesList'
import SearchBar from './components/SearchBar'

function App() {
  const [groups, setGroups] = useState<GroupInfo[]>([])
  const [routes, setRoutes] = useState<GroupData[]>([])
  const [loading, setLoading] = useState(true)
  const [searchTerm, setSearchTerm] = useState('')
  const [selectedGroup, setSelectedGroup] = useState<string | null>(null)

  useEffect(() => {
    const ensureString = (value: unknown): string =>
      typeof value === 'string' ? value : ''

    const normalizeGroup = (rawGroup: unknown): GroupData => {
      const group = (rawGroup ?? {}) as Partial<GroupData>
      return {
        name: ensureString(group.name),
        prefix: ensureString(group.prefix),
        routes: Array.isArray(group.routes) ? group.routes : [],
        websockets: Array.isArray(group.websockets) ? group.websockets : []
      }
    }

    const normalizeRouteGroups = (rawGroups: unknown): GroupData[] =>
      Array.isArray(rawGroups) ? rawGroups.map(normalizeGroup) : []

    Promise.all([
      fetch('/api/portal/groups').then(r => r.json()),
      fetch('/api/portal/routes').then(r => r.json())
    ])
      .then(([groupsData, routesData]) => {
        const normalizedRoutes = normalizeRouteGroups(routesData?.groups)
        setGroups(Array.isArray(groupsData?.groups) ? groupsData.groups : [])
        setRoutes(normalizedRoutes)
      })
      .catch((error) => {
        console.error('Errore durante il caricamento dei dati del portale.', error)
        setGroups([])
        setRoutes([])
      })
      .finally(() => {
        setLoading(false)
      })
  }, [])

  const filteredRoutes = routes.filter(group => {
    if (selectedGroup && group.name !== selectedGroup) return false
    if (!searchTerm) return true
    
    const term = searchTerm.toLowerCase()
    return group.name.toLowerCase().includes(term) ||
           group.prefix.toLowerCase().includes(term) ||
           group.routes.some(r => r.path_prefix.toLowerCase().includes(term))
  })
  const sortGroupsDefaultFirst = <T extends { name: string }>(items: T[]) => {
    return [...items].sort((a, b) => {
      const aIsDefault = a.name.toLowerCase() === 'default'
      const bIsDefault = b.name.toLowerCase() === 'default'
      if (aIsDefault && !bIsDefault) return -1
      if (!aIsDefault && bIsDefault) return 1
      return a.name.localeCompare(b.name)
    })
  }

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600 mx-auto"></div>
          <p className="mt-4 text-gray-600">Caricamento...</p>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="bg-white shadow-sm border-b">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
          <h1 className="text-3xl font-bold text-gray-900 flex items-center gap-3">
            <span className="text-4xl">🚀</span>
            API Gateway - Developer Portal
          </h1>
        </div>
      </header>

      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <section className="mb-8">
          <h2 className="text-2xl font-semibold text-gray-800 mb-4">📦 Route Groups</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {sortGroupsDefaultFirst(groups).map(group => (
              <GroupCard 
                key={group.name} 
                group={group}
                isSelected={selectedGroup === group.name}
                onClick={() => setSelectedGroup(selectedGroup === group.name ? null : group.name)}
              />
            ))}
          </div>
        </section>

        <section>
          <div className="flex justify-between items-center mb-4">
            <h2 className="text-2xl font-semibold text-gray-800">🔹 API Routes</h2>
            {selectedGroup && (
              <button
                onClick={() => setSelectedGroup(null)}
                className="text-sm text-blue-600 hover:text-blue-800"
              >
                Mostra tutti
              </button>
            )}
          </div>
          
          <SearchBar value={searchTerm} onChange={setSearchTerm} />
          
          <div className="space-y-6">
            {sortGroupsDefaultFirst(filteredRoutes).map(group => (
              <RoutesList key={group.name} group={group} />
            ))}
          </div>
        </section>
      </main>
    </div>
  )
}

export default App
