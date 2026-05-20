package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nimburion/nimburion/pkg/cli"
	nimbcfg "github.com/nimburion/nimburion/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var httpMethods = map[string]struct{}{
	"GET":     {},
	"POST":    {},
	"PUT":     {},
	"PATCH":   {},
	"DELETE":  {},
	"OPTIONS": {},
}

type generatedConfig struct {
	App        generatedApp        `yaml:"app"`
	Auth       generatedAuth       `yaml:"auth"`
	HTTP       generatedHTTP       `yaml:"http"`
	Management generatedManagement `yaml:"management"`
	Routes     generatedRoutesRoot `yaml:"routes"`
}

type generatedApp struct {
	Name string `yaml:"name"`
}

type generatedAuth struct {
	Enabled bool `yaml:"enabled"`
}

type generatedHTTP struct {
	Port int `yaml:"port"`
}

type generatedManagement struct {
	Enabled     bool `yaml:"enabled"`
	Port        int  `yaml:"port"`
	AuthEnabled bool `yaml:"auth_enabled"`
}

type generatedRoutesRoot struct {
	Groups map[string]generatedGroup `yaml:"groups"`
}

type generatedGroup struct {
	Prefix string           `yaml:"prefix"`
	Routes []generatedRoute `yaml:"routes"`
}

type generatedRoute struct {
	PathPrefix string              `yaml:"path_prefix"`
	TargetURL  string              `yaml:"target_url"`
	Endpoints  []generatedEndpoint `yaml:"endpoints"`
}

type generatedEndpoint struct {
	Path    string                         `yaml:"path"`
	Methods map[string]map[string]struct{} `yaml:"methods"`
}

func NewGenerateCommand() *cobra.Command {
	var (
		outputPath string
		force      bool
	)
	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a minimal configuration that allows apigateway to start",
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := yaml.Marshal(minimalConfig())
			if err != nil {
				return fmt.Errorf("marshal generated config: %w", err)
			}
			if outputPath == "" {
				fmt.Print(string(payload))
				return nil
			}
			if !force {
				if _, err := os.Stat(outputPath); err == nil {
					return fmt.Errorf("output file already exists: %s", outputPath)
				}
			}
			if err := os.WriteFile(outputPath, payload, 0o644); err != nil {
				return fmt.Errorf("write generated config: %w", err)
			}
			fmt.Printf("config written to %s\n", outputPath)
			return nil
		},
	}
	generateCmd.Flags().StringVar(&outputPath, "output", "", "write generated config to a file instead of stdout")
	generateCmd.Flags().BoolVar(&force, "force", false, "overwrite the output file if it already exists")
	return generateCmd
}

