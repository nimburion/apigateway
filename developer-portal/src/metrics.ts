import { GroupData, PortalMetricMatch, PortalMetricsData, PortalPathMetric, PortalSurfaceMetricSummary } from './types'

type PathAggregate = {
  methods: Set<string>;
  requests: number;
  success_responses: number;
  client_errors: number;
  server_errors: number;
  rate_limited_responses: number;
  status_401_responses: number;
  status_403_responses: number;
  status_429_responses: number;
  status_502_responses: number;
  status_503_responses: number;
  status_504_responses: number;
  latency_sum_ms: number;
  latency_count: number;
}

type RouteCandidate = {
  group_name: string;
  group_prefix: string;
  path_pattern: string;
  pattern: RegExp;
  static_segments: number;
  segment_count: number;
  methods: string[];
  metadata: GroupData['metadata'];
  auth_required: boolean;
  has_rate_limit: boolean;
  has_openapi: boolean;
  deprecated: boolean;
}

const metricLinePattern = /^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{([^}]*)\})?\s+([^\s]+)$/
const labelPattern = /([a-zA-Z_][a-zA-Z0-9_]*)="((?:\\.|[^"])*)"/g

function emptyMetrics(): PortalMetricsData {
  return {
    summary: {
      total_requests: 0,
      in_flight_requests: 0,
      success_responses: 0,
      client_errors: 0,
      server_errors: 0,
      rate_limited_responses: 0,
      status_401_responses: 0,
      status_403_responses: 0,
      status_429_responses: 0,
      status_502_responses: 0,
      status_503_responses: 0,
      status_504_responses: 0,
      average_latency_ms: 0
    },
    runtime: {
      goroutines: 0,
      heap_alloc_bytes: 0,
      resident_memory_bytes: 0
    },
    paths: [],
    catalog_coverage: {
      matched_paths: 0,
      unmatched_paths: 0,
      matched_requests: 0,
      unmatched_requests: 0
    }
  }
}

