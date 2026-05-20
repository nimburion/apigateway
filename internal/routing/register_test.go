package routing

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nimburion/apigateway/internal/config"
	openapimiddleware "github.com/nimburion/nimburion/pkg/http/contract/openapi"
	"github.com/nimburion/nimburion/pkg/http/router"
	logpkg "github.com/nimburion/nimburion/pkg/observability/logger"
)

type testResponseWriter struct {
	*httptest.ResponseRecorder
}

func (w testResponseWriter) Status() int   { return w.Code }
func (w testResponseWriter) Written() bool { return w.Code != 0 }

type testContext struct {
	req  *http.Request
	resp testResponseWriter
}

func newTestContext(req *http.Request) *testContext {
	return &testContext{req: req, resp: testResponseWriter{httptest.NewRecorder()}}
}

func (c *testContext) Request() *http.Request              { return c.req }
func (c *testContext) SetRequest(r *http.Request)          { c.req = r }
func (c *testContext) Response() router.ResponseWriter     { return c.resp }
func (c *testContext) SetResponse(w router.ResponseWriter) { c.resp = w.(testResponseWriter) }
func (c *testContext) Param(name string) string            { return "" }
func (c *testContext) Query(name string) string            { return "" }
func (c *testContext) Bind(v interface{}) error            { return nil }
func (c *testContext) Get(key string) interface{}          { return nil }
func (c *testContext) Set(key string, value interface{})   {}
func (c *testContext) JSON(code int, v interface{}) error  { c.resp.WriteHeader(code); return nil }
func (c *testContext) String(code int, s string) error {
	c.resp.WriteHeader(code)
	_, err := c.resp.Write([]byte(s))
	return err
}

type fakeRouter struct {
	routes map[string][]routeRegistration
}

type routeRegistration struct {
	path            string
	middlewareCount int
}

func newFakeRouter() *fakeRouter {
	return &fakeRouter{routes: map[string][]routeRegistration{}}
}

func (f *fakeRouter) record(method, path string, middlewareCount int) {
	f.routes[method] = append(f.routes[method], routeRegistration{
		path:            path,
		middlewareCount: middlewareCount,
	})
}

