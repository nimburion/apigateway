package server

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/auth"
	authcfg "github.com/nimburion/nimburion/pkg/auth/config"
	baseconfig "github.com/nimburion/nimburion/pkg/config"
	httpserver "github.com/nimburion/nimburion/pkg/http/server"
	logpkg "github.com/nimburion/nimburion/pkg/observability/logger"
)

func TestServiceName(t *testing.T) {
	cfg := &baseconfig.Config{}
	if got := serviceName(cfg); got != "api-gateway" {
		t.Fatalf("expected fallback name, got %s", got)
	}
	cfg.App.Name = "Gateway"
	if got := serviceName(cfg); got != "Gateway" {
		t.Fatalf("expected service name, got %s", got)
	}
}

func TestManagementSecurityMiddleware(t *testing.T) {
	if got := managementSecurityMiddleware(false, nil, "scope"); got != nil {
		t.Fatalf("expected nil middleware when auth disabled")
	}
	if got := managementSecurityMiddleware(true, nil, "scope"); len(got) != 2 {
		t.Fatalf("expected 2 middleware when auth enabled, got %d", len(got))
	}
}

func TestRunServerNilConfig(t *testing.T) {
	err := RunServer(nil, nil, nil)
	if !errors.Is(err, ErrNilConfig) {
		t.Fatalf("expected ErrNilConfig, got %v", err)
	}
}

func TestRunServerAuthDisabledAllowsPublicRoutes(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })
	called := false
	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		called = true
		return nil
	}

	cfg := &baseconfig.Config{}
	gwCfg := gatewaycfg.NewDefaultConfig()
	gwCfg.Routes = gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix: "/",
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/users",
						TargetURL:  "http://example.com",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
				},
			},
		},
	}
	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("expected public gateway startup to succeed, got %v", err)
	}
	if !called {
		t.Fatalf("expected runHTTPServersWithSignals to be called")
	}
}

func TestRunServerServesPortalAssetsOnManagementRouter(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })

	var capturedServers *httpserver.HTTPServers
	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		capturedServers = servers
		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Management.Enabled = true
	gwCfg := gatewaycfg.NewDefaultConfig()
	gwCfg.Routes = gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix: "/",
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/users",
						TargetURL:  "http://example.com",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
	if capturedServers == nil || capturedServers.Management == nil {
		t.Fatalf("expected management server to be built")
	}

	assetEntries, err := os.ReadDir(filepath.Join("..", "..", "internal", "handlers", "portal", "dist", "assets"))
	if err != nil {
		t.Fatalf("read portal assets: %v", err)
	}
	if len(assetEntries) == 0 {
		t.Fatalf("expected at least one portal asset")
	}
	req := httptest.NewRequest(http.MethodGet, "/portal/assets/"+assetEntries[0].Name(), nil)
	w := httptest.NewRecorder()
	capturedServers.Management.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for portal asset, got %d", w.Code)
	}
	expectedContentType := mime.TypeByExtension(filepath.Ext(assetEntries[0].Name()))
	if got := w.Header().Get("Content-Type"); !matchesAssetContentType(expectedContentType, got) {
		t.Fatalf("unexpected content-type: %q", got)
	}
}

func TestRunServerServesPortalGroupPageOnManagementRouter(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })

	var capturedServers *httpserver.HTTPServers
	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		capturedServers = servers
		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Management.Enabled = true
	gwCfg := gatewaycfg.NewDefaultConfig()
	gwCfg.Routes = gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix: "/",
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/healthz",
						TargetURL:  "http://example.com",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
	if capturedServers == nil || capturedServers.Management == nil {
		t.Fatalf("expected management server to be built")
	}

	req := httptest.NewRequest(http.MethodGet, "/portal/groups/default", nil)
	w := httptest.NewRecorder()
	capturedServers.Management.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for portal group page, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected html content-type, got %q", got)
	}
}

func TestRunServerServesPortalSurfacePageOnManagementRouter(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })

	var capturedServers *httpserver.HTTPServers
	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		capturedServers = servers
		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Management.Enabled = true
	gwCfg := gatewaycfg.NewDefaultConfig()
	gwCfg.Routes = gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix: "/",
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/healthz",
						TargetURL:  "http://example.com",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
	if capturedServers == nil || capturedServers.Management == nil {
		t.Fatalf("expected management server to be built")
	}

	req := httptest.NewRequest(http.MethodGet, "/portal/posture/default/route/GET/L2hlYWx0aHo", nil)
	w := httptest.NewRecorder()
	capturedServers.Management.Router().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for portal surface page, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("expected html content-type, got %q", got)
	}
}

