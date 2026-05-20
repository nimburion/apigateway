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
  const navItems: Array<{ key: PortalPage; label: string; count: number }> = [
    { key: 'groups', label: t('nav.groupsInfo'), count: groups.filter(g => g.name !== '__management__').length },
    { key: 'posture', label: t('nav.posture'), count: visibleSurfaceCount },
    { key: 'metrics', label: t('nav.metrics'), count: 0 },
    { key: 'metrics-trend', label: t('metrics.trendTitle'), count: 0 },
  ]
  if (managed) {
    navItems.push({ key: 'admin', label: 'Config Admin', count: 0 })
  }

  return (
    <aside className="app-sidebar">
      {/* Logo */}
      <div className="sidebar-header">
        <img src={theme === 'dark' ? logoDark : logoLight} alt="Nimburion" style={{ height: 28, width: 'auto' }} />
      </div>

      {/* Main nav */}
      <div className="sidebar-section-label">{t('nav.overview')}</div>
      {navItems.map(item => (
        <button
          key={item.key}
          type="button"
          onClick={() => onNavigate(item.key, selectedGroup)}
          className={`sidebar-item${activePage === item.key ? ' active' : ''}`}
          aria-current={activePage === item.key ? 'page' : undefined}
        >
          <span>{item.label}</span>
          {item.count > 0 && <span className="sidebar-badge">{item.count}</span>}
        </button>
      ))}

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
