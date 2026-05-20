package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/nimburion/nimburion/pkg/http/router"
)

//go:generate go run ../tools/configschema
type Gateway struct {
	Routes      Routing           `mapstructure:"routes"`
	RoutesFiles []string          `mapstructure:"routes_files"`
	Portal      PortalConfig      `mapstructure:"portal"`
	ConfigStore ConfigStoreConfig `mapstructure:"config_store"`
	Management  ManagementConfig  `mapstructure:"management"`
	Database    DatabaseConfig    `mapstructure:"database"`
	ConfigDir   string            `mapstructure:"-"`
}

const (
	PortalModeReadOnly = "read-only"
	PortalModeManaged  = "managed"

	ConfigSourceOfTruthFile     = "file"
	ConfigSourceOfTruthDatabase = "database"

	ConfigStoreBackendPostgres = "postgres"
)

type PortalConfig struct {
	Enabled        bool                       `yaml:"enabled" mapstructure:"enabled"`
	Mode           string                     `yaml:"mode" mapstructure:"mode"`
	Auth           PortalAuthConfig           `yaml:"auth" mapstructure:"auth"`
	Catalog        PortalCatalogConfig        `yaml:"catalog" mapstructure:"catalog"`
	MetricsHistory PortalMetricsHistoryConfig `yaml:"metrics_history" mapstructure:"metrics_history"`
}

type PortalAuthConfig struct {
	ReadScopes     []string `yaml:"read_scopes" mapstructure:"read_scopes"`
	WriteScopes    []string `yaml:"write_scopes" mapstructure:"write_scopes"`
	PublishScopes  []string `yaml:"publish_scopes" mapstructure:"publish_scopes"`
	RollbackScopes []string `yaml:"rollback_scopes" mapstructure:"rollback_scopes"`
}

type PortalCatalogConfig struct {
	ExposeTargetURLs    bool `yaml:"expose_target_urls" mapstructure:"expose_target_urls"`
	ExposeOpenAPIErrors bool `yaml:"expose_openapi_errors" mapstructure:"expose_openapi_errors"`
}

type PortalMetricsHistoryConfig struct {
	Enabled          bool                            `yaml:"enabled" mapstructure:"enabled"`
	Backend          string                          `yaml:"backend" mapstructure:"backend"`
	SnapshotInterval time.Duration                   `yaml:"snapshot_interval" mapstructure:"snapshot_interval"`
	MaxSnapshots     int                             `yaml:"max_snapshots" mapstructure:"max_snapshots"`
	MaxAge           time.Duration                   `yaml:"max_age" mapstructure:"max_age"`
	Redis            PortalMetricsHistoryRedisConfig `yaml:"redis" mapstructure:"redis"`
}

type PortalMetricsHistoryRedisConfig struct {
	URL              string        `yaml:"url" mapstructure:"url"`
	Prefix           string        `yaml:"prefix" mapstructure:"prefix"`
	MaxConns         int           `yaml:"max_conns" mapstructure:"max_conns"`
	OperationTimeout time.Duration `yaml:"operation_timeout" mapstructure:"operation_timeout"`
}

type ConfigStoreConfig struct {
	Enabled                        bool          `yaml:"enabled" mapstructure:"enabled"`
	SourceOfTruth                  string        `yaml:"source_of_truth" mapstructure:"source_of_truth"`
	Backend                        string        `yaml:"backend" mapstructure:"backend"`
	BootstrapFromFile              bool          `yaml:"bootstrap_from_file" mapstructure:"bootstrap_from_file"`
	AutoReload                     bool          `yaml:"auto_reload" mapstructure:"auto_reload"`
	PollInterval                   time.Duration `yaml:"poll_interval" mapstructure:"poll_interval"`
	ActivationTimeout              time.Duration `yaml:"activation_timeout" mapstructure:"activation_timeout"`
	LastGoodCachePath              string        `yaml:"last_good_cache_path" mapstructure:"last_good_cache_path"`
	RequireValidationBeforePublish bool          `yaml:"require_validation_before_publish" mapstructure:"require_validation_before_publish"`
	RequireBaseVersionMatch        bool          `yaml:"require_base_version_match" mapstructure:"require_base_version_match"`
}

type ManagementConfig struct {
	Enabled     bool `mapstructure:"enabled"`
	AuthEnabled bool `mapstructure:"auth_enabled"`
}

type DatabaseConfig struct {
	Type string `mapstructure:"type"`
}

