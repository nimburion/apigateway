import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { PortalMetricsData } from '../types'
import { parsePrometheusMetrics } from '../metrics'
import { appendMetricsSnapshot } from '../metricsHistory'

export type MetricsErrorKind = 'auth' | 'not_found' | 'network' | 'parse' | 'unknown'

export interface MetricsState {
  data: PortalMetricsData | null
  loading: boolean
  refreshing: boolean
  error: string
  errorStatus: number | null
  errorKind: MetricsErrorKind | null
  lastUpdated: string | null
  sourceUrl: string
  refresh: () => void
}

type MetricsFetchError = Error & {
  status?: number
  kind?: MetricsErrorKind
}

function resolveMetricsUrl(): string {
  const metricsUrl = import.meta.env.VITE_METRICS_URL?.trim()
  if (metricsUrl) {
    return metricsUrl
  }

  const managementBaseUrl = import.meta.env.VITE_MANAGEMENT_BASE_URL?.trim()
  if (managementBaseUrl) {
    try {
      return new URL('/metrics', managementBaseUrl).toString()
    } catch {
      return '/metrics'
    }
  }

  return '/metrics'
}

function resolvePollIntervalMs(): number {
  const raw = import.meta.env.VITE_METRICS_POLL_INTERVAL_MS?.trim()
  if (!raw) {
    return 30000
  }
  const parsed = Number(raw)
  return Number.isFinite(parsed) && parsed >= 5000 ? parsed : 30000
}

function buildMetricsError(status: number | null, sourceUrl: string, responseText = ''): MetricsFetchError {
  const trimmed = responseText.trim()

  if (status === 401 || status === 403) {
    const error = new Error(trimmed || `Unable to read ${sourceUrl}`) as MetricsFetchError
    error.status = status
    error.kind = 'auth'
    return error
  }

  if (status === 404) {
    const error = new Error(trimmed || `Metrics endpoint not found at ${sourceUrl}`) as MetricsFetchError
    error.status = status
    error.kind = 'not_found'
    return error
  }

  const error = new Error(trimmed || (status ? `HTTP ${status}` : `Unable to read ${sourceUrl}`)) as MetricsFetchError
  error.status = status ?? undefined
  error.kind = 'unknown'
  return error
}

export function useMetrics(): MetricsState {
  const [data, setData] = useState<PortalMetricsData | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState('')
  const [errorStatus, setErrorStatus] = useState<number | null>(null)
  const [errorKind, setErrorKind] = useState<MetricsErrorKind | null>(null)
  const [lastUpdated, setLastUpdated] = useState<string | null>(null)
  const [key, setKey] = useState(0)
  const dataRef = useRef<PortalMetricsData | null>(null)
  const sourceUrl = useMemo(() => resolveMetricsUrl(), [])
  const pollIntervalMs = useMemo(() => resolvePollIntervalMs(), [])

  useEffect(() => {
    const controller = new AbortController()
    const isInitialLoad = !dataRef.current

    if (isInitialLoad) {
      setLoading(true)
    } else {
      setRefreshing(true)
    }
    setError('')
    setErrorStatus(null)
    setErrorKind(null)

    fetch(sourceUrl, {
      headers: { Accept: 'text/plain' },
      credentials: 'same-origin',
      signal: controller.signal
    })
      .then(async (res) => {
        const text = await res.text()
        if (!res.ok) {
          throw buildMetricsError(res.status, sourceUrl, text)
        }
        try {
          return parsePrometheusMetrics(text)
        } catch (cause) {
          const error = new Error(cause instanceof Error ? cause.message : `Unable to parse ${sourceUrl}`) as MetricsFetchError
          error.kind = 'parse'
          throw error
        }
      })
      .then((parsed) => {
        if (controller.signal.aborted) return
        dataRef.current = parsed
        setData(parsed)
        setLastUpdated(new Date().toISOString())
        appendMetricsSnapshot(parsed)
      })
      .catch((err: MetricsFetchError) => {
        if (controller.signal.aborted) return

        if (err.name === 'AbortError') {
          return
        }

        const inferredKind: MetricsErrorKind =
          err.kind
          ?? (err.status === 401 || err.status === 403
            ? 'auth'
            : err.status === 404
              ? 'not_found'
              : err instanceof TypeError
                ? 'network'
                : 'unknown')

        if (!dataRef.current) {
          setData(null)
        }
        setError(err.message || 'Failed to load metrics')
        setErrorStatus(err.status ?? null)
        setErrorKind(inferredKind)
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoading(false)
          setRefreshing(false)
        }
      })

    return () => { controller.abort() }
  }, [key, sourceUrl])

  useEffect(() => {
    if (pollIntervalMs <= 0) {
      return
    }

    const interval = window.setInterval(() => {
      if (document.visibilityState === 'visible') {
        setKey((current) => current + 1)
      }
    }, pollIntervalMs)

    return () => window.clearInterval(interval)
  }, [pollIntervalMs])

  const refresh = useCallback(() => setKey(k => k + 1), [])
  return { data, loading, refreshing, error, errorStatus, errorKind, lastUpdated, sourceUrl, refresh }
}
