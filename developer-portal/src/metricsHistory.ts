import { GroupData, PortalMetricsData, PortalMetricsDelta, PortalMetricsSnapshot, PortalMetricsTrendPoint, PortalMetricsTrendSummary, PortalPathMetric } from './types'
import { correlateMetricsWithCatalog, filterMetricsByGroup, filterMetricsBySurface } from './metrics'

const storageKey = 'nimburion.portal.metrics.history'
const maxSnapshots = 96
const maxAgeMs = 24 * 60 * 60 * 1000
function canUseStorage(): boolean {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined'
}

function isMetricsSnapshot(value: unknown): value is PortalMetricsSnapshot {
  if (!value || typeof value !== 'object') {
    return false
  }

  const candidate = value as Partial<PortalMetricsSnapshot>
  return typeof candidate.captured_at === 'string' && Boolean(candidate.data)
}

function pruneHistory(history: PortalMetricsSnapshot[]): PortalMetricsSnapshot[] {
  const now = Date.now()
  return history
    .filter((snapshot) => {
      const capturedAt = Date.parse(snapshot.captured_at)
      return Number.isFinite(capturedAt) && now - capturedAt <= maxAgeMs
    })
    .sort((left, right) => Date.parse(left.captured_at) - Date.parse(right.captured_at))
    .slice(-maxSnapshots)
}

export function readMetricsHistory(): PortalMetricsSnapshot[] {
  if (!canUseStorage()) {
    return []
  }

  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) {
      return []
    }

    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) {
      return []
    }

    return pruneHistory(parsed.filter(isMetricsSnapshot))
  } catch {
    return []
  }
}

function writeMetricsHistory(history: PortalMetricsSnapshot[]) {
  if (!canUseStorage()) {
    return
  }

  window.localStorage.setItem(storageKey, JSON.stringify(pruneHistory(history)))
}

export function appendMetricsSnapshot(data: PortalMetricsData): PortalMetricsSnapshot[] {
  const nextHistory = pruneHistory([
    ...readMetricsHistory(),
    {
      captured_at: new Date().toISOString(),
      data
    }
  ])
  writeMetricsHistory(nextHistory)
  return nextHistory
}

function applyCurrentScope(
  data: PortalMetricsData,
  routes: GroupData[],
  groupName: string | null,
  pathPattern: string | null,
  methods: string[]
): PortalMetricsData {
  const correlated = correlateMetricsWithCatalog(data, routes)
  const groupScoped = filterMetricsByGroup(correlated, groupName)
  return filterMetricsBySurface(groupScoped, pathPattern, methods)
}

function buildDelta(current: number, previous: number): PortalMetricsDelta {
  const absolute = current - previous
  const ratio = previous !== 0 ? absolute / previous : current !== 0 ? 1 : 0
  return {
    absolute,
    ratio,
    direction: absolute === 0 ? 'flat' : absolute > 0 ? 'up' : 'down'
  }
}

function totalErrorRate(data: PortalMetricsData): number {
  if (data.summary.total_requests <= 0) {
    return 0
  }

  return (data.summary.client_errors + data.summary.server_errors) / data.summary.total_requests
}

function coverageRate(data: PortalMetricsData): number {
  const totalObservedRequests = data.catalog_coverage.matched_requests + data.catalog_coverage.unmatched_requests
  if (totalObservedRequests <= 0) {
    return 0
  }

  return data.catalog_coverage.matched_requests / totalObservedRequests
}

function normalizeObservedPath(path: string): string {
  const trimmed = path.trim()
  if (!trimmed) {
    return '/'
  }
  const withoutQuery = trimmed.split('?')[0] || '/'
  const normalized = withoutQuery.replace(/\/{2,}/g, '/')
  if (normalized.length > 1 && normalized.endsWith('/')) {
    return normalized.slice(0, -1)
  }
  return normalized || '/'
}

function buildPathIndex(data: PortalMetricsData): Map<string, PortalPathMetric> {
  const index = new Map<string, PortalPathMetric>()
  for (const pathMetric of data.paths) {
    index.set(normalizeObservedPath(pathMetric.path), pathMetric)
  }
  return index
}

function findWindowBaselineSnapshot(
  history: PortalMetricsSnapshot[],
  comparisonWindowMinutes: number
): PortalMetricsSnapshot | null {
  if (history.length < 2) {
    return null
  }

  const currentSnapshot = history[history.length - 1]
  const currentTimestamp = Date.parse(currentSnapshot.captured_at)
  if (!Number.isFinite(currentTimestamp)) {
    return null
  }

  const comparisonWindowMs = Math.max(1, comparisonWindowMinutes) * 60 * 1000
  const baseline = [...history]
    .slice(0, -1)
    .reverse()
    .find((snapshot) => currentTimestamp - Date.parse(snapshot.captured_at) >= comparisonWindowMs)

  return baseline ?? history[history.length - 2] ?? null
}