func NewDefaultConfig() (config *Gateway) {
	config = &Gateway{
		Routes:      Routing{},
		RoutesFiles: nil,
		Portal: PortalConfig{
			Enabled: true,
			Mode:    PortalModeReadOnly,
			Auth: PortalAuthConfig{
				ReadScopes: []string{"management:portal"},
			},
			Catalog: PortalCatalogConfig{
				ExposeTargetURLs:    true,
				ExposeOpenAPIErrors: true,
			},
			MetricsHistory: PortalMetricsHistoryConfig{
				Enabled:          true,
				Backend:          "local",
				SnapshotInterval: time.Minute,
				MaxSnapshots:     96,
				MaxAge:           24 * time.Hour,
				Redis: PortalMetricsHistoryRedisConfig{
					Prefix:           "nimburion:portal:metrics_history",
					MaxConns:         10,
					OperationTimeout: 5 * time.Second,
				},
			},
		},
		ConfigStore: ConfigStoreConfig{
			Enabled:                        false,
			SourceOfTruth:                  ConfigSourceOfTruthFile,
			Backend:                        "",
			BootstrapFromFile:              false,
			AutoReload:                     false,
			PollInterval:                   10 * time.Second,
			ActivationTimeout:              5 * time.Second,
			LastGoodCachePath:              "",
			RequireValidationBeforePublish: true,
			RequireBaseVersionMatch:        true,
		},
		ConfigDir: "",
	}
	return
}

