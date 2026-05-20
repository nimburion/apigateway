export type PriorityLevel = 'critical' | 'review' | 'controlled'

export type PriorityReasonCode =
  | 'managementPublic'
  | 'public'
  | 'noRateLimit'
  | 'deprecated'
  | 'experimental'
  | 'missingOwner'
  | 'missingDocs'
  | 'missingRunbook'
  | 'missingSupport'
  | 'highTraffic'
  | 'highErrors'
  | 'rateLimited'
  | 'openapiMissing'

export interface PriorityInputs {
  authRequired: boolean
  hasRateLimit: boolean
  deprecated?: boolean
  experimental?: boolean
  hasOpenApi?: boolean
  managementSurface?: boolean
  ownerTeam?: string
  docsUrl?: string
  runbookUrl?: string
  supportChannel?: string
  requests?: number
  errorRate?: number
  rateLimitedResponses?: number
}

export function surfacePriorityScore(input: PriorityInputs) {
  const score =
    (input.managementSurface && !input.authRequired ? 25 : 0) +
    (!input.authRequired ? 35 : -10) +
    (!input.hasRateLimit ? 20 : -10) +
    (input.deprecated ? 15 : 0) +
    (input.experimental ? 10 : 0) +
    (input.ownerTeam ? -8 : 12) +
    (input.docsUrl ? -4 : 6) +
    (input.runbookUrl ? -4 : 6) +
    (input.supportChannel ? -2 : 4) +
    (input.hasOpenApi === false ? 4 : -2) +
    ((input.requests ?? 0) >= 1000 ? 8 : 0) +
    ((input.errorRate ?? 0) >= 0.2 ? 12 : (input.errorRate ?? 0) >= 0.05 ? 6 : 0) +
    ((input.rateLimitedResponses ?? 0) > 0 ? 6 : 0)

  return Math.max(0, Math.min(100, Math.round(score)))
}

export function surfacePriorityLevel(score: number): PriorityLevel {
  if (score >= 60) return 'critical'
  if (score >= 30) return 'review'
  return 'controlled'
}

export function surfacePriorityReasonCodes(input: PriorityInputs): PriorityReasonCode[] {
  const reasons: PriorityReasonCode[] = []
  if (input.managementSurface && !input.authRequired) reasons.push('managementPublic')
  if (!input.authRequired) reasons.push('public')
  if (!input.hasRateLimit) reasons.push('noRateLimit')
  if (input.deprecated) reasons.push('deprecated')
  if (input.experimental) reasons.push('experimental')
  if (!input.ownerTeam) reasons.push('missingOwner')
  if (!input.docsUrl) reasons.push('missingDocs')
  if (!input.runbookUrl) reasons.push('missingRunbook')
  if (!input.supportChannel) reasons.push('missingSupport')
  if ((input.errorRate ?? 0) >= 0.2) reasons.push('highErrors')
  if ((input.requests ?? 0) >= 1000) reasons.push('highTraffic')
  if ((input.rateLimitedResponses ?? 0) > 0) reasons.push('rateLimited')
  if (input.hasOpenApi === false) reasons.push('openapiMissing')
  return Array.from(new Set(reasons))
}