func TestRunServerPublicRoutesContributeToManagementMetricsRegistry(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)

	var capturedServers *httpserver.HTTPServers
	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		capturedServers = servers
		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Management.Enabled = true
	gwCfg := gatewaycfg.NewDefaultConfig()
	gwCfg.Routes = gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix: "/",
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/healthz",
						TargetURL:  upstream.URL,
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
	if capturedServers == nil || capturedServers.Public == nil || capturedServers.Management == nil {
		t.Fatalf("expected both public and management servers to be built")
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	(*capturedServers.Public.Router()).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for public route, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w = httptest.NewRecorder()
	capturedServers.Management.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for metrics endpoint, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `http_requests_total{method="GET",path="/healthz",status="204"} 1`) {
		t.Fatalf("expected public route counter in metrics output, got: %s", body)
	}
	if !strings.Contains(body, `http_request_duration_seconds_count{method="GET",path="/healthz",status="204"} 1`) {
		t.Fatalf("expected public route duration count in metrics output, got: %s", body)
	}
}

func TestRunServerRecords404RequestsInMetricsRegistry(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })

	var capturedServers *httpserver.HTTPServers
	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		capturedServers = servers
		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Management.Enabled = true
	gwCfg := gatewaycfg.NewDefaultConfig()
	gwCfg.Routes = gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix: "/",
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/healthz",
						TargetURL:  "http://example.com",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
	if capturedServers == nil || capturedServers.Public == nil || capturedServers.Management == nil {
		t.Fatalf("expected both public and management servers to be built")
	}

	req := httptest.NewRequest(http.MethodGet, "/missing-route", nil)
	w := httptest.NewRecorder()
	(*capturedServers.Public.Router()).ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing public route, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w = httptest.NewRecorder()
	capturedServers.Management.Router().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for metrics endpoint, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `http_requests_total{method="GET",path="/missing-route",status="404"} 1`) {
		t.Fatalf("expected 404 request counter in metrics output, got: %s", body)
	}
	if !strings.Contains(body, `http_request_duration_seconds_count{method="GET",path="/missing-route",status="404"} 1`) {
		t.Fatalf("expected 404 duration count in metrics output, got: %s", body)
	}
}

func matchesAssetContentType(expected, got string) bool {
	if expected == "" {
		return got != ""
	}
	if strings.Contains(got, expected) {
		return true
	}
	if expected == "text/javascript; charset=utf-8" && strings.Contains(got, "application/javascript") {
		return true
	}
	return false
}

func TestRunServerAuthDisabledRejectsProtectedGroup(t *testing.T) {
	cfg := &baseconfig.Config{}
	gwCfg := gatewaycfg.NewDefaultConfig()
	gwCfg.Routes = gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix:      "/",
				Middlewares: []string{"Authenticate"},
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/users",
						TargetURL:  "http://example.com",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
				},
			},
		},
	}

	err := RunServer(cfg, gwCfg, noopLogger{})
	if err == nil || !strings.Contains(err.Error(), `requires auth.enabled=true`) {
		t.Fatalf("expected explicit auth requirement error, got %v", err)
	}
}

func TestRunServerAuthDisabledRejectsClaimsGuardMiddleware(t *testing.T) {
	cfg := &baseconfig.Config{}
	gwCfg := gatewaycfg.NewDefaultConfig()
	gwCfg.Routes = gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix: "/",
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/users",
						TargetURL:  "http://example.com",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {Middlewares: []string{"ClaimsGuardFromConfig"}}}},
						},
					},
				},
			},
		},
	}

	err := RunServer(cfg, gwCfg, noopLogger{})
	if err == nil || !strings.Contains(err.Error(), `ClaimsGuardFromConfig`) {
		t.Fatalf("expected claims guard auth requirement error, got %v", err)
	}
}

func TestRunServerAuthDisabledAllowsForwardIdentityHeadersOnlyGroup(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })

	called := false
	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		called = true
		return nil
	}

	cfg := &baseconfig.Config{}
	gwCfg := gatewaycfg.NewDefaultConfig()
	gwCfg.Routes = gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix:      "/",
				Middlewares: []string{"ForwardIdentityHeaders"},
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/users",
						TargetURL:  "http://example.com",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("expected startup to succeed with ForwardIdentityHeaders only, got %v", err)
	}
	if !called {
		t.Fatalf("expected runHTTPServersWithSignals to be called")
	}
}

