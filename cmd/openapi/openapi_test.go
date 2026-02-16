package openapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/cli"
	nimbcfg "github.com/nimburion/nimburion/pkg/config"
	"github.com/nimburion/nimburion/pkg/server/router"
	"github.com/spf13/cobra"
)

func TestNormalizeOpenAPIPath(t *testing.T) {
	cases := map[string]string{
		"users/:id/":   "/users/{id}",
		"/files/*path": "/files/{path}",
		"/":            "/",
		"":             "",
	}

	for input, expected := range cases {
		if got := normalizeOpenAPIPath(input); got != expected {
			t.Fatalf("normalizeOpenAPIPath(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestPathParameters(t *testing.T) {
	path := "/users/{id}/files/{file_id}"
	params := pathParameters(path)
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(params))
	}
	if params[0] != "id" || params[1] != "file_id" {
		t.Fatalf("unexpected params: %#v", params)
	}
}

func TestJoinPaths(t *testing.T) {
	if got := joinPaths("/api", "users", "/"); got != "/api/users" {
		t.Fatalf("unexpected joinPaths: %s", got)
	}
	if got := joinPaths(); got != "/" {
		t.Fatalf("unexpected joinPaths empty: %s", got)
	}
}

func TestBuildSpecIncludesRoutes(t *testing.T) {
	cfg := &nimbcfg.Config{}
	cfg.Service.Name = "Gateway"
	cfg.HTTP.Port = 8080
	cfg.Management.Enabled = true
	cfg.Management.Port = 9001

	gwCfg := &gatewaycfg.Gateway{
		Routes: gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix: "/api",
				AuthEndpoints: &gatewaycfg.AuthEndpoints{
					Me:     true,
					OAuth2: &gatewaycfg.OAuth2Config{Enabled: true},
				},
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/users",
						TargetURL:  "http://example.com",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/:id", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
				},
				WebSockets: []gatewaycfg.WebSocket{{Path: "/ws", TargetURL: "ws://example.com"}},
			},
		}},
	}

	spec := buildSpec(cfg, gwCfg)
	if spec.Info == nil || spec.Info.Title != "Gateway" {
		t.Fatalf("expected service name in Info.Title")
	}

	servers := buildServers(cfg)
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}

	pathItem := spec.Paths.Value("/api/users/{id}")
	if pathItem == nil || pathItem.Get == nil {
		t.Fatalf("expected GET operation for /api/users/{id}")
	}

	if pathItem.Get.Responses == nil {
		t.Fatalf("expected responses to be initialized")
	}

	if wsItem := spec.Paths.Value("/api/ws"); wsItem == nil || wsItem.Get == nil {
		t.Fatalf("expected websocket GET operation")
	}

	if spec.Paths.Value("/portal") == nil {
		t.Fatalf("expected management routes to be included")
	}
}

func TestSortPaths(t *testing.T) {
	paths := openapi3.NewPaths()
	paths.Set("/b", &openapi3.PathItem{})
	paths.Set("/a", &openapi3.PathItem{})

	sortPaths(paths)
	keys := make([]string, 0, len(paths.Map()))
	for key := range paths.Map() {
		keys = append(keys, key)
	}
	if keys[0] != "/a" || keys[1] != "/b" {
		t.Fatalf("paths not sorted: %#v", keys)
	}
}

func TestGenerateCommandWritesOpenAPIFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	outputPath := filepath.Join(tmp, "openapi", "openapi.yml")
	cfg := `
service:
  name: test-gateway
routes:
  groups:
    default:
      prefix: /
      routes:
        - path_prefix: /users
          target_url: http://users.example.com
          endpoints:
            - path: /
              methods:
                GET: {}
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	gwCfg := gatewaycfg.NewDefaultConfig()
	opts := &cli.ServiceCommandOptions{
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
	if !strings.Contains(content, "/users") {
		t.Fatalf("expected generated route in output")
	}
}

func TestDetermineRoutesBaseDirAndResolveConfigPath(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if got := determineRoutesBaseDir(cfgPath); got != tmp {
		t.Fatalf("expected %q, got %q", tmp, got)
	}
	if got := determineRoutesBaseDir(""); got != "" {
		t.Fatalf("expected empty base dir for empty path")
	}
	if got := resolveConfigPath(cfgPath); got != cfgPath {
		t.Fatalf("expected existing path to be returned")
	}
	if got := resolveConfigPath(filepath.Join(tmp, "missing.yml")); got != "" {
		t.Fatalf("expected empty for missing path")
	}
}

func TestFlagValueAndRoutesMiddlewareRegistryAndServiceName(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("config-file", "", "")
	_ = root.PersistentFlags().Set("config-file", "from-root.yml")
	cmd := &cobra.Command{Use: "openapi"}
	root.AddCommand(cmd)

	if got := flagValue(cmd, "config-file", "fallback.yml"); got != "from-root.yml" {
		t.Fatalf("expected root flag value, got %q", got)
	}

	reg := routesMiddlewareRegistry()
	if len(reg) != 2 {
		t.Fatalf("expected 2 middleware factories, got %d", len(reg))
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

	if got := serviceName(&nimbcfg.Config{}); got != "api-gateway" {
		t.Fatalf("expected fallback service name, got %q", got)
	}
	cfg := &nimbcfg.Config{}
	cfg.Service.Name = "gateway-x"
	if got := serviceName(cfg); got != "gateway-x" {
		t.Fatalf("expected configured service name, got %q", got)
	}
}
