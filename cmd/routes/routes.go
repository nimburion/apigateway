package routes

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/apigateway/internal/routing"
	"github.com/nimburion/nimburion/pkg/cli"
	basecfg "github.com/nimburion/nimburion/pkg/config"
	"github.com/nimburion/nimburion/pkg/http/router"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const DefaultConfigPath = ""

var loadConfigAndLogger = cli.LoadConfigAndLogger

func NewCommand(opts *cli.AppCommandOptions, gwCfg *gatewaycfg.Gateway) *cobra.Command {
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

	var failOnWarning bool
	reviewCmd := &cobra.Command{
		Use:   "review",
		Short: "Review effective config for staging/prod rollout checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadGatewayConfig(cmd, opts, gwCfg)
			if err != nil {
				return err
			}
			report := buildReviewReport(cfg, gwCfg.Routes, flagValue(cmd, "config-file", opts.ConfigPath))
			out, err := yaml.Marshal(report)
			if err != nil {
				return fmt.Errorf("marshal review report: %w", err)
			}
			fmt.Print(string(out))
			if failOnWarning && len(report.Warnings) > 0 {
				return fmt.Errorf("review report contains %d warning(s)", len(report.Warnings))
			}
			return nil
		},
	}
	reviewCmd.Flags().BoolVar(&failOnWarning, "fail-on-warning", false, "exit non-zero when warnings are present")

	var (
		otherConfigPath string
		otherSecretPath string
		otherAppName    string
		leftLabel       string
		rightLabel      string
		failOnDrift     bool
	)
	compareCmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare two effective configs for staging/prod drift checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			leftCfg, err := loadGatewayConfig(cmd, opts, gwCfg)
			if err != nil {
				return err
			}

			otherGwCfg := gatewaycfg.NewDefaultConfig()
			otherOpts := *opts
			otherOpts.ConfigPath = otherConfigPath
			otherOpts.ConfigExtensions = cloneConfigExtensionsWithGateway(opts.ConfigExtensions, otherGwCfg)
			rightCfg, err := loadGatewayConfigWithOverrides(cmd, &otherOpts, otherGwCfg, otherSecretPath, otherAppName)
			if err != nil {
				return err
			}

			leftReport := buildReviewReport(leftCfg, gwCfg.Routes, flagValue(cmd, "config-file", opts.ConfigPath))
			rightReport := buildReviewReport(rightCfg, otherGwCfg.Routes, otherConfigPath)
			report := buildComparisonReport(leftLabel, rightLabel, leftReport, rightReport)

			out, err := yaml.Marshal(report)
			if err != nil {
				return fmt.Errorf("marshal comparison report: %w", err)
			}
			fmt.Print(string(out))
			if failOnDrift && len(report.Drifts) > 0 {
				return fmt.Errorf("comparison report contains %d drift item(s)", len(report.Drifts))
			}
			return nil
		},
	}
	compareCmd.Flags().StringVar(&otherConfigPath, "other-config-file", "", "config file path for the comparison environment")
	compareCmd.Flags().StringVar(&otherSecretPath, "other-secret-file", "", "secret file path for the comparison environment")
	compareCmd.Flags().StringVar(&otherAppName, "other-app-name", "", "application name override for the comparison environment")
	compareCmd.Flags().StringVar(&leftLabel, "left-label", "staging", "label for the primary config")
	compareCmd.Flags().StringVar(&rightLabel, "right-label", "production", "label for the comparison config")
	compareCmd.Flags().BoolVar(&failOnDrift, "fail-on-drift", false, "exit non-zero when drift items are present")
	_ = compareCmd.MarkFlagRequired("other-config-file")

	cmd.AddCommand(showCmd, validateCmd, reviewCmd, compareCmd)
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

func prepareRoutes(cmd *cobra.Command, opts *cli.AppCommandOptions, gwCfg *gatewaycfg.Gateway) error {
	_, err := loadGatewayConfig(cmd, opts, gwCfg)
	return err
}