export function summarizeMetricsHistory(
  history: PortalMetricsSnapshot[],
  routes: GroupData[],
  groupName: string | null,
  pathPattern: string | null,
  methods: string[],
  comparisonWindowMinutes = 15
): PortalMetricsTrendSummary | null {
  if (history.length < 2) {
    return null
  }

  const currentSnapshot = history[history.length - 1]
  const currentTimestamp = Date.parse(currentSnapshot.captured_at)
  if (!Number.isFinite(currentTimestamp)) {
    return null
  }

  const comparisonWindowMs = Math.max(1, comparisonWindowMinutes) * 60 * 1000
  const previousInWindow = [...history]
    .slice(0, -1)
    .reverse()
    .find((snapshot) => currentTimestamp - Date.parse(snapshot.captured_at) >= comparisonWindowMs)

  const baselineSnapshot = previousInWindow ?? history[history.length - 2]
  const baselineTimestamp = Date.parse(baselineSnapshot.captured_at)
  if (!Number.isFinite(baselineTimestamp)) {
    return null
  }

  const current = applyCurrentScope(currentSnapshot.data, routes, groupName, pathPattern, methods)
  const baseline = applyCurrentScope(baselineSnapshot.data, routes, groupName, pathPattern, methods)
  const baselineAgeMinutes = Math.max(1, Math.round((currentTimestamp - baselineTimestamp) / 60000))

  return {
    baseline_label: previousInWindow ? 'window' : 'previous',
    baseline_age_minutes: baselineAgeMinutes,
    snapshot_count: history.length,
    total_requests: buildDelta(current.summary.total_requests, baseline.summary.total_requests),
    average_latency_ms: buildDelta(current.summary.average_latency_ms, baseline.summary.average_latency_ms),
    error_rate: buildDelta(totalErrorRate(current), totalErrorRate(baseline)),
    coverage_rate: buildDelta(coverageRate(current), coverageRate(baseline))
  }
}

export function buildMetricsTrendPoints(
  history: PortalMetricsSnapshot[],
  routes: GroupData[],
  groupName: string | null,
  pathPattern: string | null,
  methods: string[],
  comparisonWindowMinutes = 15
): PortalMetricsTrendPoint[] {
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
      const scoped = applyCurrentScope(snapshot.data, routes, groupName, pathPattern, methods)
      const previousSnapshot = index > 0 ? recent[index - 1] : null
      const previousScoped = previousSnapshot ? applyCurrentScope(previousSnapshot.data, routes, groupName, pathPattern, methods) : null
      const intervalRequests = previousScoped
        ? Math.max(0, scoped.summary.total_requests - previousScoped.summary.total_requests)
        : 0
      const intervalClientErrors = previousScoped
        ? Math.max(0, scoped.summary.client_errors - previousScoped.summary.client_errors)
        : 0
      const intervalServerErrors = previousScoped
        ? Math.max(0, scoped.summary.server_errors - previousScoped.summary.server_errors)
        : 0
      const intervalErrors = intervalClientErrors + intervalServerErrors
      const intervalLatency = previousScoped && intervalRequests > 0
        ? Math.max(0, (scoped.summary.average_latency_ms * scoped.summary.total_requests) - (previousScoped.summary.average_latency_ms * previousScoped.summary.total_requests)) / intervalRequests
        : scoped.summary.average_latency_ms
      const matchedRequests = previousScoped
        ? Math.max(0, scoped.catalog_coverage.matched_requests - previousScoped.catalog_coverage.matched_requests)
        : 0
      const unmatchedRequests = previousScoped
        ? Math.max(0, scoped.catalog_coverage.unmatched_requests - previousScoped.catalog_coverage.unmatched_requests)
        : 0
      const matchedPaths = previousScoped
        ? Math.max(0, scoped.catalog_coverage.matched_paths - previousScoped.catalog_coverage.matched_paths)
        : 0
      const unmatchedPaths = previousScoped
        ? Math.max(0, scoped.catalog_coverage.unmatched_paths - previousScoped.catalog_coverage.unmatched_paths)
        : 0
      const intervalObservedRequests = matchedRequests + unmatchedRequests
      const intervalCoverageRate = intervalObservedRequests > 0 ? matchedRequests / intervalObservedRequests : 0

      return {
        captured_at: snapshot.captured_at,
        total_requests: intervalRequests,
        average_latency_ms: intervalLatency,
        error_rate: intervalRequests > 0 ? intervalErrors / intervalRequests : 0,
        coverage_rate: intervalCoverageRate,
        matched_requests: matchedRequests,
        unmatched_requests: unmatchedRequests,
        matched_paths: matchedPaths,
        unmatched_paths: unmatchedPaths
      }
    })
}

