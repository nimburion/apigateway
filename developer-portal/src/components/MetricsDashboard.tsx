import { KeyboardEvent, MouseEvent, ReactNode, useEffect, useState } from 'react'
import { PortalMetricsData, PortalMetricsSnapshot, PortalMetricsTrendPoint, PortalPathMetric } from '../types'
import { t } from '../i18n'
import { runtimeSignals } from '../runtimeSignals'
import { getGroupDisplayName } from '../groupDisplay'
import { MetricsErrorKind } from '../hooks/useMetrics'

interface Props {
  data: PortalMetricsData | null;
  history: PortalMetricsSnapshot[];
  comparisonWindowMinutes: number;
  trendPoints: PortalMetricsTrendPoint[];
  loading: boolean;
  refreshing: boolean;
  error: string;
  errorStatus: number | null;
  errorKind: MetricsErrorKind | null;
  sourceUrl: string;
  onRefresh: () => void;
  onOpenCatalogSurface: (pathMetric: PortalPathMetric, focusTarget?: 'catalog' | 'errors') => void;
  view?: 'overview' | 'trend';
}

type MatchFilter = 'all' | 'matched' | 'unmatched'
type SignalFilter = 'all' | 'hot' | 'slow' | 'erroring'
type SortMode = 'requests' | 'latency' | 'errorRate' | 'path'

function SkeletonLine({
  className = ''
}: {
  className?: string
}) {
  return <div className={`animate-pulse rounded-full bg-slate-200/80 ${className}`.trim()} />
}

function SkeletonCard() {
  return (
    <div className="portal-card rounded-[1.6rem] p-5">
      <SkeletonLine className="h-3 w-28" />
      <SkeletonLine className="mt-4 h-10 w-24" />
      <SkeletonLine className="mt-4 h-3 w-32" />
      <SkeletonLine className="mt-3 h-3 w-40" />
    </div>
  )
}

function SkeletonTableRow() {
  return (
    <div className="grid gap-4 border-t border-slate-100 px-6 py-4 md:grid-cols-[minmax(0,1.4fr)_minmax(220px,1fr)_110px_120px_120px]">
      <div>
        <SkeletonLine className="h-4 w-4/5" />
        <div className="mt-3 flex flex-wrap gap-2">
          <SkeletonLine className="h-6 w-12 rounded-full" />
          <SkeletonLine className="h-6 w-16 rounded-full" />
        </div>
      </div>
      <div>
        <SkeletonLine className="h-6 w-20 rounded-full" />
        <SkeletonLine className="mt-3 h-4 w-32" />
        <SkeletonLine className="mt-2 h-3 w-40" />
      </div>
      <SkeletonLine className="h-4 w-16 md:justify-self-end" />
      <SkeletonLine className="h-4 w-20 md:justify-self-end" />
      <SkeletonLine className="h-4 w-20 md:justify-self-end" />
    </div>
  )
}

function TopPathMobileCard({
  pathMetric,
  trendValues
}: {
  pathMetric: PortalPathMetric
  trendValues: number[]
}) {
  const pathErrors = pathMetric.client_errors + pathMetric.server_errors
  const pathErrorRate = pathMetric.requests > 0 ? pathErrors / pathMetric.requests : 0
  const signals = runtimeSignals(pathMetric)

  return (
    <article className="rounded-[1.4rem] border border-slate-200 bg-white p-4 shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <code className="min-w-0 text-xs text-slate-900">{pathMetric.path}</code>
        <div className="shrink-0 rounded-full bg-slate-100 px-2 py-1 text-[11px] font-semibold text-slate-700">
          {formatNumber(pathMetric.requests)}
        </div>
      </div>
      <div className="mt-3 flex flex-wrap gap-2">
        {pathMetric.methods.length > 0 ? pathMetric.methods.map((method) => (
          <span key={`${pathMetric.path}-${method}`} className="rounded-full bg-slate-100 px-2 py-1 text-[11px] font-semibold text-slate-700">
            {method}
          </span>
        )) : (
          <span className="text-slate-600">-</span>
        )}
        {signals.map((signal) => (
          <span key={`${pathMetric.path}-${signal.id}`} className={`rounded-full px-2 py-1 text-[11px] font-semibold ${signal.className}`}>
            {signal.label}
          </span>
        ))}
      </div>
      <div className="mt-4 grid grid-cols-2 gap-3 text-sm">
        <div className="rounded-2xl bg-slate-50 px-3 py-3">
          <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-600">{t('metrics.avgLatency')}</div>
          <div className={`mt-2 font-semibold ${latencyTone(pathMetric.average_latency_ms)}`}>{formatLatency(pathMetric.average_latency_ms)}</div>
        </div>
        <div className="rounded-2xl bg-slate-50 px-3 py-3">
          <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-600">{t('metrics.errorRate')}</div>
          <div className={`mt-2 font-semibold ${errorTone(pathErrorRate)}`}>{formatPercent(pathErrorRate)}</div>
          <div className="mt-1 text-xs text-slate-600">{formatNumber(pathErrors)} {t('metrics.errors')}</div>
        </div>
      </div>
      <div className="mt-4 rounded-2xl border border-slate-200 bg-slate-50/70 px-3 py-3">
        <div className="flex items-center justify-between gap-3">
          <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-600">{t('metrics.pathTrend')}</div>
          <span className="text-[11px] font-medium text-slate-500">
            {trendValues.length < 2
              ? t('metrics.noHistoryYet')
              : trendValues.every((value) => value === 0)
                ? t('metrics.noHistoryYet')
                : trendValues[trendValues.length - 1] === trendValues[trendValues.length - 2]
                  ? t('metrics.stable')
                  : t('metrics.requests')}
          </span>
        </div>
        <PathSparkline values={trendValues} stroke="#0f766e" />
      </div>
      <div className="mt-4 rounded-2xl border border-slate-200 bg-slate-50/60 px-3 py-3">
        <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-600">{t('metrics.coverage')}</div>
        {pathMetric.primary_match ? (
          <div className="mt-2 space-y-2">
            <div className="flex flex-wrap gap-2">
              <span className="rounded-full bg-emerald-100 px-2 py-1 text-[11px] font-semibold text-emerald-800">
                {t('metrics.mappedToCatalog')}
              </span>
              {normalizeObservedTrendPath(pathMetric.primary_match.path_pattern) !== normalizeObservedTrendPath(pathMetric.path) && (
                <span className="rounded-full bg-white px-2 py-1 text-[11px] font-medium text-slate-700">
                  {t('metrics.catalogPath')} {pathMetric.primary_match.path_pattern}
                </span>
              )}
            </div>
            <div className="text-sm font-medium text-slate-900">{t('metrics.apiGroup')} {getGroupDisplayName(pathMetric.primary_match.group_name)}</div>
            <div className="text-xs text-slate-600">
              {pathMetric.primary_match.metadata.owner_team || '-'} · {pathMetric.primary_match.deprecated || pathMetric.primary_match.metadata.status === 'deprecated' ? 'deprecated' : pathMetric.primary_match.metadata.status || 'active'}
            </div>
          </div>
        ) : (
          <div className="mt-2 space-y-2">
            <span className="rounded-full bg-amber-100 px-2 py-1 text-[11px] font-semibold text-amber-800">
              {t('metrics.notMappedShort')}
            </span>
            <div className="text-xs text-slate-600">{t('metrics.noCatalogMatchBody')}</div>
          </div>
        )}
      </div>
    </article>
  )
}

function normalizeObservedTrendPath(path: string): string {
  const trimmed = path.trim()
  if (!trimmed) return '/'
  const withoutQuery = trimmed.split('?')[0] || '/'
  const normalized = withoutQuery.replace(/\/{2,}/g, '/')
  if (normalized.length > 1 && normalized.endsWith('/')) {
    return normalized.slice(0, -1)
  }
  return normalized || '/'
}

function buildPathTrendValues(
  history: PortalMetricsSnapshot[],
  path: string,
  comparisonWindowMinutes: number
): number[] {
  const normalizedPath = normalizeObservedTrendPath(path)
  if (history.length === 0) {
    return []
  }

  const currentTimestamp = Date.parse(history[history.length - 1].captured_at)
  const comparisonWindowMs = Math.max(1, comparisonWindowMinutes) * 60 * 1000
  const scopedHistory = Number.isFinite(currentTimestamp)
    ? history.filter((snapshot) => currentTimestamp - Date.parse(snapshot.captured_at) <= comparisonWindowMs)
    : history.slice()
  const recent = scopedHistory.length >= 2 ? scopedHistory : history.slice(-2)

  return recent
    .map((snapshot, index) => {
      const entry = snapshot.data.paths.find((candidate) => normalizeObservedTrendPath(candidate.path) === normalizedPath)
      const previousSnapshot = index > 0 ? recent[index - 1] : null
      const previousEntry = previousSnapshot?.data.paths.find((candidate) => normalizeObservedTrendPath(candidate.path) === normalizedPath)
      const currentRequests = entry?.requests ?? 0
      const previousRequests = previousEntry?.requests ?? 0
      return previousSnapshot ? Math.max(0, currentRequests - previousRequests) : 0
    })
}

function buildSparklinePath(values: number[], width: number, height: number, paddingX: number, paddingY: number): string {
  if (values.length === 0) return ''
  const min = Math.min(...values)
  const max = Math.max(...values)
  const range = max - min || 1
  const usableWidth = width - paddingX * 2
  const usableHeight = height - paddingY * 2

  return values.map((value, index) => {
    const x = values.length === 1 ? width / 2 : paddingX + (index / (values.length - 1)) * usableWidth
    const y = height - paddingY - ((value - min) / range) * usableHeight
    return `${index === 0 ? 'M' : 'L'} ${x.toFixed(2)} ${y.toFixed(2)}`
  }).join(' ')
}

