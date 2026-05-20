package portal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	httpopenapi "github.com/nimburion/nimburion/pkg/http/openapi"
	"github.com/nimburion/nimburion/pkg/http/router"
)

func TestNewPortalHandlerNilConfig(t *testing.T) {
	if _, err := NewPortalHandler(nil, nil, nil, nil); err == nil {
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

	h, err := NewPortalHandler(&routes, nil, nil, nil)
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
			Metadata: gatewaycfg.ResourceMetadata{
				OwnerTeam:  "platform",
				Visibility: gatewaycfg.MetadataVisibilityInternal,
			},
			AuthEndpoints: &gatewaycfg.AuthEndpoints{
				Me:     true,
				OAuth2: &gatewaycfg.OAuth2Config{Enabled: true},
			},
			Routes: []gatewaycfg.Route{
				{
					PathPrefix:  "/users",
					TargetURL:   "http://example.com",
					Middlewares: []string{"ForwardIdentityHeaders"},
					Metadata: gatewaycfg.ResourceMetadata{
						Status: gatewaycfg.MetadataStatusDeprecated,
					},
					Endpoints: []gatewaycfg.Endpoint{
						{
							Path: "/",
							Methods: map[string]*gatewaycfg.Method{
								"GET": {
									Scopes:      []string{"users:read"},
									Middlewares: []string{"ClaimsGuardFromConfig"},
								},
							},
						},
					},
				},
			},
			WebSockets: []gatewaycfg.WebSocket{
				{
					Path:      "/events",
					TargetURL: "ws://example.com",
					Scopes:    []string{"events:read"},
				},
			},
		},
	}}
	h, err := NewPortalHandler(&routes, nil, &RuntimeInfo{
		AuthEnabled:           true,
		ManagementEnabled:     true,
		ManagementAuthEnabled: true,
		PortalMode:            gatewaycfg.PortalModeReadOnly,
	}, nil)
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
	var groupsPayload struct {
		Groups []GroupInfo `json:"groups"`
	}
	if err := json.Unmarshal(ctx.resp.Body.Bytes(), &groupsPayload); err != nil {
		t.Fatalf("decode groups payload: %v", err)
	}
	if len(groupsPayload.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groupsPayload.Groups))
	}
	if groupsPayload.Groups[0].RouteCount != 1 {
		t.Fatalf("expected route_count=1, got %d", groupsPayload.Groups[0].RouteCount)
	}
	if !groupsPayload.Groups[0].AuthRequired {
		t.Fatalf("expected group auth_required=true")
	}
	if !groupsPayload.Groups[0].Deprecated {
		t.Fatalf("expected deprecated group aggregate from route metadata")
	}
	if !groupsPayload.Groups[0].RuntimeInfo.AuthEnabled || !groupsPayload.Groups[0].RuntimeInfo.ManagementAuthEnabled {
		t.Fatalf("expected runtime auth flags in group payload: %#v", groupsPayload.Groups[0].RuntimeInfo)
	}

	ctx = newTestContext(httptest.NewRequest(http.MethodGet, "/api/portal/routes", nil))
	if err := h.GetRoutes(ctx); err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}
	if ctx.resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", ctx.resp.Code)
	}
	var routesPayload struct {
		Groups []GroupData `json:"groups"`
	}
	if err := json.Unmarshal(ctx.resp.Body.Bytes(), &routesPayload); err != nil {
		t.Fatalf("decode routes payload: %v", err)
	}
	if len(routesPayload.Groups) != 1 {
		t.Fatalf("expected 1 route group, got %d", len(routesPayload.Groups))
	}
	groupPayload := routesPayload.Groups[0]
	if len(groupPayload.Routes) != 1 {
		t.Fatalf("expected 1 route entry, got %d", len(groupPayload.Routes))
	}
	routePayload := groupPayload.Routes[0]
	if !routePayload.AuthRequired {
		t.Fatalf("expected route auth_required=true")
	}
	if routePayload.TargetURL == "" {
		t.Fatalf("expected target_url to be exposed by default")
	}
	if !strings.Contains(strings.Join(routePayload.Middlewares, ","), "ForwardIdentityHeaders") {
		t.Fatalf("expected effective route middlewares to include route additions")
	}
	if !strings.Contains(strings.Join(routePayload.DeclaredMiddlewares, ","), "ForwardIdentityHeaders") {
		t.Fatalf("expected declared route middlewares to be exposed")
	}
	if len(routePayload.Methods) != 1 || !routePayload.Methods[0].AuthRequired {
		t.Fatalf("expected method auth_required=true")
	}
	if !strings.Contains(strings.Join(routePayload.Methods[0].Middlewares, ","), "ClaimsGuardFromConfig") {
		t.Fatalf("expected effective method middlewares to include method additions")
	}
	if !strings.Contains(strings.Join(routePayload.Methods[0].DeclaredMiddlewares, ","), "ClaimsGuardFromConfig") {
		t.Fatalf("expected declared method middlewares to be exposed")
	}
	if len(groupPayload.WebSockets) != 1 || !groupPayload.WebSockets[0].AuthRequired {
		t.Fatalf("expected websocket auth_required=true")
	}
}

