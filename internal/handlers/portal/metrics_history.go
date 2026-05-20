package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	frameworkmetrics "github.com/nimburion/nimburion/pkg/observability/metrics"
	dto "github.com/prometheus/client_model/go"
	"github.com/redis/go-redis/v9"
)

type PortalMetricsSummary struct {
	TotalRequests        float64 `json:"total_requests"`
	InFlightRequests     float64 `json:"in_flight_requests"`
	SuccessResponses     float64 `json:"success_responses"`
	ClientErrors         float64 `json:"client_errors"`
	ServerErrors         float64 `json:"server_errors"`
	RateLimitedResponses float64 `json:"rate_limited_responses"`
	Status401Responses   float64 `json:"status_401_responses"`
	Status403Responses   float64 `json:"status_403_responses"`
	Status429Responses   float64 `json:"status_429_responses"`
	Status502Responses   float64 `json:"status_502_responses"`
	Status503Responses   float64 `json:"status_503_responses"`
	Status504Responses   float64 `json:"status_504_responses"`
	AverageLatencyMs     float64 `json:"average_latency_ms"`
}

type PortalRuntimeMetrics struct {
	Goroutines          float64 `json:"goroutines"`
	HeapAllocBytes      float64 `json:"heap_alloc_bytes"`
	ResidentMemoryBytes float64 `json:"resident_memory_bytes"`
}

type PortalPathMetric struct {
	Path                 string          `json:"path"`
	Methods              []string        `json:"methods"`
	Requests             float64         `json:"requests"`
	SuccessResponses     float64         `json:"success_responses"`
	ClientErrors         float64         `json:"client_errors"`
	ServerErrors         float64         `json:"server_errors"`
	RateLimitedResponses float64         `json:"rate_limited_responses"`
	Status401Responses   float64         `json:"status_401_responses"`
	Status403Responses   float64         `json:"status_403_responses"`
	Status429Responses   float64         `json:"status_429_responses"`
	Status502Responses   float64         `json:"status_502_responses"`
	Status503Responses   float64         `json:"status_503_responses"`
	Status504Responses   float64         `json:"status_504_responses"`
	AverageLatencyMs     float64         `json:"average_latency_ms"`
	PrimaryMatch         json.RawMessage `json:"primary_match,omitempty"`
	MatchCount           int             `json:"match_count,omitempty"`
}

type PortalMetricsCatalogCoverage struct {
	MatchedPaths      float64 `json:"matched_paths"`
	UnmatchedPaths    float64 `json:"unmatched_paths"`
	MatchedRequests   float64 `json:"matched_requests"`
	UnmatchedRequests float64 `json:"unmatched_requests"`
}

type PortalMetricsData struct {
	Summary         PortalMetricsSummary         `json:"summary"`
	Runtime         PortalRuntimeMetrics         `json:"runtime"`
	Paths           []PortalPathMetric           `json:"paths"`
	CatalogCoverage PortalMetricsCatalogCoverage `json:"catalog_coverage"`
}

type PortalMetricsSnapshot struct {
	CapturedAt string            `json:"captured_at"`
	Data       PortalMetricsData `json:"data"`
}

type PortalMetricsHistoryResponse struct {
	Source             string                  `json:"source"`
	SnapshotCount      int                     `json:"snapshot_count"`
	SnapshotIntervalMs int64                   `json:"snapshot_interval_ms"`
	RetentionMs        int64                   `json:"retention_ms"`
	Snapshots          []PortalMetricsSnapshot `json:"snapshots"`
}

type MetricsHistoryStore interface {
	Append(context.Context, PortalMetricsSnapshot) error
	Read() PortalMetricsHistoryResponse
	Close() error
}

type LocalMetricsHistoryStore struct {
	mu               sync.RWMutex
	snapshots        []PortalMetricsSnapshot
	maxSnapshots     int
	maxAge           time.Duration
	snapshotInterval time.Duration
}