func TestRunServerAuthDisabledRejectsScopedRoute(t *testing.T) {
	cfg := &baseconfig.Config{}
	gwCfg := gatewaycfg.NewDefaultConfig()
	gwCfg.Routes = gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix: "/",
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/users",
						TargetURL:  "http://example.com",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {Scopes: []string{"users:read"}}}},
						},
					},
				},
			},
		},
	}

	err := RunServer(cfg, gwCfg, noopLogger{})
	if err == nil || !strings.Contains(err.Error(), `defines scopes`) {
		t.Fatalf("expected scoped route auth requirement error, got %v", err)
	}
}

func TestRunServerAuthDisabledRejectsManagementAuth(t *testing.T) {
	cfg := &baseconfig.Config{}
	cfg.Management.Enabled = true
	cfg.Management.AuthEnabled = true
	gwCfg := gatewaycfg.NewDefaultConfig()
	gwCfg.Routes = gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix: "/",
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/users",
						TargetURL:  "http://example.com",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
				},
			},
		},
	}

	err := RunServer(cfg, gwCfg, noopLogger{})
	if err == nil || !strings.Contains(err.Error(), `management.auth_enabled=true`) {
		t.Fatalf("expected management auth requirement error, got %v", err)
	}
}

func TestRunServerAuthDisabledAllowsRouteThatDisablesGroupAuthenticate(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })
	called := false
	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		called = true
		return nil
	}

	cfg := &baseconfig.Config{}
	gwCfg := &gatewaycfg.Gateway{
		Portal: gatewaycfg.PortalConfig{
			Enabled: true,
		},
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix:      "/",
					Middlewares: []string{"Authenticate"},
					Routes: []gatewaycfg.Route{
						{
							PathPrefix:         "/public",
							TargetURL:          "http://example.com",
							DisableMiddlewares: []string{"Authenticate"},
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("expected startup to succeed for route that disables group auth, got %v", err)
	}
	if !called {
		t.Fatalf("expected runHTTPServersWithSignals to be called")
	}
}

func TestRunServerLoadRoutesError(t *testing.T) {
	cfg := &baseconfig.Config{}
	cfg.Auth.Enabled = true
	gwCfg := gatewaycfg.NewDefaultConfig()

	err := RunServer(cfg, gwCfg, nil)
	if err == nil {
		t.Fatalf("expected load routes error")
	}
}

func TestRunServerFailsBeforeBuildingServersOnUnsupportedMethod(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })

	called := false
	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		called = true
		return nil
	}

	cfg := &baseconfig.Config{}
	gwCfg := &gatewaycfg.Gateway{
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix: "/",
					Routes: []gatewaycfg.Route{
						{
							PathPrefix: "/users",
							TargetURL:  "http://example.com",
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"TRACE": {}}},
							},
						},
					},
				},
			},
		},
	}

	err := RunServer(cfg, gwCfg, noopLogger{})
	if err == nil || !strings.Contains(err.Error(), `methods.TRACE is not supported`) {
		t.Fatalf("expected unsupported method error, got %v", err)
	}
	if called {
		t.Fatalf("expected startup to fail before running HTTP servers")
	}
}

type noopLogger struct{}

func (l noopLogger) Debug(string, ...any)                      {}
func (l noopLogger) Info(string, ...any)                       {}
func (l noopLogger) Warn(string, ...any)                       {}
func (l noopLogger) Error(string, ...any)                      {}
func (l noopLogger) With(...any) logpkg.Logger                 { return l }
func (l noopLogger) WithContext(context.Context) logpkg.Logger { return l }

type logEntry struct {
	level  string
	msg    string
	fields []any
}

type recordingLogger struct {
	mu      *sync.Mutex
	entries *[]logEntry
}

func newRecordingLogger() *recordingLogger {
	entries := []logEntry{}
	return &recordingLogger{
		mu:      &sync.Mutex{},
		entries: &entries,
	}
}

func (l *recordingLogger) Debug(msg string, fields ...any) { l.append("debug", msg, fields...) }
func (l *recordingLogger) Info(msg string, fields ...any)  { l.append("info", msg, fields...) }
func (l *recordingLogger) Warn(msg string, fields ...any)  { l.append("warn", msg, fields...) }
func (l *recordingLogger) Error(msg string, fields ...any) { l.append("error", msg, fields...) }
func (l *recordingLogger) With(fields ...any) logpkg.Logger {
	return l
}
func (l *recordingLogger) WithContext(context.Context) logpkg.Logger { return l }

func (l *recordingLogger) append(level, msg string, fields ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	copied := append([]any(nil), fields...)
	*l.entries = append(*l.entries, logEntry{level: level, msg: msg, fields: copied})
}

func (l *recordingLogger) snapshot() []logEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]logEntry(nil), (*l.entries)...)
}

func (l *recordingLogger) containsMessage(msg string) bool {
	for _, entry := range l.snapshot() {
		if entry.msg == msg {
			return true
		}
	}
	return false
}