func TestGetMetricsHistory(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{}}
	store := NewLocalMetricsHistoryStore(gatewaycfg.PortalMetricsHistoryConfig{
		Enabled:          true,
		SnapshotInterval: time.Minute,
		MaxSnapshots:     4,
		MaxAge:           time.Hour,
	})
	if err := store.Append(context.Background(), PortalMetricsSnapshot{
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		Data: PortalMetricsData{
			Summary: PortalMetricsSummary{TotalRequests: 12},
			Runtime: PortalRuntimeMetrics{Goroutines: 8},
			Paths: []PortalPathMetric{
				{Path: "/healthz", Requests: 12, Methods: []string{"GET"}},
			},
		},
	}); err != nil {
		t.Fatalf("append metrics snapshot: %v", err)
	}

	h, err := NewPortalHandler(&routes, nil, nil, store)
	if err != nil {
		t.Fatalf("new portal handler: %v", err)
	}

	ctx := newTestContext(httptest.NewRequest(http.MethodGet, "/api/portal/metrics/history", nil))
	if err := h.GetMetricsHistory(ctx); err != nil {
		t.Fatalf("GetMetricsHistory error: %v", err)
	}
	if ctx.resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", ctx.resp.Code)
	}

	var payload PortalMetricsHistoryResponse
	if err := json.Unmarshal(ctx.resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode metrics history payload: %v", err)
	}
	if payload.Source != "local" {
		t.Fatalf("expected local source, got %q", payload.Source)
	}
	if payload.SnapshotCount != 1 {
		t.Fatalf("expected snapshot_count=1, got %d", payload.SnapshotCount)
	}
	if len(payload.Snapshots) != 1 || len(payload.Snapshots[0].Data.Paths) != 1 {
		t.Fatalf("unexpected snapshots payload: %#v", payload)
	}
	if payload.Snapshots[0].Data.Paths[0].Path != "/healthz" {
		t.Fatalf("expected /healthz path, got %#v", payload.Snapshots[0].Data.Paths[0])
	}
}