type pathAggregate struct {
	methods              map[string]struct{}
	requests             float64
	successResponses     float64
	clientErrors         float64
	serverErrors         float64
	rateLimitedResponses float64
	status401Responses   float64
	status403Responses   float64
	status429Responses   float64
	status502Responses   float64
	status503Responses   float64
	status504Responses   float64
	latencySumMs         float64
	latencyCount         float64
}

type RedisMetricsHistoryStore struct {
	client           *redis.Client
	key              string
	instanceID       string
	maxSnapshots     int
	maxAge           time.Duration
	snapshotInterval time.Duration
	operationTimeout time.Duration
}

type storedPortalMetricsSnapshot struct {
	InstanceID string            `json:"instance_id"`
	CapturedAt string            `json:"captured_at"`
	Data       PortalMetricsData `json:"data"`
}

func NewLocalMetricsHistoryStore(cfg gatewaycfg.PortalMetricsHistoryConfig) *LocalMetricsHistoryStore {
	return &LocalMetricsHistoryStore{
		snapshots:        make([]PortalMetricsSnapshot, 0, cfg.MaxSnapshots),
		maxSnapshots:     cfg.MaxSnapshots,
		maxAge:           cfg.MaxAge,
		snapshotInterval: cfg.SnapshotInterval,
	}
}

func (s *LocalMetricsHistoryStore) Append(_ context.Context, snapshot PortalMetricsSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots = s.pruneLocked(append(s.snapshots, snapshot))
	return nil
}

func (s *LocalMetricsHistoryStore) Read() PortalMetricsHistoryResponse {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshots = s.pruneLocked(s.snapshots)
	copied := append([]PortalMetricsSnapshot(nil), s.snapshots...)

	return PortalMetricsHistoryResponse{
		Source:             "local",
		SnapshotCount:      len(copied),
		SnapshotIntervalMs: s.snapshotInterval.Milliseconds(),
		RetentionMs:        s.maxAge.Milliseconds(),
		Snapshots:          copied,
	}
}

func (s *LocalMetricsHistoryStore) pruneLocked(history []PortalMetricsSnapshot) []PortalMetricsSnapshot {
	now := time.Now()
	pruned := make([]PortalMetricsSnapshot, 0, len(history))
	for _, snapshot := range history {
		capturedAt, err := time.Parse(time.RFC3339, snapshot.CapturedAt)
		if err != nil {
			continue
		}
		if now.Sub(capturedAt) > s.maxAge {
			continue
		}
		pruned = append(pruned, snapshot)
	}
	sort.Slice(pruned, func(i, j int) bool {
		return pruned[i].CapturedAt < pruned[j].CapturedAt
	})
	if len(pruned) > s.maxSnapshots {
		pruned = pruned[len(pruned)-s.maxSnapshots:]
	}
	return pruned
}

func (s *LocalMetricsHistoryStore) Close() error { return nil }

func NewRedisMetricsHistoryStore(cfg gatewaycfg.PortalMetricsHistoryConfig, serviceName, instanceID string) (*RedisMetricsHistoryStore, error) {
	opts, err := redis.ParseURL(strings.TrimSpace(cfg.Redis.URL))
	if err != nil {
		return nil, fmt.Errorf("parse metrics history redis url: %w", err)
	}
	opts.PoolSize = cfg.Redis.MaxConns
	opts.ReadTimeout = cfg.Redis.OperationTimeout
	opts.WriteTimeout = cfg.Redis.OperationTimeout
	opts.DialTimeout = 5 * time.Second

	prefix := strings.TrimSpace(cfg.Redis.Prefix)
	if prefix == "" {
		prefix = "nimburion:portal:metrics_history"
	}
	serviceKey := strings.TrimSpace(serviceName)
	if serviceKey == "" {
		serviceKey = "gateway"
	}
	if strings.TrimSpace(instanceID) == "" {
		instanceID = "unknown"
	}

	return &RedisMetricsHistoryStore{
		client:           redis.NewClient(opts),
		key:              fmt.Sprintf("%s:%s", prefix, serviceKey),
		instanceID:       instanceID,
		maxSnapshots:     cfg.MaxSnapshots,
		maxAge:           cfg.MaxAge,
		snapshotInterval: cfg.SnapshotInterval,
		operationTimeout: cfg.Redis.OperationTimeout,
	}, nil
}

