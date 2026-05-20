package routes

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/cli"
	basecfg "github.com/nimburion/nimburion/pkg/config"
	"github.com/nimburion/nimburion/pkg/http/router"
	logpkg "github.com/nimburion/nimburion/pkg/observability/logger"
	"github.com/spf13/cobra"
)

func TestDetermineRoutesBaseDir(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	if got := DetermineRoutesBaseDir(cfgPath, ""); got != tmpDir {
		t.Fatalf("expected %s, got %s", tmpDir, got)
	}
	if got := DetermineRoutesBaseDir(tmpDir, ""); got == "" {
		t.Fatalf("expected fallback for directory path")
	}
}

func TestRedactedRoutes(t *testing.T) {
	routes := gatewaycfg.Routing{Groups: map[string]gatewaycfg.Group{
		"default": {
			AuthEndpoints: &gatewaycfg.AuthEndpoints{
				OAuth2: &gatewaycfg.OAuth2Config{ClientSecret: "secret"},
			},
		},
	}}

	redacted := redactedRoutes(routes)
	secret := redacted.Groups["default"].AuthEndpoints.OAuth2.ClientSecret
	if secret != "***" {
		t.Fatalf("expected redacted secret, got %q", secret)
	}
}

func TestFlagValue(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("config-file", "", "")
	if got := flagValue(cmd, "config-file", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %s", got)
	}
	_ = cmd.Flags().Set("config-file", "config.yaml")
	if got := flagValue(cmd, "config-file", "fallback"); got != "config.yaml" {
		t.Fatalf("expected flag value, got %s", got)
	}
}

func TestNewCommandContainsSubcommands(t *testing.T) {
	opts := &cli.AppCommandOptions{Name: "api-gateway"}
	gwCfg := gatewaycfg.NewDefaultConfig()
	cmd := NewCommand(opts, gwCfg)
	if cmd == nil {
		t.Fatalf("expected non-nil command")
	}
	if len(cmd.Commands()) == 0 || cmd.Commands()[0].Name() == "" {
		t.Fatalf("expected subcommands to be registered")
	}
}

func TestPrepareRoutesLoadsAndSetsConfigDir(t *testing.T) {
	originalLoader := loadConfigAndLogger
	t.Cleanup(func() {
		loadConfigAndLogger = originalLoader
	})
	loadConfigAndLogger = func(
		cfgPath, envPrefix, secretFilePath string,
		customValidator func(*basecfg.Config) error,
		cmd *cobra.Command,
		extensions []any,
		defaultServiceName, serviceNameOverride string,
	) (*basecfg.Config, logpkg.Logger, error) {
		return &basecfg.Config{}, nil, nil
	}

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("app:\n  name: test\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

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
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
					},
				},
			},
		},
	}
	opts := &cli.AppCommandOptions{
		Name:       "api-gateway",
		ConfigPath: cfgPath,
	}
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("config-file", "", "")
	root.PersistentFlags().String("secret-file", "", "")
	root.PersistentFlags().String("app-name", "", "")

	cmd := &cobra.Command{Use: "routes"}
	root.AddCommand(cmd)
	if err := prepareRoutes(cmd, opts, gwCfg); err != nil {
		t.Fatalf("prepareRoutes error: %v", err)
	}
	if gwCfg.ConfigDir != tmp {
		t.Fatalf("expected ConfigDir %q, got %q", tmp, gwCfg.ConfigDir)
	}
}