function buildSparklineDots(values: number[], width: number, height: number, paddingX: number, paddingY: number) {
  if (values.length === 0) return []
  const min = Math.min(...values)
  const max = Math.max(...values)
  const range = max - min || 1
  const usableWidth = width - paddingX * 2
  const usableHeight = height - paddingY * 2

  return values.map((value, index) => ({
    x: values.length === 1 ? width / 2 : paddingX + (index / (values.length - 1)) * usableWidth,
    y: height - paddingY - ((value - min) / range) * usableHeight,
    value
  }))
}

function PathSparkline({
  values,
  stroke,
  variant = 'compact',
  hoveredIndex,
  onHoverIndexChange,
  trendPoints,
  formatValue
}: {
  values: number[]
  stroke: string
  variant?: 'compact' | 'large'
  hoveredIndex?: number | null
  onHoverIndexChange?: (index: number | null) => void
  trendPoints?: PortalMetricsTrendPoint[]
  formatValue?: (value: number) => string
}) {
  const width = variant === 'large' ? 420 : 120
  const height = variant === 'large' ? 120 : 26
  const paddingX = variant === 'large' ? 12 : 4
  const paddingY = variant === 'large' ? 14 : 3
  const hoverable = variant === 'large' && Boolean(onHoverIndexChange) && Boolean(trendPoints) && (trendPoints?.length ?? 0) >= 2

  if (values.length < 2) {
    return variant === 'large'
      ? <div className="mt-4 min-h-[120px] flex-1 rounded-2xl border border-dashed border-slate-200 bg-white/70" />
      : <div className="mt-2 h-[26px] rounded-full border border-dashed border-slate-200 bg-white/70" />
  }

  const path = buildSparklinePath(values, width, height, paddingX, paddingY)
  const dots = buildSparklineDots(values, width, height, paddingX, paddingY)
  const activeIndex = hoveredIndex ?? values.length - 1
  const activePoint = dots[activeIndex]
  const latest = values[values.length - 1]
  const previous = values[values.length - 2]
  const trendDelta = latest - previous
  const hasAnyTraffic = values.some((value) => value > 0)
  const hoverTimestamp = trendPoints?.[activeIndex]?.captured_at
  const hoverValue = activePoint ? (formatValue ? formatValue(activePoint.value) : formatNumber(activePoint.value)) : ''
  const showHover = hoverable && hoveredIndex !== null && activePoint && hoverTimestamp

  return (
    <div className={variant === 'large' ? 'mt-3 flex-1' : 'mt-2'}>
      <div className="relative">
        <svg
          viewBox={`0 0 ${width} ${height}`}
          className={variant === 'large' ? 'h-[120px] w-full' : 'h-[26px] w-full'}
          preserveAspectRatio={variant === 'large' ? 'none' : undefined}
          role="img"
          aria-label={t('metrics.pathTrend')}
          onMouseMove={hoverable && onHoverIndexChange
            ? (event) => onHoverIndexChange(cursorIndexFromEvent(event, values.length, paddingX, width))
            : undefined}
          onMouseLeave={hoverable && onHoverIndexChange ? () => onHoverIndexChange(null) : undefined}
        >
          <path d={path} fill="none" stroke={stroke} strokeWidth="2.5" strokeLinecap="round" />
          {hoverable && activePoint && hoveredIndex !== null && (
            <>
              <line
                x1={activePoint.x}
                y1={paddingY}
                x2={activePoint.x}
                y2={height - paddingY}
                stroke="#0f172a"
                strokeOpacity="0.22"
                strokeDasharray="4 4"
              />
              <circle cx={activePoint.x} cy={activePoint.y} r="4.5" fill={stroke} />
            </>
          )}
        </svg>
        {showHover && hoverTimestamp && (
          <div
            className="pointer-events-none absolute z-10 max-w-[calc(100%-16px)] -translate-x-1/2 rounded-xl bg-slate-950 px-3 py-2 text-xs text-white shadow-lg"
            style={buildHoverOverlayPosition(activePoint, width, height)}
          >
            <div className="font-semibold">{formatTrendPointLabel(hoverTimestamp)}</div>
            <div className="mt-0.5 text-slate-200">{t('metrics.current')}: {hoverValue}</div>
          </div>
        )}
      </div>
      <div className="mt-1 text-[10px] font-medium text-slate-500">
        {!hasAnyTraffic
          ? t('metrics.noHistoryYet')
          : trendDelta > 0
            ? `↑ ${formatNumber(Math.abs(trendDelta))} ${t('metrics.requests')}`
            : trendDelta < 0
              ? `↓ ${formatNumber(Math.abs(trendDelta))} ${t('metrics.requests')}`
              : t('metrics.stable')}
      </div>
    </div>
  )
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value)
}

function formatLatency(value: number): string {
  if (value <= 0) {
    return '0 ms'
  }
  if (value >= 1000) {
    return `${(value / 1000).toFixed(2)} s`
  }
  return `${value.toFixed(0)} ms`
}

function formatPercent(ratio: number): string {
  const safeRatio = clampRatio(ratio)
  if (safeRatio <= 0) {
    return '0%'
  }
  const percentage = safeRatio * 100
  const precision = percentage >= 10 ? 0 : 1
  return `${percentage.toFixed(precision)}%`
}

function clampRatio(value: number): number {
  if (!Number.isFinite(value) || value <= 0) {
    return 0
  }
  return Math.min(1, value)
}

function meterTone(tone: 'teal' | 'emerald' | 'amber' | 'rose' | 'violet'): string {
  if (tone === 'emerald') return 'bg-emerald-500'
  if (tone === 'amber') return 'bg-amber-500'
  if (tone === 'rose') return 'bg-rose-500'
  if (tone === 'violet') return 'bg-violet-500'
  return 'bg-teal-600'
}

function latencyTone(value: number): string {
  if (value >= 1000) {
    return 'text-rose-700'
  }
  if (value >= 500) {
    return 'text-amber-700'
  }
  return 'text-slate-700'
}

function errorTone(value: number): string {
  if (value >= 0.05) {
    return 'text-rose-700'
  }
  if (value >= 0.01) {
    return 'text-amber-700'
  }
  return 'text-emerald-700'
}

function formatDelta(value: number, unit: string, digits = 0): string {
  const prefix = value > 0 ? '+' : value < 0 ? '−' : ''
  const magnitude = digits > 0 ? Math.abs(value).toFixed(digits) : Math.abs(Math.round(value)).toString()
  return `${prefix}${magnitude}${unit}`
}

function trendDeltaTone(value: number, inverse = false): string {
  if (value === 0) return 'text-slate-500'
  const positiveIsGood = !inverse
  const good = value > 0 ? positiveIsGood : !positiveIsGood
  return good ? 'text-emerald-700' : 'text-rose-700'
}

function MetricSummaryCard({
  title,
  value,
  delta,
  deltaUnit,
  deltaDigits,
  tone,
  sparklineValues,
  trendPoints,
  hoveredIndex,
  onHoverIndexChange,
  sparklineValueFormatter,
  valueLabel,
  onExpand,
  expandLabel,
  invertDelta = false
}: {
  title: string
  value: string
  delta: number | null
  deltaUnit: string
  deltaDigits: number
  tone: 'teal' | 'amber' | 'rose' | 'violet'
  sparklineValues: number[]
  trendPoints: PortalMetricsTrendPoint[]
  hoveredIndex: number | null
  onHoverIndexChange: (index: number | null) => void
  sparklineValueFormatter: (value: number) => string
  valueLabel?: string
  onExpand?: () => void
  expandLabel?: string
  invertDelta?: boolean
}) {
  const deltaClass = delta === null ? 'text-slate-500' : trendDeltaTone(delta, invertDelta)
  const deltaText = delta === null
    ? t('metrics.noHistoryYet')
    : delta === 0
      ? t('metrics.stable')
      : `${delta > 0 ? '↑' : '↓'} ${formatDelta(delta, deltaUnit, deltaDigits)}`

  return (
    <div className={`flex min-h-[268px] flex-col rounded-[1.6rem] border bg-white px-5 py-4 shadow-sm ${tone === 'rose' ? 'border-rose-200' : tone === 'amber' ? 'border-amber-200' : tone === 'violet' ? 'border-violet-200' : 'border-teal-200'}`}>
      <div className="flex items-start justify-between gap-3">
        <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-500">{title}</div>
        {onExpand && (
          <button
            type="button"
            onClick={onExpand}
            className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-slate-200 bg-white text-slate-500 transition hover:border-slate-300 hover:text-slate-900"
            aria-label={expandLabel ?? title}
            title={expandLabel ?? title}
          >
            <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M8 3H3v5" />
              <path d="M16 3h5v5" />
              <path d="M21 16v5h-5" />
              <path d="M3 16v5h5" />
            </svg>
          </button>
        )}
      </div>
      <div className="mt-2 text-3xl font-semibold text-slate-950">{value}</div>
      {valueLabel && <div className="mt-1 text-xs text-slate-500">{valueLabel}</div>}
      <div className={`mt-3 text-sm font-semibold ${deltaClass}`}>{deltaText}</div>
      <div className="mt-4 flex-1">
        <PathSparkline
          values={sparklineValues}
          stroke={tone === 'rose' ? '#e11d48' : tone === 'amber' ? '#d97706' : tone === 'violet' ? '#7c3aed' : '#0f766e'}
          variant="large"
          trendPoints={trendPoints}
          hoveredIndex={hoveredIndex}
          onHoverIndexChange={onHoverIndexChange}
          formatValue={sparklineValueFormatter}
        />
      </div>
    </div>
  )
}