func (s *RedisMetricsHistoryStore) Append(ctx context.Context, snapshot PortalMetricsSnapshot) error {
	if s == nil || s.client == nil {
		return nil
	}
	capturedAt, err := time.Parse(time.RFC3339, snapshot.CapturedAt)
	if err != nil {
		return err
	}

	member, err := json.Marshal(storedPortalMetricsSnapshot{
		InstanceID: s.instanceID,
		CapturedAt: snapshot.CapturedAt,
		Data:       snapshot.Data,
	})
	if err != nil {
		return err
	}

	opCtx, cancel := context.WithTimeout(ctx, s.operationTimeout)
	defer cancel()

	minScore := fmt.Sprintf("%d", time.Now().Add(-s.maxAge).UnixMilli())
	pipe := s.client.TxPipeline()
	pipe.ZAdd(opCtx, s.key, redis.Z{Score: float64(capturedAt.UnixMilli()), Member: member})
	pipe.ZRemRangeByScore(opCtx, s.key, "-inf", minScore)
	pipe.ZRemRangeByRank(opCtx, s.key, 0, int64(-s.maxSnapshots-1))
	_, err = pipe.Exec(opCtx)
	return err
}

func (s *RedisMetricsHistoryStore) Read() PortalMetricsHistoryResponse {
	if s == nil || s.client == nil {
		return PortalMetricsHistoryResponse{
			Source:    "redis",
			Snapshots: []PortalMetricsSnapshot{},
		}
	}
	response := PortalMetricsHistoryResponse{
		Source:             "redis",
		SnapshotIntervalMs: s.snapshotInterval.Milliseconds(),
		RetentionMs:        s.maxAge.Milliseconds(),
		Snapshots:          []PortalMetricsSnapshot{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.operationTimeout)
	defer cancel()

	values, err := s.client.ZRange(ctx, s.key, 0, -1).Result()
	if err != nil {
		return response
	}

	stored := make([]storedPortalMetricsSnapshot, 0, len(values))
	for _, raw := range values {
		var snapshot storedPortalMetricsSnapshot
		if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
			continue
		}
		stored = append(stored, snapshot)
	}

	snapshots := aggregateStoredSnapshots(stored, s.snapshotInterval)
	response.SnapshotCount = len(snapshots)
	response.Snapshots = snapshots
	return response
}

func (s *RedisMetricsHistoryStore) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

type MetricsHistoryCollector struct {
	registry *frameworkmetrics.Registry
	store    MetricsHistoryStore
	now      func() time.Time
}