func loadGatewayConfig(cmd *cobra.Command, opts *cli.AppCommandOptions, gwCfg *gatewaycfg.Gateway) (*basecfg.Config, error) {
	return loadGatewayConfigAtPath(cmd, opts, gwCfg, flagValue(cmd, "config-file", opts.ConfigPath), flagValue(cmd, "secret-file", ""), flagValue(cmd, "app-name", ""))
}

func loadGatewayConfigWithOverrides(cmd *cobra.Command, opts *cli.AppCommandOptions, gwCfg *gatewaycfg.Gateway, secretPath, appNameOverride string) (*basecfg.Config, error) {
	return loadGatewayConfigAtPath(cmd, opts, gwCfg, opts.ConfigPath, secretPath, appNameOverride)
}

func loadGatewayConfigAtPath(cmd *cobra.Command, opts *cli.AppCommandOptions, gwCfg *gatewaycfg.Gateway, cfgPath, secretPath, appNameOverride string) (*basecfg.Config, error) {
	cfg, _, err := loadConfigAndLogger(cfgPath, opts.EnvPrefix, secretPath, opts.ValidateConfig, cmd, opts.ConfigExtensions, opts.Name, appNameOverride)
	if err != nil {
		return nil, err
	}
	baseDir := DetermineRoutesBaseDir(cfgPath, DefaultConfigPath)
	if baseDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			baseDir = cwd
		}
	}
	gwCfg.ConfigDir = baseDir
	if err := gwCfg.LoadRoutes(baseDir, routesMiddlewareRegistry()); err != nil {
		return nil, err
	}
	return cfg, nil
}

