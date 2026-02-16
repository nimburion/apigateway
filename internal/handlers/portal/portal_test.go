package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/server/router"
)

func TestNewPortalHandlerNilConfig(t *testing.T) {
	if _, err := NewPortalHandler(nil); err == nil {
		t.Fatalf("expected error for nil config")
	}
}

func TestCollectOpenAPIOperations(t *testing.T) {
	doc := &openapi3.T{Paths: openapi3.NewPaths()}
	item := &openapi3.PathItem{Get: &openapi3.Operation{Summary: "Get"}, Post: &openapi3.Operation{Summary: "Post"}}
	doc.Paths.Set("/users", item)

	ops := collectOpenAPIOperations(doc)
	if len(ops) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(ops))
	}
	if ops[0].Method != "GET" || ops[1].Method != "POST" {
		t.Fatalf("unexpected ops order: %#v", ops)
	}
}

func TestFilterOpenAPIInfo(t *testing.T) {
	info := &OpenAPIInfo{
		File: "spec.yml",
		Operations: []OpenAPIOperation{
			{Path: "/users", Method: "GET"},
			{Path: "/users/{id}", Method: "GET"},
		},
	}

	filtered := filterOpenAPIInfo(info, "/users/:id")
	if len(filtered.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(filtered.Operations))
	}
	if filtered.Operations[0].Path != "/users/{id}" {
		t.Fatalf("unexpected filtered op: %#v", filtered.Operations[0])
	}
}

func TestNormalizeOpenAPIPath(t *testing.T) {
	if got := normalizeOpenAPIPath("/users/:id/"); got != "/users/{id}" {
		t.Fatalf("unexpected normalized path: %s", got)
	}
}

func TestJoinRoutePath(t *testing.T) {
	if got := joinRoutePath("/api", "/users"); got != "/api/users" {
		t.Fatalf("unexpected join route path: %s", got)
	}
}

func TestPortalRoutesJSONOpenAPIInfo(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{
		"default": {
			Prefix: "/api",
			Routes: []gatewaycfg.Route{
				{
					PathPrefix: "/users",
					TargetURL:  "http://example.com",
					Endpoints:  []gatewaycfg.Endpoint{{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}}},
					OpenAPI:    &gatewaycfg.OpenAPI{File: ""},
				},
			},
		},
	}}

	h, err := NewPortalHandler(&routes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.routeConfig == nil {
		t.Fatalf("expected route config to be set")
	}
}

type testResponseWriter struct {
	*httptest.ResponseRecorder
}

func (w testResponseWriter) Status() int   { return w.Code }
func (w testResponseWriter) Written() bool { return w.Code != 0 }

type testContext struct {
	req  *http.Request
	resp testResponseWriter
	data map[string]interface{}
}

func newTestContext(req *http.Request) *testContext {
	return &testContext{req: req, resp: testResponseWriter{httptest.NewRecorder()}, data: map[string]interface{}{}}
}

func (c *testContext) Request() *http.Request              { return c.req }
func (c *testContext) SetRequest(r *http.Request)          { c.req = r }
func (c *testContext) Response() router.ResponseWriter     { return c.resp }
func (c *testContext) SetResponse(w router.ResponseWriter) { c.resp = w.(testResponseWriter) }
func (c *testContext) Param(name string) string            { return "" }
func (c *testContext) Query(name string) string            { return "" }
func (c *testContext) Bind(v interface{}) error            { return nil }
func (c *testContext) Get(key string) interface{}          { return c.data[key] }
func (c *testContext) Set(key string, value interface{})   { c.data[key] = value }
func (c *testContext) JSON(code int, v interface{}) error {
	c.resp.Header().Set("Content-Type", "application/json")
	c.resp.WriteHeader(code)
	return json.NewEncoder(c.resp).Encode(v)
}
func (c *testContext) String(code int, s string) error {
	c.resp.WriteHeader(code)
	_, err := c.resp.Write([]byte(s))
	return err
}

func TestGetGroupsAndRoutes(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{
		"default": {
			Prefix:      "/api",
			Middlewares: []string{"Authenticate"},
			AuthEndpoints: &gatewaycfg.AuthEndpoints{
				Me:     true,
				OAuth2: &gatewaycfg.OAuth2Config{Enabled: true},
			},
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
	}}
	h, err := NewPortalHandler(&routes)
	if err != nil {
		t.Fatalf("new portal handler: %v", err)
	}

	ctx := newTestContext(httptest.NewRequest(http.MethodGet, "/api/portal/groups", nil))
	if err := h.GetGroups(ctx); err != nil {
		t.Fatalf("GetGroups error: %v", err)
	}
	if ctx.resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", ctx.resp.Code)
	}

	ctx = newTestContext(httptest.NewRequest(http.MethodGet, "/api/portal/routes", nil))
	if err := h.GetRoutes(ctx); err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}
	if ctx.resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", ctx.resp.Code)
	}
}

func TestGetPortalHTML(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{}}
	h, err := NewPortalHandler(&routes)
	if err != nil {
		t.Fatalf("new portal handler: %v", err)
	}

	ctx := newTestContext(httptest.NewRequest(http.MethodGet, "/portal", nil))
	if err := h.GetPortalHTML(ctx); err != nil {
		t.Fatalf("GetPortalHTML error: %v", err)
	}
	if ctx.resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", ctx.resp.Code)
	}
	if got := ctx.resp.Header().Get("Cache-Control"); got == "" {
		t.Fatalf("expected cache-control header")
	}
	if got := ctx.resp.Header().Get("Content-Type"); got == "" {
		t.Fatalf("expected content-type header")
	}
}

func TestPortalMiddlewaresInvokeNext(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{}}
	h, err := NewPortalHandler(&routes)
	if err != nil {
		t.Fatalf("new portal handler: %v", err)
	}
	ctx := newTestContext(httptest.NewRequest(http.MethodGet, "/portal/assets/app.js", nil))
	called := false
	next := func(c router.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	}

	if err := h.AssetCacheMiddleware()(next)(ctx); err != nil {
		t.Fatalf("asset middleware error: %v", err)
	}
	if !called {
		t.Fatalf("expected asset middleware to call next")
	}

	called = false
	if err := h.StaticMiddleware()(next)(ctx); err != nil {
		t.Fatalf("static middleware error: %v", err)
	}
	if !called {
		t.Fatalf("expected static middleware to call next")
	}
}