func (cfg *Gateway) Validate() error {
	cfg.Portal.Mode = strings.ToLower(strings.TrimSpace(cfg.Portal.Mode))
	if cfg.Portal.Mode == "" {
		cfg.Portal.Mode = PortalModeReadOnly
	}
	if cfg.Portal.Mode != PortalModeReadOnly && cfg.Portal.Mode != PortalModeManaged {
		return fmt.Errorf("portal.mode must be %q or %q", PortalModeReadOnly, PortalModeManaged)
	}

	cfg.ConfigStore.SourceOfTruth = strings.ToLower(strings.TrimSpace(cfg.ConfigStore.SourceOfTruth))
	if cfg.ConfigStore.SourceOfTruth == "" {
		cfg.ConfigStore.SourceOfTruth = ConfigSourceOfTruthFile
	}
	if cfg.ConfigStore.SourceOfTruth != ConfigSourceOfTruthFile && cfg.ConfigStore.SourceOfTruth != ConfigSourceOfTruthDatabase {
		return fmt.Errorf("config_store.source_of_truth must be %q or %q", ConfigSourceOfTruthFile, ConfigSourceOfTruthDatabase)
	}

	cfg.ConfigStore.Backend = strings.ToLower(strings.TrimSpace(cfg.ConfigStore.Backend))
	cfg.Database.Type = strings.ToLower(strings.TrimSpace(cfg.Database.Type))

	cfg.Portal.Auth.ReadScopes = trimStrings(cfg.Portal.Auth.ReadScopes)
	cfg.Portal.Auth.WriteScopes = trimStrings(cfg.Portal.Auth.WriteScopes)
	cfg.Portal.Auth.PublishScopes = trimStrings(cfg.Portal.Auth.PublishScopes)
	cfg.Portal.Auth.RollbackScopes = trimStrings(cfg.Portal.Auth.RollbackScopes)
	cfg.ConfigStore.LastGoodCachePath = strings.TrimSpace(cfg.ConfigStore.LastGoodCachePath)
	cfg.Portal.MetricsHistory.Backend = strings.ToLower(strings.TrimSpace(cfg.Portal.MetricsHistory.Backend))
	if cfg.Portal.MetricsHistory.Backend == "" {
		cfg.Portal.MetricsHistory.Backend = "local"
	}
	cfg.Portal.MetricsHistory.Redis.URL = strings.TrimSpace(cfg.Portal.MetricsHistory.Redis.URL)
	cfg.Portal.MetricsHistory.Redis.Prefix = strings.TrimSpace(cfg.Portal.MetricsHistory.Redis.Prefix)
	if cfg.Portal.MetricsHistory.Redis.Prefix == "" {
		cfg.Portal.MetricsHistory.Redis.Prefix = "nimburion:portal:metrics_history"
	}
	if cfg.Portal.MetricsHistory.Enabled {
		if cfg.Portal.MetricsHistory.Backend != "local" && cfg.Portal.MetricsHistory.Backend != "redis" {
			return errors.New("portal.metrics_history.backend must be \"local\" or \"redis\" when metrics history is enabled")
		}
		if cfg.Portal.MetricsHistory.SnapshotInterval <= 0 {
			return errors.New("portal.metrics_history.snapshot_interval must be > 0 when metrics history is enabled")
		}
		if cfg.Portal.MetricsHistory.MaxSnapshots <= 0 {
			return errors.New("portal.metrics_history.max_snapshots must be > 0 when metrics history is enabled")
		}
		if cfg.Portal.MetricsHistory.MaxAge <= 0 {
			return errors.New("portal.metrics_history.max_age must be > 0 when metrics history is enabled")
		}
		if cfg.Portal.MetricsHistory.Backend == "redis" {
			if cfg.Portal.MetricsHistory.Redis.URL == "" {
				return errors.New("portal.metrics_history.redis.url is required when metrics history backend is redis")
			}
			if cfg.Portal.MetricsHistory.Redis.MaxConns <= 0 {
				return errors.New("portal.metrics_history.redis.max_conns must be > 0 when metrics history backend is redis")
			}
			if cfg.Portal.MetricsHistory.Redis.OperationTimeout <= 0 {
				return errors.New("portal.metrics_history.redis.operation_timeout must be > 0 when metrics history backend is redis")
			}
		}
	}

	for i, path := range cfg.RoutesFiles {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			return fmt.Errorf("routes_files[%d] cannot be empty", i)
		}
		cfg.RoutesFiles[i] = trimmed
	}

	switch cfg.ConfigStore.SourceOfTruth {
	case ConfigSourceOfTruthFile:
	case ConfigSourceOfTruthDatabase:
		if !cfg.ConfigStore.Enabled {
			return errors.New("config_store.enabled must be true when config_store.source_of_truth=database")
		}
		if cfg.ConfigStore.Backend != ConfigStoreBackendPostgres {
			return fmt.Errorf("config_store.backend must be %q when config_store.source_of_truth=database", ConfigStoreBackendPostgres)
		}
		if cfg.Database.Type != ConfigStoreBackendPostgres {
			return fmt.Errorf("database.type must be %q when config_store.source_of_truth=database", ConfigStoreBackendPostgres)
		}
	}

	if cfg.ConfigStore.Enabled {
		if cfg.ConfigStore.PollInterval <= 0 {
			return errors.New("config_store.poll_interval must be > 0 when config_store is enabled")
		}
		if cfg.ConfigStore.ActivationTimeout <= 0 {
			return errors.New("config_store.activation_timeout must be > 0 when config_store is enabled")
		}
	}

	if cfg.ConfigStore.AutoReload && cfg.ConfigStore.LastGoodCachePath == "" {
		return errors.New("config_store.last_good_cache_path is required when config_store.auto_reload=true")
	}

	if cfg.Portal.Mode == PortalModeManaged {
		if !cfg.Portal.Enabled {
			return errors.New("portal.enabled must be true when portal.mode=managed")
		}
		if !cfg.Management.Enabled {
			return errors.New("management.enabled must be true when portal.mode=managed")
		}
		if !cfg.Management.AuthEnabled {
			return errors.New("management.auth_enabled must be true when portal.mode=managed")
		}
		if len(cfg.Portal.Auth.WriteScopes) == 0 {
			return errors.New("portal.auth.write_scopes must not be empty when portal.mode=managed")
		}
		if len(cfg.Portal.Auth.PublishScopes) == 0 {
			return errors.New("portal.auth.publish_scopes must not be empty when portal.mode=managed")
		}
		if len(cfg.Portal.Auth.RollbackScopes) == 0 {
			return errors.New("portal.auth.rollback_scopes must not be empty when portal.mode=managed")
		}
	}

	return nil
}

func trimStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		candidate := strings.TrimSpace(value)
		if candidate == "" {
			continue
		}
		trimmed = append(trimmed, candidate)
	}
	return trimmed
}