function FullscreenButton({
  label,
  onClick
}: {
  label: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-500 transition hover:border-slate-300 hover:text-slate-900"
      aria-label={label}
      title={label}
    >
      <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <path d="M8 3H3v5" />
        <path d="M16 3h5v5" />
        <path d="M21 16v5h-5" />
        <path d="M3 16v5h5" />
      </svg>
    </button>
  )
}

function ChartDialog({
  open,
  title,
  subtitle,
  onClose,
  children
}: {
  open: boolean
  title: string
  subtitle: string
  onClose: () => void
  children: ReactNode
}) {
  useEffect(() => {
    if (!open) {
      return
    }

    const handleKeyDown = (event: globalThis.KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onClose()
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [open, onClose])

  if (!open) {
    return null
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/55 p-4"
      role="dialog"
      aria-modal="true"
      aria-label={title}
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose()
        }
      }}
    >
      <div
        className="max-h-[96vh] w-full max-w-[100vw] overflow-auto rounded-[1.6rem] border border-slate-200 bg-white p-2 shadow-2xl"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <div className="text-lg font-semibold text-slate-950">{title}</div>
            <div className="mt-1 text-sm text-slate-600">{subtitle}</div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="inline-flex h-10 w-10 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-500 transition hover:border-slate-300 hover:text-slate-900"
            aria-label="Close chart dialog"
          >
            <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M6 6l12 12" />
              <path d="M18 6L6 18" />
            </svg>
          </button>
        </div>
        <div className="mt-2">{children}</div>
      </div>
    </div>
  )
}

function MetricSeriesDialog({
  open,
  title,
  subtitle,
  seriesLabel,
  values,
  trendPoints,
  stroke,
  formatValue,
  onClose
}: {
  open: boolean
  title: string
  subtitle: string
  seriesLabel: string
  values: number[]
  trendPoints: PortalMetricsTrendPoint[]
  stroke: string
  formatValue: (value: number) => string
  onClose: () => void
}) {
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null)
  if (!open) {
    return null
  }

  const width = 3400
  const height = 640
  const paddingX = 64
  const paddingY = 48
  const bounds = buildCountChartBounds(values)
  const yTicks = buildYAxisTicks(bounds.min, bounds.max, 4)
  const axisTicks = buildTimeAxisTicks(trendPoints, 5)
  const linePath = buildLinePathWithBounds(values, width, height, paddingX, paddingY, bounds)
  const activeIndex = hoveredIndex ?? values.length - 1
  const activePoint = buildSeriesDotsWithBounds(values, width, height, paddingX, paddingY, bounds)[activeIndex]

  return (
    <ChartDialog open={open} title={title} subtitle={subtitle} onClose={onClose}>
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
          <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">{seriesLabel}</div>
          <div className="mt-1 text-lg font-semibold text-slate-950">{formatValue(values[activeIndex] ?? 0)}</div>
        </div>
        <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
          <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.peak')}</div>
          <div className="mt-1 text-lg font-semibold text-slate-950">{formatValue(Math.max(...values))}</div>
        </div>
        <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
          <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.low')}</div>
          <div className="mt-1 text-lg font-semibold text-slate-950">{formatValue(Math.min(...values))}</div>
        </div>
      </div>
      <div className="relative mt-2 rounded-[1.4rem] border border-slate-200 bg-slate-50 px-2 py-2">
        <svg
          viewBox={`0 0 ${width} ${height}`}
          className="h-[580px] w-full"
          preserveAspectRatio="none"
          role="img"
          aria-label={title}
          onMouseMove={(event) => setHoveredIndex(cursorIndexFromEvent(event, trendPoints.length, paddingX, width))}
          onMouseLeave={() => setHoveredIndex(null)}
        >
          {yTicks.map((tick) => {
            const y = paddingY + tick.ratio * (height - paddingY * 2)
            return (
              <g key={`summary-dialog-${title}-${tick.value}`}>
                <line x1={paddingX} y1={y} x2={width - paddingX} y2={y} stroke="#e2e8f0" />
                <text x={paddingX - 10} y={y + 4} textAnchor="end" fill="#64748b" fontSize="13" fontWeight={600}>
                  {formatValue(tick.value)}
                </text>
              </g>
            )
          })}
          <path d={linePath} fill="none" stroke={stroke} strokeWidth="3.5" strokeLinecap="round" />
          {activePoint && (
            <circle cx={activePoint.x} cy={activePoint.y} r={6} fill={stroke} />
          )}
          <line
            x1={activePoint ? activePoint.x : width - paddingX}
            y1={paddingY}
            x2={activePoint ? activePoint.x : width - paddingX}
            y2={height - paddingY}
            stroke="#0f172a"
            strokeOpacity="0.24"
            strokeDasharray="5 5"
          />
          {axisTicks.map((tick) => {
            const x = buildPointX(tick.index, trendPoints.length, width, paddingX)
            return (
              <g key={`summary-dialog-${title}-x-${tick.index}`}>
                <line x1={x} y1={height - paddingY} x2={x} y2={height - paddingY + 12} stroke="#cbd5e1" />
                <text x={x} y={height - 2} textAnchor="middle" fill="#64748b" fontSize="13" fontWeight={600}>
                  {tick.label}
                </text>
              </g>
            )
          })}
        </svg>
          {hoveredIndex !== null && activePoint && (
            <div
              className="pointer-events-none absolute z-10 max-w-[calc(100%-16px)] -translate-x-1/2 rounded-xl bg-slate-950 px-3 py-2 text-xs text-white shadow-lg"
              style={buildHoverOverlayPosition(activePoint, width, height)}
            >
            <div className="font-semibold">{formatTrendPointLabel(trendPoints[activeIndex].captured_at)}</div>
            <div className="mt-0.5 text-slate-200">{seriesLabel}: {formatValue(activePoint.value)}</div>
          </div>
        )}
      </div>
    </ChartDialog>
  )
}

function buildChartBounds(values: number[]) {
  if (values.length === 0) {
    return { min: 0, max: 1, range: 1 }
  }
  const minRaw = Math.min(...values)
  const maxRaw = Math.max(...values)
  const span = maxRaw - minRaw
  const padding = span === 0 ? Math.max(Math.abs(maxRaw) * 0.15, 1) : span * 0.12
  const min = minRaw - padding
  const max = maxRaw + padding
  return { min, max, range: max - min || 1 }
}

function buildCountChartBounds(values: number[]) {
  const dynamic = buildChartBounds(values)
  return {
    min: 0,
    max: Math.max(1, dynamic.max),
    range: Math.max(1, dynamic.max)
  }
}

function buildPercentChartBounds() {
  return {
    min: 0,
    max: 100,
    range: 100
  }
}

function buildYAxisTicks(min: number, max: number, count = 3) {
  if (count <= 1) {
    return [{ value: max, ratio: 0 }]
  }
  return Array.from({ length: count }, (_, index) => {
    const ratio = index / (count - 1)
    return { value: max - ratio * (max - min), ratio }
  })
}

function buildLinePathWithBounds(
  values: number[],
  width: number,
  height: number,
  paddingX: number,
  paddingY: number,
  bounds: { min: number; max: number; range: number }
): string {
  if (values.length === 0) return ''
  const usableWidth = width - paddingX * 2
  const usableHeight = height - paddingY * 2

  return values
    .map((value, index) => {
      const x = values.length === 1 ? width / 2 : paddingX + (index / (values.length - 1)) * usableWidth
      const y = height - paddingY - ((value - bounds.min) / (bounds.range || 1)) * usableHeight
      return `${index === 0 ? 'M' : 'L'} ${x.toFixed(2)} ${y.toFixed(2)}`
    })
    .join(' ')
}

function formatTrendPointLabel(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' }).format(date)
}

function formatMetricsEndpoint(sourceUrl: string): string {
  try {
    const url = new URL(sourceUrl, window.location.origin)
    return `${url.origin}${url.pathname}`
  } catch {
    return sourceUrl
  }
}

function metricsErrorCopy(errorKind: MetricsErrorKind | null) {
  if (errorKind === 'auth') {
    return {
      eyebrow: t('metrics.accessTitle'),
      title: t('metrics.accessTitle'),
      body: t('metrics.accessBody'),
      tone: 'amber' as const
    }
  }
  if (errorKind === 'not_found') {
    return {
      eyebrow: t('metrics.notFoundTitle'),
      title: t('metrics.notFoundTitle'),
      body: t('metrics.notFoundBody'),
      tone: 'amber' as const
    }
  }
  if (errorKind === 'network') {
    return {
      eyebrow: t('metrics.networkTitle'),
      title: t('metrics.networkTitle'),
      body: t('metrics.networkBody'),
      tone: 'rose' as const
    }
  }
  if (errorKind === 'parse') {
    return {
      eyebrow: t('metrics.parseTitle'),
      title: t('metrics.parseTitle'),
      body: t('metrics.parseBody'),
      tone: 'rose' as const
    }
  }
  return {
    eyebrow: t('metrics.errorTitle'),
    title: t('metrics.errorTitle'),
    body: t('metrics.errorBody'),
    tone: 'rose' as const
  }
}