func (l *recordingLogger) findMessage(msg string) *logEntry {
	for _, entry := range l.snapshot() {
		if entry.msg == msg {
			entryCopy := entry
			return &entryCopy
		}
	}
	return nil
}

type testJWTValidator struct {
	validate func(ctx context.Context, token string) (*auth.Claims, error)
}

func (v testJWTValidator) Validate(ctx context.Context, token string) (*auth.Claims, error) {
	return v.validate(ctx, token)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRunServerSuccessWithStubbedRun(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })
	called := false
	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		called = true
		if servers == nil || servers.Public == nil {
			t.Fatalf("expected public server to be built")
		}
		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Auth.Enabled = true
	gwCfg := &gatewaycfg.Gateway{
		Portal: gatewaycfg.PortalConfig{
			Enabled: true,
		},
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix: "/",
					Routes: []gatewaycfg.Route{
						{
							PathPrefix: "/users",
							TargetURL:  "http://example.com",
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
					},
				},
			},
		},
	}
	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
	if !called {
		t.Fatalf("expected runHTTPServersWithSignals to be called")
	}
}

func TestRunServerLogsRegisteredRoutesAndWebSockets(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })
	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		return nil
	}

	log := newRecordingLogger()
	cfg := &baseconfig.Config{}
	gwCfg := &gatewaycfg.Gateway{
		Portal: gatewaycfg.PortalConfig{
			Enabled: true,
		},
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix: "/",
					Routes: []gatewaycfg.Route{
						{
							PathPrefix: "/users",
							TargetURL:  "http://example.com",
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
					},
					WebSockets: []gatewaycfg.WebSocket{
						{Path: "/events", TargetURL: "ws://example.com"},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, log); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
	if !log.containsMessage("registered route") {
		t.Fatalf("expected route registration log entry")
	}
	if !log.containsMessage("registered websocket") {
		t.Fatalf("expected websocket registration log entry")
	}
}

func TestRunServerSecuresPortalManagementAPIsWhenManagementAuthEnabled(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	originalValidatorFactory := newJWTValidator
	t.Cleanup(func() {
		runHTTPServersWithSignals = originalRun
		newJWTValidator = originalValidatorFactory
	})

	newJWTValidator = func(cfg *baseconfig.Config, log logpkg.Logger) auth.JWTValidator {
		return testJWTValidator{
			validate: func(ctx context.Context, token string) (*auth.Claims, error) {
				if token == "portal" {
					return &auth.Claims{Subject: "u1", Scopes: []string{"management:portal"}}, nil
				}
				return nil, errors.New("invalid token")
			},
		}
	}

	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		if servers == nil || servers.Management == nil {
			t.Fatalf("expected management server to be built")
		}

		req := httptest.NewRequest(http.MethodGet, "/api/portal/routes", nil)
		rec := httptest.NewRecorder()
		servers.Management.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected portal routes API to require auth, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/api/portal/groups", nil)
		req.Header.Set("Authorization", "Bearer portal")
		rec = httptest.NewRecorder()
		servers.Management.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected portal groups API to allow management:portal token, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/api/portal/openapi.yaml", nil)
		req.Header.Set("Authorization", "Bearer portal")
		rec = httptest.NewRecorder()
		servers.Management.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected runtime openapi endpoint to allow management:portal token, got %d", rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store, private" {
			t.Fatalf("expected runtime openapi endpoint to disable caching, got %q", got)
		}
		if !strings.Contains(rec.Body.String(), "openapi: 3.0.3") {
			t.Fatalf("expected runtime openapi endpoint to return yaml spec")
		}

		req = httptest.NewRequest(http.MethodGet, "/health", nil)
		rec = httptest.NewRecorder()
		servers.Management.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected /health to remain public, got %d", rec.Code)
		}

		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Auth.Enabled = true
	cfg.Auth.Issuer = "https://issuer.example.com"
	cfg.Auth.JWKSUrl = "https://issuer.example.com/.well-known/jwks.json"
	cfg.Auth.Audience = "apigateway"
	cfg.Management.Enabled = true
	cfg.Management.AuthEnabled = true
	gwCfg := &gatewaycfg.Gateway{
		Portal: gatewaycfg.PortalConfig{
			Enabled: true,
		},
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix: "/",
					Routes: []gatewaycfg.Route{
						{
							PathPrefix: "/users",
							TargetURL:  "http://example.com",
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
}

func TestRunServerManagementEndpointsRemainPublicWhenManagementAuthDisabled(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })

	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		if servers == nil || servers.Management == nil {
			t.Fatalf("expected management server to be built")
		}

		paths := []string{
			"/health",
			"/ready",
			"/metrics",
			"/portal",
			"/api/portal/routes",
			"/api/portal/groups",
			"/api/portal/openapi.yaml",
		}
		for _, path := range paths {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			servers.Management.Router().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected %s to remain public when management auth is disabled, got %d", path, rec.Code)
			}
		}

		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Management.Enabled = true
	cfg.Management.AuthEnabled = false
	gwCfg := &gatewaycfg.Gateway{
		Portal: gatewaycfg.PortalConfig{
			Enabled: true,
		},
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix: "/",
					Routes: []gatewaycfg.Route{
						{
							PathPrefix: "/users",
							TargetURL:  "http://example.com",
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
}

func TestRunServerDoesNotMountPortalWhenPortalDisabled(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })

	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		if servers == nil || servers.Management == nil {
			t.Fatalf("expected management server to be built")
		}

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		servers.Management.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected /health to remain available, got %d", rec.Code)
		}

		for _, path := range []string{"/portal", "/api/portal/routes", "/api/portal/groups", "/api/portal/openapi.yaml"} {
			req = httptest.NewRequest(http.MethodGet, path, nil)
			rec = httptest.NewRecorder()
			servers.Management.Router().ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected %s to be unmounted when portal.enabled=false, got %d", path, rec.Code)
			}
		}

		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Management.Enabled = true
	cfg.Management.AuthEnabled = false
	gwCfg := &gatewaycfg.Gateway{
		Portal: gatewaycfg.PortalConfig{
			Enabled: false,
		},
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix: "/",
					Routes: []gatewaycfg.Route{
						{
							PathPrefix: "/users",
							TargetURL:  "http://example.com",
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
}

func TestRunServerManagementEndpointsEnforceExpectedScopesWhenManagementAuthEnabled(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	originalValidatorFactory := newJWTValidator
	t.Cleanup(func() {
		runHTTPServersWithSignals = originalRun
		newJWTValidator = originalValidatorFactory
	})

	newJWTValidator = func(cfg *baseconfig.Config, log logpkg.Logger) auth.JWTValidator {
		return testJWTValidator{
			validate: func(ctx context.Context, token string) (*auth.Claims, error) {
				switch token {
				case "read":
					return &auth.Claims{Subject: "u1", Scopes: []string{"management:read"}}, nil
				case "metrics":
					return &auth.Claims{Subject: "u1", Scopes: []string{"management:metrics"}}, nil
				case "portal":
					return &auth.Claims{Subject: "u1", Scopes: []string{"management:portal"}}, nil
				default:
					return nil, errors.New("invalid token")
				}
			},
		}
	}

	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		if servers == nil || servers.Management == nil {
			t.Fatalf("expected management server to be built")
		}

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		servers.Management.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected /health to remain public, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec = httptest.NewRecorder()
		servers.Management.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected /ready to require auth, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/ready", nil)
		req.Header.Set("Authorization", "Bearer read")
		rec = httptest.NewRecorder()
		servers.Management.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected /ready to allow management:read, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.Header.Set("Authorization", "Bearer read")
		rec = httptest.NewRecorder()
		servers.Management.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected /metrics to reject management:read without metrics scope, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.Header.Set("Authorization", "Bearer metrics")
		rec = httptest.NewRecorder()
		servers.Management.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected /metrics to allow management:metrics, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/portal", nil)
		rec = httptest.NewRecorder()
		servers.Management.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected /portal to require auth, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/portal", nil)
		req.Header.Set("Authorization", "Bearer portal")
		rec = httptest.NewRecorder()
		servers.Management.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected /portal to allow management:portal, got %d", rec.Code)
		}

		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Auth.Enabled = true
	cfg.Auth.Issuer = "https://issuer.example.com"
	cfg.Auth.JWKSUrl = "https://issuer.example.com/.well-known/jwks.json"
	cfg.Auth.Audience = "apigateway"
	cfg.Management.Enabled = true
	cfg.Management.AuthEnabled = true
	gwCfg := &gatewaycfg.Gateway{
		Portal: gatewaycfg.PortalConfig{
			Enabled: true,
		},
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix: "/",
					Routes: []gatewaycfg.Route{
						{
							PathPrefix: "/users",
							TargetURL:  "http://example.com",
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
}

func TestRunServerPublicRouteWorksWithoutAuthorization(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/public" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString("public-ok")),
			Request:    r,
		}, nil
	})

	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		if servers == nil || servers.Public == nil {
			t.Fatalf("expected public server to be built")
		}

		publicRouter := *servers.Public.Router()

		req := httptest.NewRequest(http.MethodGet, "/public", nil)
		rec := httptest.NewRecorder()
		publicRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected public route to return 200, got %d", rec.Code)
		}
		if body := rec.Body.String(); body != "public-ok" {
			t.Fatalf("unexpected public route body: %q", body)
		}

		return nil
	}

	cfg := &baseconfig.Config{}
	gwCfg := &gatewaycfg.Gateway{
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix: "/",
					Routes: []gatewaycfg.Route{
						{
							PathPrefix: "/public",
							TargetURL:  "http://example.com",
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
}

func TestRunServerGroupAuthenticateAllowsExplicitPublicRouteWithoutAuthorization(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	originalValidatorFactory := newJWTValidator
	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		runHTTPServersWithSignals = originalRun
		newJWTValidator = originalValidatorFactory
		http.DefaultTransport = originalTransport
	})
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/public":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString("public-ok")),
				Request:    r,
			}, nil
		case "/private":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewBufferString("private-ok")),
				Request:    r,
			}, nil
		default:
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
			return nil, errors.New("unexpected path")
		}
	})

	newJWTValidator = func(cfg *baseconfig.Config, log logpkg.Logger) auth.JWTValidator {
		return testJWTValidator{
			validate: func(ctx context.Context, token string) (*auth.Claims, error) {
				if token == "valid" {
					return &auth.Claims{Subject: "u1"}, nil
				}
				return nil, errors.New("invalid token")
			},
		}
	}

	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		if servers == nil || servers.Public == nil {
			t.Fatalf("expected public server to be built")
		}

		publicRouter := *servers.Public.Router()

		req := httptest.NewRequest(http.MethodGet, "/public", nil)
		rec := httptest.NewRecorder()
		publicRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected public route to return 200 without auth, got %d", rec.Code)
		}
		if body := rec.Body.String(); body != "public-ok" {
			t.Fatalf("unexpected public route body: %q", body)
		}

		req = httptest.NewRequest(http.MethodGet, "/private", nil)
		rec = httptest.NewRecorder()
		publicRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected protected sibling route to return 401 without auth, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/private", nil)
		req.Header.Set("Authorization", "Bearer valid")
		rec = httptest.NewRecorder()
		publicRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected protected sibling route to return 200 with auth, got %d", rec.Code)
		}

		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Auth.Enabled = true
	cfg.Auth.Issuer = "https://issuer.example.com"
	cfg.Auth.JWKSUrl = "https://issuer.example.com/.well-known/jwks.json"
	cfg.Auth.Audience = "apigateway"
	gwCfg := &gatewaycfg.Gateway{
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix:      "/",
					Middlewares: []string{"Authenticate"},
					Routes: []gatewaycfg.Route{
						{
							PathPrefix:         "/public",
							TargetURL:          "http://example.com",
							DisableMiddlewares: []string{"Authenticate"},
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
						{
							PathPrefix: "/private",
							TargetURL:  "http://example.com",
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
}

func TestRunServerProtectedRouteEnforcesAuthAndScopes(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	originalValidatorFactory := newJWTValidator
	originalTransport := http.DefaultTransport
	log := newRecordingLogger()
	t.Cleanup(func() {
		runHTTPServersWithSignals = originalRun
		newJWTValidator = originalValidatorFactory
		http.DefaultTransport = originalTransport
	})

	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/scoped" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString("scoped-ok")),
			Request:    r,
		}, nil
	})

	newJWTValidator = func(cfg *baseconfig.Config, log logpkg.Logger) auth.JWTValidator {
		return testJWTValidator{
			validate: func(ctx context.Context, token string) (*auth.Claims, error) {
				switch token {
				case "valid-with-scope":
					return &auth.Claims{Subject: "u1", Scopes: []string{"users:read"}}, nil
				case "valid-without-scope":
					return &auth.Claims{Subject: "u1", Scopes: []string{"users:write"}}, nil
				default:
					return nil, errors.New("invalid token")
				}
			},
		}
	}

	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		if servers == nil || servers.Public == nil {
			t.Fatalf("expected public server to be built")
		}

		publicRouter := *servers.Public.Router()

		req := httptest.NewRequest(http.MethodGet, "/scoped", nil)
		rec := httptest.NewRecorder()
		publicRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected scoped route to return 401 without auth, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/scoped", nil)
		req.Header.Set("Authorization", "Bearer valid-without-scope")
		rec = httptest.NewRecorder()
		publicRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected scoped route to return 403 without required scope, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/scoped", nil)
		req.Header.Set("Authorization", "Bearer valid-with-scope")
		rec = httptest.NewRecorder()
		publicRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected scoped route to return 200 with required scope, got %d", rec.Code)
		}
		if body := rec.Body.String(); body != "scoped-ok" {
			t.Fatalf("unexpected scoped route body: %q", body)
		}

		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Auth.Enabled = true
	cfg.Auth.Issuer = "https://issuer.example.com"
	cfg.Auth.JWKSUrl = "https://issuer.example.com/.well-known/jwks.json"
	cfg.Auth.Audience = "apigateway"
	gwCfg := &gatewaycfg.Gateway{
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix:      "/",
					Middlewares: []string{"Authenticate"},
					Routes: []gatewaycfg.Route{
						{
							PathPrefix: "/scoped",
							TargetURL:  "http://example.com",
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {Scopes: []string{"users:read"}}}},
							},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, log); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
	denied := log.findMessage("security request denied")
	if denied == nil {
		t.Fatalf("expected denied security log entry")
	}
	if !containsFieldPair(denied.fields, "surface", "public") {
		t.Fatalf("expected public surface in denied log, got %+v", denied.fields)
	}
	if !containsFieldPair(denied.fields, "status", http.StatusUnauthorized) {
		t.Fatalf("expected unauthorized status in denied log, got %+v", denied.fields)
	}
	if !containsFieldPair(denied.fields, "authorization_header_present", false) {
		t.Fatalf("expected auth header presence flag in denied log, got %+v", denied.fields)
	}
	for _, field := range denied.fields {
		if value, ok := field.(string); ok && strings.Contains(value, "valid-with") {
			t.Fatalf("expected denied log to avoid token values, got %+v", denied.fields)
		}
	}
}