func TestGetRoutesRespectsPortalCatalogVisibility(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{
		"default": {
			Prefix: "/api",
			Routes: []gatewaycfg.Route{
				{
					PathPrefix: "/users",
					TargetURL:  "http://example.com",
					OpenAPI: &gatewaycfg.OpenAPI{
						File: "",
						Mode: gatewaycfg.OpenAPIValidationModeStrict,
					},
					Endpoints: []gatewaycfg.Endpoint{
						{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
					},
				},
			},
		},
	}}
	portalCfg := &gatewaycfg.PortalConfig{
		Catalog: gatewaycfg.PortalCatalogConfig{
			ExposeTargetURLs:    false,
			ExposeOpenAPIErrors: false,
		},
	}
	h, err := NewPortalHandler(&routes, portalCfg, nil, nil)
	if err != nil {
		t.Fatalf("new portal handler: %v", err)
	}

	ctx := newTestContext(httptest.NewRequest(http.MethodGet, "/api/portal/routes", nil))
	if err := h.GetRoutes(ctx); err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}
	if ctx.resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", ctx.resp.Code)
	}

	var payload struct {
		Groups []GroupData `json:"groups"`
	}
	if err := json.Unmarshal(ctx.resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode routes payload: %v", err)
	}
	routePayload := payload.Groups[0].Routes[0]
	if routePayload.TargetURL != "" {
		t.Fatalf("expected target_url to be hidden")
	}
	if routePayload.OpenAPI == nil {
		t.Fatalf("expected openapi payload")
	}
	if routePayload.OpenAPI.Error != "" {
		t.Fatalf("expected openapi error to be hidden")
	}
}

func TestGetRoutesDoesNotTreatForwardIdentityHeadersAsAuthRequirement(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{
		"default": {
			Prefix:      "/api",
			Middlewares: []string{"ForwardIdentityHeaders"},
			Routes: []gatewaycfg.Route{
				{
					PathPrefix:  "/users",
					TargetURL:   "http://example.com",
					Middlewares: []string{"ForwardIdentityHeaders"},
					Endpoints: []gatewaycfg.Endpoint{
						{
							Path: "/",
							Methods: map[string]*gatewaycfg.Method{
								"GET": {},
							},
						},
					},
				},
			},
		},
	}}

	h, err := NewPortalHandler(&routes, nil, nil, nil)
	if err != nil {
		t.Fatalf("new portal handler: %v", err)
	}

	ctx := newTestContext(httptest.NewRequest(http.MethodGet, "/api/portal/routes", nil))
	if err := h.GetRoutes(ctx); err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}

	var payload struct {
		Groups []GroupData `json:"groups"`
	}
	if err := json.Unmarshal(ctx.resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode routes payload: %v", err)
	}
	if len(payload.Groups) != 1 || len(payload.Groups[0].Routes) != 1 {
		t.Fatalf("unexpected routes payload: %#v", payload)
	}
	groupPayload := payload.Groups[0]
	routePayload := groupPayload.Routes[0]
	if groupPayload.AuthRequired {
		t.Fatalf("expected group auth_required=false with ForwardIdentityHeaders only")
	}
	if routePayload.AuthRequired {
		t.Fatalf("expected route auth_required=false with ForwardIdentityHeaders only")
	}
	if routePayload.Methods[0].AuthRequired {
		t.Fatalf("expected method auth_required=false with ForwardIdentityHeaders only")
	}
}

