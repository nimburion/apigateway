package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/cli"
	"github.com/nimburion/nimburion/pkg/http/router"
	"github.com/spf13/cobra"
)

func TestNewCommandContainsGenerate(t *testing.T) {
	cmd := NewGenerateCommand()
	if cmd == nil {
		t.Fatalf("expected command")
	}
	if cmd.Name() != "generate" {
		t.Fatalf("expected generate command")
	}
}

func TestConfigGenerateWritesMinimalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "minimal.yaml")

	cmd := NewGenerateCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--output", outputPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("generate command failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"app:",
		"name: api-gateway",
		"auth:",
		"enabled: false",
		"management:",
		"routes:",
		"path_prefix: /healthz",
		"target_url: http://localhost:8081",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected generated config to contain %q, got %q", want, content)
		}
	}
}

func TestGeneratedConfigLoadsWithGatewayExtension(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "minimal.yaml")

	cmd := NewGenerateCommand()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--output", outputPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("generate command failed: %v", err)
	}

	gwCfg := gatewaycfg.NewDefaultConfig()
	_, _, err := cli.LoadConfigAndLogger(
		outputPath,
		"APP",
		"",
		nil,
		nil,
		[]any{gwCfg},
		"api-gateway",
		"",
	)
	if err != nil {
		t.Fatalf("expected generated config to load, got %v", err)
	}
	if err := gwCfg.LoadRoutes(tmpDir, map[string]func() router.MiddlewareFunc{
		"Authenticate": func() router.MiddlewareFunc {
			return func(next router.HandlerFunc) router.HandlerFunc { return next }
		},
		"ClaimsGuardFromConfig": func() router.MiddlewareFunc {
			return func(next router.HandlerFunc) router.HandlerFunc { return next }
		},
		"ForwardIdentityHeaders": func() router.MiddlewareFunc {
			return func(next router.HandlerFunc) router.HandlerFunc { return next }
		},
	}); err != nil {
		t.Fatalf("expected generated routes to load, got %v", err)
	}
}

func TestConfigShowOmitsUnusedSectionsByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "minimal.yaml")

	generateCmd := NewGenerateCommand()
	generateCmd.SetOut(new(bytes.Buffer))
	generateCmd.SetErr(new(bytes.Buffer))
	generateCmd.SetArgs([]string{"--output", outputPath})
	if err := generateCmd.Execute(); err != nil {
		t.Fatalf("generate command failed: %v", err)
	}

	gwCfg := gatewaycfg.NewDefaultConfig()
	opts := cli.AppCommandOptions{
		Name:             "api-gateway",
		ConfigPath:       outputPath,
		ConfigExtensions: []any{gwCfg},
	}
	root := &cobra.Command{Use: "api-gateway"}
	root.PersistentFlags().String("config-file", outputPath, "")
	root.PersistentFlags().String("secret-file", "", "")
	root.PersistentFlags().String("app-name", "", "")
	configRoot := &cobra.Command{Use: "config"}
	configRoot.AddCommand(&cobra.Command{Use: "show"})
	root.AddCommand(configRoot)
	AttachToRoot(root, &opts)

	stdout := captureStdout(t, func() {
		root.SetArgs([]string{"config", "show"})
		if err := root.Execute(); err != nil {
			t.Fatalf("config show failed: %v", err)
		}
	})
	for _, want := range []string{"app:", "http:", "management:", "routes:"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected config show output to contain %q, got %q", want, stdout)
		}
	}
	for _, unwanted := range []string{"email:", "cache:", "scheduler:", "jobs:"} {
		if strings.Contains(stdout, unwanted) {
			t.Fatalf("expected config show output to omit %q, got %q", unwanted, stdout)
		}
	}
}

func TestConfigShowUsesConfiguredEnvPrefixForSecretsFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	secretsPath := filepath.Join(tmpDir, "custom-secrets.yaml")

	if err := os.WriteFile(configPath, []byte("app:\n  name: api-gateway\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(secretsPath, []byte("auth:\n  enabled: true\n"), 0o644); err != nil {
		t.Fatalf("write secrets: %v", err)
	}

	const envPrefix = "GATEWAY"
	envVar := envPrefix + "_SECRETS_FILE"
	t.Setenv(envVar, secretsPath)

	explicit, err := loadExplicitSettings(configPath, envPrefix)
	if err != nil {
		t.Fatalf("load explicit settings: %v", err)
	}

	authSettings, ok := explicit["auth"].(map[string]any)
	if !ok {
		t.Fatalf("expected auth settings from prefixed secrets file, got %#v", explicit)
	}
	if got, ok := authSettings["enabled"].(bool); !ok || !got {
		t.Fatalf("expected auth.enabled=true from prefixed secrets file, got %#v", authSettings["enabled"])
	}
}