func TestRunServerClaimsGuardFromConfigAllowsMatchingClaimsOnly(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	originalValidatorFactory := newJWTValidator
	originalTransport := http.DefaultTransport
	t.Cleanup(func() {
		runHTTPServersWithSignals = originalRun
		newJWTValidator = originalValidatorFactory
		http.DefaultTransport = originalTransport
	})

	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/claims" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString("claims-ok")),
			Request:    r,
		}, nil
	})

	newJWTValidator = func(cfg *baseconfig.Config, log logpkg.Logger) auth.JWTValidator {
		return testJWTValidator{
			validate: func(ctx context.Context, token string) (*auth.Claims, error) {
				switch token {
				case "gold":
					return &auth.Claims{Subject: "u1", Custom: map[string]interface{}{"tier": "gold"}}, nil
				case "silver":
					return &auth.Claims{Subject: "u1", Custom: map[string]interface{}{"tier": "silver"}}, nil
				default:
					return nil, errors.New("invalid token")
				}
			},
		}
	}

	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		if servers == nil || servers.Public == nil {
			t.Fatalf("expected public server to be built")
		}

		publicRouter := *servers.Public.Router()

		req := httptest.NewRequest(http.MethodGet, "/claims", nil)
		rec := httptest.NewRecorder()
		publicRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected claims-guarded route to return 401 without auth, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/claims", nil)
		req.Header.Set("Authorization", "Bearer silver")
		rec = httptest.NewRecorder()
		publicRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected claims-guarded route to return 403 for non-matching claim, got %d", rec.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/claims", nil)
		req.Header.Set("Authorization", "Bearer gold")
		rec = httptest.NewRecorder()
		publicRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected claims-guarded route to return 200 for matching claim, got %d", rec.Code)
		}
		if body := rec.Body.String(); body != "claims-ok" {
			t.Fatalf("unexpected claims-guarded route body: %q", body)
		}

		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Auth.Enabled = true
	cfg.Auth.Issuer = "https://issuer.example.com"
	cfg.Auth.JWKSUrl = "https://issuer.example.com/.well-known/jwks.json"
	cfg.Auth.Audience = "apigateway"
	cfg.Auth.Claims = authcfg.ClaimsConfig{
		Rules: []authcfg.ClaimRule{
			{
				Claim:    "tier",
				Operator: "one_of",
				Values:   []string{"gold"},
			},
		},
	}
	gwCfg := &gatewaycfg.Gateway{
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix:      "/",
					Middlewares: []string{"Authenticate"},
					Routes: []gatewaycfg.Route{
						{
							PathPrefix: "/claims",
							TargetURL:  "http://example.com",
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {Middlewares: []string{"ClaimsGuardFromConfig"}}}},
							},
						},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
}

