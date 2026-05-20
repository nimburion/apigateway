export interface ResourceMetadata {
  owner_team: string;
  domain: string;
  visibility: string;
  status: string;
  docs_url: string;
  runbook_url: string;
  support_channel: string;
}

export interface RateLimitInfo {
  requests_per_second: number;
  burst: number;
  source: string;
}

export interface MethodInfo {
  method: string;
  scopes: string[];
  middlewares: string[];
  declared_middlewares: string[];
  disabled_middlewares: string[];
  auth_required: boolean;
  has_rate_limit: boolean;
  rate_limit?: RateLimitInfo | null;
}

export interface RouteInfo {
  path_prefix: string;
  target_url: string;
  methods: MethodInfo[];
  openapi?: OpenAPIInfo | null;
  metadata: ResourceMetadata;
  middlewares: string[];
  declared_middlewares: string[];
  disabled_middlewares: string[];
  endpoint_middlewares: string[];
  endpoint_disabled_middlewares: string[];
  auth_required: boolean;
  has_openapi: boolean;
  has_rate_limit: boolean;
  rate_limit?: RateLimitInfo | null;
  deprecated: boolean;
  has_openapi_errors: boolean;
  exposes_target_url: boolean;
  exposes_openapi_errors: boolean;
  runtime_only: boolean;
  surface_context: string;
}

export interface OpenAPIInfo {
  file: string;
  mode: string;
  title?: string;
  version?: string;
  description?: string;
  operations?: OpenAPIOperation[];
  error?: string;
}

export interface OpenAPIOperation {
  path: string;
  method: string;
  summary?: string;
  operation_id?: string;
  deprecated?: boolean;
}

export interface WebSocketInfo {
  path: string;
  target_url: string;
  scopes: string[];
  metadata: ResourceMetadata;
  middlewares: string[];
  declared_middlewares: string[];
  disabled_middlewares: string[];
  auth_required: boolean;
  has_rate_limit: boolean;
  rate_limit?: RateLimitInfo | null;
  deprecated: boolean;
  exposes_target_url: boolean;
}

export interface GroupData {
  name: string;
  prefix: string;
  metadata: ResourceMetadata;
  middlewares: string[];
  auth_required: boolean;
  has_rate_limit: boolean;
  has_rate_limited_surfaces: boolean;
  rate_limit?: RateLimitInfo | null;
  deprecated: boolean;
  routes: RouteInfo[];
  websockets: WebSocketInfo[];
}

export interface GroupInfo {
  name: string;
  prefix: string;
  metadata: ResourceMetadata;
  middlewares: string[];
  has_oauth2: boolean;
  has_me_api: boolean;
  route_count: number;
  websocket_count: number;
  auth_required: boolean;
  has_openapi: boolean;
  has_rate_limit: boolean;
  rate_limit?: RateLimitInfo | null;
  deprecated: boolean;
  runtime_info: {
    auth_enabled: boolean;
    management_enabled: boolean;
    management_auth_enabled: boolean;
    portal_mode: string;
    framework_middlewares: string[];
  };
}

export interface SelectedRouteSurface {
  kind: 'route';
  id: string;
  group_name: string;
  group_prefix: string;
  group_middlewares: string[];
  route: RouteInfo;
  method: MethodInfo;
}

export interface SelectedWebSocketSurface {
  kind: 'websocket';
  id: string;
  group_name: string;
  group_prefix: string;
  group_middlewares: string[];
  websocket: WebSocketInfo;
}

export type SelectedSurface = SelectedRouteSurface | SelectedWebSocketSurface

export interface PortalMetricsSummary {
  total_requests: number;
  in_flight_requests: number;
  success_responses: number;
  client_errors: number;
  server_errors: number;
  rate_limited_responses: number;
  average_latency_ms: number;
}

export interface PortalRuntimeMetrics {
  goroutines: number;
  heap_alloc_bytes: number;
  resident_memory_bytes: number;
}

export interface PortalPathMetric {
  path: string;
  methods: string[];
  requests: number;
  success_responses: number;
  client_errors: number;
  server_errors: number;
  rate_limited_responses: number;
  average_latency_ms: number;
  primary_match?: PortalMetricMatch | null;
  match_count?: number;
}

export interface PortalMetricMatch {
  kind: 'route';
  group_name: string;
  group_prefix: string;
  path_pattern: string;
  methods: string[];
  matched_methods: string[];
  metadata: ResourceMetadata;
  auth_required: boolean;
  has_rate_limit: boolean;
  has_openapi: boolean;
  deprecated: boolean;
}

export interface PortalSurfaceMetricSummary {
  requests: number;
  success_responses: number;
  client_errors: number;
  server_errors: number;
  rate_limited_responses: number;
  average_latency_ms: number;
  observed_paths: number;
}

export interface PortalMetricsCatalogCoverage {
  matched_paths: number;
  unmatched_paths: number;
  matched_requests: number;
  unmatched_requests: number;
}

export interface PortalMetricsData {
  summary: PortalMetricsSummary;
  runtime: PortalRuntimeMetrics;
  paths: PortalPathMetric[];
  catalog_coverage: PortalMetricsCatalogCoverage;
}

export interface PortalMetricsSnapshot {
  captured_at: string;
  data: PortalMetricsData;
}

export interface PortalMetricsHistoryResponse {
  source: string;
  snapshot_count: number;
  snapshot_interval_ms: number;
  retention_ms: number;
  snapshots: PortalMetricsSnapshot[];
}

export interface ManagedConfigVersion {
  version: number;
  status: string;
  routes: unknown;
  checksum: string;
  created_by?: string;
  message?: string;
  base_version?: number;
  validated_at?: string;
  failed_at?: string;
  failure?: string;
  created_at: string;
  published_at?: string;
}

export interface ManagedConfigAuditEvent {
  id: number;
  version?: number;
  action: string;
  actor?: string;
  message?: string;
  created_at: string;
}

export interface PortalMetricsDelta {
  absolute: number;
  ratio: number | null;
  direction: 'up' | 'down' | 'flat';
}

export interface PortalMetricsTrendSummary {
  baseline_label: string;
  baseline_age_minutes: number | null;
  snapshot_count: number;
  total_requests: PortalMetricsDelta;
  average_latency_ms: PortalMetricsDelta;
  error_rate: PortalMetricsDelta;
  coverage_rate: PortalMetricsDelta;
}

export type PortalMetricsTrendMode = 'previous' | 'window'

export interface PortalMetricsTrendPoint {
  captured_at: string;
  total_requests: number;
  average_latency_ms: number;
  error_rate: number;
  coverage_rate: number;
  matched_requests: number;
  unmatched_requests: number;
  matched_paths: number;
  unmatched_paths: number;
}