func TestGetRoutesIncludesRuntimeOnlyRegisteredRoutes(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{
		"default": {
			Prefix: "/api",
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

	h, err := NewPortalHandler(&routes, nil, nil, nil,
		RuntimeRoute{
			Route: httpopenapi.Route{Method: "GET", Path: "/api/users/"},
		},
		RuntimeRoute{
			Route: httpopenapi.Route{
				Method: "GET",
				Path:   "/api/auth/me",
				Annotations: httpopenapi.EndpointAnnotations{
					Summary: "Authenticated user info",
				},
			},
			Metadata: gatewaycfg.ResourceMetadata{OwnerTeam: "platform", Domain: "auth", Visibility: gatewaycfg.MetadataVisibilityInternal, Status: gatewaycfg.MetadataStatusActive},
		},
	)
	if err != nil {
		t.Fatalf("new portal handler: %v", err)
	}

	ctx := newTestContext(httptest.NewRequest(http.MethodGet, "/api/portal/routes", nil))
	if err := h.GetRoutes(ctx); err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}

	var payload struct {
		Groups []GroupData `json:"groups"`
	}
	if err := json.Unmarshal(ctx.resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode routes payload: %v", err)
	}
	if len(payload.Groups) != 1 {
		t.Fatalf("expected one group, got %#v", payload.Groups)
	}
	if len(payload.Groups[0].Routes) != 2 {
		t.Fatalf("expected config route plus runtime-only route, got %#v", payload.Groups[0].Routes)
	}

	var runtimeOnly *RouteInfo
	for i := range payload.Groups[0].Routes {
		if payload.Groups[0].Routes[i].RuntimeOnly {
			runtimeOnly = &payload.Groups[0].Routes[i]
			break
		}
	}
	if runtimeOnly == nil {
		t.Fatalf("expected runtime-only route in payload")
	}
	if runtimeOnly.PathPrefix != "/api/auth/me" {
		t.Fatalf("unexpected runtime-only path: %#v", runtimeOnly)
	}
	if len(runtimeOnly.Methods) != 1 || runtimeOnly.Methods[0].Method != "GET" {
		t.Fatalf("unexpected runtime-only methods: %#v", runtimeOnly.Methods)
	}
}

func TestGetRoutesSkipsSyntheticMethodNotAllowedRuntimeRoutes(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{
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
	}}

	h, err := NewPortalHandler(&routes, nil, nil, nil,
		RuntimeRoute{Route: httpopenapi.Route{Method: "POST", Path: "/healthz"}},
		RuntimeRoute{Route: httpopenapi.Route{Method: "DELETE", Path: "/healthz"}},
	)
	if err != nil {
		t.Fatalf("new portal handler: %v", err)
	}

	ctx := newTestContext(httptest.NewRequest(http.MethodGet, "/api/portal/routes", nil))
	if err := h.GetRoutes(ctx); err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}

	var payload struct {
		Groups []GroupData `json:"groups"`
	}
	if err := json.Unmarshal(ctx.resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode routes payload: %v", err)
	}
	if len(payload.Groups) != 1 {
		t.Fatalf("expected one group, got %#v", payload.Groups)
	}
	if got := len(payload.Groups[0].Routes); got != 1 {
		t.Fatalf("expected synthetic 405 runtime routes to be skipped, got %#v", payload.Groups[0].Routes)
	}
}

func TestGetRoutesPlacesManagementRuntimeRoutesInDedicatedGroup(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{
		"default": {
			Prefix: "/",
		},
	}}

	h, err := NewPortalHandler(&routes, nil, nil, nil,
		RuntimeRoute{
			Route: httpopenapi.Route{
				Method: "GET",
				Path:   "/health",
				Annotations: httpopenapi.EndpointAnnotations{
					Summary: "Management health probe",
				},
			},
			GroupName:      "__management__",
			GroupPrefix:    "/",
			SurfaceContext: "management",
			Metadata: gatewaycfg.ResourceMetadata{
				Domain:     "management",
				Visibility: gatewaycfg.MetadataVisibilityInternal,
				Status:     gatewaycfg.MetadataStatusActive,
			},
		},
	)
	if err != nil {
		t.Fatalf("new portal handler: %v", err)
	}

	ctx := newTestContext(httptest.NewRequest(http.MethodGet, "/api/portal/routes", nil))
	if err := h.GetRoutes(ctx); err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}

	var payload struct {
		Groups []GroupData `json:"groups"`
	}
	if err := json.Unmarshal(ctx.resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode routes payload: %v", err)
	}

	var managementGroup *GroupData
	for i := range payload.Groups {
		if payload.Groups[i].Name == "__management__" {
			managementGroup = &payload.Groups[i]
			break
		}
	}
	if managementGroup == nil {
		t.Fatalf("expected __management__ group in payload: %#v", payload.Groups)
	}
	if len(managementGroup.Routes) != 1 {
		t.Fatalf("expected management group to expose one route, got %#v", managementGroup.Routes)
	}
	if managementGroup.Routes[0].SurfaceContext != "management" {
		t.Fatalf("expected management surface context, got %#v", managementGroup.Routes[0])
	}
}