func cloneConfigExtensionsWithGateway(extensions []any, replacement *gatewaycfg.Gateway) []any {
	cloned := make([]any, 0, len(extensions)+1)
	replaced := false
	for _, ext := range extensions {
		if _, ok := ext.(*gatewaycfg.Gateway); ok {
			cloned = append(cloned, replacement)
			replaced = true
			continue
		}
		cloned = append(cloned, ext)
	}
	if !replaced {
		cloned = append(cloned, replacement)
	}
	return cloned
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
		"ClaimsGuardFromConfig": func() router.MiddlewareFunc {
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

type reviewReport struct {
	AppName               string              `yaml:"app_name"`
	AuthEnabled           bool                `yaml:"auth_enabled"`
	ManagementEnabled     bool                `yaml:"management_enabled"`
	ManagementAuthEnabled bool                `yaml:"management_auth_enabled"`
	LegacyServiceKey      bool                `yaml:"legacy_service_key"`
	Warnings              []string            `yaml:"warnings,omitempty"`
	Groups                []reviewGroupReport `yaml:"groups"`
}

type reviewGroupReport struct {
	Name               string              `yaml:"name"`
	Prefix             string              `yaml:"prefix"`
	Middlewares        []string            `yaml:"middlewares,omitempty"`
	MeRequiresAuth     bool                `yaml:"me_requires_auth"`
	ProtectedRoutes    []reviewRouteReport `yaml:"protected_routes,omitempty"`
	PublicRoutes       []reviewRouteReport `yaml:"public_routes,omitempty"`
	ProtectedWebSocket []string            `yaml:"protected_websockets,omitempty"`
	PublicWebSocket    []string            `yaml:"public_websockets,omitempty"`
}

type reviewRouteReport struct {
	Path                 string   `yaml:"path"`
	Method               string   `yaml:"method"`
	EffectiveMiddlewares []string `yaml:"effective_middlewares,omitempty"`
	Scopes               []string `yaml:"scopes,omitempty"`
}

type comparisonReport struct {
	LeftLabel  string       `yaml:"left_label"`
	RightLabel string       `yaml:"right_label"`
	Drifts     []string     `yaml:"drifts,omitempty"`
	Left       reviewReport `yaml:"left"`
	Right      reviewReport `yaml:"right"`
}

func buildReviewReport(cfg *basecfg.Config, routes gatewaycfg.Routing, cfgPath string) reviewReport {
	report := reviewReport{
		AppName:               cfg.App.Name,
		AuthEnabled:           cfg.Auth.Enabled,
		ManagementEnabled:     cfg.Management.Enabled,
		ManagementAuthEnabled: cfg.Management.AuthEnabled,
		LegacyServiceKey:      hasLegacyServiceKey(cfgPath),
		Groups:                make([]reviewGroupReport, 0, len(routes.Groups)),
	}
	if report.LegacyServiceKey {
		report.Warnings = append(report.Warnings, "top-level legacy 'service' key detected in config file")
	}

	groupNames := make([]string, 0, len(routes.Groups))
	for groupName := range routes.Groups {
		groupNames = append(groupNames, groupName)
	}
	sort.Strings(groupNames)

	for _, groupName := range groupNames {
		group := routes.Groups[groupName]
		groupReport := reviewGroupReport{
			Name:            groupName,
			Prefix:          group.Prefix,
			Middlewares:     append([]string(nil), group.Middlewares...),
			MeRequiresAuth:  containsAuthAwareMiddleware(group.Middlewares),
			ProtectedRoutes: []reviewRouteReport{},
			PublicRoutes:    []reviewRouteReport{},
		}

		for _, route := range group.Routes {
			routeMiddlewares := routing.ApplyMiddlewareDirectives(group.Middlewares, route.Middlewares, route.DisableMiddlewares)
			for _, endpoint := range route.Endpoints {
				endpointMiddlewares := routing.ApplyMiddlewareDirectives(routeMiddlewares, endpoint.Middlewares, endpoint.DisableMiddlewares)
				for methodName, method := range endpoint.Methods {
					effective := routing.ApplyMiddlewareDirectives(endpointMiddlewares, method.Middlewares, method.DisableMiddlewares)
					entry := reviewRouteReport{
						Path:                 joinReportPath(group.Prefix, route.PathPrefix, endpoint.Path),
						Method:               methodName,
						EffectiveMiddlewares: append([]string(nil), effective...),
						Scopes:               append([]string(nil), method.Scopes...),
					}
					if containsAuthAwareMiddleware(effective) || len(method.Scopes) > 0 {
						groupReport.ProtectedRoutes = append(groupReport.ProtectedRoutes, entry)
					} else {
						groupReport.PublicRoutes = append(groupReport.PublicRoutes, entry)
					}
				}
			}
		}

		for _, ws := range group.WebSockets {
			effective := routing.ApplyMiddlewareDirectives(group.Middlewares, ws.Middlewares, ws.DisableMiddlewares)
			path := joinReportPath(group.Prefix, ws.Path)
			if containsAuthAwareMiddleware(effective) || len(ws.Scopes) > 0 {
				groupReport.ProtectedWebSocket = append(groupReport.ProtectedWebSocket, path)
			} else {
				groupReport.PublicWebSocket = append(groupReport.PublicWebSocket, path)
			}
		}

		report.Groups = append(report.Groups, groupReport)
	}

	return report
}

func buildComparisonReport(leftLabel, rightLabel string, left, right reviewReport) comparisonReport {
	report := comparisonReport{
		LeftLabel:  leftLabel,
		RightLabel: rightLabel,
		Left:       left,
		Right:      right,
	}

	if left.AppName != right.AppName {
		report.Drifts = append(report.Drifts, fmt.Sprintf("app.name differs: %s=%q %s=%q", leftLabel, left.AppName, rightLabel, right.AppName))
	}
	if left.AuthEnabled != right.AuthEnabled {
		report.Drifts = append(report.Drifts, fmt.Sprintf("auth.enabled differs: %s=%t %s=%t", leftLabel, left.AuthEnabled, rightLabel, right.AuthEnabled))
	}
	if left.ManagementEnabled != right.ManagementEnabled {
		report.Drifts = append(report.Drifts, fmt.Sprintf("management.enabled differs: %s=%t %s=%t", leftLabel, left.ManagementEnabled, rightLabel, right.ManagementEnabled))
	}
	if left.ManagementAuthEnabled != right.ManagementAuthEnabled {
		report.Drifts = append(report.Drifts, fmt.Sprintf("management.auth_enabled differs: %s=%t %s=%t", leftLabel, left.ManagementAuthEnabled, rightLabel, right.ManagementAuthEnabled))
	}
	if left.LegacyServiceKey != right.LegacyServiceKey {
		report.Drifts = append(report.Drifts, fmt.Sprintf("legacy service config usage differs: %s=%t %s=%t", leftLabel, left.LegacyServiceKey, rightLabel, right.LegacyServiceKey))
	}

	leftWarnings := strings.Join(left.Warnings, "; ")
	rightWarnings := strings.Join(right.Warnings, "; ")
	if leftWarnings != rightWarnings {
		report.Drifts = append(report.Drifts, fmt.Sprintf("warnings differ: %s=%q %s=%q", leftLabel, leftWarnings, rightLabel, rightWarnings))
	}

	leftGroups := reviewGroupsByName(left.Groups)
	rightGroups := reviewGroupsByName(right.Groups)
	groupNames := unionKeys(leftGroups, rightGroups)
	for _, name := range groupNames {
		leftGroup, leftOK := leftGroups[name]
		rightGroup, rightOK := rightGroups[name]
		if !leftOK {
			report.Drifts = append(report.Drifts, fmt.Sprintf("group %q exists only in %s", name, rightLabel))
			continue
		}
		if !rightOK {
			report.Drifts = append(report.Drifts, fmt.Sprintf("group %q exists only in %s", name, leftLabel))
			continue
		}
		if leftGroup.Prefix != rightGroup.Prefix {
			report.Drifts = append(report.Drifts, fmt.Sprintf("group %q prefix differs: %s=%q %s=%q", name, leftLabel, leftGroup.Prefix, rightLabel, rightGroup.Prefix))
		}
		if !equalStringSlices(leftGroup.Middlewares, rightGroup.Middlewares) {
			report.Drifts = append(report.Drifts, fmt.Sprintf("group %q middlewares differ: %s=%v %s=%v", name, leftLabel, leftGroup.Middlewares, rightLabel, rightGroup.Middlewares))
		}
		if leftGroup.MeRequiresAuth != rightGroup.MeRequiresAuth {
			report.Drifts = append(report.Drifts, fmt.Sprintf("group %q auth/me posture differs: %s=%t %s=%t", name, leftLabel, leftGroup.MeRequiresAuth, rightLabel, rightGroup.MeRequiresAuth))
		}
		report.Drifts = append(report.Drifts, compareRouteExposure(leftLabel, rightLabel, name, leftGroup, rightGroup)...)
		report.Drifts = append(report.Drifts, compareWebSocketExposure(leftLabel, rightLabel, name, leftGroup, rightGroup)...)
	}

	return report
}

func containsAuthAwareMiddleware(names []string) bool {
	for _, name := range names {
		switch name {
		case "Authenticate", "ClaimsGuardFromConfig":
			return true
		}
	}
	return false
}

func reviewGroupsByName(groups []reviewGroupReport) map[string]reviewGroupReport {
	index := make(map[string]reviewGroupReport, len(groups))
	for _, group := range groups {
		index[group.Name] = group
	}
	return index
}

func compareRouteExposure(leftLabel, rightLabel, groupName string, leftGroup, rightGroup reviewGroupReport) []string {
	drifts := []string{}
	leftRoutes := flattenRouteReports(leftGroup)
	rightRoutes := flattenRouteReports(rightGroup)
	keys := unionKeys(leftRoutes, rightRoutes)
	for _, key := range keys {
		leftRoute, leftOK := leftRoutes[key]
		rightRoute, rightOK := rightRoutes[key]
		if !leftOK {
			drifts = append(drifts, fmt.Sprintf("route %q in group %q exists only in %s", key, groupName, rightLabel))
			continue
		}
		if !rightOK {
			drifts = append(drifts, fmt.Sprintf("route %q in group %q exists only in %s", key, groupName, leftLabel))
			continue
		}
		leftProtected := containsAuthAwareMiddleware(leftRoute.EffectiveMiddlewares) || len(leftRoute.Scopes) > 0
		rightProtected := containsAuthAwareMiddleware(rightRoute.EffectiveMiddlewares) || len(rightRoute.Scopes) > 0
		if leftProtected != rightProtected {
			drifts = append(drifts, fmt.Sprintf("route %q in group %q changes exposure: %s=%t %s=%t", key, groupName, leftLabel, leftProtected, rightLabel, rightProtected))
		}
		if !equalStringSlices(leftRoute.EffectiveMiddlewares, rightRoute.EffectiveMiddlewares) {
			drifts = append(drifts, fmt.Sprintf("route %q in group %q middlewares differ: %s=%v %s=%v", key, groupName, leftLabel, leftRoute.EffectiveMiddlewares, rightLabel, rightRoute.EffectiveMiddlewares))
		}
		if !equalStringSlices(leftRoute.Scopes, rightRoute.Scopes) {
			drifts = append(drifts, fmt.Sprintf("route %q in group %q scopes differ: %s=%v %s=%v", key, groupName, leftLabel, leftRoute.Scopes, rightLabel, rightRoute.Scopes))
		}
	}
	return drifts
}

func compareWebSocketExposure(leftLabel, rightLabel, groupName string, leftGroup, rightGroup reviewGroupReport) []string {
	drifts := []string{}
	leftSockets := flattenWebSockets(leftGroup)
	rightSockets := flattenWebSockets(rightGroup)
	keys := unionKeys(leftSockets, rightSockets)
	for _, key := range keys {
		leftProtected, leftOK := leftSockets[key]
		rightProtected, rightOK := rightSockets[key]
		if !leftOK {
			drifts = append(drifts, fmt.Sprintf("websocket %q in group %q exists only in %s", key, groupName, rightLabel))
			continue
		}
		if !rightOK {
			drifts = append(drifts, fmt.Sprintf("websocket %q in group %q exists only in %s", key, groupName, leftLabel))
			continue
		}
		if leftProtected != rightProtected {
			drifts = append(drifts, fmt.Sprintf("websocket %q in group %q changes exposure: %s=%t %s=%t", key, groupName, leftLabel, leftProtected, rightLabel, rightProtected))
		}
	}
	return drifts
}

func flattenRouteReports(group reviewGroupReport) map[string]reviewRouteReport {
	index := map[string]reviewRouteReport{}
	for _, route := range group.PublicRoutes {
		index[route.Method+" "+route.Path] = route
	}
	for _, route := range group.ProtectedRoutes {
		index[route.Method+" "+route.Path] = route
	}
	return index
}

func flattenWebSockets(group reviewGroupReport) map[string]bool {
	index := map[string]bool{}
	for _, path := range group.PublicWebSocket {
		index[path] = false
	}
	for _, path := range group.ProtectedWebSocket {
		index[path] = true
	}
	return index
}

func unionKeys[T any](left, right map[string]T) []string {
	keys := make([]string, 0, len(left)+len(right))
	seen := map[string]struct{}{}
	for key := range left {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range right {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func hasLegacyServiceKey(cfgPath string) bool {
	trimmed := strings.TrimSpace(cfgPath)
	if trimmed == "" {
		return false
	}
	data, err := os.ReadFile(trimmed)
	if err != nil {
		return false
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	for {
		var raw map[string]any
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				return false
			}
			return false
		}
		if raw == nil {
			continue
		}
		if _, ok := raw["service"]; ok {
			return true
		}
	}
}

func joinReportPath(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" || trimmed == "/" {
			continue
		}
		clean = append(clean, strings.TrimSuffix(trimmed, "/"))
	}
	if len(clean) == 0 {
		return "/"
	}
	path := strings.Join(clean, "/")
	path = strings.ReplaceAll(path, "//", "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}
