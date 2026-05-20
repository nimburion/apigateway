package openapi

import (
	"fmt"
	"os"

	routescmd "github.com/nimburion/apigateway/cmd/routes"
	"github.com/nimburion/apigateway/internal/approutes"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/cli"
	httpopenapi "github.com/nimburion/nimburion/pkg/http/openapi"
	"github.com/nimburion/nimburion/pkg/http/router"
	"github.com/spf13/cobra"
)

const defaultOpenAPIPath = "config/openapi/openapi.yml"

func NewCommand(opts *cli.AppCommandOptions, gwCfg *gatewaycfg.Gateway) *cobra.Command {
	command := &cobra.Command{
		Use:   "openapi",
		Short: "Manage OpenAPI specification",
	}

	var output string
	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate OpenAPI spec for the API Gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := resolveConfigPath(flagValue(cmd, "config-file", opts.ConfigPath))
			if cfgPath == "" {
				fmt.Fprintln(os.Stderr, "warning: config file not found, using defaults")
			}
			secretPath := flagValue(cmd, "secret-file", "")
			appNameOverride := flagValue(cmd, "app-name", "")
			cfg, _, err := cli.LoadConfigAndLogger(cfgPath, opts.EnvPrefix, secretPath, opts.ValidateConfig, cmd, opts.ConfigExtensions, opts.Name, appNameOverride)
			if err != nil {
				return err
			}

			baseDir := routescmd.DetermineRoutesBaseDir(cfgPath, routescmd.DefaultConfigPath)
			if baseDir == "" {
				if cwd, err := os.Getwd(); err == nil {
					baseDir = cwd
				}
			}
			gwCfg.ConfigDir = baseDir
			if err := gwCfg.LoadRoutes(baseDir, routesMiddlewareRegistry()); err != nil {
				return fmt.Errorf("load gateway routes: %w", err)
			}
			if err := approutes.ValidateSupportedMethods(gwCfg.Routes); err != nil {
				return fmt.Errorf("validate gateway routes: %w", err)
			}

			spec, err := approutes.BuildOpenAPISpec(cfg, gwCfg.Routes, routesMiddlewareRegistry())
			if err != nil {
				return fmt.Errorf("build openapi spec: %w", err)
			}

			outPath := output
			if outPath == "" {
				outPath = defaultOpenAPIPath
			}
			if err := httpopenapi.WriteSpec(outPath, spec); err != nil {
				return fmt.Errorf("write openapi spec: %w", err)
			}

			fmt.Printf("OpenAPI spec written to %s\n", outPath)
			return nil
		},
	}
	generateCmd.Flags().StringVar(&output, "output", defaultOpenAPIPath, "output path for generated OpenAPI spec")
	command.AddCommand(generateCmd)

	return command
}

func flagValue(cmd *cobra.Command, name, fallback string) string {
	if cmd == nil {
		return fallback
	}
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		flag = cmd.InheritedFlags().Lookup(name)
	}
	if flag == nil || !flag.Changed {
		return fallback
	}
	return flag.Value.String()
}

func resolveConfigPath(cfgPath string) string {
	if cfgPath == "" {
		return ""
	}
	if _, err := os.Stat(cfgPath); err == nil {
		return cfgPath
	}
	return ""
}

func routesMiddlewareRegistry() map[string]func() router.MiddlewareFunc {
	return map[string]func() router.MiddlewareFunc{
		"Authenticate":          func() router.MiddlewareFunc { return func(next router.HandlerFunc) router.HandlerFunc { return next } },
		"ClaimsGuardFromConfig": func() router.MiddlewareFunc { return func(next router.HandlerFunc) router.HandlerFunc { return next } },
		"ForwardIdentityHeaders": func() router.MiddlewareFunc {
			return func(next router.HandlerFunc) router.HandlerFunc { return next }
		},
	}
}
