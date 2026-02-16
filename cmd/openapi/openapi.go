package openapi

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	routescmd "github.com/nimburion/apigateway/cmd/routes"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/cli"
	nimbcfg "github.com/nimburion/nimburion/pkg/config"
	"github.com/nimburion/nimburion/pkg/server/router"
	"github.com/nimburion/nimburion/pkg/version"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const defaultOpenAPIPath = "config/openapi/openapi.yml"

func NewCommand(opts *cli.ServiceCommandOptions, gwCfg *gatewaycfg.Gateway) *cobra.Command {
	cmd := &cobra.Command{
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
			serviceNameOverride := flagValue(cmd, "service-name", "")
			cfg, _, err := cli.LoadConfigAndLogger(cfgPath, opts.EnvPrefix, secretPath, opts.ValidateConfig, cmd.Root().PersistentFlags(), opts.ConfigExtensions, opts.Name, serviceNameOverride)
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

			spec := buildSpec(cfg, gwCfg)
			payload, err := yaml.Marshal(spec)
			if err != nil {
				return fmt.Errorf("marshal openapi: %w", err)
			}

			outPath := output
			if outPath == "" {
				outPath = defaultOpenAPIPath
			}
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				return fmt.Errorf("create output dir: %w", err)
			}
			if err := os.WriteFile(outPath, payload, 0o644); err != nil {
				return fmt.Errorf("write openapi: %w", err)
			}

			fmt.Printf("OpenAPI spec written to %s\n", outPath)
			return nil
		},
	}
	generateCmd.Flags().StringVar(&output, "output", defaultOpenAPIPath, "output path for generated OpenAPI spec")
	cmd.AddCommand(generateCmd)

	return cmd
}

func buildSpec(cfg *nimbcfg.Config, gwCfg *gatewaycfg.Gateway) *openapi3.T {
	generatorName := serviceName(cfg)
	doc := &openapi3.T{
		OpenAPI: "3.0.3",
		Info: &openapi3.Info{
			Title:   "API Gateway",
			Version: version.AppVersion,
		},
		Paths: openapi3.NewPaths(),
	}
	if cfg != nil && cfg.Service.Name != "" {
		doc.Info.Title = cfg.Service.Name
	}

	doc.Servers = buildServers(cfg)
	addManagementRoutes(doc, cfg, generatorName)
	addGroupRoutes(doc, gwCfg, generatorName)
	sortPaths(doc.Paths)
	return doc
}

func buildServers(cfg *nimbcfg.Config) openapi3.Servers {
	if cfg == nil {
		return nil
	}
	servers := openapi3.Servers{}
	if cfg.HTTP.Port != 0 {
		servers = append(servers, &openapi3.Server{URL: fmt.Sprintf("http://localhost:%d", cfg.HTTP.Port)})
	}
	if cfg.Management.Enabled && cfg.Management.Port != 0 {
		servers = append(servers, &openapi3.Server{URL: fmt.Sprintf("http://localhost:%d", cfg.Management.Port)})
	}
	return servers
}

func addManagementRoutes(doc *openapi3.T, cfg *nimbcfg.Config, generatorName string) {
	if cfg == nil || !cfg.Management.Enabled {
		return
	}
	addOperation(doc, "GET", "/portal", "Developer portal", []string{"management"}, generatorName)
	addOperation(doc, "GET", "/portal/{path}", "Developer portal assets", []string{"management"}, generatorName)
	addOperation(doc, "GET", "/api/portal/routes", "Portal routes", []string{"management"}, generatorName)
	addOperation(doc, "GET", "/api/portal/groups", "Portal groups", []string{"management"}, generatorName)
}

