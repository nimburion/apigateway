package routes

import (
	"fmt"
	"os"
	"path/filepath"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/cli"
	"github.com/nimburion/nimburion/pkg/server/router"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const DefaultConfigPath = ""

var loadConfigAndLogger = cli.LoadConfigAndLogger

func NewCommand(opts *cli.ServiceCommandOptions, gwCfg *gatewaycfg.Gateway) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "routes",
		Short: "Manage API Gateway routing configuration",
	}

	var showSecrets bool
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show merged routing groups",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prepareRoutes(cmd, opts, gwCfg); err != nil {
				return err
			}
			routes := gwCfg.Routes
			if !showSecrets {
				routes = redactedRoutes(routes)
			}
			out, err := yaml.Marshal(routes)
			if err != nil {
				return fmt.Errorf("marshal routes: %w", err)
			}
			fmt.Print(string(out))
			return nil
		},
	}
	showCmd.Flags().BoolVar(&showSecrets, "show-secrets", false, "show secret values")

	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate routing configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prepareRoutes(cmd, opts, gwCfg); err != nil {
				return err
			}
			fmt.Println("routing configuration is valid")
			return nil
		},
	}

	cmd.AddCommand(showCmd, validateCmd)
	return cmd
}

func DetermineRoutesBaseDir(resolvedPath, serviceConfigPath string) string {
	if dir := baseDirIfConfig(resolvedPath); dir != "" {
		return dir
	}
	if dir := baseDirIfConfig(serviceConfigPath); dir != "" {
		return dir
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func baseDirIfConfig(path string) string {
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	return filepath.Dir(path)
}

func prepareRoutes(cmd *cobra.Command, opts *cli.ServiceCommandOptions, gwCfg *gatewaycfg.Gateway) error {
	cfgPath := flagValue(cmd, "config-file", opts.ConfigPath)
	secretPath := flagValue(cmd, "secret-file", "")
	serviceNameOverride := flagValue(cmd, "service-name", "")
	if _, _, err := loadConfigAndLogger(cfgPath, opts.EnvPrefix, secretPath, opts.ValidateConfig, cmd.Root().PersistentFlags(), opts.ConfigExtensions, opts.Name, serviceNameOverride); err != nil {
		return err
	}
	baseDir := DetermineRoutesBaseDir(cfgPath, DefaultConfigPath)
	if baseDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			baseDir = cwd
		}
	}
	gwCfg.ConfigDir = baseDir
	return gwCfg.LoadRoutes(baseDir, routesMiddlewareRegistry())
}

func flagValue(cmd *cobra.Command, name, fallback string) string {
	if flag := cmd.Flags().Lookup(name); flag != nil && flag.Value.String() != "" {
		return flag.Value.String()
	}
	if flag := cmd.Root().PersistentFlags().Lookup(name); flag != nil && flag.Value.String() != "" {
		return flag.Value.String()
	}
	return fallback
}

func routesMiddlewareRegistry() map[string]func() router.MiddlewareFunc {
	return map[string]func() router.MiddlewareFunc{
		"Authenticate": func() router.MiddlewareFunc {
			return func(next router.HandlerFunc) router.HandlerFunc {
				return func(c router.Context) error { return next(c) }
			}
		},
		"ForwardIdentityHeaders": func() router.MiddlewareFunc {
			return func(next router.HandlerFunc) router.HandlerFunc {
				return func(c router.Context) error { return next(c) }
			}
		},
	}
}

func redactedRoutes(routes gatewaycfg.Routing) gatewaycfg.Routing {
	if len(routes.Groups) == 0 {
		return routes
	}

	redacted := gatewaycfg.Routing{Groups: make(map[string]gatewaycfg.Group, len(routes.Groups))}
	for name, group := range routes.Groups {
		groupCopy := group
		if group.AuthEndpoints != nil {
			authCopy := *group.AuthEndpoints
			if group.AuthEndpoints.OAuth2 != nil {
				oauthCopy := *group.AuthEndpoints.OAuth2
				if oauthCopy.ClientSecret != "" {
					oauthCopy.ClientSecret = "***"
				}
				authCopy.OAuth2 = &oauthCopy
			}
			groupCopy.AuthEndpoints = &authCopy
		}
		redacted.Groups[name] = groupCopy
	}
	return redacted
}
