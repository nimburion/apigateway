import * as Dialog from '@radix-ui/react-dialog'
import { useId, useMemo, useRef, useState } from 'react'
import { t } from '../i18n'

interface Props {
  label?: string
  searchTerm: string
  onSearchTermChange: (v: string) => void
  sort: string
  onSortChange: (v: string) => void
  team: string; onTeamChange: (v: string) => void
  domain: string; onDomainChange: (v: string) => void
  visibility: string; onVisibilityChange: (v: string) => void
  status: string; onStatusChange: (v: string) => void
  method: string; onMethodChange: (v: string) => void
  scopes: string; onScopesChange: (v: string) => void
  protection: string; onProtectionChange: (v: string) => void
  teams: string[]
  domains: string[]
  onReset: () => void
}

type FK = 'team' | 'domain' | 'visibility' | 'status' | 'method' | 'scopes' | 'protection'
interface FC { key: FK; label: string; value: string; onChange: (v: string) => void; options: { label: string; value: string }[] }

function FilterField({ config }: { config: FC }) {
  return (
    <label className="block">
      <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: 'var(--color-text-muted)', marginBottom: 6 }}>
        {config.label}
      </div>
      <select
        className="form-select"
        value={config.value}
        onChange={(e) => config.onChange(e.target.value)}
      >
        <option value="">—</option>
        {config.options.map((option) => (
          <option key={option.value} value={option.value}>{option.label}</option>
        ))}
      </select>
    </label>
  )
}