func (cfg *Gateway) LoadRoutes(baseDir string, middlewareRegistry map[string]func() router.MiddlewareFunc) error {
	if len(cfg.RoutesFiles) == 0 && len(cfg.Routes.Groups) == 0 {
		if cfg.ConfigStore.SourceOfTruth == "" || cfg.ConfigStore.SourceOfTruth == ConfigSourceOfTruthFile {
			return errors.New("either routes_files or inline routes must be provided when config_store.source_of_truth=file")
		}
		return errors.New("routes configuration must define at least one group")
	}

	merged := cfg.Routes
	if merged.Groups == nil {
		merged.Groups = make(map[string]Group)
	}
	if len(cfg.RoutesFiles) > 0 {
		routing, err := LoadMultipleWithBaseDir(cfg.RoutesFiles, baseDir, middlewareRegistry)
		if err != nil {
			return err
		}
		for name, group := range routing.Groups {
			if existing, exists := merged.Groups[name]; exists {
				combined, ok := mergeOverlayGroup(existing, group)
				if !ok {
					return fmt.Errorf("duplicate group '%s' defined inline and in routes_files", name)
				}
				merged.Groups[name] = combined
				continue
			}
			merged.Groups[name] = group
		}
	}

	for name, group := range merged.Groups {
		if isOverlayGroup(group) {
			delete(merged.Groups, name)
		}
	}

	supportedMiddlewares := make(map[string]struct{}, len(middlewareRegistry))
	for name := range middlewareRegistry {
		supportedMiddlewares[name] = struct{}{}
	}

	normalized, err := validateAndNormalize(merged, supportedMiddlewares)
	if err != nil {
		return err
	}
	normalized, err = resolveRouteOpenAPIFiles(normalized, baseDir)
	if err != nil {
		return err
	}
	docCache, err := loadAndValidateRouteOpenAPIDocs(normalized)
	if err != nil {
		return err
	}
	if err := validateOpenAPIAlignment(normalized, docCache); err != nil {
		return err
	}

	cfg.Routes = normalized
	return nil
}

func resolveRouteOpenAPIFiles(routingCfg Routing, baseDir string) (Routing, error) {
	for groupName, group := range routingCfg.Groups {
		for routeIndex := range group.Routes {
			route := group.Routes[routeIndex]
			if route.OpenAPI == nil {
				continue
			}
			resolvedPath, err := ResolvePathWithBaseDir(route.OpenAPI.File, baseDir)
			if err != nil {
				return Routing{}, fmt.Errorf("resolve groups.%s.routes[%d].openapi.file: %w", groupName, routeIndex, err)
			}
			route.OpenAPI.ResolvedFile = resolvedPath
			group.Routes[routeIndex] = route
		}
		routingCfg.Groups[groupName] = group
	}

	return routingCfg, nil
}

func loadAndValidateRouteOpenAPIDocs(routingCfg Routing) (map[string]*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	docCache := make(map[string]*openapi3.T)
	for groupName, group := range routingCfg.Groups {
		for routeIndex, route := range group.Routes {
			if route.OpenAPI == nil {
				continue
			}
			specPath := route.OpenAPI.ResolvedFile
			if specPath == "" {
				specPath = route.OpenAPI.File
			}
			if specPath == "" {
				continue
			}
			if _, alreadyValidated := docCache[specPath]; alreadyValidated {
				continue
			}

			doc, err := loader.LoadFromFile(specPath)
			if err != nil {
				routeRef := fmt.Sprintf("groups.%s.routes[%d] (path_prefix=%s)", groupName, routeIndex, route.PathPrefix)
				if errors.Is(err, os.ErrNotExist) {
					return nil, fmt.Errorf("%s: OpenAPI file not found at %q (openapi.file). Relative paths are resolved from the config file directory", routeRef, specPath)
				}
				return nil, fmt.Errorf("%s: cannot load OpenAPI file %q: %w", routeRef, specPath, err)
			}
			if err := doc.Validate(context.Background()); err != nil {
				return nil, fmt.Errorf("groups.%s.routes[%d] (path_prefix=%s): OpenAPI file %q is invalid: %w", groupName, routeIndex, route.PathPrefix, specPath, err)
			}
			docCache[specPath] = doc
		}
	}

	return docCache, nil
}