func minimalConfig() generatedConfig {
	return generatedConfig{
		App:  generatedApp{Name: "api-gateway"},
		Auth: generatedAuth{Enabled: false},
		HTTP: generatedHTTP{Port: 8080},
		Management: generatedManagement{
			Enabled:     true,
			Port:        9001,
			AuthEnabled: false,
		},
		Routes: generatedRoutesRoot{
			Groups: map[string]generatedGroup{
				"default": {
					Prefix: "/",
					Routes: []generatedRoute{
						{
							PathPrefix: "/healthz",
							TargetURL:  "http://localhost:8081",
							Endpoints: []generatedEndpoint{
								{
									Path: "/",
									Methods: map[string]map[string]struct{}{
										"GET": {},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func AttachToRoot(root *cobra.Command, opts *cli.AppCommandOptions) {
	if root == nil || opts == nil {
		return
	}

	var configCmd *cobra.Command
	for _, cmd := range root.Commands() {
		if cmd.Name() == "config" {
			configCmd = cmd
			break
		}
	}
	if configCmd == nil {
		return
	}

	for _, cmd := range configCmd.Commands() {
		if cmd.Name() == "show" {
			configCmd.RemoveCommand(cmd)
			break
		}
	}

	configCmd.AddCommand(NewShowCommand(opts))
	configCmd.AddCommand(NewGenerateCommand())
}

func NewShowCommand(opts *cli.AppCommandOptions) *cobra.Command {
	var (
		showSecrets bool
		showAll     bool
	)

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfgPath := inheritedFlagValue(cmd, "config-file", opts.ConfigPath)
			secretPath := inheritedFlagValue(cmd, "secret-file", "")
			appNameOverride := inheritedFlagValue(cmd, "app-name", "")

			if opts.ConfigPathResolved != nil {
				opts.ConfigPathResolved(cfgPath)
			}
			if err := applySecretFileEnv(opts.EnvPrefix, secretPath); err != nil {
				return err
			}

			cfg := &nimbcfg.Config{}
			provider := nimbcfg.NewConfigProvider(cfgPath, opts.EnvPrefix).
				WithAppNameDefault(opts.Name).
				WithFlags(cmd.Flags())
			secrets, err := provider.LoadWithSecrets(cfg, opts.ConfigExtensions...)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			applyAppNameSetting(cfg, opts.Name, appNameOverride)

			settings := provider.AllSettings()
			settings = ensureAppNameSetting(settings, cfg.App.Name)
			if !showSecrets {
				settings = redactSettings(settings, secrets)
			}
			if !showAll {
				explicitSettings, err := loadExplicitSettings(cfgPath, opts.EnvPrefix)
				if err != nil {
					return err
				}
				settings = filterSettingsMap(settings, explicitSettings, nil)
			}

			payload, err := yaml.Marshal(settings)
			if err != nil {
				return fmt.Errorf("marshal config settings: %w", err)
			}
			fmt.Print(string(payload))
			return nil
		},
	}
	showCmd.Flags().BoolVar(&showSecrets, "show-secrets", false, "show secret values")
	showCmd.Flags().BoolVar(&showAll, "all", false, "show the full effective config including empty/defaulted sections")
	return showCmd
}

func inheritedFlagValue(cmd *cobra.Command, name, fallback string) string {
	if flag := cmd.Flags().Lookup(name); flag != nil && flag.Value.String() != "" {
		return flag.Value.String()
	}
	if root := cmd.Root(); root != nil {
		if flag := root.PersistentFlags().Lookup(name); flag != nil && flag.Value.String() != "" {
			return flag.Value.String()
		}
	}
	return fallback
}

func applySecretFileEnv(envPrefix, secretFilePath string) error {
	if strings.TrimSpace(secretFilePath) == "" {
		return nil
	}
	prefix := strings.TrimSpace(envPrefix)
	if prefix == "" {
		prefix = "APP"
	}
	return os.Setenv(prefix+"_SECRETS_FILE", filepath.Clean(secretFilePath))
}

func applyAppNameSetting(cfg *nimbcfg.Config, defaultName, override string) {
	if cfg == nil {
		return
	}
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		cfg.App.Name = trimmed
		return
	}
	if strings.TrimSpace(cfg.App.Name) == "" {
		cfg.App.Name = defaultName
	}
}

func ensureAppNameSetting(settings map[string]any, appName string) map[string]any {
	if settings == nil {
		settings = map[string]any{}
	}
	appSettings, _ := settings["app"].(map[string]any)
	if appSettings == nil {
		appSettings = map[string]any{}
	}
	appSettings["name"] = appName
	settings["app"] = appSettings
	return settings
}

func redactSettings(settings, secrets map[string]any) map[string]any {
	if settings == nil {
		return map[string]any{}
	}
	redacted := make(map[string]any, len(settings))
	secretKeys := sortedKeys(secrets)
	for key, value := range settings {
		secretValue, hasSecret := secrets[key]
		switch typed := value.(type) {
		case map[string]any:
			secretMap, _ := secretValue.(map[string]any)
			redacted[key] = redactSettings(typed, secretMap)
		case []any:
			redacted[key] = redactSlice(typed)
		default:
			if hasSecret && contains(secretKeys, key) {
				redacted[key] = "***"
				continue
			}
			redacted[key] = value
		}
	}
	return redacted
}

func redactSlice(values []any) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		if child, ok := value.(map[string]any); ok {
			out = append(out, redactSettings(child, nil))
			continue
		}
		out = append(out, value)
	}
	return out
}

func sortedKeys(settings map[string]any) []string {
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func loadExplicitSettings(cfgPath, envPrefix string) (map[string]any, error) {
	settings := map[string]any{}
	if strings.TrimSpace(cfgPath) == "" {
		return settings, nil
	}
	loaded, err := readConfigMap(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("read explicit config settings: %w", err)
	}
	mergeSettings(settings, loaded)

	secretsPath, err := discoverSecretsPath(cfgPath, envPrefix)
	if err != nil {
		return nil, err
	}
	if secretsPath != "" {
		secrets, err := readConfigMap(secretsPath)
		if err != nil {
			return nil, fmt.Errorf("read explicit secrets settings: %w", err)
		}
		mergeSettings(settings, secrets)
	}

	return settings, nil
}

func discoverSecretsPath(cfgPath, envPrefix string) (string, error) {
	if envPath := strings.TrimSpace(os.Getenv(resolveEnvPrefix(envPrefix) + "_SECRETS_FILE")); envPath != "" {
		if _, err := os.Stat(envPath); err != nil {
			return "", fmt.Errorf("access explicit secrets file %s: %w", envPath, err)
		}
		return envPath, nil
	}

	if strings.TrimSpace(cfgPath) != "" {
		ext := filepath.Ext(cfgPath)
		candidate := filepath.Join(filepath.Dir(cfgPath), "secrets"+ext)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	for _, ext := range []string{".yaml", ".yml", ".json", ".toml"} {
		candidate := "secrets" + ext
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", nil
}

func resolveEnvPrefix(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return "APP"
	}
	return strings.ToUpper(trimmed)
}

func readConfigMap(path string) (map[string]any, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	return v.AllSettings(), nil
}

func mergeSettings(dst, src map[string]any) {
	for key, value := range src {
		srcMap, srcIsMap := value.(map[string]any)
		dstMap, dstIsMap := dst[key].(map[string]any)
		if srcIsMap {
			if !dstIsMap {
				dstMap = map[string]any{}
			}
			mergeSettings(dstMap, srcMap)
			dst[key] = dstMap
			continue
		}
		dst[key] = value
	}
}

func filterSettingsMap(settings, explicit map[string]any, path []string) map[string]any {
	filtered := make(map[string]any)
	for key, explicitValue := range explicit {
		value, ok := settings[key]
		if !ok {
			continue
		}
		nextPath := append(append([]string(nil), path...), key)
		filteredValue, keep := filterValue(value, explicitValue, nextPath)
		if keep {
			filtered[key] = filteredValue
		}
	}
	return filtered
}

func filterValue(value, explicitValue any, path []string) (any, bool) {
	switch explicitTyped := explicitValue.(type) {
	case map[string]any:
		valueMap, ok := value.(map[string]any)
		if !ok {
			return value, true
		}
		filtered := filterSettingsMap(valueMap, explicitTyped, path)
		if len(filtered) == 0 {
			if shouldKeepExplicitEmptyMap(path, explicitTyped) {
				return map[string]any{}, true
			}
			return nil, false
		}
		return filtered, true
	case []any:
		valueSlice, ok := value.([]any)
		if !ok {
			return value, true
		}
		if len(explicitTyped) == 0 {
			return nil, false
		}
		filtered := make([]any, 0, len(explicitTyped))
		for i, item := range explicitTyped {
			if i >= len(valueSlice) {
				break
			}
			filteredItem, keep := filterValue(valueSlice[i], item, path)
			if keep {
				filtered = append(filtered, filteredItem)
			}
		}
		if len(filtered) == 0 {
			return nil, false
		}
		return filtered, true
	default:
		return value, true
	}
}

func shouldKeepExplicitEmptyMap(path []string, explicit map[string]any) bool {
	if len(explicit) != 0 || len(path) < 2 {
		return false
	}
	if path[len(path)-2] != "methods" {
		return false
	}
	_, ok := httpMethods[path[len(path)-1]]
	return ok
}