export function buildWindowMetricsData(
  history: PortalMetricsSnapshot[],
  routes: GroupData[],
  comparisonWindowMinutes = 15
): PortalMetricsData | null {
  if (history.length === 0) {
    return null
  }

  const currentSnapshot = history[history.length - 1]
  const currentTimestamp = Date.parse(currentSnapshot.captured_at)
  if (!Number.isFinite(currentTimestamp)) {
    return null
  }

  const baselineSnapshot = findWindowBaselineSnapshot(history, comparisonWindowMinutes)
  if (!baselineSnapshot) {
    return correlateMetricsWithCatalog(currentSnapshot.data, routes)
  }

  const currentScoped = correlateMetricsWithCatalog(currentSnapshot.data, routes)
  const baselineScoped = baselineSnapshot ? correlateMetricsWithCatalog(baselineSnapshot.data, routes) : null
  const baselinePaths = baselineScoped ? buildPathIndex(baselineScoped) : new Map<string, PortalPathMetric>()

  const aggregatedPaths: PortalPathMetric[] = []
  let totalRequests = 0
  let totalSuccessResponses = 0
  let totalClientErrors = 0
  let totalServerErrors = 0
  let totalRateLimitedResponses = 0
  let totalLatencySum = 0
  let totalLatencyCount = 0
  let matchedRequests = 0
  let unmatchedRequests = 0
  let matchedPaths = 0
  let unmatchedPaths = 0

  for (const currentPath of currentScoped.paths) {
    const baselinePath = baselinePaths.get(normalizeObservedPath(currentPath.path)) ?? null
    const requests = baselinePath ? Math.max(0, currentPath.requests - baselinePath.requests) : currentPath.requests
    const successResponses = baselinePath ? Math.max(0, currentPath.success_responses - baselinePath.success_responses) : currentPath.success_responses
    const clientErrors = baselinePath ? Math.max(0, currentPath.client_errors - baselinePath.client_errors) : currentPath.client_errors
    const serverErrors = baselinePath ? Math.max(0, currentPath.server_errors - baselinePath.server_errors) : currentPath.server_errors
    const rateLimitedResponses = baselinePath ? Math.max(0, currentPath.rate_limited_responses - baselinePath.rate_limited_responses) : currentPath.rate_limited_responses
    const latencyCount = baselinePath ? Math.max(0, currentPath.requests - baselinePath.requests) : currentPath.requests
    const latencySum = baselinePath
      ? Math.max(0, (currentPath.average_latency_ms * currentPath.requests) - (baselinePath.average_latency_ms * baselinePath.requests))
      : currentPath.average_latency_ms * currentPath.requests
    const averageLatencyMs = latencyCount > 0 ? latencySum / latencyCount : currentPath.average_latency_ms

    const deltaPath: PortalPathMetric = {
      ...currentPath,
      requests,
      success_responses: successResponses,
      client_errors: clientErrors,
      server_errors: serverErrors,
      rate_limited_responses: rateLimitedResponses,
      average_latency_ms: averageLatencyMs
    }
    aggregatedPaths.push(deltaPath)

    totalRequests += requests
    totalSuccessResponses += successResponses
    totalClientErrors += clientErrors
    totalServerErrors += serverErrors
    totalRateLimitedResponses += rateLimitedResponses
    totalLatencySum += latencySum
    totalLatencyCount += latencyCount

    if (deltaPath.primary_match) {
      matchedPaths += 1
      matchedRequests += requests
    } else {
      unmatchedPaths += 1
      unmatchedRequests += requests
    }
  }

  return {
    summary: {
      total_requests: totalRequests,
      in_flight_requests: currentScoped.summary.in_flight_requests,
      success_responses: totalSuccessResponses,
      client_errors: totalClientErrors,
      server_errors: totalServerErrors,
      rate_limited_responses: totalRateLimitedResponses,
      average_latency_ms: totalLatencyCount > 0 ? totalLatencySum / totalLatencyCount : currentScoped.summary.average_latency_ms
    },
    runtime: currentScoped.runtime,
    paths: aggregatedPaths.sort((left, right) => right.requests - left.requests || left.path.localeCompare(right.path)),
    catalog_coverage: {
      matched_paths: matchedPaths,
      unmatched_paths: unmatchedPaths,
      matched_requests: matchedRequests,
      unmatched_requests: unmatchedRequests
    }
  }
}

export function buildGroupWindowErrorIndex(
  history: PortalMetricsSnapshot[],
  routes: GroupData[],
  comparisonWindowMinutes = 15
): Record<string, number> {
  if (history.length < 2) {
    return {}
  }

  const currentSnapshot = history[history.length - 1]
  if (!Number.isFinite(Date.parse(currentSnapshot.captured_at))) {
    return {}
  }

  const baselineSnapshot = findWindowBaselineSnapshot(history, comparisonWindowMinutes)
  if (!baselineSnapshot) {
    return {}
  }

  const index: Record<string, number> = {}
  for (const group of routes) {
    const current = applyCurrentScope(currentSnapshot.data, routes, group.name, null, [])
    const baseline = applyCurrentScope(baselineSnapshot.data, routes, group.name, null, [])
    const currentErrors = current.summary.client_errors + current.summary.server_errors
    const baselineErrors = baseline.summary.client_errors + baseline.summary.server_errors
    const delta = Math.max(0, currentErrors - baselineErrors)
    if (delta > 0) {
      index[group.name] = delta
    }
  }

  return index
}
