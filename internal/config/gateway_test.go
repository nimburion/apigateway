package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/nimburion/nimburion/pkg/http/router"
)

func TestGatewayValidate(t *testing.T) {
	cfg := &Gateway{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error without routes at config validation time: %v", err)
	}

	cfg = &Gateway{RoutesFiles: []string{"  routes.yaml "}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RoutesFiles[0] != "routes.yaml" {
		t.Fatalf("expected routes file to be trimmed")
	}
}

func TestLoadRoutesRequiresConfiguredSource(t *testing.T) {
	cfg := &Gateway{}
	err := cfg.LoadRoutes("", map[string]func() router.MiddlewareFunc{})
	if err == nil {
		t.Fatalf("expected error when no route source is configured")
	}
	expected := "either routes_files or inline routes must be provided when config_store.source_of_truth=file"
	if err.Error() != expected {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRoutesDatabaseSourceWithoutMaterializedRoutes(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Routes = Routing{}
	cfg.RoutesFiles = nil
	cfg.ConfigStore.SourceOfTruth = ConfigSourceOfTruthDatabase

	err := cfg.LoadRoutes("", map[string]func() router.MiddlewareFunc{})
	if err == nil {
		t.Fatalf("expected error when database source has no materialized routes")
	}
	if err.Error() != "routes configuration must define at least one group" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGatewayValidateDatabaseSourceOfTruth(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Routes = Routing{}
	cfg.RoutesFiles = nil
	cfg.ConfigStore.Enabled = true
	cfg.ConfigStore.SourceOfTruth = ConfigSourceOfTruthDatabase
	cfg.ConfigStore.Backend = ConfigStoreBackendPostgres
	cfg.Database.Type = ConfigStoreBackendPostgres

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected database-backed config to validate, got %v", err)
	}
}

func TestGatewayValidateManagedPortalRequiresManagementAndScopes(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.RoutesFiles = []string{"routes.yaml"}
	cfg.Portal.Mode = PortalModeManaged
	cfg.Management.Enabled = true
	cfg.Management.AuthEnabled = true
	cfg.Portal.Auth.WriteScopes = []string{"management:config:write"}
	cfg.Portal.Auth.PublishScopes = []string{"management:config:publish"}
	cfg.Portal.Auth.RollbackScopes = []string{"management:config:rollback"}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected managed portal config to validate, got %v", err)
	}
}

func TestGatewayValidateRejectsInvalidCombinations(t *testing.T) {
	testCases := []struct {
		name string
		cfg  *Gateway
	}{
		{
			name: "database source requires config store enabled",
			cfg: func() *Gateway {
				cfg := NewDefaultConfig()
				cfg.Routes = Routing{}
				cfg.RoutesFiles = nil
				cfg.ConfigStore.SourceOfTruth = ConfigSourceOfTruthDatabase
				cfg.ConfigStore.Backend = ConfigStoreBackendPostgres
				cfg.Database.Type = ConfigStoreBackendPostgres
				return cfg
			}(),
		},
		{
			name: "database source requires postgres database type",
			cfg: func() *Gateway {
				cfg := NewDefaultConfig()
				cfg.Routes = Routing{}
				cfg.RoutesFiles = nil
				cfg.ConfigStore.Enabled = true
				cfg.ConfigStore.SourceOfTruth = ConfigSourceOfTruthDatabase
				cfg.ConfigStore.Backend = ConfigStoreBackendPostgres
				cfg.Database.Type = "mysql"
				return cfg
			}(),
		},
		{
			name: "managed portal requires management auth",
			cfg: func() *Gateway {
				cfg := NewDefaultConfig()
				cfg.RoutesFiles = []string{"routes.yaml"}
				cfg.Portal.Mode = PortalModeManaged
				cfg.Management.Enabled = true
				cfg.Management.AuthEnabled = false
				cfg.Portal.Auth.WriteScopes = []string{"management:config:write"}
				cfg.Portal.Auth.PublishScopes = []string{"management:config:publish"}
				cfg.Portal.Auth.RollbackScopes = []string{"management:config:rollback"}
				return cfg
			}(),
		},
		{
			name: "managed portal requires write publish rollback scopes",
			cfg: func() *Gateway {
				cfg := NewDefaultConfig()
				cfg.RoutesFiles = []string{"routes.yaml"}
				cfg.Portal.Mode = PortalModeManaged
				cfg.Management.Enabled = true
				cfg.Management.AuthEnabled = true
				cfg.Portal.Auth.WriteScopes = []string{"management:config:write"}
				return cfg
			}(),
		},
		{
			name: "auto reload requires last good cache path",
			cfg: func() *Gateway {
				cfg := NewDefaultConfig()
				cfg.RoutesFiles = []string{"routes.yaml"}
				cfg.ConfigStore.AutoReload = true
				cfg.ConfigStore.LastGoodCachePath = ""
				return cfg
			}(),
		},
		{
			name: "enabled config store requires positive poll interval",
			cfg: func() *Gateway {
				cfg := NewDefaultConfig()
				cfg.RoutesFiles = []string{"routes.yaml"}
				cfg.ConfigStore.Enabled = true
				cfg.ConfigStore.PollInterval = 0
				return cfg
			}(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestMergeOverlayGroup(t *testing.T) {
	base := Group{
		Prefix: "/api",
		AuthEndpoints: &AuthEndpoints{
			OAuth2: &OAuth2Config{},
		},
		Routes: []Route{
			{
				PathPrefix: "/users",
				TargetURL:  "http://example.com",
				Endpoints: []Endpoint{
					{
						Path:    "/",
						Methods: map[string]*Method{"GET": {}},
					},
				},
			},
		},
	}
	overlay := Group{AuthEndpoints: &AuthEndpoints{OAuth2: &OAuth2Config{ClientSecret: "secret"}}}

	merged, ok := mergeOverlayGroup(base, overlay)
	if !ok {
		t.Fatalf("expected overlay merge")
	}
	if merged.AuthEndpoints == nil || merged.AuthEndpoints.OAuth2 == nil || merged.AuthEndpoints.OAuth2.ClientSecret != "secret" {
		t.Fatalf("expected client secret to be applied")
	}
}

func TestNormalizeOpenAPIPath(t *testing.T) {
	if got := normalizeOpenAPIPath("/users/:id/"); got != "/users/{id}" {
		t.Fatalf("unexpected normalized path: %s", got)
	}
}

func TestDiffOperations(t *testing.T) {
	expected := map[string]struct{}{"GET /users": {}, "POST /users": {}}
	actual := map[string]struct{}{"GET /users": {}}
	missing := diffOperations(expected, actual)
	if len(missing) != 1 || missing[0] != "POST /users" {
		t.Fatalf("unexpected missing ops: %#v", missing)
	}
}

func TestLoadRoutesMergesInlineAndFiles(t *testing.T) {
	tmp := t.TempDir()
	routeFile := filepath.Join(tmp, "routes.yaml")
	content := `
routes:
  groups:
    central:
      prefix: /central
      routes:
        - path_prefix: /admin
          target_url: http://admin.example.com
          endpoints:
            - path: /
              methods:
                GET: {}
`
	if err := os.WriteFile(routeFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write route file: %v", err)
	}

	cfg := &Gateway{
		Routes: Routing{Groups: map[string]Group{
			"default": {
				Prefix: "/",
				Routes: []Route{
					{
						PathPrefix: "/users",
						TargetURL:  "http://users.example.com",
						Endpoints: []Endpoint{
							{Path: "/", Methods: map[string]*Method{"GET": {}}},
						},
					},
				},
			},
		}},
		RoutesFiles: []string{filepath.Base(routeFile)},
	}

	if err := cfg.LoadRoutes(tmp, map[string]func() router.MiddlewareFunc{}); err != nil {
		t.Fatalf("load routes: %v", err)
	}
	if len(cfg.Routes.Groups) != 2 {
		t.Fatalf("expected 2 groups after merge, got %d", len(cfg.Routes.Groups))
	}
}

func TestLoadRoutesOpenAPIAlignmentError(t *testing.T) {
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "spec.yaml")
	spec := `openapi: 3.0.3
info:
  title: test
  version: "1.0.0"
paths:
  /orders:
    get:
      responses:
        "200":
          description: ok
`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	cfg := &Gateway{
		Routes: Routing{Groups: map[string]Group{
			"default": {
				Prefix: "/",
				Routes: []Route{
					{
						PathPrefix: "/users",
						TargetURL:  "http://users.example.com",
						OpenAPI:    &OpenAPI{File: filepath.Base(specPath), Mode: OpenAPIValidationModeStrict},
						Endpoints: []Endpoint{
							{Path: "/", Methods: map[string]*Method{"GET": {}}},
						},
					},
				},
			},
		}},
	}

	err := cfg.LoadRoutes(tmp, map[string]func() router.MiddlewareFunc{})
	if err == nil {
		t.Fatalf("expected openapi alignment error")
	}
}

func TestCollectOpenAPISpecOperations(t *testing.T) {
	doc := &openapi3.T{Paths: openapi3.NewPaths()}
	doc.Paths.Set("/users", &openapi3.PathItem{
		Get:  &openapi3.Operation{},
		Post: &openapi3.Operation{},
	})
	doc.Paths.Set("/admin/users", &openapi3.PathItem{
		Delete: &openapi3.Operation{},
	})
	doc.Paths.Set("/users/{id}", &openapi3.PathItem{
		Patch: &openapi3.Operation{},
	})

	ops := collectOpenAPISpecOperations(doc, "/users")
	if _, ok := ops["GET /users"]; !ok {
		t.Fatalf("expected GET /users operation")
	}
	if _, ok := ops["PATCH /users/{id}"]; !ok {
		t.Fatalf("expected PATCH /users/{id} operation")
	}
	if _, ok := ops["DELETE /admin/users"]; ok {
		t.Fatalf("did not expect /admin/users operation for /users prefix")
	}
}

func TestNewDefaultConfigAndHelpers(t *testing.T) {
	cfg := NewDefaultConfig()
	if cfg == nil {
		t.Fatalf("expected default config")
	}
	if cfg.Routes.Groups != nil && len(cfg.Routes.Groups) != 0 {
		t.Fatalf("expected empty default routes")
	}
	if cfg.Portal.Mode != PortalModeReadOnly {
		t.Fatalf("expected portal mode %q, got %q", PortalModeReadOnly, cfg.Portal.Mode)
	}
	if !cfg.Portal.Enabled {
		t.Fatalf("expected portal enabled by default")
	}
	if len(cfg.Portal.Auth.ReadScopes) != 1 || cfg.Portal.Auth.ReadScopes[0] != "management:portal" {
		t.Fatalf("unexpected portal read scopes: %#v", cfg.Portal.Auth.ReadScopes)
	}
	if cfg.ConfigStore.SourceOfTruth != ConfigSourceOfTruthFile {
		t.Fatalf("expected config store source %q, got %q", ConfigSourceOfTruthFile, cfg.ConfigStore.SourceOfTruth)
	}
	if cfg.ConfigStore.Backend != "" {
		t.Fatalf("expected empty config store backend by default, got %q", cfg.ConfigStore.Backend)
	}
	if cfg.ConfigStore.PollInterval <= 0 || cfg.ConfigStore.ActivationTimeout <= 0 {
		t.Fatalf("expected positive config store timing defaults: %#v", cfg.ConfigStore)
	}

	if !matchesOpenAPIPrefix("/users/{id}", "/users") {
		t.Fatalf("expected prefix match")
	}
	if matchesOpenAPIPrefix("/admin/users", "/users") {
		t.Fatalf("unexpected prefix match")
	}

	if got := joinRoutePath("/", "/users"); got != "/users" {
		t.Fatalf("unexpected joinRoutePath: %s", got)
	}
	if got := normalizeOpenAPIPath("users/:id/"); got != "/users/{id}" {
		t.Fatalf("unexpected normalizeOpenAPIPath: %s", got)
	}

	base := Group{
		Prefix: "/api",
		AuthEndpoints: &AuthEndpoints{
			OAuth2: &OAuth2Config{},
		},
	}
	overlay := Group{
		AuthEndpoints: &AuthEndpoints{
			OAuth2: &OAuth2Config{ClientSecret: "x"},
		},
	}
	merged, ok := mergeOverlayGroup(base, overlay)
	if !ok || merged.AuthEndpoints.OAuth2.ClientSecret != "x" {
		t.Fatalf("expected overlay merge on oauth2 secret")
	}
}