func TestRunServerPublicWebSocketAllowsAnonymousUpgradePath(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })

	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		if servers == nil || servers.Public == nil {
			t.Fatalf("expected public server to be built")
		}

		publicRouter := *servers.Public.Router()
		serverConn, clientConn := net.Pipe()
		defer func() { _ = clientConn.Close() }()
		defer func() { _ = serverConn.Close() }()

		rec := &wsHijackRecorder{
			ResponseRecorder: httptest.NewRecorder(),
			conn:             serverConn,
			rw:               bufio.NewReadWriter(bufio.NewReader(serverConn), bufio.NewWriter(serverConn)),
		}
		req := newWebSocketRequest("/public-ws")

		done := make(chan struct{})
		go func() {
			defer close(done)
			publicRouter.ServeHTTP(rec, req)
		}()

		assertWebSocketProxyAttempt(t, clientConn)
		<-done
		return nil
	}

	cfg := &baseconfig.Config{}
	gwCfg := &gatewaycfg.Gateway{
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix: "/",
					WebSockets: []gatewaycfg.WebSocket{
						{Path: "/public-ws", TargetURL: "ws://127.0.0.1:1"},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
}

func TestRunServerProtectedWebSocketEnforcesAuthAndScopes(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	originalValidatorFactory := newJWTValidator
	t.Cleanup(func() {
		runHTTPServersWithSignals = originalRun
		newJWTValidator = originalValidatorFactory
	})

	newJWTValidator = func(cfg *baseconfig.Config, log logpkg.Logger) auth.JWTValidator {
		return testJWTValidator{
			validate: func(ctx context.Context, token string) (*auth.Claims, error) {
				switch token {
				case "valid-with-scope":
					return &auth.Claims{Subject: "u1", Scopes: []string{"ws:read"}}, nil
				case "valid-without-scope":
					return &auth.Claims{Subject: "u1", Scopes: []string{"ws:write"}}, nil
				default:
					return nil, errors.New("invalid token")
				}
			},
		}
	}

	runHTTPServersWithSignals = func(servers *httpserver.HTTPServers, opts *httpserver.RunHTTPServersOptions, signals ...os.Signal) error {
		if servers == nil || servers.Public == nil {
			t.Fatalf("expected public server to be built")
		}

		publicRouter := *servers.Public.Router()

		req := newWebSocketRequest("/protected-ws")
		rec := httptest.NewRecorder()
		publicRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected protected websocket to return 401 without auth, got %d", rec.Code)
		}

		req = newWebSocketRequest("/protected-ws")
		req.Header.Set("Authorization", "Bearer valid-without-scope")
		rec = httptest.NewRecorder()
		publicRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected protected websocket to return 403 without required scope, got %d", rec.Code)
		}

		serverConn, clientConn := net.Pipe()
		defer func() { _ = clientConn.Close() }()
		defer func() { _ = serverConn.Close() }()

		req = newWebSocketRequest("/protected-ws")
		req.Header.Set("Authorization", "Bearer valid-with-scope")
		hijackRec := &wsHijackRecorder{
			ResponseRecorder: httptest.NewRecorder(),
			conn:             serverConn,
			rw:               bufio.NewReadWriter(bufio.NewReader(serverConn), bufio.NewWriter(serverConn)),
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			publicRouter.ServeHTTP(hijackRec, req)
		}()

		assertWebSocketProxyAttempt(t, clientConn)
		<-done
		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Auth.Enabled = true
	cfg.Auth.Issuer = "https://issuer.example.com"
	cfg.Auth.JWKSUrl = "https://issuer.example.com/.well-known/jwks.json"
	cfg.Auth.Audience = "apigateway"
	gwCfg := &gatewaycfg.Gateway{
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix:      "/",
					Middlewares: []string{"Authenticate"},
					WebSockets: []gatewaycfg.WebSocket{
						{Path: "/protected-ws", TargetURL: "ws://127.0.0.1:1", Scopes: []string{"ws:read"}},
					},
				},
			},
		},
	}

	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
}

type wsHijackRecorder struct {
	*httptest.ResponseRecorder
	conn net.Conn
	rw   *bufio.ReadWriter
}

func (w *wsHijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, w.rw, nil
}

func newWebSocketRequest(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "test-key")
	return req
}

func assertWebSocketProxyAttempt(t *testing.T, conn net.Conn) {
	t.Helper()
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read websocket proxy response failed: %v", err)
	}
	if n == 0 {
		t.Fatalf("expected websocket proxy response bytes")
	}
	if got := string(buf[:n]); !strings.HasPrefix(got, "HTTP/1.1 502") {
		t.Fatalf("expected websocket proxy attempt to yield 502, got %q", got)
	}
}

func containsFieldPair(fields []any, key string, value any) bool {
	for i := 0; i+1 < len(fields); i += 2 {
		if fields[i] == key && fields[i+1] == value {
			return true
		}
	}
	return false
}