func TestConfigShowAllIncludesFrameworkDefaultSections(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "minimal.yaml")

	generateCmd := NewGenerateCommand()
	generateCmd.SetOut(new(bytes.Buffer))
	generateCmd.SetErr(new(bytes.Buffer))
	generateCmd.SetArgs([]string{"--output", outputPath})
	if err := generateCmd.Execute(); err != nil {
		t.Fatalf("generate command failed: %v", err)
	}

	root := newConfigShowRoot(t, outputPath)

	stdout := captureStdout(t, func() {
		root.SetArgs([]string{"config", "show", "--all"})
		if err := root.Execute(); err != nil {
			t.Fatalf("config show --all failed: %v", err)
		}
	})

	for _, want := range []string{"app:", "http:", "management:", "routes:", "email:", "cache:"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected config show --all output to contain %q, got %q", want, stdout)
		}
	}
}

func TestConfigShowRedactsSecretValuesByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	secretsPath := filepath.Join(tmpDir, "secrets.yaml")

	if err := os.WriteFile(configPath, []byte("app:\n  name: api-gateway\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	secretValue := "https://issuer.example.com/.well-known/jwks.json"
	if err := os.WriteFile(secretsPath, []byte("auth:\n  jwks_url: "+secretValue+"\n"), 0o644); err != nil {
		t.Fatalf("write secrets: %v", err)
	}

	root := newConfigShowRoot(t, configPath)

	stdout := captureStdout(t, func() {
		root.SetArgs([]string{"config", "show", "--all", "--secret-file", secretsPath})
		if err := root.Execute(); err != nil {
			t.Fatalf("config show with secret file failed: %v", err)
		}
	})

	if strings.Contains(stdout, secretValue) {
		t.Fatalf("expected config show output to redact secret value, got %q", stdout)
	}
	if !strings.Contains(stdout, "jwks_url: '***'") && !strings.Contains(stdout, "jwks_url: \"***\"") && !strings.Contains(stdout, "jwks_url: '***'\n") && !strings.Contains(stdout, "jwks_url: ***") {
		t.Fatalf("expected config show output to contain redacted secret marker, got %q", stdout)
	}
}

func TestConfigShowShowSecretsPreservesSecretValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	secretsPath := filepath.Join(tmpDir, "secrets.yaml")

	if err := os.WriteFile(configPath, []byte("app:\n  name: api-gateway\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	secretValue := "https://issuer.example.com/.well-known/jwks.json"
	if err := os.WriteFile(secretsPath, []byte("auth:\n  jwks_url: "+secretValue+"\n"), 0o644); err != nil {
		t.Fatalf("write secrets: %v", err)
	}

	root := newConfigShowRoot(t, configPath)

	stdout := captureStdout(t, func() {
		root.SetArgs([]string{"config", "show", "--all", "--show-secrets", "--secret-file", secretsPath})
		if err := root.Execute(); err != nil {
			t.Fatalf("config show --show-secrets failed: %v", err)
		}
	})

	if !strings.Contains(stdout, "jwks_url: "+secretValue) {
		t.Fatalf("expected config show --show-secrets output to keep secret value, got %q", stdout)
	}
}

func newConfigShowRoot(t *testing.T, configPath string) *cobra.Command {
	t.Helper()

	gwCfg := gatewaycfg.NewDefaultConfig()
	opts := cli.AppCommandOptions{
		Name:             "api-gateway",
		ConfigPath:       configPath,
		ConfigExtensions: []any{gwCfg},
	}
	root := &cobra.Command{Use: "api-gateway"}
	root.PersistentFlags().String("config-file", configPath, "")
	root.PersistentFlags().String("secret-file", "", "")
	root.PersistentFlags().String("app-name", "", "")
	configRoot := &cobra.Command{Use: "config"}
	configRoot.AddCommand(&cobra.Command{Use: "show"})
	root.AddCommand(configRoot)
	AttachToRoot(root, &opts)
	return root
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