function parseLabels(raw: string): Record<string, string> {
  const labels: Record<string, string> = {}
  for (const match of raw.matchAll(labelPattern)) {
    const [, name, value] = match
    labels[name] = value.replace(/\\"/g, '"').replace(/\\\\/g, '\\')
  }
  return labels
}

function metricValue(raw: string): number | null {
  const value = Number(raw)
  return Number.isFinite(value) ? value : null
}

function ensurePath(paths: Map<string, PathAggregate>, path: string): PathAggregate {
  const key = normalizeObservedPath(path)
  const current = paths.get(key)
  if (current) {
    return current
  }
  const created: PathAggregate = {
    methods: new Set<string>(),
    requests: 0,
    success_responses: 0,
    client_errors: 0,
    server_errors: 0,
    rate_limited_responses: 0,
    status_401_responses: 0,
    status_403_responses: 0,
    status_429_responses: 0,
    status_502_responses: 0,
    status_503_responses: 0,
    status_504_responses: 0,
    latency_sum_ms: 0,
    latency_count: 0
  }
  paths.set(key, created)
  return created
}

function sortedMethods(methods: Set<string>): string[] {
  return Array.from(methods).sort((a, b) => a.localeCompare(b))
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

function joinCatalogPath(...parts: string[]): string {
  const clean: string[] = []
  for (const part of parts) {
    const trimmed = part.trim()
    if (!trimmed || trimmed === '/') {
      continue
    }
    clean.push(trimmed.replace(/\/+$/g, ''))
  }
  if (clean.length === 0) {
    return '/'
  }
  const joined = clean.join('/').replace(/\/{2,}/g, '/')
  return joined.startsWith('/') ? joined : `/${joined}`
}

export function buildRouteMetricKey(groupName: string, pathPattern: string): string {
  return `${groupName}::${normalizeObservedPath(pathPattern)}`
}

function escapeRegExp(input: string): string {
  return input.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function pathPatternToRegExp(path: string): RegExp {
  const normalized = normalizeObservedPath(path)
  const segments = normalized.split('/').filter(Boolean)
  if (segments.length === 0) {
    return /^\/$/
  }
  const compiled = segments.map((segment) => {
    if (segment.startsWith(':') || (segment.startsWith('{') && segment.endsWith('}'))) {
      return '[^/]+'
    }
    return escapeRegExp(segment)
  })
  return new RegExp(`^/${compiled.join('/')}$`)
}

function staticSegmentCount(path: string): number {
  return normalizeObservedPath(path)
    .split('/')
    .filter(Boolean)
    .filter((segment) => !segment.startsWith(':') && !(segment.startsWith('{') && segment.endsWith('}')))
    .length
}

function segmentCount(path: string): number {
  return normalizeObservedPath(path).split('/').filter(Boolean).length
}

function buildRouteCandidates(groups: GroupData[]): RouteCandidate[] {
  const candidates: RouteCandidate[] = []
  for (const group of groups) {
    for (const route of group.routes) {
      const fullPath = joinCatalogPath(group.prefix, route.path_prefix)
      const methods = route.methods.map((method) => method.method.toUpperCase())
      candidates.push({
        group_name: group.name,
        group_prefix: group.prefix,
        path_pattern: fullPath,
        pattern: pathPatternToRegExp(fullPath),
        static_segments: staticSegmentCount(fullPath),
        segment_count: segmentCount(fullPath),
        methods,
        metadata: route.metadata,
        auth_required: route.auth_required,
        has_rate_limit: route.has_rate_limit,
        has_openapi: route.has_openapi,
        deprecated: route.deprecated
      })
    }
  }
  return candidates
}

function overlapMethods(left: string[], right: string[]): string[] {
  if (left.length === 0 || right.length === 0) {
    return []
  }
  const rightSet = new Set(right.map((value) => value.toUpperCase()))
  return left.filter((value) => rightSet.has(value.toUpperCase()))
}

function compareCandidates(a: RouteCandidate, b: RouteCandidate, matchedMethodsA: string[], matchedMethodsB: string[]): number {
  if (matchedMethodsA.length !== matchedMethodsB.length) {
    return matchedMethodsB.length - matchedMethodsA.length
  }
  if (a.static_segments !== b.static_segments) {
    return b.static_segments - a.static_segments
  }
  if (a.segment_count !== b.segment_count) {
    return b.segment_count - a.segment_count
  }
  return b.path_pattern.length - a.path_pattern.length
}

function correlatePathMetric(metric: PortalPathMetric, candidates: RouteCandidate[]): PortalPathMetric {
  const normalizedPath = normalizeObservedPath(metric.path)
  const matches = candidates
    .filter((candidate) => candidate.pattern.test(normalizedPath))
    .map((candidate) => ({
      candidate,
      matched_methods: overlapMethods(metric.methods, candidate.methods)
    }))
    .filter(({ candidate, matched_methods }) => matched_methods.length > 0 || metric.methods.length === 0 || candidate.methods.length === 0)
    .sort((left, right) => compareCandidates(left.candidate, right.candidate, left.matched_methods, right.matched_methods))

  if (matches.length === 0) {
    return {
      ...metric,
      primary_match: null,
      match_count: 0
    }
  }

  const primary = matches[0]
  const primaryMatch: PortalMetricMatch = {
    kind: 'route',
    group_name: primary.candidate.group_name,
    group_prefix: primary.candidate.group_prefix,
    path_pattern: primary.candidate.path_pattern,
    methods: primary.candidate.methods,
    matched_methods: primary.matched_methods,
    metadata: primary.candidate.metadata,
    auth_required: primary.candidate.auth_required,
    has_rate_limit: primary.candidate.has_rate_limit,
    has_openapi: primary.candidate.has_openapi,
    deprecated: primary.candidate.deprecated
  }

  return {
    ...metric,
    primary_match: primaryMatch,
    match_count: matches.length
  }
}

function summarizePathMetrics(paths: Map<string, PathAggregate>): PortalPathMetric[] {
  return Array.from(paths.entries())
    .map(([path, value]) => ({
      path,
      methods: sortedMethods(value.methods),
      requests: value.requests,
      success_responses: value.success_responses,
      client_errors: value.client_errors,
      server_errors: value.server_errors,
      rate_limited_responses: value.rate_limited_responses,
      status_401_responses: value.status_401_responses,
      status_403_responses: value.status_403_responses,
      status_429_responses: value.status_429_responses,
      status_502_responses: value.status_502_responses,
      status_503_responses: value.status_503_responses,
      status_504_responses: value.status_504_responses,
      average_latency_ms: value.latency_count > 0 ? value.latency_sum_ms / value.latency_count : 0
    }))
    .sort((a, b) => {
      if (a.requests === b.requests) {
        return a.path.localeCompare(b.path)
      }
      return b.requests - a.requests
    })
}

export function parsePrometheusMetrics(text: string): PortalMetricsData {
  const payload = emptyMetrics()
  const paths = new Map<string, PathAggregate>()
  let totalLatencySumMs = 0
  let totalLatencyCount = 0

  for (const rawLine of text.split('\n')) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) {
      continue
    }

    const match = line.match(metricLinePattern)
    if (!match) {
      continue
    }

    const [, metricName, , rawLabels = '', rawValue] = match
    const value = metricValue(rawValue)
    if (value === null) {
      continue
    }

    const labels = parseLabels(rawLabels)

    switch (metricName) {
      case 'http_requests_total': {
        const aggregate = ensurePath(paths, labels.path ?? '/')
        aggregate.requests += value
        payload.summary.total_requests += value
        if (labels.method) {
          aggregate.methods.add(labels.method.toUpperCase())
        }
        if ((labels.status ?? '').startsWith('2')) {
          aggregate.success_responses += value
          payload.summary.success_responses += value
        } else if ((labels.status ?? '').startsWith('4')) {
          aggregate.client_errors += value
          payload.summary.client_errors += value
          switch (labels.status ?? '') {
            case '401':
              aggregate.status_401_responses += value
              payload.summary.status_401_responses += value
              break
            case '403':
              aggregate.status_403_responses += value
              payload.summary.status_403_responses += value
              break
            case '429':
              aggregate.status_429_responses += value
              payload.summary.status_429_responses += value
              aggregate.rate_limited_responses += value
              payload.summary.rate_limited_responses += value
              break
          }
        } else if ((labels.status ?? '').startsWith('5')) {
          aggregate.server_errors += value
          payload.summary.server_errors += value
          switch (labels.status ?? '') {
            case '502':
              aggregate.status_502_responses += value
              payload.summary.status_502_responses += value
              break
            case '503':
              aggregate.status_503_responses += value
              payload.summary.status_503_responses += value
              break
            case '504':
              aggregate.status_504_responses += value
              payload.summary.status_504_responses += value
              break
          }
        }
        break
      }
      case 'http_request_duration_seconds_sum': {
        const aggregate = ensurePath(paths, labels.path ?? '/')
        if (labels.method) {
          aggregate.methods.add(labels.method.toUpperCase())
        }
        aggregate.latency_sum_ms += value * 1000
        totalLatencySumMs += value * 1000
        break
      }
      case 'http_request_duration_seconds_count': {
        const aggregate = ensurePath(paths, labels.path ?? '/')
        if (labels.method) {
          aggregate.methods.add(labels.method.toUpperCase())
        }
        aggregate.latency_count += value
        totalLatencyCount += value
        break
      }
      case 'http_requests_in_flight':
        payload.summary.in_flight_requests += value
        break
      case 'go_goroutines':
        payload.runtime.goroutines += value
        break
      case 'go_memstats_heap_alloc_bytes':
        payload.runtime.heap_alloc_bytes += value
        break
      case 'process_resident_memory_bytes':
        payload.runtime.resident_memory_bytes += value
        break
      default:
        break
    }
  }

  if (totalLatencyCount > 0) {
    payload.summary.average_latency_ms = totalLatencySumMs / totalLatencyCount
  }
  payload.paths = summarizePathMetrics(paths)

  return payload
}

export function correlateMetricsWithCatalog(metricsData: PortalMetricsData, groups: GroupData[]): PortalMetricsData {
  const candidates = buildRouteCandidates(groups)
  const paths = metricsData.paths.map((pathMetric) => correlatePathMetric(pathMetric, candidates))
  let matchedPaths = 0
  let unmatchedPaths = 0
  let matchedRequests = 0
  let unmatchedRequests = 0

  for (const pathMetric of paths) {
    if (pathMetric.primary_match) {
      matchedPaths += 1
      matchedRequests += pathMetric.requests
    } else {
      unmatchedPaths += 1
      unmatchedRequests += pathMetric.requests
    }
  }

  return {
    ...metricsData,
    paths,
    catalog_coverage: {
      matched_paths: matchedPaths,
      unmatched_paths: unmatchedPaths,
      matched_requests: matchedRequests,
      unmatched_requests: unmatchedRequests
    }
  }
}

function summarizeFilteredMetrics(metricsData: PortalMetricsData, paths: PortalPathMetric[]): PortalMetricsData {
  let totalRequests = 0
  let successResponses = 0
  let clientErrors = 0
  let serverErrors = 0
  let rateLimitedResponses = 0
  let status_401_responses = 0
  let status_403_responses = 0
  let status_429_responses = 0
  let status_502_responses = 0
  let status_503_responses = 0
  let status_504_responses = 0
  let weightedLatencyMs = 0
  let weightedLatencyCount = 0

  for (const pathMetric of paths) {
    totalRequests += pathMetric.requests
    successResponses += pathMetric.success_responses
    clientErrors += pathMetric.client_errors
    serverErrors += pathMetric.server_errors
    rateLimitedResponses += pathMetric.rate_limited_responses
    status_401_responses += pathMetric.status_401_responses ?? 0
    status_403_responses += pathMetric.status_403_responses ?? 0
    status_429_responses += pathMetric.status_429_responses ?? 0
    status_502_responses += pathMetric.status_502_responses ?? 0
    status_503_responses += pathMetric.status_503_responses ?? 0
    status_504_responses += pathMetric.status_504_responses ?? 0
    if (pathMetric.requests > 0) {
      weightedLatencyMs += pathMetric.average_latency_ms * pathMetric.requests
      weightedLatencyCount += pathMetric.requests
    }
  }

  return {
    ...metricsData,
    summary: {
      ...metricsData.summary,
      total_requests: totalRequests,
      success_responses: successResponses,
      client_errors: clientErrors,
      server_errors: serverErrors,
      rate_limited_responses: rateLimitedResponses,
      status_401_responses,
      status_403_responses,
      status_429_responses,
      status_502_responses,
      status_503_responses,
      status_504_responses,
      average_latency_ms: weightedLatencyCount > 0 ? weightedLatencyMs / weightedLatencyCount : 0
    },
    paths,
    catalog_coverage: {
      matched_paths: paths.filter((pathMetric) => Boolean(pathMetric.primary_match)).length,
      unmatched_paths: paths.filter((pathMetric) => !pathMetric.primary_match).length,
      matched_requests: paths.filter((pathMetric) => Boolean(pathMetric.primary_match)).reduce((total, pathMetric) => total + pathMetric.requests, 0),
      unmatched_requests: paths.filter((pathMetric) => !pathMetric.primary_match).reduce((total, pathMetric) => total + pathMetric.requests, 0)
    }
  }
}

export function filterMetricsByGroup(metricsData: PortalMetricsData, groupName: string | null): PortalMetricsData {
  if (!groupName) {
    return metricsData
  }

  const paths = metricsData.paths.filter((pathMetric) => pathMetric.primary_match?.group_name === groupName)
  return summarizeFilteredMetrics(metricsData, paths)
}

export function filterMetricsBySurface(metricsData: PortalMetricsData, pathPattern: string | null, methods: string[] = []): PortalMetricsData {
  if (!pathPattern) {
    return metricsData
  }

  const normalizedPattern = normalizeObservedPath(pathPattern)
  const normalizedMethods = methods.map((method) => method.toUpperCase())
  const paths = metricsData.paths.filter((pathMetric) => {
    if (!pathMetric.primary_match) {
      return false
    }
    if (normalizeObservedPath(pathMetric.primary_match.path_pattern) !== normalizedPattern) {
      return false
    }
    if (normalizedMethods.length === 0) {
      return true
    }
    return overlapMethods(pathMetric.methods, normalizedMethods).length > 0
  })

  return summarizeFilteredMetrics(metricsData, paths)
}

export function buildRouteMetricsIndex(metricsData: PortalMetricsData): Record<string, PortalSurfaceMetricSummary> {
  const index: Record<string, PortalSurfaceMetricSummary> = {}

  for (const pathMetric of metricsData.paths) {
    const match = pathMetric.primary_match
    if (!match) {
      continue
    }

    const key = buildRouteMetricKey(match.group_name, match.path_pattern)
    const current = index[key] ?? {
      requests: 0,
      success_responses: 0,
      client_errors: 0,
      server_errors: 0,
      rate_limited_responses: 0,
      average_latency_ms: 0,
      observed_paths: 0
    }

    const currentWeight = current.requests > 0 ? current.requests : current.observed_paths
    const nextWeight = pathMetric.requests > 0 ? pathMetric.requests : 1
    const weightedLatencyTotal = current.average_latency_ms * currentWeight
    current.requests += pathMetric.requests
    current.success_responses += pathMetric.success_responses
    current.client_errors += pathMetric.client_errors
    current.server_errors += pathMetric.server_errors
    current.rate_limited_responses += pathMetric.rate_limited_responses
    current.observed_paths += 1
    current.average_latency_ms = currentWeight+nextWeight > 0
      ? (weightedLatencyTotal + (pathMetric.average_latency_ms * nextWeight)) / (currentWeight + nextWeight)
      : 0
    index[key] = current
  }

  return index
}

export function buildGroupMetricsIndex(metricsData: PortalMetricsData): Record<string, PortalSurfaceMetricSummary> {
  const index: Record<string, PortalSurfaceMetricSummary> = {}

  for (const pathMetric of metricsData.paths) {
    const match = pathMetric.primary_match
    if (!match) {
      continue
    }

    const key = match.group_name
    const current = index[key] ?? {
      requests: 0,
      success_responses: 0,
      client_errors: 0,
      server_errors: 0,
      rate_limited_responses: 0,
      average_latency_ms: 0,
      observed_paths: 0
    }

    const currentWeight = current.requests > 0 ? current.requests : current.observed_paths
    const nextWeight = pathMetric.requests > 0 ? pathMetric.requests : 1
    const weightedLatencyTotal = current.average_latency_ms * currentWeight
    current.requests += pathMetric.requests
    current.success_responses += pathMetric.success_responses
    current.client_errors += pathMetric.client_errors
    current.server_errors += pathMetric.server_errors
    current.rate_limited_responses += pathMetric.rate_limited_responses
    current.observed_paths += 1
    current.average_latency_ms = currentWeight+nextWeight > 0
      ? (weightedLatencyTotal + (pathMetric.average_latency_ms * nextWeight)) / (currentWeight + nextWeight)
      : 0
    index[key] = current
  }

  return index
}
