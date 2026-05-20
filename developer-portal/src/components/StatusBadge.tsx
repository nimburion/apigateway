export type StatusBadgeStatus = 'active' | 'deprecated' | 'experimental' | 'disabled'

interface Props {
  status: StatusBadgeStatus | string;
  className?: string;
}

const STATUS_MAP: Record<string, { label: string; className: string }> = {
  active: {
    label: 'active',
    className: 'bg-emerald-100 text-emerald-700'
  },
  deprecated: {
    label: 'deprecated',
    className: 'bg-amber-100 text-amber-700'
  },
  experimental: {
    label: 'experimental',
    className: 'bg-sky-100 text-sky-700'
  },
  disabled: {
    label: 'disabled',
    className: 'bg-rose-100 text-rose-700'
  }
}

export default function StatusBadge({ status, className = '' }: Props) {
  const normalized = status.toLowerCase()
  const config = STATUS_MAP[normalized] ?? {
    label: normalized || 'active',
    className: 'bg-slate-100 text-slate-700'
  }

  return (
    <span className={`rounded-full px-2 py-1 text-[11px] font-medium ${config.className} ${className}`.trim()}>
      {config.label}
    </span>
  )
}