func TestRoutesCommandValidateAndShow(t *testing.T) {
	originalLoader := loadConfigAndLogger
	t.Cleanup(func() {
		loadConfigAndLogger = originalLoader
	})
	loadConfigAndLogger = func(
		cfgPath, envPrefix, secretFilePath string,
		customValidator func(*basecfg.Config) error,
		cmd *cobra.Command,
		extensions []any,
		defaultServiceName, serviceNameOverride string,
	) (*basecfg.Config, logpkg.Logger, error) {
		return &basecfg.Config{}, nil, nil
	}

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("app:\n  name: test\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
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
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
					},
				},
			},
		},
	}
	opts := &cli.AppCommandOptions{Name: "api-gateway", ConfigPath: cfgPath}
	cmd := NewCommand(opts, gwCfg)
	cmd.PersistentFlags().String("config-file", "", "")
	cmd.PersistentFlags().String("secret-file", "", "")
	cmd.PersistentFlags().String("app-name", "", "")

	cmd.SetArgs([]string{"validate"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("validate command failed: %v", err)
	}

	cmd.SetArgs([]string{"show"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("show command failed: %v", err)
	}
}

func TestBuildReviewReportClassifiesPublicAndProtectedRoutes(t *testing.T) {
	cfg := &basecfg.Config{}
	cfg.App.Name = "api-gateway"
	cfg.Auth.Enabled = true
	cfg.Management.Enabled = true
	cfg.Management.AuthEnabled = true

	routes := gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"mixed": {
				Prefix:      "/api",
				Middlewares: []string{"Authenticate"},
				Routes: []gatewaycfg.Route{
					{
						PathPrefix:         "/public",
						DisableMiddlewares: []string{"Authenticate"},
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
					{
						PathPrefix: "/protected",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {Scopes: []string{"users:read"}}}},
						},
					},
				},
				WebSockets: []gatewaycfg.WebSocket{
					{Path: "/public-ws", DisableMiddlewares: []string{"Authenticate"}},
					{Path: "/protected-ws", Middlewares: []string{"Authenticate"}},
				},
			},
		},
	}

	report := buildReviewReport(cfg, routes, "")
	if len(report.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(report.Groups))
	}

	group := report.Groups[0]
	if !group.MeRequiresAuth {
		t.Fatalf("expected group auth requirement to be true")
	}
	if len(group.PublicRoutes) != 1 || group.PublicRoutes[0].Path != "/api/public" {
		t.Fatalf("unexpected public routes: %+v", group.PublicRoutes)
	}
	if len(group.ProtectedRoutes) != 1 || group.ProtectedRoutes[0].Path != "/api/protected" {
		t.Fatalf("unexpected protected routes: %+v", group.ProtectedRoutes)
	}
	if len(group.PublicWebSocket) != 1 || group.PublicWebSocket[0] != "/api/public-ws" {
		t.Fatalf("unexpected public websockets: %+v", group.PublicWebSocket)
	}
	if len(group.ProtectedWebSocket) != 1 || group.ProtectedWebSocket[0] != "/api/protected-ws" {
		t.Fatalf("unexpected protected websockets: %+v", group.ProtectedWebSocket)
	}
}

func TestBuildReviewReportDoesNotTreatForwardIdentityHeadersAsAuthRequirement(t *testing.T) {
	cfg := &basecfg.Config{}
	cfg.App.Name = "api-gateway"

	routes := gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"b-group": {
				Prefix:      "/b",
				Middlewares: []string{"ForwardIdentityHeaders"},
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/public",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
				},
			},
			"a-group": {
				Prefix: "/a",
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/public",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
				},
			},
		},
	}

	report := buildReviewReport(cfg, routes, "")
	if len(report.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(report.Groups))
	}
	if report.Groups[0].Name != "a-group" || report.Groups[1].Name != "b-group" {
		t.Fatalf("expected deterministic group ordering, got %#v", []string{report.Groups[0].Name, report.Groups[1].Name})
	}
	for _, group := range report.Groups {
		if group.MeRequiresAuth {
			t.Fatalf("expected group %q to remain public, got auth_required=true", group.Name)
		}
		if len(group.ProtectedRoutes) != 0 {
			t.Fatalf("expected group %q to have no protected routes, got %+v", group.Name, group.ProtectedRoutes)
		}
		if len(group.PublicRoutes) != 1 {
			t.Fatalf("expected group %q to have one public route, got %+v", group.Name, group.PublicRoutes)
		}
	}
}

func TestHasLegacyServiceKey(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("service:\n  name: legacy\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if !hasLegacyServiceKey(cfgPath) {
		t.Fatalf("expected legacy service key detection")
	}
	if hasLegacyServiceKey(filepath.Join(tmpDir, "missing.yaml")) {
		t.Fatalf("expected missing file to return false")
	}
}

