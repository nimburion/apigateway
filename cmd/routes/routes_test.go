package routes

import (
	"os"
	"path/filepath"
	"testing"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/cli"
	basecfg "github.com/nimburion/nimburion/pkg/config"
	logpkg "github.com/nimburion/nimburion/pkg/observability/logger"
	"github.com/nimburion/nimburion/pkg/server/router"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	opts := &cli.ServiceCommandOptions{Name: "api-gateway"}
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
		flags *pflag.FlagSet,
		extensions []any,
		defaultServiceName, serviceNameOverride string,
	) (*basecfg.Config, logpkg.Logger, error) {
		return &basecfg.Config{}, nil, nil
	}

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("service:\n  name: test\n"), 0o644); err != nil {
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
	opts := &cli.ServiceCommandOptions{
		Name:       "api-gateway",
		ConfigPath: cfgPath,
	}
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("config-file", "", "")
	root.PersistentFlags().String("secret-file", "", "")
	root.PersistentFlags().String("service-name", "", "")

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
		flags *pflag.FlagSet,
		extensions []any,
		defaultServiceName, serviceNameOverride string,
	) (*basecfg.Config, logpkg.Logger, error) {
		return &basecfg.Config{}, nil, nil
	}

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("service:\n  name: test\n"), 0o644); err != nil {
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
	opts := &cli.ServiceCommandOptions{Name: "api-gateway", ConfigPath: cfgPath}
	cmd := NewCommand(opts, gwCfg)
	cmd.PersistentFlags().String("config-file", "", "")
	cmd.PersistentFlags().String("secret-file", "", "")
	cmd.PersistentFlags().String("service-name", "", "")

	cmd.SetArgs([]string{"validate"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("validate command failed: %v", err)
	}

	cmd.SetArgs([]string{"show"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("show command failed: %v", err)
	}
}

func TestRoutesMiddlewareRegistryCallsNext(t *testing.T) {
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
}
