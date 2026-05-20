import { useMemo, RefObject } from 'react'
import { GroupInfo } from '../types'
import { getGroupDisplayName } from '../groupDisplay'
import { t } from '../i18n'

interface Props {
  groups: GroupInfo[]
  selectedGroup: string | null
  query: string
  onQueryChange: (value: string) => void
  onSelectGroup: (group: string | null) => void
  searchInputRef?: RefObject<HTMLInputElement | null>
  variant?: 'sidebar' | 'dialog'
  frameworkGroupNames?: Set<string>
}

function groupSurfaceCount(group: GroupInfo) {
  return (group.route_count ?? 0) + (group.websocket_count ?? 0)
}

function GroupButton({
  group, selectedGroup, onSelectGroup, inherited = false,
}: {
  group: GroupInfo
  selectedGroup: string | null
  onSelectGroup: (group: string | null) => void
  inherited?: boolean
}) {
  const isSelected = selectedGroup === group.name
  return (
    <button
      type="button"
      onClick={() => onSelectGroup(group.name)}
      className={`sidebar-item${isSelected && !inherited ? ' active' : ''}`}
      style={inherited ? {
        background: isSelected ? 'var(--color-warning-subtle)' : 'color-mix(in srgb, var(--color-warning-subtle) 40%, transparent)',
        color: 'var(--color-warning)',
        borderLeftColor: isSelected ? 'var(--color-warning)' : 'transparent',
        fontWeight: isSelected ? 600 : 500,
      } : undefined}
    >
      <span style={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', display: 'inline-flex', alignItems: 'center', gap: 6 }}>
        {getGroupDisplayName(group.name)}
        {inherited && <span className="badge badge-amber" style={{ fontSize: 9, padding: '1px 5px' }}>{t('groupsInfo.frameworkTitle')}</span>}
      </span>
      <span className="sidebar-badge" style={inherited ? { background: 'var(--color-warning-subtle)', color: 'var(--color-warning)' } : undefined}>
        {groupSurfaceCount(group)}
      </span>
    </button>
  )
}

export default function GroupSelectorList({
  groups, selectedGroup, query, onQueryChange, onSelectGroup, searchInputRef, variant = 'sidebar', frameworkGroupNames,
}: Props) {
  const filteredGroups = useMemo(() => {
    const normalized = query.trim().toLowerCase()
    if (!normalized) return groups
    return groups.filter((group) =>
      getGroupDisplayName(group.name).toLowerCase().includes(normalized) ||
      group.prefix.toLowerCase().includes(normalized) ||
      group.metadata.owner_team.toLowerCase().includes(normalized) ||
      group.metadata.domain.toLowerCase().includes(normalized)
    )
  }, [groups, query])

  const declaredGroups = useMemo(
    () => variant === 'dialog' && frameworkGroupNames
      ? filteredGroups.filter((group) => !frameworkGroupNames.has(group.name))
      : filteredGroups,
    [filteredGroups, frameworkGroupNames, variant],
  )

  const inheritedGroups = useMemo(
    () => variant === 'dialog' && frameworkGroupNames
      ? filteredGroups.filter((group) => frameworkGroupNames.has(group.name))
      : [],
    [filteredGroups, frameworkGroupNames, variant],
  )

  const searchClassName = variant === 'dialog' ? 'form-input' : 'sidebar-search'

  return (
    <div style={{ display: 'grid', gap: 8 }}>
      <label style={{ minWidth: 0 }}>
        <span className="sr-only">{t('groupsMenu.searchLabel')}</span>
        <input
          ref={searchInputRef}
          type="text"
          className={searchClassName}
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          placeholder={t('groupsMenu.searchPlaceholder')}
          aria-label={t('groupsMenu.searchLabel')}
        />
      </label>

      <button
        type="button"
        onClick={() => onSelectGroup(null)}
        className={`sidebar-item${!selectedGroup ? ' active' : ''}`}
      >
        <span>{t('groupsMenu.all')}</span>
        <span className="sidebar-badge">{groups.length}</span>
      </button>

      {variant === 'dialog' && frameworkGroupNames ? (
        <div style={{ display: 'grid', gap: 10 }}>
          <div>
            <div className="sidebar-section-label" style={{ padding: '4px 0 6px' }}>
              {t('groupsInfo.declaredTitle')}
            </div>
            <div style={{ display: 'grid', gap: 4 }}>
              {declaredGroups.map((group) => (
                <GroupButton key={group.name} group={group} selectedGroup={selectedGroup} onSelectGroup={onSelectGroup} />
              ))}
              {declaredGroups.length === 0 && query.trim() && (
                <div className="sidebar-item" style={{ justifyContent: 'flex-start', cursor: 'default', color: 'var(--color-text-faint)' }}>
                  {t('groupsMenu.empty')}
                </div>
              )}
            </div>
          </div>

          <div>
            <div className="sidebar-section-label" style={{ padding: '4px 0 6px' }}>
              {t('groupsInfo.frameworkTitle')}
            </div>
            <div style={{ display: 'grid', gap: 4 }}>
              {inheritedGroups.map((group) => (
                <GroupButton key={group.name} group={group} selectedGroup={selectedGroup} onSelectGroup={onSelectGroup} inherited />
              ))}
              {inheritedGroups.length === 0 && query.trim() && (
                <div className="sidebar-item" style={{ justifyContent: 'flex-start', cursor: 'default', color: 'var(--color-text-faint)' }}>
                  {t('groupsMenu.empty')}
                </div>
              )}
            </div>
          </div>
        </div>
      ) : (
        <div style={{ display: 'grid', gap: 4 }}>
          {filteredGroups.map((group) => (
            <GroupButton key={group.name} group={group} selectedGroup={selectedGroup} onSelectGroup={onSelectGroup} />
          ))}
          {filteredGroups.length === 0 && query.trim() && (
            <div className="sidebar-item" style={{ justifyContent: 'flex-start', cursor: 'default', color: 'var(--color-text-faint)' }}>
              {t('groupsMenu.empty')}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
