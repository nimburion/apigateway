package openapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nimburion/apigateway/internal/approutes"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/cli"
	httpopenapi "github.com/nimburion/nimburion/pkg/http/openapi"
	"github.com/nimburion/nimburion/pkg/http/router"
	"github.com/spf13/cobra"
)

func TestGenerateCommandWritesOpenAPIFileFromRegisteredRoutes(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	outputPath := filepath.Join(tmp, "openapi", "openapi.yml")
	cfg := `
app:
  name: test-gateway
routes:
  groups:
    default:
      prefix: /
      auth_endpoints:
        me: true
      routes:
        - path_prefix: /users
          target_url: http://users.example.com
          endpoints:
            - path: /:id
              methods:
                GET: {}
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	gwCfg := gatewaycfg.NewDefaultConfig()
	opts := &cli.AppCommandOptions{
		Name:       "api-gateway",
		ConfigPath: cfgPath,
		EnvPrefix:  "APP",
		ConfigExtensions: []any{
			gwCfg,
		},
	}

	cmd := NewCommand(opts, gwCfg)
	cmd.SetArgs([]string{"generate", "--output", outputPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute openapi generate: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "openapi: 3.0.3") {
		t.Fatalf("expected openapi header in output")
	}
	if !strings.Contains(content, "/users/{id}:") {
		t.Fatalf("expected registered HTTP route in output: %s", content)
	}
	if !strings.Contains(content, "/auth/me:") {
		t.Fatalf("expected registered auth endpoint in output: %s", content)
	}
}

func TestGenerateCommandRejectsUnsupportedHTTPMethod(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	outputPath := filepath.Join(tmp, "openapi.yml")
	cfg := `
app:
  name: test-gateway
routes:
  groups:
    default:
      prefix: /
      routes:
        - path_prefix: /coffee
          target_url: http://brew.example.com
          endpoints:
            - path: /
              methods:
                BREW: {}
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	gwCfg := gatewaycfg.NewDefaultConfig()
	opts := &cli.AppCommandOptions{
		Name:       "api-gateway",
		ConfigPath: cfgPath,
		EnvPrefix:  "APP",
		ConfigExtensions: []any{
			gwCfg,
		},
	}

	cmd := NewCommand(opts, gwCfg)
	cmd.SetArgs([]string{"generate", "--output", outputPath})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected unsupported method error")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveConfigPathAndFlagValueAndMiddlewareRegistry(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if got := resolveConfigPath(cfgPath); got != cfgPath {
		t.Fatalf("expected existing path to be returned")
	}
	if got := resolveConfigPath(filepath.Join(tmp, "missing.yml")); got != "" {
		t.Fatalf("expected empty for missing path")
	}

	root := &cobraCommandStub{}
	cmd := root.command()
	if got := flagValue(cmd, "config-file", "fallback.yml"); got != "from-root.yml" {
		t.Fatalf("expected root flag value, got %q", got)
	}

	reg := routesMiddlewareRegistry()
	if len(reg) != 3 {
		t.Fatalf("expected 3 middleware factories, got %d", len(reg))
	}
	for name, factory := range reg {
		called := false
		err := factory()(func(c router.Context) error {
			called = true
			return nil
		})(nil)
		if err != nil {
			t.Fatalf("middleware %s returned error: %v", name, err)
		}
		if !called {
			t.Fatalf("middleware %s did not call next", name)
		}
	}
}

func TestFrameworkOpenAPICollectsWebSocketRoute(t *testing.T) {
	routes := httpopenapi.CollectRoutes(func(r router.Router) {
		err := approutes.Register(r, gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix: "/",
					WebSockets: []gatewaycfg.WebSocket{
						{Path: "/events", TargetURL: "wss://events.example.com"},
					},
				},
			},
		}, routesMiddlewareRegistry(), nil)
		if err != nil {
			t.Fatalf("register routes: %v", err)
		}
	})

	spec := httpopenapi.BuildSpec("Gateway", "1.0.0", routes)
	if spec.Paths["/events"] == nil || spec.Paths["/events"].Get == nil {
		t.Fatalf("expected websocket route to be collected into spec")
	}
}