func TestHasLegacyServiceKeyAcrossYAMLDocuments(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	content := strings.Join([]string{
		"app:",
		"  name: api-gateway",
		"---",
		"service:",
		"  name: legacy",
	}, "\n")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if !hasLegacyServiceKey(cfgPath) {
		t.Fatalf("expected legacy service key detection across YAML documents")
	}
}

func TestJoinReportPath(t *testing.T) {
	if got := joinReportPath("/", "/api/", "/users", "/"); got != "/api/users" {
		t.Fatalf("unexpected joined path: %s", got)
	}
	if got := joinReportPath("", "/"); got != "/" {
		t.Fatalf("unexpected root path: %s", got)
	}
}

func TestRoutesCommandReviewAndFailOnWarning(t *testing.T) {
	originalLoader := loadConfigAndLogger
	t.Cleanup(func() {
		loadConfigAndLogger = originalLoader
	})
	loadConfigAndLogger = func(
		cfgPath, envPrefix, secretFilePath string,
		customValidator func(*basecfg.Config) error,
		cmd *cobra.Command,
		extensions []any,
		defaultServiceName, serviceNameOverride string,
	) (*basecfg.Config, logpkg.Logger, error) {
		cfg := &basecfg.Config{}
		cfg.App.Name = "api-gateway"
		cfg.Auth.Enabled = false
		return cfg, nil, nil
	}

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("service:\n  name: legacy\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

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
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
					},
				},
			},
		},
	}
	opts := &cli.AppCommandOptions{Name: "api-gateway", ConfigPath: cfgPath}

	cmd := NewCommand(opts, gwCfg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.PersistentFlags().String("config-file", "", "")
	cmd.PersistentFlags().String("secret-file", "", "")
	cmd.PersistentFlags().String("app-name", "", "")

	stdout := captureStdout(t, func() {
		cmd.SetArgs([]string{"review"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("review command failed: %v", err)
		}
	})
	if !strings.Contains(stdout, "legacy_service_key: true") {
		t.Fatalf("expected review output to include legacy service warning, got %q", stdout)
	}
	for _, want := range []string{
		"app_name: api-gateway",
		"warnings:",
		"public_routes:",
		"path: /users",
		"method: GET",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected review output to contain %q, got %q", want, stdout)
		}
	}

	cmd = NewCommand(opts, gwCfg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.PersistentFlags().String("config-file", "", "")
	cmd.PersistentFlags().String("secret-file", "", "")
	cmd.PersistentFlags().String("app-name", "", "")
	cmd.SetArgs([]string{"review", "--fail-on-warning"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected fail-on-warning to return error")
	}
}

func TestBuildComparisonReportDetectsDrift(t *testing.T) {
	left := reviewReport{
		AppName:               "gateway-staging",
		AuthEnabled:           true,
		ManagementEnabled:     true,
		ManagementAuthEnabled: false,
		Groups: []reviewGroupReport{
			{
				Name:           "default",
				Prefix:         "/",
				Middlewares:    []string{"Authenticate"},
				MeRequiresAuth: true,
				PublicRoutes: []reviewRouteReport{
					{Path: "/public", Method: "GET"},
				},
				ProtectedRoutes: []reviewRouteReport{
					{Path: "/private", Method: "GET", EffectiveMiddlewares: []string{"Authenticate"}, Scopes: []string{"users:read"}},
				},
			},
		},
	}
	right := reviewReport{
		AppName:               "gateway-prod",
		AuthEnabled:           false,
		ManagementEnabled:     true,
		ManagementAuthEnabled: true,
		LegacyServiceKey:      true,
		Warnings:              []string{"top-level legacy 'service' key detected in config file"},
		Groups: []reviewGroupReport{
			{
				Name:           "default",
				Prefix:         "/",
				Middlewares:    nil,
				MeRequiresAuth: false,
				PublicRoutes: []reviewRouteReport{
					{Path: "/private", Method: "GET"},
				},
			},
		},
	}

	report := buildComparisonReport("staging", "production", left, right)
	if len(report.Drifts) == 0 {
		t.Fatalf("expected drift items to be reported")
	}
	got := strings.Join(report.Drifts, "\n")
	for _, want := range []string{
		"auth.enabled differs",
		"management.auth_enabled differs",
		"legacy service config usage differs",
		`route "GET /private" in group "default" changes exposure`,
		`route "GET /public" in group "default" exists only in staging`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected drift output to contain %q, got %q", want, got)
		}
	}
}

func TestRoutesCommandCompareAndFailOnDrift(t *testing.T) {
	originalLoader := loadConfigAndLogger
	t.Cleanup(func() {
		loadConfigAndLogger = originalLoader
	})
	loadConfigAndLogger = func(
		cfgPath, envPrefix, secretFilePath string,
		customValidator func(*basecfg.Config) error,
		cmd *cobra.Command,
		extensions []any,
		defaultServiceName, serviceNameOverride string,
	) (*basecfg.Config, logpkg.Logger, error) {
		cfg := &basecfg.Config{}
		var gwExt *gatewaycfg.Gateway
		for _, ext := range extensions {
			if candidate, ok := ext.(*gatewaycfg.Gateway); ok {
				gwExt = candidate
				break
			}
		}
		switch filepath.Base(cfgPath) {
		case "staging.yaml":
			cfg.App.Name = "gateway-staging"
			cfg.Auth.Enabled = true
			if gwExt != nil {
				gwExt.Routes = gatewaycfg.Routing{
					Groups: map[string]gatewaycfg.Group{
						"default": {
							Prefix: "/",
							Routes: []gatewaycfg.Route{
								{
									PathPrefix: "/private",
									TargetURL:  "http://example.com",
									Endpoints: []gatewaycfg.Endpoint{
										{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {Scopes: []string{"users:read"}}}},
									},
								},
							},
						},
					},
				}
			}
		case "production.yaml":
			cfg.App.Name = "gateway-prod"
			cfg.Auth.Enabled = false
			cfg.Management.AuthEnabled = true
			if gwExt != nil {
				gwExt.Routes = gatewaycfg.Routing{
					Groups: map[string]gatewaycfg.Group{
						"default": {
							Prefix: "/",
							Routes: []gatewaycfg.Route{
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
				}
			}
		default:
			t.Fatalf("unexpected cfg path: %s", cfgPath)
		}
		return cfg, nil, nil
	}

	tmp := t.TempDir()
	stagingPath := filepath.Join(tmp, "staging.yaml")
	productionPath := filepath.Join(tmp, "production.yaml")
	if err := os.WriteFile(stagingPath, []byte("app:\n  name: staging\n"), 0o644); err != nil {
		t.Fatalf("write staging config: %v", err)
	}
	if err := os.WriteFile(productionPath, []byte("service:\n  name: legacy\n"), 0o644); err != nil {
		t.Fatalf("write production config: %v", err)
	}

	gwCfg := gatewaycfg.NewDefaultConfig()
	opts := &cli.AppCommandOptions{Name: "api-gateway", ConfigPath: stagingPath, ConfigExtensions: []any{gwCfg}}

	cmd := NewCommand(opts, gwCfg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.PersistentFlags().String("config-file", "", "")
	cmd.PersistentFlags().String("secret-file", "", "")
	cmd.PersistentFlags().String("app-name", "", "")

	stdout := captureStdout(t, func() {
		cmd.SetArgs([]string{"compare", "--other-config-file", productionPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("compare command failed: %v", err)
		}
	})
	if !strings.Contains(stdout, "drifts:") {
		t.Fatalf("expected compare output to include drifts, got %q", stdout)
	}
	for _, want := range []string{
		"left_label: staging",
		"right_label: production",
		"auth.enabled differs",
		"legacy service config usage differs",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected compare output to contain %q, got %q", want, stdout)
		}
	}

	cmd = NewCommand(opts, gwCfg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.PersistentFlags().String("config-file", "", "")
	cmd.PersistentFlags().String("secret-file", "", "")
	cmd.PersistentFlags().String("app-name", "", "")
	cmd.SetArgs([]string{"compare", "--other-config-file", productionPath, "--fail-on-drift"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected fail-on-drift to return error")
	}
}

func TestRoutesMiddlewareRegistryCallsNext(t *testing.T) {
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

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = original
	})

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	os.Stdout = original
	return buf.String()
}
