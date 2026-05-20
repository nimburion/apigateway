import { useEffect, useState } from 'react'
import { PortalMetricsHistoryResponse, PortalMetricsSnapshot } from '../types'
import { readMetricsHistory } from '../metricsHistory'

export interface MetricsHistoryState {
  history: PortalMetricsSnapshot[]
  snapshotIntervalMs: number
  retentionMs: number
}

const defaultSnapshotIntervalMs = 15 * 60 * 1000
const defaultRetentionMs = 24 * 60 * 60 * 1000

function normalizeSnapshots(value: unknown): PortalMetricsSnapshot[] {
  if (!Array.isArray(value)) {
    return []
  }

  return value.filter((item): item is PortalMetricsSnapshot => {
    if (!item || typeof item !== 'object') {
      return false
    }
    const candidate = item as Partial<PortalMetricsSnapshot>
    return typeof candidate.captured_at === 'string' && Boolean(candidate.data)
  })
}

function mergeSnapshots(primary: PortalMetricsSnapshot[], secondary: PortalMetricsSnapshot[]): PortalMetricsSnapshot[] {
  const merged = new Map<string, PortalMetricsSnapshot>()

  for (const snapshot of secondary) {
    merged.set(snapshot.captured_at, snapshot)
  }

  for (const snapshot of primary) {
    merged.set(snapshot.captured_at, snapshot)
  }

  return Array.from(merged.values()).sort((left, right) => Date.parse(left.captured_at) - Date.parse(right.captured_at))
}

function normalizeHistoryState(payload: PortalMetricsHistoryResponse | null, fallbackHistory: PortalMetricsSnapshot[]): MetricsHistoryState {
  const payloadSnapshots = payload ? normalizeSnapshots(payload.snapshots) : []
  const mergedHistory = mergeSnapshots(payloadSnapshots, fallbackHistory)
  return {
    history: mergedHistory,
    snapshotIntervalMs: payload && Number.isFinite(payload.snapshot_interval_ms) ? payload.snapshot_interval_ms : defaultSnapshotIntervalMs,
    retentionMs: payload && Number.isFinite(payload.retention_ms) ? payload.retention_ms : defaultRetentionMs
  }
}

export function useMetricsHistory(refreshKey: string | null): MetricsHistoryState {
  const [state, setState] = useState<MetricsHistoryState>(() => ({
    history: readMetricsHistory(),
    snapshotIntervalMs: defaultSnapshotIntervalMs,
    retentionMs: defaultRetentionMs
  }))

  useEffect(() => {
    const controller = new AbortController()

    fetch('/api/portal/metrics/history', {
      headers: { Accept: 'application/json' },
      credentials: 'same-origin',
      signal: controller.signal
    })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`)
        }
        return response.json() as Promise<PortalMetricsHistoryResponse>
      })
      .then((payload) => {
        if (controller.signal.aborted) {
          return
        }
        setState(normalizeHistoryState(payload, readMetricsHistory()))
      })
      .catch(() => {
        if (controller.signal.aborted) {
          return
        }
        setState(normalizeHistoryState(null, readMetricsHistory()))
      })

    return () => controller.abort()
  }, [refreshKey])

  return state
}
