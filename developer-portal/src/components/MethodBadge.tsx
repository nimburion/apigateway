interface Props {
  method: string
  expanded?: boolean
  authRequired?: boolean
  primaryScope?: string
}

const CLASS: Record<string, string> = {
  GET: 'method-get', POST: 'method-post', PUT: 'method-put',
  PATCH: 'method-patch', DELETE: 'method-delete',
}

export default function MethodBadge({ method, expanded = false, authRequired = false, primaryScope = '' }: Props) {
  const cls = CLASS[method.toUpperCase()] ?? 'method-default'
  return (
    <span
      style={{
        display: 'inline-flex', alignItems: 'center', gap: 5,
        outline: expanded ? '2px solid var(--color-accent)' : 'none',
        outlineOffset: 2, borderRadius: 4, padding: '0 2px',
      }}
    >
      <span className={`method ${cls}`}>{method.toUpperCase()}</span>
      {authRequired && <span style={{ fontSize: 11, opacity: 0.7 }} title="Auth required">🔒</span>}
      {primaryScope && (
        <span className="badge badge-gray" style={{ fontSize: 10, padding: '1px 5px' }}>
          {primaryScope}
        </span>
      )}
    </span>
  )
}
