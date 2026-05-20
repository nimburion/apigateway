import { useEffect, useMemo, useState } from 'react'
import { ManagedConfigAuditEvent, ManagedConfigVersion } from '../types'

type LoadState = {
  active: ManagedConfigVersion | null
  drafts: ManagedConfigVersion[]
  versions: ManagedConfigVersion[]
  events: ManagedConfigAuditEvent[]
}

const emptyState: LoadState = { active: null, drafts: [], versions: [], events: [] }

async function readJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
  })
  const data = await response.json().catch(() => ({}))
  if (!response.ok) {
    throw new Error(typeof data.error === 'string' ? data.error : `Request failed: ${response.status}`)
  }
  return data as T
}

function statusClass(status: string) {
  switch (status) {
    case 'active': return 'badge-green'
    case 'validated': return 'badge-blue'
    case 'failed': return 'badge-red'
    case 'archived': return 'badge-gray'
    default: return 'badge-amber'
  }
}

function formatDate(value?: string) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

export default function AdminPage() {
  const [state, setState] = useState<LoadState>(emptyState)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')
  const [draftText, setDraftText] = useState('')
  const [message, setMessage] = useState('')
  const [selectedRollback, setSelectedRollback] = useState('')

  const reload = async () => {
    setLoading(true)
    setError('')
    try {
      const [active, drafts, versions, events] = await Promise.all([
        readJSON<ManagedConfigVersion>('/api/portal/v1/config/active'),
        readJSON<{ drafts: ManagedConfigVersion[] }>('/api/portal/v1/config/drafts'),
        readJSON<{ versions: ManagedConfigVersion[] }>('/api/portal/v1/config/versions'),
        readJSON<{ events: ManagedConfigAuditEvent[] }>('/api/portal/v1/audit-events'),
      ])
      setState({ active, drafts: drafts.drafts ?? [], versions: versions.versions ?? [], events: events.events ?? [] })
      if (!draftText) {
        setDraftText(JSON.stringify(active.routes ?? { groups: {} }, null, 2))
      }
      if (!selectedRollback) {
        const candidate = versions.versions?.find(v => v.status === 'archived')
        setSelectedRollback(candidate ? String(candidate.version) : '')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void reload() }, [])

  const latestDraft = useMemo(() => state.drafts[0] ?? null, [state.drafts])

  const runAction = async (name: string, action: () => Promise<void>) => {
    setBusy(name)
    setError('')
    try {
      await action()
      await reload()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy('')
    }
  }

  const createDraft = () => runAction('create', async () => {
    const routes = JSON.parse(draftText)
    await readJSON('/api/portal/v1/config/drafts', {
      method: 'POST',
      body: JSON.stringify({ routes, message, base_version: state.active?.version }),
    })
  })

  const validateDraft = (version: number) => runAction(`validate-${version}`, async () => {
    await readJSON(`/api/portal/v1/config/drafts/${version}/validate`, { method: 'POST', body: '{}' })
  })

  const publishDraft = (version: number) => runAction(`publish-${version}`, async () => {
    await readJSON(`/api/portal/v1/config/drafts/${version}/publish`, {
      method: 'POST',
      body: JSON.stringify({ base_version: state.active?.version }),
    })
  })

  const rollback = () => runAction('rollback', async () => {
    await readJSON(`/api/portal/v1/config/versions/${selectedRollback}/rollback`, {
      method: 'POST',
      body: JSON.stringify({ message: message || 'rollback from portal' }),
    })
  })

  return (
    <div className="admin-grid">
      {error && <div className="alert alert-danger admin-span">{error}</div>}
      <section className="card admin-panel">
        <div className="section-header">
          <h2 className="section-title">Active Config</h2>
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => void reload()} disabled={loading}>Refresh</button>
        </div>
        <div className="admin-kpis">
          <div className="stat-card">
            <div className="stat-card-label">Version</div>
            <div className="stat-card-value">{state.active?.version ?? '-'}</div>
            <div className="stat-card-sub">{state.active?.checksum?.slice(0, 12) ?? 'No active checksum'}</div>
          </div>
          <div className="stat-card">
            <div className="stat-card-label">Status</div>
            <div className="stat-card-value">
              <span className={`badge ${statusClass(state.active?.status ?? '')}`}>{state.active?.status ?? 'unknown'}</span>
            </div>
            <div className="stat-card-sub">{formatDate(state.active?.published_at)}</div>
          </div>
        </div>
      </section>

      <section className="card admin-panel">
        <div className="section-header">
          <h2 className="section-title">Draft Editor</h2>
          <button type="button" className="btn btn-primary btn-sm" onClick={createDraft} disabled={!!busy}>
            {busy === 'create' ? 'Creating' : 'Create Draft'}
          </button>
        </div>
        <input className="form-input" value={message} onChange={event => setMessage(event.target.value)} placeholder="Change message" />
        <textarea
          className="form-input admin-editor"
          value={draftText}
          onChange={event => setDraftText(event.target.value)}
          spellCheck={false}
          aria-label="Draft routes JSON"
        />
      </section>

      <section className="card admin-panel admin-span">
        <div className="section-header">
          <h2 className="section-title">Drafts</h2>
          {latestDraft && <span className="badge badge-outline">latest #{latestDraft.version}</span>}
        </div>
        <table className="data-table">
          <thead><tr><th>Version</th><th>Status</th><th>Base</th><th>Message</th><th>Updated</th><th>Actions</th></tr></thead>
          <tbody>
            {state.drafts.map(draft => (
              <tr key={draft.version}>
                <td className="path-text">#{draft.version}</td>
                <td><span className={`badge ${statusClass(draft.status)}`}>{draft.status}</span></td>
                <td>{draft.base_version ?? '-'}</td>
                <td>{draft.message || '-'}</td>
                <td>{formatDate(draft.validated_at || draft.created_at)}</td>
                <td>
                  <div className="toolbar" style={{ padding: 0 }}>
                    <button type="button" className="btn btn-secondary btn-sm" onClick={() => validateDraft(draft.version)} disabled={!!busy}>Validate</button>
                    <button type="button" className="btn btn-primary btn-sm" onClick={() => publishDraft(draft.version)} disabled={!!busy}>Publish</button>
                  </div>
                </td>
              </tr>
            ))}
            {state.drafts.length === 0 && <tr><td colSpan={6}>No drafts</td></tr>}
          </tbody>
        </table>
      </section>

      <section className="card admin-panel">
        <div className="section-header"><h2 className="section-title">Rollback</h2></div>
        <div className="toolbar">
          <select className="form-select" value={selectedRollback} onChange={event => setSelectedRollback(event.target.value)}>
            <option value="">Select version</option>
            {state.versions.filter(v => v.status === 'archived').map(version => (
              <option key={version.version} value={version.version}>#{version.version} {version.message}</option>
            ))}
          </select>
          <button type="button" className="btn btn-danger btn-sm" onClick={rollback} disabled={!selectedRollback || !!busy}>Rollback</button>
        </div>
      </section>

      <section className="card admin-panel">
        <div className="section-header"><h2 className="section-title">Audit Log</h2></div>
        <div className="admin-audit-list">
          {state.events.map(event => (
            <div key={event.id} className="admin-audit-row">
              <span className="badge badge-outline">{event.action}</span>
              <span className="path-text">#{event.version ?? '-'}</span>
              <span>{event.actor || 'system'}</span>
              <span>{formatDate(event.created_at)}</span>
            </div>
          ))}
          {state.events.length === 0 && <div className="path-text-muted">No audit events</div>}
        </div>
      </section>
    </div>
  )
}
