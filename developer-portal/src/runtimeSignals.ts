import type { PortalSurfaceMetricSummary } from './types'
import { t } from './i18n'

export interface RuntimeSignal {
  id: 'hot' | 'slow' | 'erroring' | 'rate_limited';
  label: string;
  className: string;
}

export interface RuntimeSignalMetric {
  requests: number;
  client_errors: number;
  server_errors: number;
  rate_limited_responses?: number;
  average_latency_ms: number;
}

const hotRequestsThreshold = 1000
const slowLatencyThresholdMs = 500
const slowRequestsThreshold = 10
const errorRequestsThreshold = 20
const errorCountThreshold = 5
const errorRateThreshold = 0.05
const rateLimitedThreshold = 1

export function runtimeSignals(metric: RuntimeSignalMetric | PortalSurfaceMetricSummary | null | undefined): RuntimeSignal[] {
  if (!metric) {
    return []
  }

  const requests = metric.requests
  const errors = metric.client_errors + metric.server_errors
  const rateLimitedResponses = metric.rate_limited_responses ?? 0
  const errorRate = requests > 0 ? errors / requests : 0
  const signals: RuntimeSignal[] = []

  if (requests >= hotRequestsThreshold) {
    signals.push({
      id: 'hot',
      label: t('runtime.hot'),
      className: 'bg-orange-100 text-orange-800'
    })
  }

  if (requests >= slowRequestsThreshold && metric.average_latency_ms >= slowLatencyThresholdMs) {
    signals.push({
      id: 'slow',
      label: t('runtime.slow'),
      className: 'bg-violet-100 text-violet-800'
    })
  }

  if (requests >= errorRequestsThreshold && errors >= errorCountThreshold && errorRate >= errorRateThreshold) {
    signals.push({
      id: 'erroring',
      label: t('runtime.erroring'),
      className: 'bg-rose-100 text-rose-800'
    })
  }

  if (rateLimitedResponses >= rateLimitedThreshold) {
    signals.push({
      id: 'rate_limited',
      label: t('runtime.rateLimited'),
      className: 'bg-amber-100 text-amber-800'
    })
  }

  return signals
}