function AlertBanner({
  errorKind,
  error,
  sourceUrl,
  stale
}: {
  errorKind: MetricsErrorKind | null
  error: string
  sourceUrl: string
  stale: boolean
}) {
  const copy = metricsErrorCopy(errorKind)
  const toneClasses = copy.tone === 'amber'
    ? 'border-amber-200 bg-amber-50/90 text-amber-950'
    : 'border-rose-200 bg-rose-50/90 text-rose-950'
  const eyebrowClasses = copy.tone === 'amber' ? 'text-amber-700' : 'text-rose-700'
  const detailClasses = copy.tone === 'amber' ? 'bg-white/70 text-slate-700' : 'bg-white/70 text-slate-700'

  return (
    <div role="alert" className={`rounded-[1.6rem] border px-5 py-4 ${toneClasses}`}>
      <div className={`text-[11px] font-semibold uppercase tracking-[0.18em] ${eyebrowClasses}`}>{copy.eyebrow}</div>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <h3 className="text-lg font-semibold">{copy.title}</h3>
        {stale && (
          <span className="rounded-full bg-white/70 px-3 py-1 text-[11px] font-semibold text-slate-700">
            {t('metrics.showingLastSnapshot')}
          </span>
        )}
      </div>
      <p className="mt-2 text-sm">{copy.body}</p>
      <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-slate-700">
        <span className="rounded-full bg-white/70 px-3 py-1 font-medium">{t('metrics.endpoint')}: {formatMetricsEndpoint(sourceUrl)}</span>
        {errorKind === 'auth' && <span>{t('metrics.scopeHint')}</span>}
        {errorKind === 'not_found' && <span>{t('metrics.devProxyHint')}</span>}
      </div>
      {error && <div className={`mt-3 rounded-2xl px-4 py-3 font-mono text-xs ${detailClasses}`}>{error}</div>}
    </div>
  )
}

function buildSeriesDotsWithBounds(
  values: number[],
  width: number,
  height: number,
  paddingX: number,
  paddingY: number,
  bounds: { min: number; max: number; range: number }
) {
  if (values.length === 0) return []
  const usableWidth = width - paddingX * 2
  const usableHeight = height - paddingY * 2

  return values.map((value, index) => ({
    x: values.length === 1 ? width / 2 : paddingX + (index / (values.length - 1)) * usableWidth,
    y: height - paddingY - ((value - bounds.min) / (bounds.range || 1)) * usableHeight,
    value
  }))
}

function buildPointX(index: number, pointCount: number, width: number, paddingX: number): number {
  if (pointCount <= 1) {
    return width / 2
  }
  return paddingX + (index / (pointCount - 1)) * (width - paddingX * 2)
}

function buildHoverOverlayPosition(
  point: { x: number; y: number },
  width: number,
  height: number
): { left: string; top: string } {
  return {
    left: `clamp(12px, ${(point.x / width) * 100}%, calc(100% - 12px))`,
    top: `clamp(8px, calc(${(point.y / height) * 100}% - 54px), calc(100% - 12px))`
  }
}

function buildTimeAxisTicks(trendPoints: PortalMetricsTrendPoint[], maxTicks = 5) {
  if (trendPoints.length === 0) return []
  const rawIndexes = Array.from({ length: Math.min(maxTicks, trendPoints.length) }, (_, index) =>
    Math.round((index / Math.max(1, Math.min(maxTicks, trendPoints.length) - 1)) * (trendPoints.length - 1))
  )
  const indexes = Array.from(new Set(rawIndexes))
  return indexes.map((index) => ({
    index,
    label: formatTrendPointLabel(trendPoints[index].captured_at)
  }))
}

function cursorIndexFromEvent(event: MouseEvent<SVGSVGElement>, pointCount: number, paddingX: number, width: number) {
  const rect = event.currentTarget.getBoundingClientRect()
  if (rect.width <= 0 || pointCount <= 1) {
    return 0
  }
  const relativeX = ((event.clientX - rect.left) / rect.width) * width
  let bestIndex = 0
  let bestDistance = Number.POSITIVE_INFINITY
  for (let index = 0; index < pointCount; index += 1) {
    const pointX = buildPointX(index, pointCount, width, paddingX)
    const distance = Math.abs(pointX - relativeX)
    if (distance < bestDistance) {
      bestDistance = distance
      bestIndex = index
    }
  }
  return bestIndex
}

function CoverageTrendChart({
  trendPoints,
  comparisonWindowMinutes,
  hoveredIndex,
  onHoverIndexChange,
  frameClassName = ''
}: {
  trendPoints: PortalMetricsTrendPoint[]
  comparisonWindowMinutes: number
  hoveredIndex: number | null
  onHoverIndexChange: (index: number | null) => void
  frameClassName?: string
}) {
  const [dialogOpen, setDialogOpen] = useState(false)
  if (trendPoints.length < 2) return null
  const activeIndex = hoveredIndex ?? trendPoints.length - 1
  const values = trendPoints.map((point) => point.coverage_rate * 100)

  const renderChart = (expanded = false) => {
    const width = expanded ? 1440 : 1200
    const height = expanded ? 300 : 168
    const paddingX = expanded ? 64 : 52
    const paddingY = expanded ? 30 : 20
    const axisTicks = buildTimeAxisTicks(trendPoints, expanded ? 5 : 4)
    const bounds = buildPercentChartBounds()
    const yTicks = buildYAxisTicks(bounds.min, bounds.max, 3)
    const linePath = buildLinePathWithBounds(values, width, height, paddingX, paddingY, bounds)
    const activePoint = buildSeriesDotsWithBounds(values, width, height, paddingX, paddingY, bounds)[activeIndex]

    return (
      <>
        <div className={`rounded-2xl bg-slate-50 px-3 py-3 ${expanded ? 'border border-slate-200' : ''}`}>
          <svg
            viewBox={`0 0 ${width} ${height}`}
            className={expanded ? 'h-72 w-full' : 'h-44 w-full'}
            preserveAspectRatio="none"
            role="img"
            aria-label={t('metrics.coverageTrendTitle')}
            onMouseMove={(event) => onHoverIndexChange(cursorIndexFromEvent(event, trendPoints.length, paddingX, width))}
            onMouseLeave={() => onHoverIndexChange(null)}
          >
            {yTicks.map((tick) => {
              const y = paddingY + tick.ratio * (height - paddingY * 2)
              return (
                <g key={`cov-${expanded ? 'expanded' : 'inline'}-${tick.value}`}>
                  <line x1={paddingX} y1={y} x2={width - paddingX} y2={y} stroke="#e2e8f0" />
                  <text x={paddingX - 8} y={y + 3} textAnchor="end" fill="#64748b" fontSize="11" fontWeight={600}>
                    {Math.max(0, Math.min(100, Math.round(tick.value)))}%
                  </text>
                </g>
              )
            })}
            <path d={linePath} fill="none" stroke="#0f766e" strokeWidth="3" strokeLinecap="round" />
            {activePoint && (
              <circle
                cx={activePoint.x}
                cy={activePoint.y}
                r={5}
                fill="#0f766e"
                className="transition"
              >
                <title>{`${formatTrendPointLabel(trendPoints[activeIndex].captured_at)} · ${Math.round(trendPoints[activeIndex].coverage_rate * 100)}% ${t('metrics.catalogCoverage')}`}</title>
              </circle>
            )}
            {activeIndex >= 0 && (
              <line
                x1={trendPoints.length === 1 ? width / 2 : paddingX + (activeIndex / (trendPoints.length - 1)) * (width - paddingX * 2)}
                y1={paddingY}
                x2={trendPoints.length === 1 ? width / 2 : paddingX + (activeIndex / (trendPoints.length - 1)) * (width - paddingX * 2)}
                y2={height - paddingY}
                stroke="#0f172a"
                strokeOpacity="0.24"
                strokeDasharray="5 5"
              />
            )}
          </svg>
        </div>
        <div className="mt-2 text-[11px] text-slate-500">
          <div className="mb-2 font-medium uppercase tracking-[0.14em] text-slate-400">{t('metrics.timeline')}</div>
          <div
            className="relative h-5"
            style={{
              marginLeft: `${(paddingX / width) * 100}%`,
              width: `${((width - paddingX * 2) / width) * 100}%`
            }}
          >
            {axisTicks.map((tick) => (
              <span
                key={`coverage-${expanded ? 'expanded' : 'inline'}-tick-${tick.index}`}
                className="absolute -translate-x-1/2 whitespace-nowrap"
                style={{ left: `${(tick.index / Math.max(1, trendPoints.length - 1)) * 100}%` }}
              >
                {tick.label}
              </span>
            ))}
          </div>
        </div>
      </>
    )
  }

  return (
    <>
    <div className={`rounded-2xl border bg-white px-4 py-4 shadow-sm ${frameClassName || 'border-slate-200'}`}>
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-semibold text-slate-900">{t('metrics.coverageTrendTitle')}</div>
          <div className="mt-1 text-xs text-slate-600">{t('metrics.coverageTrendBody')}</div>
        </div>
        <div className="flex items-start gap-3">
          <div className="text-right">
            <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">{`${comparisonWindowMinutes}m`}</div>
            <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.current')}</div>
            <div className="mt-1 text-lg font-semibold text-slate-900">{Math.round(trendPoints[activeIndex].coverage_rate * 100)}%</div>
          </div>
          <FullscreenButton label="Expand coverage chart" onClick={() => setDialogOpen(true)} />
        </div>
      </div>
      {trendPoints.length < 4 && (
        <div className="mt-3 rounded-2xl border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
          {t('metrics.lowDataNotice')}
        </div>
      )}
      {renderChart(false)}
    </div>
    <ChartDialog
      open={dialogOpen}
      title={t('metrics.coverageTrendTitle')}
      subtitle={`${t('metrics.coverageTrendBody')} · ${comparisonWindowMinutes}m`}
      onClose={() => setDialogOpen(false)}
    >
      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
          <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.current')}</div>
          <div className="mt-1 text-lg font-semibold text-slate-950">{Math.round(values[activeIndex])}%</div>
        </div>
        <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
          <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.peak')}</div>
          <div className="mt-1 text-lg font-semibold text-slate-950">{Math.round(Math.max(...values))}%</div>
        </div>
        <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3">
          <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.low')}</div>
          <div className="mt-1 text-lg font-semibold text-slate-950">{Math.round(Math.min(...values))}%</div>
        </div>
      </div>
      {renderChart(true)}
    </ChartDialog>
    </>
  )
}