func addGroupRoutes(doc *openapi3.T, gwCfg *gatewaycfg.Gateway, generatorName string) {
	if gwCfg == nil {
		return
	}
	for groupName, group := range gwCfg.Routes.Groups {
		groupPrefix := normalizeOpenAPIPath(group.Prefix)
		if group.AuthEndpoints != nil {
			if group.AuthEndpoints.Me {
				addOperation(doc, "GET", joinPaths(groupPrefix, "/auth/me"), "Authenticated user info", []string{groupName}, generatorName)
			}
			if group.AuthEndpoints.OAuth2 != nil && group.AuthEndpoints.OAuth2.Enabled {
				addOperation(doc, "GET", joinPaths(groupPrefix, "/auth/login"), "OAuth2 login", []string{groupName}, generatorName)
				addOperation(doc, "GET", joinPaths(groupPrefix, "/auth/callback"), "OAuth2 callback", []string{groupName}, generatorName)
				addOperation(doc, "POST", joinPaths(groupPrefix, "/auth/logout"), "OAuth2 logout", []string{groupName}, generatorName)
				addOperation(doc, "POST", joinPaths(groupPrefix, "/auth/refresh"), "OAuth2 refresh", []string{groupName}, generatorName)
			}
		}

		for _, route := range group.Routes {
			for _, endpoint := range route.Endpoints {
				path := normalizeOpenAPIPath(joinPaths(groupPrefix, route.PathPrefix, endpoint.Path))
				for method := range endpoint.Methods {
					summary := fmt.Sprintf("Proxy to %s", route.TargetURL)
					addOperation(doc, method, path, summary, []string{"proxied", groupName}, generatorName)
				}
			}
		}

		for _, ws := range group.WebSockets {
			path := normalizeOpenAPIPath(joinPaths(groupPrefix, ws.Path))
			addOperation(doc, "GET", path, "WebSocket proxy", []string{"websocket", groupName}, generatorName)
		}
	}
}

func addOperation(doc *openapi3.T, method, path, summary string, tags []string, generatorName string) {
	if doc.Paths == nil {
		doc.Paths = openapi3.NewPaths()
	}
	path = normalizeOpenAPIPath(path)
	if path == "" {
		return
	}

	item := doc.Paths.Value(path)
	if item == nil {
		item = &openapi3.PathItem{}
	}

	operation := &openapi3.Operation{
		Summary:    summary,
		Tags:       tags,
		Responses:  openapi3.NewResponses(),
		Extensions: map[string]any{"x-generated-by": generatorName},
	}

	for _, param := range pathParameters(path) {
		operation.Parameters = append(operation.Parameters, &openapi3.ParameterRef{
			Value: &openapi3.Parameter{
				Name:     param,
				In:       "path",
				Required: true,
				Schema:   &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
			},
		})
	}

	operation.Responses.Set("200", &openapi3.ResponseRef{Value: &openapi3.Response{Description: ptr("OK")}})

	switch strings.ToUpper(method) {
	case "GET":
		item.Get = operation
	case "POST":
		item.Post = operation
	case "PUT":
		item.Put = operation
	case "PATCH":
		item.Patch = operation
	case "DELETE":
		item.Delete = operation
	default:
		item.Get = operation
	}

	doc.Paths.Set(path, item)
}

func pathParameters(path string) []string {
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(path, -1)
	if len(matches) == 0 {
		return nil
	}
	params := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			params = append(params, match[1])
		}
	}
	return params
}

func joinPaths(parts ...string) string {
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" || trimmed == "/" {
			continue
		}
		segments = append(segments, trimmed)
	}
	if len(segments) == 0 {
		return "/"
	}
	path := strings.Join(segments, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.ReplaceAll(path, "//", "/")
	return path
}

func normalizeOpenAPIPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	if trimmed != "/" {
		trimmed = strings.TrimRight(trimmed, "/")
	}

	parts := strings.Split(trimmed, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") && len(part) > 1 {
			parts[i] = "{" + part[1:] + "}"
			continue
		}
		if strings.HasPrefix(part, "*") && len(part) > 1 {
			parts[i] = "{" + part[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

func sortPaths(paths *openapi3.Paths) {
	if paths == nil {
		return
	}
	keys := make([]string, 0, len(paths.Map()))
	for key := range paths.Map() {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	sorted := openapi3.NewPaths()
	for _, key := range keys {
		sorted.Set(key, paths.Value(key))
	}
	*paths = *sorted
}

func determineRoutesBaseDir(cfgPath string) string {
	if cfgPath == "" {
		return ""
	}
	info, err := os.Stat(cfgPath)
	if err != nil || info.IsDir() {
		return ""
	}
	return filepath.Dir(cfgPath)
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

func resolveConfigPath(cfgPath string) string {
	if cfgPath == "" {
		return ""
	}
	if _, err := os.Stat(cfgPath); err == nil {
		return cfgPath
	}
	// Missing config file: fall back to defaults (empty config path).
	return ""
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

func serviceName(cfg *nimbcfg.Config) string {
	if cfg != nil && strings.TrimSpace(cfg.Service.Name) != "" {
		return cfg.Service.Name
	}
	return "api-gateway"
}

func ptr(value string) *string {
	return &value
}