func TestFrameworkOpenAPIAppliesAnnotationsToInternalAuthEndpoints(t *testing.T) {
	routes := httpopenapi.CollectRoutes(func(r router.Router) {
		err := approutes.Register(r, gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix: "/",
					AuthEndpoints: &gatewaycfg.AuthEndpoints{
						Me: true,
						OAuth2: &gatewaycfg.OAuth2Config{
							Enabled: true,
						},
					},
				},
			},
		}, routesMiddlewareRegistry(), nil)
		if err != nil {
			t.Fatalf("register routes: %v", err)
		}
	})

	spec := httpopenapi.BuildSpec("Gateway", "1.0.0", routes)
	if spec.Paths["/auth/me"] == nil || spec.Paths["/auth/me"].Get == nil {
		t.Fatalf("expected /auth/me route in spec")
	}
	if spec.Paths["/auth/me"].Get.Summary != "Authenticated user info" {
		t.Fatalf("expected annotated summary for /auth/me, got %q", spec.Paths["/auth/me"].Get.Summary)
	}
	if len(spec.Paths["/auth/me"].Get.Tags) == 0 || spec.Paths["/auth/me"].Get.Tags[0] != "auth" {
		t.Fatalf("expected auth tag for /auth/me, got %#v", spec.Paths["/auth/me"].Get.Tags)
	}
	if spec.Paths["/auth/login"] == nil || spec.Paths["/auth/login"].Get == nil {
		t.Fatalf("expected /auth/login route in spec")
	}
	if spec.Paths["/auth/login"].Get.Summary != "OAuth2 login" {
		t.Fatalf("expected annotated summary for /auth/login, got %q", spec.Paths["/auth/login"].Get.Summary)
	}
	if !contains(spec.Paths["/auth/login"].Get.Tags, "oauth2") {
		t.Fatalf("expected oauth2 tag for /auth/login, got %#v", spec.Paths["/auth/login"].Get.Tags)
	}
	if spec.Paths["/auth/callback"] == nil || spec.Paths["/auth/callback"].Get == nil {
		t.Fatalf("expected /auth/callback route in spec")
	}
	if spec.Paths["/auth/logout"] == nil || spec.Paths["/auth/logout"].Post == nil {
		t.Fatalf("expected /auth/logout route in spec")
	}
	if spec.Paths["/auth/refresh"] == nil || spec.Paths["/auth/refresh"].Post == nil {
		t.Fatalf("expected /auth/refresh route in spec")
	}
}

func TestBuildOpenAPISpecExcludesSyntheticMethodNotAllowedRoutes(t *testing.T) {
	spec, err := approutes.BuildOpenAPISpec(nil, gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix: "/",
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/healthz",
						TargetURL:  "http://status.example.com",
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
		},
	}, routesMiddlewareRegistry())
	if err != nil {
		t.Fatalf("build openapi spec: %v", err)
	}

	pathItem := spec.Paths["/healthz"]
	if pathItem == nil || pathItem.Get == nil {
		t.Fatalf("expected GET /healthz in spec")
	}
	if pathItem.Post != nil || pathItem.Put != nil || pathItem.Patch != nil || pathItem.Delete != nil {
		t.Fatalf("expected synthetic 405 methods to be excluded from spec: %#v", pathItem)
	}
}

func TestFrameworkOpenAPILosesAnnotationsWhenAnnotatedHandlerIsWrapped(t *testing.T) {
	annotated := httpopenapi.Annotate(func(c router.Context) error { return nil }, httpopenapi.EndpointAnnotations{
		Summary:     "Wrapped summary",
		OperationID: "wrappedOperation",
	})

	routes := httpopenapi.CollectRoutes(func(r router.Router) {
		r.GET("/wrapped", func(c router.Context) error {
			return annotated(c)
		})
	})

	spec := httpopenapi.BuildSpec("Gateway", "1.0.0", routes)
	op := spec.Paths["/wrapped"].Get
	if op == nil {
		t.Fatalf("expected /wrapped route in spec")
	}
	if op.Summary == "Wrapped summary" {
		t.Fatalf("expected annotation to be lost after wrapping handler")
	}
	if op.OperationID == "wrappedOperation" {
		t.Fatalf("expected operation id annotation to be lost after wrapping handler")
	}
}

type cobraCommandStub struct{}

func (cobraCommandStub) command() *cobra.Command {
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("config-file", "", "")
	_ = root.PersistentFlags().Set("config-file", "from-root.yml")
	cmd := &cobra.Command{Use: "openapi"}
	root.AddCommand(cmd)
	return cmd
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