func TestGetRoutesSortsGroupsDeterministically(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{
		"zeta": {
			Prefix: "/z",
		},
		"default": {
			Prefix: "/",
		},
		"alpha": {
			Prefix: "/a",
		},
	}}

	h, err := NewPortalHandler(&routes, nil, nil, nil)
	if err != nil {
		t.Fatalf("new portal handler: %v", err)
	}

	ctx := newTestContext(httptest.NewRequest(http.MethodGet, "/api/portal/routes", nil))
	if err := h.GetRoutes(ctx); err != nil {
		t.Fatalf("GetRoutes error: %v", err)
	}

	var payload struct {
		Groups []GroupData `json:"groups"`
	}
	if err := json.Unmarshal(ctx.resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode routes payload: %v", err)
	}
	if len(payload.Groups) != 3 {
		t.Fatalf("expected three groups, got %#v", payload.Groups)
	}
	gotOrder := []string{payload.Groups[0].Name, payload.Groups[1].Name, payload.Groups[2].Name}
	wantOrder := []string{"default", "alpha", "zeta"}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("unexpected group order: got %v want %v", gotOrder, wantOrder)
		}
	}
}

func TestGetGroupsIncludesManagementRuntimeGroup(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{
		"default": {
			Prefix: "/",
		},
	}}

	h, err := NewPortalHandler(&routes, nil, nil, nil,
		RuntimeRoute{
			Route:          httpopenapi.Route{Method: "GET", Path: "/health"},
			GroupName:      "__management__",
			GroupPrefix:    "/",
			SurfaceContext: "management",
			Metadata: gatewaycfg.ResourceMetadata{
				Domain:     "management",
				Visibility: gatewaycfg.MetadataVisibilityInternal,
				Status:     gatewaycfg.MetadataStatusActive,
			},
		},
	)
	if err != nil {
		t.Fatalf("new portal handler: %v", err)
	}

	ctx := newTestContext(httptest.NewRequest(http.MethodGet, "/api/portal/groups", nil))
	if err := h.GetGroups(ctx); err != nil {
		t.Fatalf("GetGroups error: %v", err)
	}

	var payload struct {
		Groups []GroupInfo `json:"groups"`
	}
	if err := json.Unmarshal(ctx.resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode groups payload: %v", err)
	}

	var managementGroup *GroupInfo
	for i := range payload.Groups {
		if payload.Groups[i].Name == "__management__" {
			managementGroup = &payload.Groups[i]
			break
		}
	}
	if managementGroup == nil {
		t.Fatalf("expected __management__ group in groups payload: %#v", payload.Groups)
	}
	if managementGroup.RouteCount != 1 {
		t.Fatalf("expected one management runtime route, got %#v", managementGroup)
	}
}

func TestGetPortalHTML(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{}}
	h, err := NewPortalHandler(&routes, nil, nil, nil)
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

func TestGetPortalHTMLRejectsInvalidAssetPath(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{}}
	h, err := NewPortalHandler(&routes, nil, nil, nil)
	if err != nil {
		t.Fatalf("new portal handler: %v", err)
	}

	ctx := newTestContext(httptest.NewRequest(http.MethodGet, "/portal/assets/%2e%2e/index.html", nil))
	if err := h.GetPortalHTML(ctx); err != nil {
		t.Fatalf("GetPortalHTML error: %v", err)
	}
	if ctx.resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid portal asset path, got %d", ctx.resp.Code)
	}
	if !strings.Contains(ctx.resp.Body.String(), "invalid portal asset path") {
		t.Fatalf("expected invalid path error body, got %q", ctx.resp.Body.String())
	}
}
