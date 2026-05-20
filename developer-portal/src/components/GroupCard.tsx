import { GroupInfo, PortalSurfaceMetricSummary } from '../types'
import { t } from '../i18n'
import { runtimeSignals } from '../runtimeSignals'
import { getGroupDisplayName } from '../groupDisplay'

interface Props {
  group: GroupInfo;
  runtimeMetricsAvailable?: boolean;
  groupMetric?: PortalSurfaceMetricSummary | null;
  isSelected: boolean;
  onClick: () => void;
}

export default function GroupCard({ group, runtimeMetricsAvailable = false, groupMetric = null, isSelected, onClick }: Props) {
  const surfaceCount = group.route_count + group.websocket_count
  const signals = runtimeSignals(groupMetric)
  const topSignal = signals[0]
  void runtimeMetricsAvailable

  return (
    <button
      type="button"
      onClick={onClick}
      className={`w-full rounded-2xl px-3 py-2.5 text-left transition-all ${
        isSelected
          ? 'bg-indigo-500/20 text-white ring-1 ring-indigo-400/40'
          : 'text-slate-300 hover:bg-white/[0.05] hover:text-slate-100'
      }`}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="min-w-0 flex items-center gap-2">
          {isSelected && <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-teal-400" />}
          <span className="truncate text-xs font-semibold">{getGroupDisplayName(group.name)}</span>
        </div>
        <span className={`shrink-0 text-[10px] font-semibold ${isSelected ? 'text-indigo-200' : 'text-slate-500'}`}>
          {surfaceCount}
        </span>
      </div>

      {isSelected && (
        <div className="mt-2 flex items-center gap-3 text-[11px] text-indigo-200">
          <span>{group.route_count} {t('group.routes')}</span>
          <span>{group.websocket_count} {t('group.websockets')}</span>
          {topSignal && (
            <span className={`ml-auto rounded-full px-2 py-0.5 text-[10px] font-semibold ${topSignal.className}`}>
              {topSignal.label}
            </span>
          )}
        </div>
      )}

      {isSelected && (group.metadata.owner_team || group.metadata.domain) && (
        <div className="mt-1 truncate text-[10px] text-indigo-300">
          {[group.metadata.owner_team, group.metadata.domain].filter(Boolean).join(' · ')}
        </div>
      )}
    </button>
  )
}