export default function MetricsDashboard({
  data,
  history,
  comparisonWindowMinutes,
  trendPoints,
  loading,
  refreshing,
  error,
  errorStatus,
  errorKind,
  sourceUrl,
  onRefresh,
  onOpenCatalogSurface,
  view = 'overview'
}: Props) {
  const [query, setQuery] = useState('')
  const [matchFilter, setMatchFilter] = useState<MatchFilter>('all')
  const [signalFilter, setSignalFilter] = useState<SignalFilter>('all')
  const [sortMode, setSortMode] = useState<SortMode>('requests')
  const [topPathLimit, setTopPathLimit] = useState(5)
  const [hoveredSummaryIndex, setHoveredSummaryIndex] = useState<number | null>(null)
  const [hoveredTrendIndex, setHoveredTrendIndex] = useState<number | null>(null)
  const [summaryDialog, setSummaryDialog] = useState<null | 'requests' | 'latency' | 'errors'>(null)
  const effectiveErrorKind = errorKind ?? (errorStatus === 401 || errorStatus === 403 ? 'auth' : null)

  const totalErrors = data ? data.summary.client_errors + data.summary.server_errors : 0
  const totalRateLimited = data ? data.summary.rate_limited_responses : 0
  const successRate = data && data.summary.total_requests > 0
    ? data.summary.success_responses / data.summary.total_requests
    : 0
  const clientErrorRate = data && data.summary.total_requests > 0
    ? data.summary.client_errors / data.summary.total_requests
    : 0
  const serverErrorRate = data && data.summary.total_requests > 0
    ? data.summary.server_errors / data.summary.total_requests
    : 0

  const allPaths = data?.paths ?? []
  const normalizedQuery = query.trim().toLowerCase()
  const filteredPaths = [...allPaths]
    .filter((pathMetric) => {
      if (normalizedQuery) {
        const haystack = [
          pathMetric.path,
          pathMetric.primary_match?.group_name ?? '',
          pathMetric.primary_match?.metadata.owner_team ?? '',
          pathMetric.primary_match?.path_pattern ?? '',
          ...pathMetric.methods
        ].join(' ').toLowerCase()
        if (!haystack.includes(normalizedQuery)) {
          return false
        }
      }

      if (matchFilter === 'matched' && !pathMetric.primary_match) {
        return false
      }
      if (matchFilter === 'unmatched' && pathMetric.primary_match) {
        return false
      }

      if (signalFilter !== 'all') {
        const signals = runtimeSignals(pathMetric)
        if (!signals.some((signal) => signal.id === signalFilter)) {
          return false
        }
      }

      return true
    })
    .sort((a, b) => {
      if (sortMode === 'latency') {
        const byLatency = b.average_latency_ms - a.average_latency_ms
        if (byLatency !== 0) {
          return byLatency
        }
        return b.requests - a.requests
      }

      if (sortMode === 'errorRate') {
        const aErrors = a.client_errors + a.server_errors
        const bErrors = b.client_errors + b.server_errors
        const aErrorRate = a.requests > 0 ? aErrors / a.requests : 0
        const bErrorRate = b.requests > 0 ? bErrors / b.requests : 0
        const byErrorRate = bErrorRate - aErrorRate
        if (byErrorRate !== 0) {
          return byErrorRate
        }
        return b.requests - a.requests
      }

      if (sortMode === 'path') {
        return a.path.localeCompare(b.path)
      }

      const byRequests = b.requests - a.requests
      if (byRequests !== 0) {
        return byRequests
      }
      return b.average_latency_ms - a.average_latency_ms
    })

  const visiblePaths = filteredPaths.slice(0, topPathLimit)
  const hasPathData = allPaths.length > 0
  const hasFilteredResults = visiblePaths.length > 0
  const activeFilterCount = [normalizedQuery, matchFilter !== 'all' ? matchFilter : '', signalFilter !== 'all' ? signalFilter : ''].filter(Boolean).length

  const resetTrafficControls = () => {
    setQuery('')
    setMatchFilter('all')
    setSignalFilter('all')
    setSortMode('requests')
    setTopPathLimit(5)
  }

  const hasData = Boolean(data)
  const hasChartHistory = trendPoints.length >= 2
  const latestTrendPoint = trendPoints.length > 0 ? trendPoints[trendPoints.length - 1] : null
  const previousTrendPoint = trendPoints.length > 1 ? trendPoints[trendPoints.length - 2] : null
  const latestTrendRequests = latestTrendPoint?.total_requests ?? 0
  const previousTrendRequests = previousTrendPoint?.total_requests ?? 0
  const latestTrendErrors = latestTrendPoint && latestTrendPoint.total_requests > 0
    ? Math.round(latestTrendPoint.error_rate * latestTrendPoint.total_requests)
    : 0
  const currentErrorRate = latestTrendPoint && latestTrendRequests > 0 ? clampRatio(latestTrendPoint.error_rate) : 0
  const previousErrorRate = previousTrendPoint && previousTrendRequests > 0 ? clampRatio(previousTrendPoint.error_rate) : 0
  const latestTrendLatency = latestTrendPoint?.average_latency_ms ?? 0
  const previousTrendLatency = previousTrendPoint?.average_latency_ms ?? 0
  const latestTrendMatchedPaths = latestTrendPoint?.matched_paths ?? 0
  const latestTrendUnmatchedPaths = latestTrendPoint?.unmatched_paths ?? 0
  const latestTrendTotalObservedPaths = latestTrendMatchedPaths + latestTrendUnmatchedPaths
  const matchedPathRate = latestTrendTotalObservedPaths > 0
    ? latestTrendMatchedPaths / latestTrendTotalObservedPaths
    : 0
  const coverageGapRate = latestTrendTotalObservedPaths > 0 ? 1 - matchedPathRate : 0
  const coverageState: 'no-traffic' | 'stable' | 'gap' = latestTrendTotalObservedPaths === 0
    ? 'no-traffic'
    : coverageGapRate > 0
      ? 'gap'
      : 'stable'

  const worstPath = data?.paths.reduce<PortalPathMetric | null>((best, current) => {
    if (!best) return current
    const bestErrors = best.client_errors + best.server_errors
    const currentErrors = current.client_errors + current.server_errors
    const bestRate = best.requests > 0 ? bestErrors / best.requests : 0
    const currentRate = current.requests > 0 ? currentErrors / current.requests : 0
    if (currentRate > bestRate) return current
    if (currentRate === bestRate && currentErrors > bestErrors) return current
    return best
  }, null)
  const worstPathErrorRate = worstPath && worstPath.requests > 0
    ? (worstPath.client_errors + worstPath.server_errors) / worstPath.requests
    : 0
  const criticalIssueVisible = Boolean(
    data && latestTrendPoint && latestTrendRequests > 0 && (
      latestTrendPoint.error_rate >= 0.5 ||
      worstPathErrorRate >= 0.5 ||
      (totalRateLimited > 0 && currentErrorRate >= 0.2)
    )
  )
  const criticalIssuePath = worstPath && worstPathErrorRate >= 0.5 ? worstPath.path : null
  const criticalIssueRate = criticalIssuePath ? worstPathErrorRate : currentErrorRate
  const criticalIssueMetric = criticalIssuePath ? worstPath : null
  const criticalIssueBody = criticalIssueMetric
    ? t('metrics.criticalIssueBody', {
      path: criticalIssueMetric.path,
      rate: formatPercent(criticalIssueRate),
      count: formatNumber(latestTrendRequests),
      failed: formatNumber(latestTrendErrors)
    })
    : t('metrics.criticalIssueFallbackBody', {
      rate: formatPercent(currentErrorRate),
      failed: formatNumber(totalErrors),
      count: formatNumber(data?.summary.total_requests ?? 0)
    })
  const criticalIssueHeadline = criticalIssuePath
    ? t('metrics.criticalIssueHeadline', { path: criticalIssuePath, rate: formatPercent(criticalIssueRate) })
    : t('metrics.criticalIssueOverallHeadline', { rate: formatPercent(currentErrorRate) })
  const investigateMetric = criticalIssueMetric ?? worstPath ?? data?.paths[0] ?? null
  const summaryDialogValues = {
    requests: trendPoints.map((point) => point.total_requests),
    latency: trendPoints.map((point) => point.average_latency_ms),
    errors: trendPoints.map((point) => clampRatio(point.error_rate) * 100)
  }
  const summaryDialogConfig = summaryDialog
    ? {
        title: summaryDialog === 'requests'
          ? t('metrics.requests')
          : summaryDialog === 'latency'
            ? t('metrics.avgLatency')
            : t('metrics.errorRate'),
        subtitle: summaryDialog === 'requests'
          ? t('metrics.requestsSeriesHint')
          : summaryDialog === 'latency'
            ? t('metrics.avgLatencyHint')
            : t('metrics.errorsSeriesHint'),
        seriesLabel: summaryDialog === 'requests'
          ? t('metrics.requests')
          : summaryDialog === 'latency'
            ? t('metrics.avgLatency')
            : t('metrics.errorRate'),
        values: summaryDialogValues[summaryDialog],
        stroke: summaryDialog === 'requests'
          ? '#0f766e'
          : summaryDialog === 'latency'
            ? '#7c3aed'
            : '#e11d48',
        formatValue: summaryDialog === 'requests'
          ? (value: number) => formatNumber(value)
          : summaryDialog === 'latency'
            ? (value: number) => formatLatency(value)
            : (value: number) => `${value.toFixed(value >= 10 ? 0 : 1)}%`
      }
    : null

  if (view === 'trend') {
    if (loading && !hasData) {
      return (
        <section className="space-y-5" aria-busy>
          <div className="rounded-[1.8rem] border border-slate-200 bg-white px-6 py-5 shadow-sm">
            <SkeletonLine className="h-3 w-28" />
            <SkeletonLine className="mt-4 h-9 w-64" />
            <SkeletonLine className="mt-3 h-4 w-[min(46rem,100%)]" />
            <div className="mt-5 flex flex-wrap gap-2">
              <SkeletonLine className="h-9 w-28 rounded-full" />
              <SkeletonLine className="h-9 w-28 rounded-full" />
              <SkeletonLine className="h-9 w-32 rounded-full" />
              <SkeletonLine className="h-9 w-36 rounded-full" />
            </div>
          </div>
          <div className="grid gap-5 xl:grid-cols-[minmax(0,1.35fr)_minmax(0,0.95fr)]">
            <div className="rounded-[1.7rem] border border-slate-200 bg-white p-6 shadow-[0_14px_32px_rgba(15,23,42,0.06)]">
              <SkeletonLine className="h-3 w-24" />
              <SkeletonLine className="mt-4 h-7 w-48" />
              <SkeletonLine className="mt-3 h-4 w-72" />
              <div className="mt-6 h-[380px] rounded-[1.5rem] bg-slate-100/70" />
            </div>
            <div className="space-y-5">
              <div className="rounded-[1.7rem] border border-slate-200 bg-white p-6 shadow-[0_14px_32px_rgba(15,23,42,0.06)]">
                <SkeletonLine className="h-3 w-28" />
                <SkeletonLine className="mt-4 h-7 w-40" />
                <SkeletonLine className="mt-3 h-4 w-64" />
                <div className="mt-6 h-[260px] rounded-[1.5rem] bg-slate-100/70" />
              </div>
              <div className="rounded-[1.7rem] border border-slate-200 bg-white p-6 shadow-[0_14px_32px_rgba(15,23,42,0.06)]">
                <SkeletonLine className="h-3 w-24" />
                <SkeletonLine className="mt-4 h-7 w-40" />
                <SkeletonLine className="mt-3 h-4 w-64" />
                <div className="mt-6 grid gap-3 sm:grid-cols-2">
                  {Array.from({ length: 4 }).map((_, index) => (
                    <div key={`trend-runtime-skeleton-${index}`} className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-4">
                      <SkeletonLine className="h-3 w-20" />
                      <SkeletonLine className="mt-4 h-6 w-24" />
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </section>
      )
    }
    return (
      <section className="space-y-5" aria-busy={loading || refreshing}>
        {!loading && error && (
          <AlertBanner errorKind={effectiveErrorKind} error={error} sourceUrl={sourceUrl} stale={hasData} />
        )}

        {criticalIssueVisible && (
          <div
            role="alert"
            className="sticky z-20 rounded-[1.6rem] border border-rose-300 bg-rose-50 px-5 py-4 text-rose-950 shadow-lg"
            style={{ top: 'calc(var(--header-height) + 12px)' }}
          >
            <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-start">
              <div className="flex items-start gap-3">
                <div className="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-rose-100 text-rose-700 ring-1 ring-rose-200" aria-hidden="true">
                  <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M12 9v4" />
                    <path d="M12 17h.01" />
                    <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z" />
                  </svg>
                </div>
                <div className="min-w-0">
                  <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-rose-700">
                    {t('metrics.criticalIssueTitle')}
                  </div>
                  <div className="mt-2 text-lg font-semibold">
                    {criticalIssueHeadline}
                  </div>
                  <p className="mt-2 max-w-3xl text-sm text-rose-900">
                    {criticalIssueBody}
                  </p>
                </div>
              </div>
              <div className="flex items-start justify-end gap-2 md:pt-1">
                {investigateMetric ? (
                  <button
                    type="button"
                    className="inline-flex min-h-[44px] items-center justify-center rounded-full bg-rose-700 px-4 text-sm font-semibold text-white transition hover:bg-rose-800"
                    onClick={() => onOpenCatalogSurface(investigateMetric, 'errors')}
                  >
                    {t('metrics.openFailingRoute')}
                  </button>
                ) : (
                  <button
                    type="button"
                    disabled
                    className="inline-flex min-h-[44px] items-center justify-center rounded-full bg-rose-200 px-4 text-sm font-semibold text-rose-500"
                  >
                    {t('metrics.openFailingRoute')}
                  </button>
                )}
              </div>
            </div>
          </div>
        )}

        {hasChartHistory ? (
          <div className="space-y-6">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.summary')}</div>
                <h3 className="mt-1 text-lg font-semibold text-slate-950">{t('metrics.trendTitle')}</h3>
                <p className="mt-1 text-sm text-slate-600">{t('metrics.subtitle')}</p>
              </div>
            </div>
            <div className="grid gap-5 md:grid-cols-3">
              <MetricSummaryCard
                title={t('metrics.requests')}
                value={formatNumber(latestTrendRequests)}
                delta={latestTrendRequests - previousTrendRequests}
                deltaUnit=" requests"
                deltaDigits={0}
                tone="teal"
                sparklineValues={trendPoints.map((point) => point.total_requests)}
                trendPoints={trendPoints}
                hoveredIndex={hoveredSummaryIndex}
                onHoverIndexChange={setHoveredSummaryIndex}
                sparklineValueFormatter={(value) => formatNumber(value)}
                onExpand={() => setSummaryDialog('requests')}
                expandLabel={t('metrics.requests')}
              />
              <MetricSummaryCard
                title={t('metrics.avgLatency')}
                value={formatLatency(latestTrendLatency)}
                delta={latestTrendLatency - previousTrendLatency}
                deltaUnit=" ms"
                deltaDigits={0}
                tone="violet"
                sparklineValues={trendPoints.map((point) => point.average_latency_ms)}
                trendPoints={trendPoints}
                hoveredIndex={hoveredSummaryIndex}
                onHoverIndexChange={setHoveredSummaryIndex}
                sparklineValueFormatter={(value) => formatLatency(value)}
                invertDelta
                onExpand={() => setSummaryDialog('latency')}
                expandLabel={t('metrics.avgLatency')}
              />
              <MetricSummaryCard
                title={t('metrics.errorRate')}
                value={formatPercent(currentErrorRate)}
                delta={currentErrorRate - previousErrorRate}
                deltaUnit=" pp"
                deltaDigits={1}
                tone="rose"
                sparklineValues={trendPoints.map((point) => clampRatio(point.error_rate) * 100)}
                trendPoints={trendPoints}
                hoveredIndex={hoveredSummaryIndex}
                onHoverIndexChange={setHoveredSummaryIndex}
                sparklineValueFormatter={(value) => `${value.toFixed(value >= 10 ? 0 : 1)}%`}
                invertDelta
                onExpand={() => setSummaryDialog('errors')}
                expandLabel={t('metrics.errorRate')}
              />
            </div>
            <CoverageTrendChart
              trendPoints={trendPoints}
              comparisonWindowMinutes={comparisonWindowMinutes}
              hoveredIndex={hoveredTrendIndex}
              onHoverIndexChange={setHoveredTrendIndex}
              frameClassName={coverageState === 'stable'
                ? 'border-emerald-200 bg-emerald-50/35'
                : coverageState === 'gap'
                  ? 'border-amber-200 bg-white'
                  : 'border-slate-200 bg-white'}
            />
          </div>
        ) : (
          <div className="rounded-[1.6rem] border border-dashed border-slate-300 bg-slate-50 px-6 py-8 text-sm text-slate-600">
            {t('metrics.noHistoryBody')}
          </div>
        )}

        {summaryDialogConfig && (
          <MetricSeriesDialog
            open
            title={summaryDialogConfig.title}
            subtitle={`${summaryDialogConfig.subtitle} · ${comparisonWindowMinutes}m`}
            seriesLabel={summaryDialogConfig.seriesLabel}
            values={summaryDialogConfig.values}
            trendPoints={trendPoints}
            stroke={summaryDialogConfig.stroke}
            formatValue={summaryDialogConfig.formatValue}
            onClose={() => setSummaryDialog(null)}
          />
        )}
      </section>
    )
  }

  return (
    <section className="space-y-5" aria-busy={loading || refreshing}>
        {criticalIssueVisible && (
          <div role="alert" className="rounded-[1.6rem] border border-rose-200 bg-rose-50 px-5 py-4 text-rose-950 shadow-sm">
            <div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-start">
              <div className="min-w-0">
                <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-rose-700">
                  {t('metrics.criticalIssueTitle')}
                </div>
                <div className="mt-2 text-lg font-semibold">
                  {criticalIssueHeadline}
                </div>
                <p className="mt-2 max-w-3xl text-sm text-rose-900">
                  {criticalIssueBody}
                </p>
              </div>
              {investigateMetric && (
                <div className="flex shrink-0 items-start justify-end md:pt-1">
                  <button
                    type="button"
                    className="inline-flex min-h-[44px] items-center justify-center rounded-full bg-rose-700 px-4 text-sm font-semibold text-white transition hover:bg-rose-800"
                    onClick={() => onOpenCatalogSurface(investigateMetric, 'errors')}
                  >
                    {t('metrics.openFailingRoute')}
                  </button>
                </div>
              )}
            </div>
          </div>
        )}

      {!loading && error && (
        <AlertBanner errorKind={effectiveErrorKind} error={error} sourceUrl={sourceUrl} stale={hasData} />
      )}

      {loading && !hasData && (
        <div aria-live="polite" role="status" className="space-y-6">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            {Array.from({ length: 4 }).map((_, index) => (
              <SkeletonCard key={`metrics-card-skeleton-${index}`} />
            ))}
          </div>
          <div className="grid gap-6 xl:grid-cols-[minmax(0,1.72fr)_minmax(300px,0.72fr)]">
            <div className="portal-card overflow-hidden rounded-[1.8rem]">
              <div className="border-b border-slate-200 px-6 py-5">
                <SkeletonLine className="h-3 w-20" />
                <SkeletonLine className="mt-4 h-8 w-56" />
                <div className="mt-5 grid gap-3 xl:grid-cols-[minmax(0,1.4fr)_180px_180px_180px_auto]">
                  <SkeletonLine className="h-12 w-full rounded-2xl" />
                  <SkeletonLine className="h-12 w-full rounded-2xl" />
                  <SkeletonLine className="h-12 w-full rounded-2xl" />
                  <SkeletonLine className="h-12 w-full rounded-2xl" />
                  <SkeletonLine className="h-12 w-full rounded-full" />
                </div>
              </div>
              <div className="bg-white">
                {Array.from({ length: 5 }).map((_, index) => (
                  <SkeletonTableRow key={`metrics-row-skeleton-${index}`} />
                ))}
              </div>
            </div>
            <div className="space-y-5">
              <div className="portal-card rounded-[1.8rem] p-6">
                <SkeletonLine className="h-3 w-24" />
                <SkeletonLine className="mt-4 h-8 w-44" />
                <div className="mt-5 space-y-4">
                  {Array.from({ length: 3 }).map((_, index) => (
                    <div key={`metrics-side-skeleton-${index}`} className="rounded-2xl border border-slate-200 bg-white px-4 py-4">
                      <SkeletonLine className="h-4 w-32" />
                      <SkeletonLine className="mt-4 h-2 w-full rounded-full" />
                      <SkeletonLine className="mt-4 h-3 w-28" />
                    </div>
                  ))}
                </div>
              </div>
              <div className="portal-card rounded-[1.8rem] p-6">
                <SkeletonLine className="h-3 w-24" />
                <SkeletonLine className="mt-4 h-8 w-40" />
                <div className="mt-5 grid gap-3 sm:grid-cols-2">
                  {Array.from({ length: 4 }).map((_, index) => (
                    <div key={`metrics-runtime-skeleton-${index}`} className="rounded-2xl border border-slate-200 bg-white px-4 py-4">
                      <SkeletonLine className="h-3 w-20" />
                      <SkeletonLine className="mt-4 h-6 w-24" />
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {!loading && data && (
        <>
          <div className="rounded-[1.5rem] border border-slate-200 bg-slate-50/70 px-4 py-4 shadow-sm">
            <div className="flex flex-col gap-2 lg:flex-row lg:items-start lg:justify-between">
              <div>
                <div className="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.coverage')}</div>
                <h3 className="mt-1 text-lg font-semibold text-slate-950">{t('metrics.coverageAndOutcomes')}</h3>
              </div>
            </div>

            <div className="mt-4 grid gap-3 lg:grid-cols-[minmax(18rem,0.78fr)_minmax(0,1.22fr)]">
              <div className={`self-start rounded-2xl border px-4 py-3 ${coverageState === 'gap' ? 'border-amber-200 bg-white' : coverageState === 'stable' ? 'border-emerald-200 bg-white' : 'border-slate-200 bg-white'}`}>
                {coverageState === 'no-traffic' ? (
                  <div className="flex flex-wrap items-center gap-3">
                    <span className="rounded-full border border-slate-200 bg-slate-50 px-3 py-1.5 text-xs font-semibold text-slate-700">
                      {t('metrics.noTrafficTitle')}
                    </span>
                    <span className="text-sm text-slate-600">
                      {t('metrics.noTrafficBody')}
                    </span>
                  </div>
                ) : coverageState === 'stable' ? (
                  <div className="flex flex-wrap items-center gap-3">
                    <span className="rounded-full border border-emerald-200 bg-emerald-50 px-3 py-1.5 text-xs font-semibold text-emerald-800">
                      {t('metrics.coverageStableMapped')}
                    </span>
                    <span className="text-sm text-slate-600">
                      {t('metrics.allObservedPathsMapped')}
                    </span>
                  </div>
                ) : (
                  <div>
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <div className="text-[10px] font-semibold uppercase tracking-[0.16em] text-slate-500">{t('metrics.coverageGap')}</div>
                        <div className="mt-1 text-sm text-slate-600">
                          {`${t('metrics.coverageGap')}: ${formatPercent(coverageGapRate)}`}
                        </div>
                      </div>
                      <div className="text-3xl font-semibold text-slate-950">{formatNumber(latestTrendUnmatchedPaths)}</div>
                    </div>
                    <div className="mt-3 h-2 overflow-hidden rounded-full bg-slate-100">
                      <div className="flex h-full">
                        <div
                          className={meterTone('emerald')}
                          style={{ width: `${Math.max(0, Math.min(100, matchedPathRate * 100))}%` }}
                        />
                        <div
                          className={meterTone('amber')}
                          style={{ width: `${Math.max(0, Math.min(100, coverageGapRate * 100))}%` }}
                        />
                      </div>
                    </div>
                  </div>
                )}
              </div>

              <div className="rounded-2xl border border-slate-200 bg-white px-4 py-3">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <div className="text-[10px] font-semibold uppercase tracking-[0.16em] text-slate-500">{t('metrics.outcomeTitle')}</div>
                    <div className="mt-1 text-sm text-slate-600">{t('metrics.requests')}</div>
                  </div>
                  <div className="text-2xl font-semibold text-slate-950">{formatNumber(data.summary.total_requests)}</div>
                </div>
                <div className="mt-3 h-2 overflow-hidden rounded-full bg-slate-100">
                  <div className="flex h-full">
                    <div className={meterTone('emerald')} style={{ width: `${Math.max(0, Math.min(100, successRate * 100))}%` }} />
                    <div className={meterTone('amber')} style={{ width: `${Math.max(0, Math.min(100, clientErrorRate * 100))}%` }} />
                    <div className={meterTone('rose')} style={{ width: `${Math.max(0, Math.min(100, serverErrorRate * 100))}%` }} />
                  </div>
                </div>
                <div className="mt-3 grid gap-2 sm:grid-cols-3">
                  {[
                    { label: t('metrics.success'), value: data.summary.success_responses, ratio: successRate, tone: 'emerald' as const },
                    { label: t('metrics.clientErrors'), value: data.summary.client_errors, ratio: clientErrorRate, tone: 'amber' as const },
                    { label: t('metrics.serverErrors'), value: data.summary.server_errors, ratio: serverErrorRate, tone: 'rose' as const }
                  ].map((item) => (
                    <div key={item.label} className="flex items-center justify-between gap-3 rounded-xl border border-slate-100 bg-slate-50 px-3 py-2 text-sm">
                      <div className="flex items-center gap-2 text-slate-700">
                        <span className={`h-2.5 w-2.5 rounded-full ${meterTone(item.tone)}`} />
                        <span>{item.label}</span>
                      </div>
                      <div className="text-right">
                        <div className="font-semibold text-slate-950">{formatPercent(item.ratio)}</div>
                        <div className="text-xs text-slate-500">{formatNumber(item.value)}</div>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>

          <div className="space-y-5">
              <div className="overflow-hidden rounded-[1.7rem] border border-slate-200 bg-white shadow-[0_14px_32px_rgba(15,23,42,0.06)]">
                <div className="border-b border-slate-200 px-6 py-5">
                  <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                    <div>
                      <div className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.topPaths')}</div>
                      <h3 className="mt-1 text-lg font-semibold text-slate-900">{t('metrics.topPathsTitle')}</h3>
                      <p className="mt-2 text-sm text-slate-600">{t('metrics.explorerBody')}</p>
                    </div>
                    <div className="rounded-full bg-slate-100 px-3 py-1 text-xs font-semibold text-slate-700">
                      {formatNumber(visiblePaths.length)} / {formatNumber(filteredPaths.length)} / {formatNumber(allPaths.length)}
                    </div>
                  </div>

                  <div className="mt-5 grid gap-3 xl:grid-cols-[minmax(0,1.35fr)_180px_180px_180px_180px_auto]">
                    <label className="block">
                      <div className="mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.topPathsControls')}</div>
                      <input
                        type="text"
                        value={query}
                        onChange={(event) => setQuery(event.target.value)}
                        placeholder={t('metrics.searchPlaceholder')}
                        className="w-full rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-900 placeholder:text-slate-600 focus:border-teal-500 focus:bg-white focus:outline-none focus:ring-4 focus:ring-teal-100"
                      />
                    </label>

                    <label className="block">
                      <div className="mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.matchFilterLabel')}</div>
                      <select
                        value={matchFilter}
                        onChange={(event) => setMatchFilter(event.target.value as MatchFilter)}
                        className="form-select w-full rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-900 focus:border-teal-500 focus:bg-white focus:outline-none focus:ring-4 focus:ring-teal-100"
                      >
                        <option value="all">{t('metrics.filterAll')}</option>
                        <option value="matched">{t('metrics.filterMatchedOnly')}</option>
                        <option value="unmatched">{t('metrics.filterUnmatchedOnly')}</option>
                      </select>
                    </label>

                    <label className="block">
                      <div className="mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.signalFilterLabel')}</div>
                      <select
                        value={signalFilter}
                        onChange={(event) => setSignalFilter(event.target.value as SignalFilter)}
                        className="form-select w-full rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-900 focus:border-teal-500 focus:bg-white focus:outline-none focus:ring-4 focus:ring-teal-100"
                      >
                        <option value="all">{t('metrics.signalAll')}</option>
                        <option value="hot">{t('metrics.signalHot')}</option>
                        <option value="slow">{t('metrics.signalSlow')}</option>
                        <option value="erroring">{t('metrics.signalErroring')}</option>
                      </select>
                    </label>

                    <label className="block">
                      <div className="mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.sortLabel')}</div>
                      <select
                        value={sortMode}
                        onChange={(event) => setSortMode(event.target.value as SortMode)}
                        className="form-select w-full rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-900 focus:border-teal-500 focus:bg-white focus:outline-none focus:ring-4 focus:ring-teal-100"
                      >
                        <option value="requests">{t('metrics.sortRequests')}</option>
                        <option value="latency">{t('metrics.sortLatency')}</option>
                        <option value="errorRate">{t('metrics.sortErrorRate')}</option>
                        <option value="path">{t('metrics.sortPath')}</option>
                      </select>
                    </label>

                    <label className="block">
                      <div className="mb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-500">
                        {t('metrics.topPathsLimitLabel')}
                      </div>
                      <select
                        value={topPathLimit}
                        onChange={(event) => setTopPathLimit(Number(event.target.value))}
                        className="form-select w-full rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-900 focus:border-teal-500 focus:bg-white focus:outline-none focus:ring-4 focus:ring-teal-100"
                      >
                        {[5, 10, 20, 50].map((limit) => (
                          <option key={limit} value={limit}>
                            {limit}
                          </option>
                        ))}
                      </select>
                    </label>

                    <div className="flex items-end">
                      <button
                        type="button"
                        onClick={resetTrafficControls}
                        className="w-full rounded-full border border-slate-200 bg-white px-4 py-3 text-sm font-semibold text-slate-700 transition hover:border-slate-300 hover:text-slate-900"
                      >
                        {t('search.clear')}
                      </button>
                    </div>
                  </div>

                  {activeFilterCount > 0 && (
                    <div className="mt-4 flex flex-wrap gap-2">
                      {query.trim() && (
                        <span className="rounded-full bg-slate-100 px-3 py-1 text-xs font-semibold text-slate-700">
                          {query.trim()}
                        </span>
                      )}
                      {matchFilter !== 'all' && (
                        <span className="rounded-full bg-teal-100 px-3 py-1 text-xs font-semibold text-teal-800">
                          {matchFilter === 'matched' ? t('metrics.filterMatchedOnly') : t('metrics.filterUnmatchedOnly')}
                        </span>
                      )}
                      {signalFilter !== 'all' && (
                        <span className="rounded-full bg-orange-100 px-3 py-1 text-xs font-semibold text-orange-800">
                          {signalFilter === 'hot'
                            ? t('metrics.signalHot')
                            : signalFilter === 'slow'
                              ? t('metrics.signalSlow')
                              : t('metrics.signalErroring')}
                        </span>
                      )}
                    </div>
                  )}
                </div>

                {hasFilteredResults ? (
                  <>
                    <div className="space-y-3 px-4 py-4 md:hidden">
                      {visiblePaths.map((pathMetric) => (
                        <TopPathMobileCard
                          key={`mobile-${pathMetric.path}`}
                          pathMetric={pathMetric}
                          trendValues={buildPathTrendValues(history, pathMetric.path, comparisonWindowMinutes)}
                        />
                      ))}
                    </div>
                    <div className="hidden overflow-x-auto md:block">
                      <table className="min-w-full divide-y divide-slate-200 text-sm" aria-label={t('metrics.topPathsTitle')}>
                        <caption className="sr-only">{t('metrics.topPathsTitle')}</caption>
                        <thead className="bg-slate-50 text-left text-[11px] font-semibold uppercase tracking-[0.18em] text-slate-500">
                          <tr>
                            <th className="px-6 py-3">{t('metrics.path')}</th>
                        <th className="px-6 py-3">{t('metrics.coverage')}</th>
                        <th className="px-6 py-3 text-right">{t('metrics.requests')}</th>
                        <th className="px-6 py-3">{t('metrics.pathTrend')}</th>
                        <th className="px-6 py-3 text-right">{t('metrics.errorRate')}</th>
                      </tr>
                        </thead>
                        <tbody className="divide-y divide-slate-100 bg-white">
                          {visiblePaths.map((pathMetric) => {
                            const pathErrors = pathMetric.client_errors + pathMetric.server_errors
                            const pathErrorRate = pathMetric.requests > 0 ? pathErrors / pathMetric.requests : 0
                            const signals = runtimeSignals(pathMetric)
                            const trendValues = buildPathTrendValues(history, pathMetric.path, comparisonWindowMinutes)
                            const actionable = Boolean(pathMetric.primary_match)
                            const openCatalog = () => {
                              if (actionable) {
                                onOpenCatalogSurface(pathMetric, pathErrorRate > 0 ? 'errors' : 'catalog')
                              }
                            }
                            const handleRowKeyDown = (event: KeyboardEvent<HTMLTableRowElement>) => {
                              if (!actionable) return
                              if (event.key === 'Enter' || event.key === ' ') {
                                event.preventDefault()
                                onOpenCatalogSurface(pathMetric, pathErrorRate > 0 ? 'errors' : 'catalog')
                              }
                            }

                            return (
                              <tr
                                key={pathMetric.path}
                                className={`align-top transition ${actionable ? 'cursor-pointer hover:bg-slate-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-teal-500' : ''}`}
                                onClick={openCatalog}
                                onKeyDown={handleRowKeyDown}
                                tabIndex={actionable ? 0 : -1}
                                role={actionable ? 'button' : undefined}
                                aria-label={actionable ? `${pathMetric.path} ${t('metrics.openCatalog')}` : undefined}
                              >
                                <td className="px-6 py-4">
                                  <code className="text-xs text-slate-900">{pathMetric.path}</code>
                                  <div className="mt-2 flex flex-wrap gap-2">
                                    {pathMetric.methods.length > 0 ? pathMetric.methods.map((method) => (
                                      <span key={`${pathMetric.path}-${method}`} className="rounded-full bg-slate-100 px-2 py-1 text-[11px] font-semibold text-slate-700">
                                        {method}
                                      </span>
                                    )) : (
                                      <span className="text-slate-600">-</span>
                                    )}
                                    {signals.map((signal) => (
                                      <span key={`${pathMetric.path}-${signal.id}`} className={`rounded-full px-2 py-1 text-[11px] font-semibold ${signal.className}`}>
                                        {signal.label}
                                      </span>
                                    ))}
                                  </div>
                                </td>
                                <td className="px-6 py-4">
                                  {pathMetric.primary_match ? (
                                    <div className="space-y-2">
                                      <div className="flex flex-wrap gap-2">
                                        <span className="rounded-full bg-emerald-100 px-2 py-1 text-[11px] font-semibold text-emerald-800">
                                          {t('metrics.mappedToCatalog')}
                                        </span>
                                        {normalizeObservedTrendPath(pathMetric.primary_match.path_pattern) !== normalizeObservedTrendPath(pathMetric.path) && (
                                          <span className="rounded-full bg-slate-100 px-2 py-1 text-[11px] font-medium text-slate-700">
                                            {t('metrics.catalogPath')} {pathMetric.primary_match.path_pattern}
                                          </span>
                                        )}
                                      </div>
                                      <div className="text-xs text-slate-600">
                                        {t('metrics.apiGroup')} {getGroupDisplayName(pathMetric.primary_match.group_name)}
                                      </div>
                                    </div>
                                  ) : (
                                    <div className="space-y-2">
                                      <span className="rounded-full bg-amber-100 px-2 py-1 text-[11px] font-semibold text-amber-800">
                                        {t('metrics.notMappedShort')}
                                      </span>
                                      <div className="text-xs text-slate-600">{t('metrics.noCatalogMatchBody')}</div>
                                    </div>
                                  )}
                                </td>
                                <td className="px-6 py-4 text-right font-semibold text-slate-900">{formatNumber(pathMetric.requests)}</td>
                                <td className="px-6 py-4">
                                  <PathSparkline values={trendValues} stroke={actionable ? '#0f766e' : '#94a3b8'} />
                                </td>
                                <td className="px-6 py-4 text-right">
                                  <div className={`font-semibold ${errorTone(pathErrorRate)}`}>{formatPercent(pathErrorRate)}</div>
                                  <div className="mt-1 text-xs text-slate-600">{formatNumber(pathErrors)} {t('metrics.errors')}</div>
                                </td>
                              </tr>
                            )
                          })}
                        </tbody>
                      </table>
                    </div>
                  </>
                ) : (
                  <div className="px-6 py-10">
                    <div className="rounded-[1.6rem] border border-dashed border-slate-300 bg-slate-50 px-6 py-8 text-center">
                      <div className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">
                        {hasPathData ? t('metrics.filteredEmptyTitle') : t('metrics.noTrafficTitle')}
                      </div>
                      <h3 className="mt-2 text-2xl font-semibold text-slate-900">
                        {hasPathData ? t('metrics.filteredEmptyTitle') : t('metrics.noTrafficTitle')}
                      </h3>
                      <p className="mx-auto mt-2 max-w-xl text-sm text-slate-600">
                        {hasPathData ? t('metrics.filteredEmptyBody') : t('metrics.noTrafficBody')}
                      </p>
                      <div className="mt-5 flex flex-wrap justify-center gap-3">
                        <button
                          type="button"
                          onClick={resetTrafficControls}
                          className="rounded-full border border-slate-200 bg-white px-5 py-2 text-sm font-semibold text-slate-700 transition hover:border-slate-300 hover:text-slate-900"
                        >
                          {t('search.clear')}
                        </button>
                        <button
                          type="button"
                          onClick={onRefresh}
                          className="rounded-full bg-slate-950 px-5 py-2 text-sm font-semibold text-white transition hover:bg-slate-800"
                        >
                          {t('metrics.refresh')}
                        </button>
                      </div>
                    </div>
                  </div>
                )}
              </div>
          </div>
        </>
      )}

      {!loading && !data && !error && (
        <div className="rounded-[1.8rem] border border-slate-200 bg-white px-6 py-10 text-center shadow-sm">
          <div className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500">{t('metrics.noTrafficTitle')}</div>
          <h3 className="mt-2 text-2xl font-semibold text-slate-900">{t('metrics.noTrafficTitle')}</h3>
          <p className="mx-auto mt-2 max-w-2xl text-sm text-slate-600">{t('metrics.empty')}</p>
        </div>
      )}
    </section>
  )
}