func NewMetricsHistoryCollector(registry *frameworkmetrics.Registry, store MetricsHistoryStore) *MetricsHistoryCollector {
	if registry == nil || store == nil {
		return nil
	}
	return &MetricsHistoryCollector{registry: registry, store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (c *MetricsHistoryCollector) Start(ctx context.Context) error {
	if c == nil {
		return nil
	}

	c.captureSnapshot()

	response := c.store.Read()
	interval := time.Duration(response.SnapshotIntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.captureSnapshot()
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

func (c *MetricsHistoryCollector) captureSnapshot() {
	data, err := snapshotMetricsRegistry(c.registry)
	if err != nil {
		return
	}
	_ = c.store.Append(context.Background(), PortalMetricsSnapshot{
		CapturedAt: c.now().Format(time.RFC3339),
		Data:       data,
	})
}

func snapshotMetricsRegistry(registry *frameworkmetrics.Registry) (PortalMetricsData, error) {
	payload := PortalMetricsData{
		Paths: make([]PortalPathMetric, 0),
	}
	if registry == nil {
		return payload, nil
	}

	families, err := registry.Gatherer().Gather()
	if err != nil {
		return payload, err
	}

	paths := map[string]*pathAggregate{}
	var totalLatencySumMs float64
	var totalLatencyCount float64

	for _, family := range families {
		switch family.GetName() {
		case "http_requests_total":
			for _, metric := range family.GetMetric() {
				labels := metricLabels(metric)
				aggregate := ensurePathAggregate(paths, normalizeObservedPath(labels["path"]))
				if method := strings.ToUpper(strings.TrimSpace(labels["method"])); method != "" {
					aggregate.methods[method] = struct{}{}
				}
				value := metric.GetCounter().GetValue()
				status := strings.TrimSpace(labels["status"])
				aggregate.requests += value
				payload.Summary.TotalRequests += value
				switch {
				case strings.HasPrefix(status, "2"):
					aggregate.successResponses += value
					payload.Summary.SuccessResponses += value
				case strings.HasPrefix(status, "4"):
					aggregate.clientErrors += value
					payload.Summary.ClientErrors += value
					switch status {
					case "401":
						aggregate.status401Responses += value
						payload.Summary.Status401Responses += value
					case "403":
						aggregate.status403Responses += value
						payload.Summary.Status403Responses += value
					case "429":
						aggregate.status429Responses += value
						payload.Summary.Status429Responses += value
						aggregate.rateLimitedResponses += value
						payload.Summary.RateLimitedResponses += value
					}
				case strings.HasPrefix(status, "5"):
					aggregate.serverErrors += value
					payload.Summary.ServerErrors += value
					switch status {
					case "502":
						aggregate.status502Responses += value
						payload.Summary.Status502Responses += value
					case "503":
						aggregate.status503Responses += value
						payload.Summary.Status503Responses += value
					case "504":
						aggregate.status504Responses += value
						payload.Summary.Status504Responses += value
					}
				}
			}
		case "http_request_duration_seconds":
			for _, metric := range family.GetMetric() {
				labels := metricLabels(metric)
				aggregate := ensurePathAggregate(paths, normalizeObservedPath(labels["path"]))
				if method := strings.ToUpper(strings.TrimSpace(labels["method"])); method != "" {
					aggregate.methods[method] = struct{}{}
				}
				histogram := metric.GetHistogram()
				sumMs := histogram.GetSampleSum() * 1000
				count := float64(histogram.GetSampleCount())
				aggregate.latencySumMs += sumMs
				aggregate.latencyCount += count
				totalLatencySumMs += sumMs
				totalLatencyCount += count
			}
		case "http_requests_in_flight":
			for _, metric := range family.GetMetric() {
				payload.Summary.InFlightRequests += metric.GetGauge().GetValue()
			}
		case "go_goroutines":
			for _, metric := range family.GetMetric() {
				payload.Runtime.Goroutines += metric.GetGauge().GetValue()
			}
		case "go_memstats_heap_alloc_bytes":
			for _, metric := range family.GetMetric() {
				payload.Runtime.HeapAllocBytes += metric.GetGauge().GetValue()
			}
		case "process_resident_memory_bytes":
			for _, metric := range family.GetMetric() {
				payload.Runtime.ResidentMemoryBytes += metric.GetGauge().GetValue()
			}
		}
	}

	if totalLatencyCount > 0 {
		payload.Summary.AverageLatencyMs = totalLatencySumMs / totalLatencyCount
	}
	payload.Paths = summarizePortalPathMetrics(paths)
	return payload, nil
}

func metricLabels(metric *dto.Metric) map[string]string {
	labels := make(map[string]string, len(metric.GetLabel()))
	for _, label := range metric.GetLabel() {
		labels[label.GetName()] = label.GetValue()
	}
	return labels
}

func ensurePathAggregate(paths map[string]*pathAggregate, path string) *pathAggregate {
	if existing, ok := paths[path]; ok {
		return existing
	}
	created := &pathAggregate{methods: map[string]struct{}{}}
	paths[path] = created
	return created
}

func summarizePortalPathMetrics(paths map[string]*pathAggregate) []PortalPathMetric {
	summaries := make([]PortalPathMetric, 0, len(paths))
	for path, aggregate := range paths {
		methods := make([]string, 0, len(aggregate.methods))
		for method := range aggregate.methods {
			methods = append(methods, method)
		}
		sort.Strings(methods)
		summaries = append(summaries, PortalPathMetric{
			Path:                 path,
			Methods:              methods,
			Requests:             aggregate.requests,
			SuccessResponses:     aggregate.successResponses,
			ClientErrors:         aggregate.clientErrors,
			ServerErrors:         aggregate.serverErrors,
			RateLimitedResponses: aggregate.rateLimitedResponses,
			Status401Responses:   aggregate.status401Responses,
			Status403Responses:   aggregate.status403Responses,
			Status429Responses:   aggregate.status429Responses,
			Status502Responses:   aggregate.status502Responses,
			Status503Responses:   aggregate.status503Responses,
			Status504Responses:   aggregate.status504Responses,
			AverageLatencyMs: func() float64 {
				if aggregate.latencyCount <= 0 {
					return 0
				}
				return aggregate.latencySumMs / aggregate.latencyCount
			}(),
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Requests == summaries[j].Requests {
			return summaries[i].Path < summaries[j].Path
		}
		return summaries[i].Requests > summaries[j].Requests
	})
	return summaries
}

func normalizeObservedPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "/"
	}
	withoutQuery := strings.Split(trimmed, "?")[0]
	if withoutQuery == "" {
		return "/"
	}
	normalized := strings.ReplaceAll(withoutQuery, "//", "/")
	for strings.Contains(normalized, "//") {
		normalized = strings.ReplaceAll(normalized, "//", "/")
	}
	if len(normalized) > 1 && strings.HasSuffix(normalized, "/") {
		normalized = normalized[:len(normalized)-1]
	}
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	return normalized
}

func aggregateStoredSnapshots(stored []storedPortalMetricsSnapshot, step time.Duration) []PortalMetricsSnapshot {
	if step <= 0 {
		step = time.Minute
	}
	type bucketKey struct {
		bucketStart string
		instanceID  string
	}
	latestByBucketAndInstance := make(map[bucketKey]storedPortalMetricsSnapshot)
	for _, snapshot := range stored {
		capturedAt, err := time.Parse(time.RFC3339, snapshot.CapturedAt)
		if err != nil {
			continue
		}
		bucketStart := capturedAt.Truncate(step).UTC().Format(time.RFC3339)
		key := bucketKey{bucketStart: bucketStart, instanceID: strings.TrimSpace(snapshot.InstanceID)}
		current, ok := latestByBucketAndInstance[key]
		if !ok || snapshot.CapturedAt > current.CapturedAt {
			latestByBucketAndInstance[key] = snapshot
		}
	}

	mergedByBucket := make(map[string]PortalMetricsSnapshot)
	for key, snapshot := range latestByBucketAndInstance {
		current := mergedByBucket[key.bucketStart]
		current.CapturedAt = key.bucketStart
		current.Data = mergePortalMetricsData(current.Data, snapshot.Data)
		mergedByBucket[key.bucketStart] = current
	}

	merged := make([]PortalMetricsSnapshot, 0, len(mergedByBucket))
	for _, snapshot := range mergedByBucket {
		merged = append(merged, snapshot)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].CapturedAt < merged[j].CapturedAt
	})
	return merged
}

func mergePortalMetricsData(left, right PortalMetricsData) PortalMetricsData {
	merged := PortalMetricsData{
		Summary: PortalMetricsSummary{
			TotalRequests:        left.Summary.TotalRequests + right.Summary.TotalRequests,
			InFlightRequests:     left.Summary.InFlightRequests + right.Summary.InFlightRequests,
			SuccessResponses:     left.Summary.SuccessResponses + right.Summary.SuccessResponses,
			ClientErrors:         left.Summary.ClientErrors + right.Summary.ClientErrors,
			ServerErrors:         left.Summary.ServerErrors + right.Summary.ServerErrors,
			RateLimitedResponses: left.Summary.RateLimitedResponses + right.Summary.RateLimitedResponses,
			Status401Responses:   left.Summary.Status401Responses + right.Summary.Status401Responses,
			Status403Responses:   left.Summary.Status403Responses + right.Summary.Status403Responses,
			Status429Responses:   left.Summary.Status429Responses + right.Summary.Status429Responses,
			Status502Responses:   left.Summary.Status502Responses + right.Summary.Status502Responses,
			Status503Responses:   left.Summary.Status503Responses + right.Summary.Status503Responses,
			Status504Responses:   left.Summary.Status504Responses + right.Summary.Status504Responses,
		},
		Runtime: PortalRuntimeMetrics{
			Goroutines:          left.Runtime.Goroutines + right.Runtime.Goroutines,
			HeapAllocBytes:      left.Runtime.HeapAllocBytes + right.Runtime.HeapAllocBytes,
			ResidentMemoryBytes: left.Runtime.ResidentMemoryBytes + right.Runtime.ResidentMemoryBytes,
		},
		CatalogCoverage: PortalMetricsCatalogCoverage{
			MatchedPaths:      left.CatalogCoverage.MatchedPaths + right.CatalogCoverage.MatchedPaths,
			UnmatchedPaths:    left.CatalogCoverage.UnmatchedPaths + right.CatalogCoverage.UnmatchedPaths,
			MatchedRequests:   left.CatalogCoverage.MatchedRequests + right.CatalogCoverage.MatchedRequests,
			UnmatchedRequests: left.CatalogCoverage.UnmatchedRequests + right.CatalogCoverage.UnmatchedRequests,
		},
	}

	leftWeight := left.Summary.TotalRequests
	rightWeight := right.Summary.TotalRequests
	if leftWeight+rightWeight > 0 {
		merged.Summary.AverageLatencyMs = ((left.Summary.AverageLatencyMs * leftWeight) + (right.Summary.AverageLatencyMs * rightWeight)) / (leftWeight + rightWeight)
	}

	pathIndex := map[string]*PortalPathMetric{}
	appendPath := func(pathMetric PortalPathMetric) {
		key := pathMetric.Path
		if existing, ok := pathIndex[key]; ok {
			existing.Requests += pathMetric.Requests
			existing.SuccessResponses += pathMetric.SuccessResponses
			existing.ClientErrors += pathMetric.ClientErrors
			existing.ServerErrors += pathMetric.ServerErrors
			existing.RateLimitedResponses += pathMetric.RateLimitedResponses
			existing.Status401Responses += pathMetric.Status401Responses
			existing.Status403Responses += pathMetric.Status403Responses
			existing.Status429Responses += pathMetric.Status429Responses
			existing.Status502Responses += pathMetric.Status502Responses
			existing.Status503Responses += pathMetric.Status503Responses
			existing.Status504Responses += pathMetric.Status504Responses
			existingWeight := existing.Requests - pathMetric.Requests
			if existingWeight+pathMetric.Requests > 0 {
				existing.AverageLatencyMs = ((existing.AverageLatencyMs * existingWeight) + (pathMetric.AverageLatencyMs * pathMetric.Requests)) / (existingWeight + pathMetric.Requests)
			}
			methods := map[string]struct{}{}
			for _, method := range existing.Methods {
				methods[method] = struct{}{}
			}
			for _, method := range pathMetric.Methods {
				methods[method] = struct{}{}
			}
			existing.Methods = existing.Methods[:0]
			for method := range methods {
				existing.Methods = append(existing.Methods, method)
			}
			sort.Strings(existing.Methods)
			return
		}
		copied := pathMetric
		copied.Methods = append([]string(nil), pathMetric.Methods...)
		pathIndex[key] = &copied
	}
	for _, pathMetric := range left.Paths {
		appendPath(pathMetric)
	}
	for _, pathMetric := range right.Paths {
		appendPath(pathMetric)
	}
	merged.Paths = make([]PortalPathMetric, 0, len(pathIndex))
	for _, pathMetric := range pathIndex {
		merged.Paths = append(merged.Paths, *pathMetric)
	}
	sort.Slice(merged.Paths, func(i, j int) bool {
		if merged.Paths[i].Requests == merged.Paths[j].Requests {
			return merged.Paths[i].Path < merged.Paths[j].Path
		}
		return merged.Paths[i].Requests > merged.Paths[j].Requests
	})
	return merged
}
