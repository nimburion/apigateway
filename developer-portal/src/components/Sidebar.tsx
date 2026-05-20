import { GroupInfo } from '../types'
import { PortalPage } from '../hooks/useNavigation'
import { t } from '../i18n'
import logoDark from '../assets/nimburion-logo-dark.svg'
import logoLight from '../assets/nimburion-logo-light.svg'

interface Props {
  groups: GroupInfo[]
  visibleSurfaceCount: number
  selectedGroup: string | null
  activePage: PortalPage
  theme: 'light' | 'dark'
  locale: string
  onNavigate: (page: PortalPage, group: string | null) => void
  onThemeToggle: () => void
  onLocaleChange: (locale: string) => void
}

export default function Sidebar({
  groups, visibleSurfaceCount, selectedGroup, activePage, theme, locale,
  onNavigate, onThemeToggle, onLocaleChange,
}: Props) {
  const managed = groups.some(g => g.runtime_info?.portal_mode === 'managed')
  const metricsExpanded = activePage === 'metrics' || activePage === 'metrics-trend'

  return (
    <aside className="app-sidebar">
      {/* Logo */}
      <div className="sidebar-header">
        <img src={theme === 'dark' ? logoDark : logoLight} alt="Nimburion" style={{ height: 28, width: 'auto' }} />
      </div>

      {/* Main nav */}
      <div className="sidebar-section-label">{t('nav.overview')}</div>
      <button
        type="button"
        onClick={() => onNavigate('groups', selectedGroup)}
        className={`sidebar-item${activePage === 'groups' ? ' active' : ''}`}
        aria-current={activePage === 'groups' ? 'page' : undefined}
      >
        <span>{t('nav.groupsInfo')}</span>
        <span className="sidebar-badge">{groups.filter(g => g.name !== '__management__').length}</span>
      </button>

      <button
        type="button"
        onClick={() => onNavigate('posture', selectedGroup)}
        className={`sidebar-item${activePage === 'posture' ? ' active' : ''}`}
        aria-current={activePage === 'posture' ? 'page' : undefined}
      >
        <span>{t('nav.posture')}</span>
        <span className="sidebar-badge">{visibleSurfaceCount}</span>
      </button>

      <button
        type="button"
        onClick={() => onNavigate('metrics', selectedGroup)}
        className={`sidebar-item${metricsExpanded ? ' active' : ''}`}
        aria-current={metricsExpanded ? 'page' : undefined}
      >
        <span>{t('nav.metrics')}</span>
      </button>

      {managed && (
        <button
          type="button"
          onClick={() => onNavigate('admin', null)}
          className={`sidebar-item${activePage === 'admin' ? ' active' : ''}`}
          aria-current={activePage === 'admin' ? 'page' : undefined}
        >
          <span>Config Admin</span>
        </button>
      )}

      {/* Footer */}
      <div className="sidebar-footer">
        <button
          type="button"
          onClick={onThemeToggle}
          className="btn btn-ghost btn-sm"
          title={t('theme.toggle')}
          style={{ padding: '4px 8px', fontSize: 14 }}
        >
          {theme === 'dark' ? '☀' : '☾'}
        </button>
        <label style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0, marginLeft: 'auto' }}>
          <span className="sr-only">{t('theme.language')}</span>
          <select
            className="form-select"
            style={{ minWidth: 84, height: 32, paddingTop: 4, paddingBottom: 4 }}
            value={locale}
            onChange={(event) => onLocaleChange(event.target.value)}
            aria-label={t('theme.language')}
          >
            <option value="en">EN</option>
            <option value="it">IT</option>
          </select>
        </label>
      </div>
    </aside>
  )
}