export default function SearchBar({
  label,
  searchTerm, onSearchTermChange, sort, onSortChange,
  team, onTeamChange, domain, onDomainChange,
  visibility, onVisibilityChange, status, onStatusChange,
  method, onMethodChange, scopes, onScopesChange,
  protection, onProtectionChange, teams, domains, onReset
}: Props) {
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [searchFocused, setSearchFocused] = useState(false)
  const titleId = useId()
  const triggerRef = useRef<HTMLButtonElement | null>(null)

  const configs = useMemo<FC[]>(() => [
    { key: 'team', label: t('search.team'), value: team, onChange: onTeamChange, options: teams.map(v => ({ label: v, value: v })) },
    { key: 'domain', label: t('search.domain'), value: domain, onChange: onDomainChange, options: domains.map(v => ({ label: v, value: v })) },
    { key: 'visibility', label: t('routes.visibility'), value: visibility, onChange: onVisibilityChange, options: [{ label: 'public', value: 'public' }, { label: 'partner', value: 'partner' }, { label: 'internal', value: 'internal' }] },
    { key: 'status', label: t('routes.lifecycle'), value: status, onChange: onStatusChange, options: [{ label: 'active', value: 'active' }, { label: 'experimental', value: 'experimental' }, { label: 'deprecated', value: 'deprecated' }] },
    { key: 'method', label: t('search.method'), value: method, onChange: onMethodChange, options: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].map(v => ({ label: v, value: v })) },
    { key: 'scopes', label: t('routes.scopes'), value: scopes, onChange: onScopesChange, options: [{ label: t('search.withScopes'), value: 'with' }, { label: t('search.withoutScopes'), value: 'without' }] },
    { key: 'protection', label: t('detail.security'), value: protection, onChange: onProtectionChange, options: [{ label: t('search.publicOnly'), value: 'public' }, { label: t('search.protectedOnly'), value: 'protected' }] },
  ], [team, domain, visibility, status, method, scopes, protection, teams, domains, onDomainChange, onMethodChange, onProtectionChange, onScopesChange, onStatusChange, onTeamChange, onVisibilityChange])

  const active = configs.filter(c => c.value)
  const hasFilters = Boolean(searchTerm.trim()) || active.length > 0

  return (
    <div className="toolbar" style={{ borderBottom: '1px solid var(--color-border)', paddingBottom: 10, marginBottom: 12 }}>
      <div className="grid w-full items-end gap-3 md:grid-cols-[minmax(200px,1fr)_auto_auto] md:gap-4">
        <label style={{ minWidth: 0, position: 'relative', display: 'grid', gap: 6 }}>
          {label && (
            <span style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: 'var(--color-text-muted)' }}>
              {label}
            </span>
          )}
          <input
            id="portal-search-input"
            type="text"
            className="form-input"
            style={{ width: '100%', paddingRight: 30 }}
            value={searchTerm}
            onChange={e => onSearchTermChange(e.target.value)}
            onFocus={() => setSearchFocused(true)}
            onBlur={() => setSearchFocused(false)}
            placeholder={t('search.placeholder')}
          />
          <kbd
            aria-hidden="true"
            style={{
              position: 'absolute',
              right: 10,
              top: '50%',
              transform: 'translateY(-50%)',
              fontSize: 10,
              fontFamily: 'var(--font-mono)',
              padding: '1px 5px',
              border: '1px solid var(--color-border)',
              borderRadius: 4,
              background: 'var(--color-bg)',
              color: 'var(--color-text-faint)',
              pointerEvents: 'none',
              lineHeight: '18px',
              opacity: searchFocused ? 0 : 1,
              transition: 'opacity 120ms ease'
            }}
          >
            /
          </kbd>
        </label>

        <Dialog.Root open={filtersOpen} onOpenChange={setFiltersOpen}>
          <label style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <span style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: 'var(--color-text-muted)' }}>
              {t('search.filterGroup')}
            </span>
            <Dialog.Trigger asChild>
              <button
                ref={triggerRef}
                type="button"
                className="btn btn-secondary btn-sm"
                style={{ minWidth: 220, justifyContent: 'center', display: 'inline-flex', alignItems: 'center', gap: 8, height: 40 }}
              >
                <span>{active.length > 0 ? t('search.filtersApplied', { count: active.length }) : t('search.manageFilters')}</span>
              </button>
            </Dialog.Trigger>
          </label>

          <Dialog.Portal>
            <Dialog.Overlay className="drawer-overlay" />
            <Dialog.Content
              aria-labelledby={titleId}
              onCloseAutoFocus={(e) => {
                e.preventDefault()
                triggerRef.current?.focus()
              }}
              style={{ position: 'fixed', inset: 0, zIndex: 50, display: 'grid', placeItems: 'center', padding: 16 }}
            >
              <div className="card" style={{ width: 'min(760px, 100%)', maxHeight: '85vh', overflow: 'auto', padding: 20 }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, marginBottom: 16 }}>
                  <div>
                    <Dialog.Title id={titleId} style={{ margin: 0, fontSize: 20, fontWeight: 700 }}>
                      {t('search.filterGroup')}
                    </Dialog.Title>
                    <div style={{ marginTop: 6, fontSize: 13, color: 'var(--color-text-muted)' }}>
                      {t('app.filtersBody')}
                    </div>
                  </div>
                  <Dialog.Close asChild>
                    <button type="button" className="btn btn-ghost" aria-label={t('detail.close')} style={{ minWidth: 44, minHeight: 44 }}>
                      ×
                    </button>
                  </Dialog.Close>
                </div>

                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 14 }}>
                  {configs.map((config) => (
                    <FilterField key={config.key} config={config} />
                  ))}
                </div>

                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 10, marginTop: 18, paddingTop: 14, borderTop: '1px solid var(--color-border)' }}>
                  <button type="button" className="btn btn-ghost btn-sm" onClick={onReset}>
                    {t('search.clearAll')}
                  </button>
                  <Dialog.Close asChild>
                    <button type="button" className="btn btn-primary btn-sm">
                      {t('search.applyFilters')}
                    </button>
                  </Dialog.Close>
                </div>
              </div>
            </Dialog.Content>
          </Dialog.Portal>
        </Dialog.Root>

        <label style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <span style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: 'var(--color-text-muted)' }}>
            {t('search.sortLabel')}
          </span>
          <select
            className="form-select"
            style={{ width: 'auto', minWidth: 220, height: 40 }}
            value={sort}
            onChange={e => onSortChange(e.target.value)}
            aria-label={t('search.sortLabel')}
          >
            <option value="default">{t('search.sortDefault')}</option>
            <option value="owner">{t('search.sortOwner')}</option>
            <option value="risk">{t('search.sortRisk')}</option>
            <option value="surface">{t('search.sortSurface')}</option>
            <option value="traffic">{t('search.sortTraffic')}</option>
            <option value="errorRate">{t('search.sortErrorRate')}</option>
          </select>
        </label>
      </div>

      {hasFilters && (
        <div style={{ display: 'flex', width: '100%', alignItems: 'center', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
            {active.map(f => (
              <span key={f.key} className="badge badge-blue" style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '4px 10px' }}>
                {f.label}: {f.options.find(o => o.value === f.value)?.label ?? f.value}
                <button
                  type="button"
                  onClick={() => f.onChange('')}
                  style={{ background: 'none', border: 'none', cursor: 'pointer', fontSize: 14, lineHeight: 1, padding: '0 2px', color: 'inherit' }}
                  aria-label={`Remove ${f.label} filter`}
                >×</button>
              </span>
            ))}
          </div>
          <button type="button" className="btn btn-ghost btn-sm" onClick={onReset}>
            {t('search.clearAll')}
          </button>
        </div>
      )}
    </div>
  )
}