func (f *fakeRouter) GET(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	f.record(http.MethodGet, path, len(middleware))
}
func (f *fakeRouter) POST(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	f.record(http.MethodPost, path, len(middleware))
}
func (f *fakeRouter) PUT(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	f.record(http.MethodPut, path, len(middleware))
}
func (f *fakeRouter) DELETE(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	f.record(http.MethodDelete, path, len(middleware))
}
func (f *fakeRouter) PATCH(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	f.record(http.MethodPatch, path, len(middleware))
}
func (f *fakeRouter) Group(prefix string, middleware ...router.MiddlewareFunc) router.Router {
	return f
}
func (f *fakeRouter) Use(middleware ...router.MiddlewareFunc)          {}
func (f *fakeRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {}

func TestBuildAllowHeaderValue(t *testing.T) {
	allow := buildAllowHeaderValue(map[string]struct{}{http.MethodGet: {}, http.MethodPost: {}})
	if allow != "GET, POST, OPTIONS" {
		t.Fatalf("unexpected allow header: %s", allow)
	}
}

func TestRegisterMethodNotAllowedRoutes(t *testing.T) {
	r := newFakeRouter()
	registerMethodNotAllowedRoutes(r, "/path", map[string]struct{}{http.MethodGet: {}})

	if len(r.routes[http.MethodGet]) != 0 {
		t.Fatalf("expected no GET registration for method not allowed")
	}
	if len(r.routes[http.MethodPost]) != 1 || len(r.routes[http.MethodPut]) != 1 || len(r.routes[http.MethodPatch]) != 1 || len(r.routes[http.MethodDelete]) != 1 {
		t.Fatalf("expected method not allowed handlers for non-allowed methods")
	}
}

func TestJoinRoutePath(t *testing.T) {
	if got := joinRoutePath("/api", "/users"); got != "/api/users" {
		t.Fatalf("unexpected joined path: %s", got)
	}
}

func TestRegisterProxyRouteOpenAPIMiddlewareCreatedAndAttached(t *testing.T) {
	originalFactory := newOpenAPIRequestValidationMiddleware
	t.Cleanup(func() {
		newOpenAPIRequestValidationMiddleware = originalFactory
	})

	called := 0
	var gotCfg openapimiddleware.Config
	newOpenAPIRequestValidationMiddleware = func(cfg openapimiddleware.Config, _ logpkg.Logger) (router.MiddlewareFunc, error) {
		called++
		gotCfg = cfg
		return func(next router.HandlerFunc) router.HandlerFunc { return next }, nil
	}

	r := newFakeRouter()
	err := RegisterProxyRoute(r, config.Route{
		PathPrefix:  "/users",
		TargetURL:   "http://example.com",
		StripPrefix: " /api ",
		OpenAPI: &config.OpenAPI{
			File:         "fallback.yaml",
			ResolvedFile: "/tmp/spec.yaml",
			Mode:         config.OpenAPIValidationModeWarnOnly,
		},
		Endpoints: []config.Endpoint{
			{
				Path: "/",
				Methods: map[string]*config.Method{
					http.MethodGet: {},
				},
			},
		},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected openapi middleware factory to be called once, got %d", called)
	}
	if gotCfg.SpecPath != "/tmp/spec.yaml" {
		t.Fatalf("expected resolved spec path, got %q", gotCfg.SpecPath)
	}
	if gotCfg.StripPrefix != "/api" {
		t.Fatalf("expected strip prefix '/api', got %q", gotCfg.StripPrefix)
	}
	if gotCfg.Mode != config.OpenAPIValidationModeWarnOnly {
		t.Fatalf("expected mode %q, got %q", config.OpenAPIValidationModeWarnOnly, gotCfg.Mode)
	}

	getRoutes := r.routes[http.MethodGet]
	if len(getRoutes) == 0 {
		t.Fatalf("expected GET route registration")
	}
	if getRoutes[0].middlewareCount != 1 {
		t.Fatalf("expected 1 middleware on GET route, got %d", getRoutes[0].middlewareCount)
	}
}

func TestRegisterProxyRouteWithoutOpenAPIDoesNotBuildValidationMiddleware(t *testing.T) {
	originalFactory := newOpenAPIRequestValidationMiddleware
	t.Cleanup(func() {
		newOpenAPIRequestValidationMiddleware = originalFactory
	})

	called := 0
	newOpenAPIRequestValidationMiddleware = func(cfg openapimiddleware.Config, _ logpkg.Logger) (router.MiddlewareFunc, error) {
		called++
		return func(next router.HandlerFunc) router.HandlerFunc { return next }, nil
	}

	r := newFakeRouter()
	err := RegisterProxyRoute(r, config.Route{
		PathPrefix: "/users",
		TargetURL:  "http://example.com",
		Endpoints: []config.Endpoint{
			{
				Path: "/",
				Methods: map[string]*config.Method{
					http.MethodGet: {},
				},
			},
		},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != 0 {
		t.Fatalf("expected openapi middleware factory to not be called, got %d", called)
	}

	getRoutes := r.routes[http.MethodGet]
	if len(getRoutes) == 0 {
		t.Fatalf("expected GET route registration")
	}
	if getRoutes[0].middlewareCount != 0 {
		t.Fatalf("expected 0 middleware on GET route, got %d", getRoutes[0].middlewareCount)
	}
}

func TestRegisterWebSocketRouteUsesRouteRateLimitAndScopes(t *testing.T) {
	r := newFakeRouter()
	RegisterWebSocketRoute(r, config.WebSocket{
		Path:      "/ws",
		TargetURL: "ws://example.com",
		Scopes:    []string{"ws:read"},
		RateLimit: &config.RateLimit{RequestsPerSecond: 5, Burst: 2},
	}, &config.RateLimit{RequestsPerSecond: 100, Burst: 20}, nil)

	getRoutes := r.routes[http.MethodGet]
	if len(getRoutes) != 1 {
		t.Fatalf("expected one websocket GET registration, got %d", len(getRoutes))
	}
	if getRoutes[0].path != "/ws" {
		t.Fatalf("unexpected websocket path: %s", getRoutes[0].path)
	}
	if getRoutes[0].middlewareCount != 2 {
		t.Fatalf("expected rate limit + scopes middleware, got %d", getRoutes[0].middlewareCount)
	}
}

func TestRegisterWebSocketRouteUsesGroupRateLimitWhenRouteMissing(t *testing.T) {
	r := newFakeRouter()
	RegisterWebSocketRoute(r, config.WebSocket{
		Path:      "/ws",
		TargetURL: "ws://example.com",
	}, &config.RateLimit{RequestsPerSecond: 10, Burst: 5}, nil)

	getRoutes := r.routes[http.MethodGet]
	if len(getRoutes) != 1 {
		t.Fatalf("expected one websocket GET registration, got %d", len(getRoutes))
	}
	if getRoutes[0].middlewareCount != 1 {
		t.Fatalf("expected group rate limit middleware only, got %d", getRoutes[0].middlewareCount)
	}
}

func TestApplyMiddlewareDirectivesSupportsDisableAndReenable(t *testing.T) {
	base := []string{"Authenticate", "ForwardIdentityHeaders"}
	got := ApplyMiddlewareDirectives(base, []string{"Authenticate"}, []string{"Authenticate"})
	if len(got) != 1 || got[0] != "ForwardIdentityHeaders" {
		t.Fatalf("unexpected middleware resolution after disable: %#v", got)
	}

	got = ApplyMiddlewareDirectives(got, []string{"Authenticate"}, nil)
	if len(got) != 2 || got[1] != "Authenticate" {
		t.Fatalf("expected middleware re-enable at deeper level, got %#v", got)
	}
}

func TestRegisterProxyRouteAppliesResolvedConfiguredMiddlewares(t *testing.T) {
	r := newFakeRouter()
	registry := map[string]func() router.MiddlewareFunc{
		"Authenticate": func() router.MiddlewareFunc {
			return func(next router.HandlerFunc) router.HandlerFunc { return next }
		},
		"ForwardIdentityHeaders": func() router.MiddlewareFunc {
			return func(next router.HandlerFunc) router.HandlerFunc { return next }
		},
	}

	err := RegisterProxyRouteWithMiddlewareRegistry(r, config.Route{
		PathPrefix:         "/users",
		TargetURL:          "http://example.com",
		Middlewares:        []string{"ForwardIdentityHeaders"},
		DisableMiddlewares: []string{"Authenticate"},
		Endpoints: []config.Endpoint{
			{
				Path:        "/",
				Middlewares: []string{"Authenticate"},
				Methods: map[string]*config.Method{
					http.MethodGet: {
						DisableMiddlewares: []string{"ForwardIdentityHeaders"},
					},
				},
			},
		},
	}, nil, nil, nil, registry, []string{"Authenticate"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	getRoutes := r.routes[http.MethodGet]
	if len(getRoutes) == 0 {
		t.Fatalf("expected GET route registration")
	}
	if getRoutes[0].middlewareCount != 1 {
		t.Fatalf("expected exactly one configured middleware on GET route, got %d", getRoutes[0].middlewareCount)
	}
}

func TestRegisterWebSocketRouteAppliesResolvedConfiguredMiddlewares(t *testing.T) {
	r := newFakeRouter()
	registry := map[string]func() router.MiddlewareFunc{
		"Authenticate": func() router.MiddlewareFunc {
			return func(next router.HandlerFunc) router.HandlerFunc { return next }
		},
		"ForwardIdentityHeaders": func() router.MiddlewareFunc {
			return func(next router.HandlerFunc) router.HandlerFunc { return next }
		},
	}

	RegisterWebSocketRouteWithMiddlewareRegistry(r, config.WebSocket{
		Path:               "/ws",
		TargetURL:          "ws://example.com",
		DisableMiddlewares: []string{"Authenticate"},
		Middlewares:        []string{"ForwardIdentityHeaders"},
	}, nil, nil, registry, []string{"Authenticate"})

	getRoutes := r.routes[http.MethodGet]
	if len(getRoutes) != 1 {
		t.Fatalf("expected one websocket GET registration, got %d", len(getRoutes))
	}
	if getRoutes[0].middlewareCount != 1 {
		t.Fatalf("expected one configured websocket middleware, got %d", getRoutes[0].middlewareCount)
	}
}

func TestForwardHTTPRequestFollowsBackendRedirects(t *testing.T) {
	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok-final"))
	}))
	defer finalServer.Close()

	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalServer.URL+"/done", http.StatusTemporaryRedirect)
	}))
	defer redirectServer.Close()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	ctx := newTestContext(req)

	targetURL, err := url.Parse(redirectServer.URL)
	if err != nil {
		t.Fatalf("parse target url: %v", err)
	}

	if err := forwardHTTPRequest(ctx.Response(), ctx.Request(), targetURL, ""); err != nil {
		t.Fatalf("forward request: %v", err)
	}

	if ctx.resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", ctx.resp.Code)
	}
	if got := ctx.resp.Header().Get("Location"); got != "" {
		t.Fatalf("expected no redirect Location header, got %q", got)
	}
	body, err := io.ReadAll(ctx.resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if string(body) != "ok-final" {
		t.Fatalf("unexpected response body: %q", string(body))
	}
}