func validateOpenAPIAlignment(routingCfg Routing, docCache map[string]*openapi3.T) error {
	for groupName, group := range routingCfg.Groups {
		for routeIndex, route := range group.Routes {
			if route.OpenAPI == nil {
				continue
			}
			specPath := route.OpenAPI.ResolvedFile
			if specPath == "" {
				specPath = route.OpenAPI.File
			}
			if specPath == "" {
				continue
			}

			doc, ok := docCache[specPath]
			if !ok {
				return fmt.Errorf("groups.%s.routes[%d] (path_prefix=%s): OpenAPI file %q was not loaded during validation", groupName, routeIndex, route.PathPrefix, specPath)
			}

			specOps := collectOpenAPISpecOperations(doc, route.PathPrefix)
			routeOps := collectRouteOperations(route)

			missing := diffOperations(routeOps, specOps)
			if len(missing) > 0 {
				return fmt.Errorf("groups.%s.routes[%d] (path_prefix=%s): OpenAPI spec missing operations: %s", groupName, routeIndex, route.PathPrefix, strings.Join(missing, ", "))
			}

			extra := diffOperations(specOps, routeOps)
			if len(extra) > 0 {
				return fmt.Errorf("groups.%s.routes[%d] (path_prefix=%s): OpenAPI spec defines operations not in routes: %s", groupName, routeIndex, route.PathPrefix, strings.Join(extra, ", "))
			}
		}
	}

	return nil
}

func collectRouteOperations(route Route) map[string]struct{} {
	ops := make(map[string]struct{})
	for _, endpoint := range route.Endpoints {
		fullPath := joinRoutePath(route.PathPrefix, endpoint.Path)
		openapiPath := normalizeOpenAPIPath(fullPath)
		for method := range endpoint.Methods {
			key := strings.ToUpper(method) + " " + openapiPath
			ops[key] = struct{}{}
		}
	}
	return ops
}

func collectOpenAPISpecOperations(doc *openapi3.T, pathPrefix string) map[string]struct{} {
	ops := make(map[string]struct{})
	if doc == nil || doc.Paths == nil {
		return ops
	}

	prefix := normalizeOpenAPIPath(pathPrefix)
	if prefix == "" {
		prefix = "/"
	}

	addOp := func(method, path string, op *openapi3.Operation) {
		if op == nil {
			return
		}
		key := strings.ToUpper(method) + " " + path
		ops[key] = struct{}{}
	}

	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		if !matchesOpenAPIPrefix(path, prefix) {
			continue
		}
		addOp("GET", path, item.Get)
		addOp("POST", path, item.Post)
		addOp("PUT", path, item.Put)
		addOp("PATCH", path, item.Patch)
		addOp("DELETE", path, item.Delete)
		addOp("HEAD", path, item.Head)
		addOp("OPTIONS", path, item.Options)
		addOp("TRACE", path, item.Trace)
	}

	return ops
}

func diffOperations(expected, actual map[string]struct{}) []string {
	out := make([]string, 0)
	for key := range expected {
		if _, ok := actual[key]; !ok {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func matchesOpenAPIPrefix(path, prefix string) bool {
	if prefix == "/" {
		return true
	}
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}

func joinRoutePath(prefix, suffix string) string {
	normalizedSuffix := strings.TrimSpace(suffix)
	if normalizedSuffix == "" || normalizedSuffix == "/" {
		return prefix
	}
	if prefix == "/" {
		return normalizedSuffix
	}
	return prefix + normalizedSuffix
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
		}
	}
	return strings.Join(parts, "/")
}

func mergeOverlayGroup(existing, incoming Group) (Group, bool) {
	switch {
	case isOverlayGroup(existing) && !isOverlayGroup(incoming):
		return applyGroupOverlay(incoming, existing), true
	case !isOverlayGroup(existing) && isOverlayGroup(incoming):
		return applyGroupOverlay(existing, incoming), true
	default:
		return Group{}, false
	}
}

func isOverlayGroup(group Group) bool {
	return group.Prefix == "" &&
		len(group.Middlewares) == 0 &&
		group.RateLimit == nil &&
		len(group.Routes) == 0 &&
		len(group.WebSockets) == 0 &&
		group.AuthEndpoints != nil
}

func applyGroupOverlay(base, overlay Group) Group {
	if overlay.AuthEndpoints == nil {
		return base
	}
	if base.AuthEndpoints == nil {
		base.AuthEndpoints = &AuthEndpoints{}
	}
	if overlay.AuthEndpoints.OAuth2 != nil {
		if base.AuthEndpoints.OAuth2 == nil {
			base.AuthEndpoints.OAuth2 = &OAuth2Config{}
		}
		if overlay.AuthEndpoints.OAuth2.ClientSecret != "" {
			base.AuthEndpoints.OAuth2.ClientSecret = overlay.AuthEndpoints.OAuth2.ClientSecret
		}
	}
	return base
}
