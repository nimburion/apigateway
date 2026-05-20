package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/nimburion/apigateway/internal/approutes"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/apigateway/internal/configstore"
	"github.com/nimburion/apigateway/internal/handlers/portal"
	"github.com/nimburion/apigateway/internal/hotswap"
	"github.com/nimburion/apigateway/internal/routing"
	"github.com/nimburion/nimburion/pkg/auth"
	baseconfig "github.com/nimburion/nimburion/pkg/config"
	"github.com/nimburion/nimburion/pkg/http/authentication"
	"github.com/nimburion/nimburion/pkg/http/authorization"
	httpopenapi "github.com/nimburion/nimburion/pkg/http/openapi"
	"github.com/nimburion/nimburion/pkg/http/router"
	nethttprouter "github.com/nimburion/nimburion/pkg/http/router/nethttp"
	httpserver "github.com/nimburion/nimburion/pkg/http/server"
	logpkg "github.com/nimburion/nimburion/pkg/observability/logger"
	frameworkmetrics "github.com/nimburion/nimburion/pkg/observability/metrics"
	"gopkg.in/yaml.v3"
)

var ErrNilConfig = errors.New("nil config provided")
var runHTTPServersWithSignals = httpserver.RunHTTPServersWithSignals
var newJWTValidator = func(cfg *baseconfig.Config, log logpkg.Logger) auth.JWTValidator {
	jwksClient := auth.NewJWKSClient(cfg.Auth.JWKSUrl, cfg.Auth.JWKSCacheTTL, log)
	return auth.NewJWKSValidator(
		jwksClient,
		cfg.Auth.Issuer,
		cfg.Auth.Audience,
		log,
		auth.WithClaimMappings(cfg.Auth.Claims.Mappings),
	)
}

// RunServer starts the API gateway using the provided configs and log.
func RunServer(cfg *baseconfig.Config, gwCfg *gatewaycfg.Gateway, log logpkg.Logger) error {
	if cfg == nil || gwCfg == nil {
		return ErrNilConfig
	}

	var validator auth.JWTValidator
	if cfg.Auth.Enabled {
		validator = newJWTValidator(cfg, log)
	}

	middlewareRegistry := map[string]func() router.MiddlewareFunc{
		"Authenticate": func() router.MiddlewareFunc {
			return authentication.Authenticate(validator)
		},
		"ClaimsGuardFromConfig": func() router.MiddlewareFunc {
			return authorization.ClaimsGuardFromConfig(cfg.Auth)
		},
		"ForwardIdentityHeaders": func() router.MiddlewareFunc {
			return authentication.ForwardIdentityHeaders(authentication.IdentityHeaderConfig{})
		},
	}

	baseDir := gwCfg.ConfigDir
	if baseDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			baseDir = cwd
		}
	}
	databaseConfigStore := gwCfg.ConfigStore.Enabled && gwCfg.ConfigStore.SourceOfTruth == gatewaycfg.ConfigSourceOfTruthDatabase

	var publicRouter router.Router
	var dynamicRouter *hotswap.Router
	var configRuntime *configstore.Runtime
	var configStore configstore.Store
	if databaseConfigStore {
		dynamicRouter = hotswap.NewRouter()
		publicRouter = dynamicRouter
	} else {
		publicRouter = nethttprouter.NewRouter()
	}
	publicRouter.Use(securityFailureLoggingMiddleware(log, "public"))

	var managementRouter router.Router
	if cfg.Management.Enabled {
		mgmt := nethttprouter.NewRouter()
		mgmt.Use(securityFailureLoggingMiddleware(log, "management"))
		managementRouter = mgmt
	}

	httpServerConfig := httpserver.NewDefaultRunHTTPServersOptions()
	httpServerConfig.Config = cfg
	httpServerConfig.Logger = log
	httpServerConfig.PublicRouter = publicRouter
	httpServerConfig.ManagementRouter = managementRouter
	httpServerConfig.ManagementJWTValidator = validator
	metricsRegistry := frameworkmetrics.NewRegistry()
	httpServerConfig.MetricsRegistry = metricsRegistry
	httpServerConfig.PublicRouter = newNotFoundLoggingRouterWithMetrics(httpServerConfig.PublicRouter, log, "public", metricsRegistry)
	if httpServerConfig.ManagementRouter != nil {
		httpServerConfig.ManagementRouter = newNotFoundLoggingRouterWithMetrics(httpServerConfig.ManagementRouter, log, "management", metricsRegistry)
	}

	if databaseConfigStore {
		store, err := configstore.NewPostgresStore(*cfg)
		if err != nil {
			return fmt.Errorf("initialize config store: %w", err)
		}
		configStore = store
		httpServerConfig.ShutdownHooks = append(httpServerConfig.ShutdownHooks, httpserver.LifecycleHook{
			Name: "config_store_close",
			Fn: func(context.Context) error {
				return store.Close()
			},
		})
		storeReady := true
		if err := configStore.EnsureSchema(context.Background()); err != nil {
			if gwCfg.ConfigStore.LastGoodCachePath == "" {
				return err
			}
			storeReady = false
			log.Warn("failed to ensure config store schema; will try last-good cache", "error", err)
		}
		if storeReady && gwCfg.ConfigStore.BootstrapFromFile {
			if err := bootstrapConfigStore(context.Background(), configStore, gwCfg, baseDir, middlewareRegistry); err != nil {
				return err
			}
		}
		configRuntime = configstore.NewRuntime(configStore, dynamicRouter, middlewareRegistry, baseDir, gwCfg.ConfigStore.LastGoodCachePath, log)
		if err := configRuntime.LoadAndActivate(context.Background()); err != nil {
			return fmt.Errorf("load active gateway config: %w", err)
		}
		if err := validateAuthRequirements(cfg, configRuntime.CurrentRoutes(), validator); err != nil {
			return err
		}
		if gwCfg.ConfigStore.AutoReload {
			httpServerConfig.StartupHooks = append(httpServerConfig.StartupHooks, httpserver.LifecycleHook{
				Name: "config_store_poll",
				Fn: func(ctx context.Context) error {
					go func() {
						if err := configRuntime.StartPolling(ctx, gwCfg.ConfigStore.PollInterval); err != nil && log != nil {
							log.Error("config store polling stopped", "error", err)
						}
					}()
					return nil
				},
			})
		}
	} else {
		if err := gwCfg.LoadRoutes(baseDir, middlewareRegistry); err != nil {
			return fmt.Errorf("failed to load gateway routes: %w", err)
		}
		if err := validateAuthRequirements(cfg, gwCfg.Routes, validator); err != nil {
			return err
		}
	}
	routeDefs := gwCfg.Routes
	currentRoutes := func() gatewaycfg.Routing {
		if configRuntime != nil {
			return configRuntime.CurrentRoutes()
		}
		return routeDefs
	}

	var metricsHistoryStore portal.MetricsHistoryStore
	if cfg.Management.Enabled && gwCfg.Portal.Enabled && gwCfg.Portal.MetricsHistory.Enabled {
		switch gwCfg.Portal.MetricsHistory.Backend {
		case "redis":
			serviceName := strings.TrimSpace(cfg.App.Name)
			if serviceName == "" {
				serviceName = "gateway"
			}
			instanceID, _ := os.Hostname()
			if strings.TrimSpace(instanceID) == "" {
				instanceID = fmt.Sprintf("pid-%d", os.Getpid())
			}
			store, storeErr := portal.NewRedisMetricsHistoryStore(gwCfg.Portal.MetricsHistory, serviceName, instanceID)
			if storeErr != nil {
				return fmt.Errorf("initialize portal metrics history redis store: %w", storeErr)
			}
			metricsHistoryStore = store
			httpServerConfig.ShutdownHooks = append(httpServerConfig.ShutdownHooks, httpserver.LifecycleHook{
				Name: "portal_metrics_history_redis_close",
				Fn: func(context.Context) error {
					return store.Close()
				},
			})
		default:
			metricsHistoryStore = portal.NewLocalMetricsHistoryStore(gwCfg.Portal.MetricsHistory)
		}
		if collector := portal.NewMetricsHistoryCollector(metricsRegistry, metricsHistoryStore); collector != nil {
			httpServerConfig.StartupHooks = append(httpServerConfig.StartupHooks, httpserver.LifecycleHook{
				Name: "portal_metrics_history",
				Fn: func(ctx context.Context) error {
					return collector.Start(ctx)
				},
			})
		}
	}

	httpServers, err := httpserver.BuildHTTPServers(httpServerConfig)
	if err != nil {
		log.Error("failed to build HTTP servers", "error", err)
		return err
	}

	if err := approutes.ValidateSupportedMethods(currentRoutes()); err != nil {
		log.Error("failed to validate application routes", "error", err)
		return err
	}
	if !databaseConfigStore {
		if err := approutes.Register(publicRouter, routeDefs, middlewareRegistry, log); err != nil {
			log.Error("failed to register application routes", "error", err)
			return err
		}
	}

	if httpServers.Management != nil && gwCfg.Portal.Enabled {
		collected := httpopenapi.CollectRoutes(func(r router.Router) {
			_ = approutes.Register(r, currentRoutes(), middlewareRegistry, nil)
		})
		collectedMetadata := approutes.CollectRuntimeRoutes(func(r router.Router) {
			_ = approutes.Register(r, currentRoutes(), middlewareRegistry, nil)
		})
		metadataByKey := make(map[string]gatewaycfg.ResourceMetadata, len(collectedMetadata))
		authByKey := make(map[string]bool, len(collectedMetadata))
		scopesByKey := make(map[string][]string, len(collectedMetadata))
		rateLimitByKey := make(map[string]bool, len(collectedMetadata))
		for _, route := range collectedMetadata {
			key := strings.ToUpper(strings.TrimSpace(route.Method)) + " " + strings.TrimSpace(route.Path)
			metadataByKey[key] = route.Metadata
			authByKey[key] = route.AuthRequired
			scopesByKey[key] = append([]string(nil), route.Scopes...)
			rateLimitByKey[key] = route.HasRateLimit
		}
		runtimeRoutes := make([]portal.RuntimeRoute, 0, len(collected))
		for _, route := range collected {
			key := strings.ToUpper(strings.TrimSpace(route.Method)) + " " + strings.TrimSpace(route.Path)
			runtimeRoutes = append(runtimeRoutes, portal.RuntimeRoute{
				Route:          route,
				Metadata:       metadataByKey[key],
				AuthRequired:   authByKey[key],
				Scopes:         append([]string(nil), scopesByKey[key]...),
				HasRateLimit:   rateLimitByKey[key],
				SurfaceContext: "public",
			})
		}
		runtimeRoutes = append(runtimeRoutes, buildManagementRuntimeRoutes(cfg)...)
		portalHandler, err := portal.NewPortalHandler(&routeDefs, &gwCfg.Portal, &portal.RuntimeInfo{
			AuthEnabled:           cfg.Auth.Enabled,
			ManagementEnabled:     cfg.Management.Enabled,
			ManagementAuthEnabled: cfg.Management.AuthEnabled,
			PortalMode:            gwCfg.Portal.Mode,
			FrameworkMiddlewares:  publicFrameworkMiddlewares(cfg),
		}, metricsHistoryStore, runtimeRoutes...)
		if err != nil {
			return fmt.Errorf("initialize developer portal handler: %w", err)
		}
		portalHandler.SetRouteConfigProvider(currentRoutes)

		mgmtRouter := httpServers.Management.Router()
		portalSecurity := managementSecurityMiddleware(cfg.Management.AuthEnabled, validator, "management:portal")
		mgmtRouter.GET("/portal", portalHandler.GetPortalHTML, portalSecurity...)
		mgmtRouter.GET("/portal/", portalHandler.GetPortalHTML, portalSecurity...)
		mgmtRouter.GET("/portal/groups", portalHandler.GetPortalHTML, portalSecurity...)
		mgmtRouter.GET("/portal/groups/:group", portalHandler.GetPortalHTML, portalSecurity...)
		mgmtRouter.GET("/portal/posture", portalHandler.GetPortalHTML, portalSecurity...)
		mgmtRouter.GET("/portal/posture/:group", portalHandler.GetPortalHTML, portalSecurity...)
		mgmtRouter.GET("/portal/posture/:group/route/:method/:surface", portalHandler.GetPortalHTML, portalSecurity...)
		mgmtRouter.GET("/portal/posture/:group/websocket/:surface", portalHandler.GetPortalHTML, portalSecurity...)
		mgmtRouter.GET("/portal/admin", portalHandler.GetPortalHTML, portalSecurity...)
		mgmtRouter.GET("/portal/assets/:file", portalHandler.GetPortalHTML, portalSecurity...)
		mgmtRouter.GET("/portal/:file", portalHandler.GetPortalHTML, portalSecurity...)
		mgmtRouter.GET("/api/portal/routes", portalHandler.GetRoutes, portalSecurity...)
		mgmtRouter.GET("/api/portal/groups", portalHandler.GetGroups, portalSecurity...)
		mgmtRouter.GET("/api/portal/v1/catalog/routes", portalHandler.GetRoutes, portalSecurity...)
		mgmtRouter.GET("/api/portal/v1/catalog/groups", portalHandler.GetGroups, portalSecurity...)
		mgmtRouter.GET("/api/portal/v1/catalog/summary", portalHandler.GetSummary, portalSecurity...)
		mgmtRouter.GET("/api/portal/metrics/history", portalHandler.GetMetricsHistory, portalSecurity...)
		mgmtRouter.GET("/api/portal/openapi.yaml", func(c router.Context) error {
			spec, err := approutes.BuildOpenAPISpec(cfg, currentRoutes(), middlewareRegistry)
			if err != nil {
				return fmt.Errorf("build runtime openapi spec: %w", err)
			}
			payload, err := yaml.Marshal(spec)
			if err != nil {
				return fmt.Errorf("marshal runtime openapi spec: %w", err)
			}
			c.Response().Header().Set("Cache-Control", "no-store, private")
			c.Response().Header().Set("Content-Type", "application/yaml; charset=utf-8")
			c.Response().Header().Set("Content-Disposition", `inline; filename="openapi.yaml"`)
			return c.String(http.StatusOK, string(payload))
		}, portalSecurity...)
		if configRuntime != nil && configStore != nil && gwCfg.Portal.Mode == gatewaycfg.PortalModeManaged {
			configHandler := configstore.NewHandler(configStore, configRuntime, configstore.HandlerOptions{
				RequireValidationBeforePublish: gwCfg.ConfigStore.RequireValidationBeforePublish,
				RequireBaseVersionMatch:        gwCfg.ConfigStore.RequireBaseVersionMatch,
			})
			readSecurity := managementSecurityMiddleware(cfg.Management.AuthEnabled, validator, gwCfg.Portal.Auth.ReadScopes...)
			writeSecurity := managementSecurityMiddleware(cfg.Management.AuthEnabled, validator, gwCfg.Portal.Auth.WriteScopes...)
			publishSecurity := managementSecurityMiddleware(cfg.Management.AuthEnabled, validator, gwCfg.Portal.Auth.PublishScopes...)
			rollbackSecurity := managementSecurityMiddleware(cfg.Management.AuthEnabled, validator, gwCfg.Portal.Auth.RollbackScopes...)
			mgmtRouter.GET("/api/portal/v1/config/active", configHandler.GetConfig, readSecurity...)
			mgmtRouter.GET("/api/portal/v1/config/versions", configHandler.ListVersions, readSecurity...)
			mgmtRouter.GET("/api/portal/v1/config/versions/:version", configHandler.GetVersion, readSecurity...)
			mgmtRouter.GET("/api/portal/v1/config/drafts", configHandler.ListDrafts, readSecurity...)
			mgmtRouter.POST("/api/portal/v1/config/drafts", configHandler.CreateDraft, writeSecurity...)
			mgmtRouter.GET("/api/portal/v1/config/drafts/:version", configHandler.GetVersion, readSecurity...)
			mgmtRouter.PUT("/api/portal/v1/config/drafts/:version", configHandler.UpdateDraft, writeSecurity...)
			mgmtRouter.POST("/api/portal/v1/config/drafts/:version/validate", configHandler.ValidateDraft, writeSecurity...)
			mgmtRouter.POST("/api/portal/v1/config/drafts/:version/publish", configHandler.PublishDraft, publishSecurity...)
			mgmtRouter.POST("/api/portal/v1/config/versions/:version/rollback", configHandler.Rollback, rollbackSecurity...)
			mgmtRouter.GET("/api/portal/v1/audit-events", configHandler.ListAuditEvents, readSecurity...)
		}
		log.Info("registered posture portal on management server", "path", "/portal")
	}

	return runHTTPServersWithSignals(httpServers, httpServerConfig)
}

func publicFrameworkMiddlewares(cfg *baseconfig.Config) []string {
	if cfg == nil {
		cfg = baseconfig.DefaultConfig()
	}

	names := []string{
		"request_id",
		"http_signature",
		"security_headers",
		"session",
		"csrf",
		"cors",
		"i18n",
		"logging",
		"recovery",
		"metrics",
	}
	if cfg.Observability.TracingEnabled && cfg.Observability.RequestTracing.Enabled {
		names = append(names, "tracing")
	}
	names = append(names, "timeout", "request_size")
	return names
}

func buildManagementRuntimeRoutes(cfg *baseconfig.Config) []portal.RuntimeRoute {
	routes := []portal.RuntimeRoute{
		newManagementRuntimeRoute(http.MethodGet, "/health", "Management health probe", nil),
		newManagementRuntimeRoute(http.MethodGet, "/ready", "Management readiness probe", managementScopes(cfg, "management:read")),
		newManagementRuntimeRoute(http.MethodGet, "/metrics", "Management metrics", managementScopes(cfg, "management:metrics")),
		newManagementRuntimeRoute(http.MethodGet, "/portal", "Posture Portal", managementScopes(cfg, "management:portal")),
		newManagementRuntimeRoute(http.MethodGet, "/api/portal/routes", "Posture Portal routes catalog (legacy)", managementScopes(cfg, "management:portal")),
		newManagementRuntimeRoute(http.MethodGet, "/api/portal/groups", "Posture Portal groups catalog (legacy)", managementScopes(cfg, "management:portal")),
		newManagementRuntimeRoute(http.MethodGet, "/api/portal/metrics/history", "Posture Portal metrics history", managementScopes(cfg, "management:portal")),
		newManagementRuntimeRoute(http.MethodGet, "/api/portal/openapi.yaml", "Posture Portal OpenAPI spec", managementScopes(cfg, "management:portal")),
	}
	sortManagementRuntimeRoutes(routes)
	return routes
}

func newManagementRuntimeRoute(method, path, summary string, scopes []string) portal.RuntimeRoute {
	return portal.RuntimeRoute{
		Route: httpopenapi.Route{
			Method: method,
			Path:   path,
			Annotations: httpopenapi.EndpointAnnotations{
				Summary: summary,
				Tags:    []string{"management"},
			},
		},
		Metadata: gatewaycfg.ResourceMetadata{
			OwnerTeam:      "platform",
			Domain:         "management",
			Visibility:     gatewaycfg.MetadataVisibilityInternal,
			Status:         gatewaycfg.MetadataStatusActive,
			SupportChannel: "#api-platform",
		},
		AuthRequired:   len(scopes) > 0,
		Scopes:         append([]string(nil), scopes...),
		GroupName:      "__management__",
		GroupPrefix:    "/",
		SurfaceContext: "management",
	}
}

func managementScopes(cfg *baseconfig.Config, scope string) []string {
	if cfg == nil || !cfg.Management.AuthEnabled || strings.TrimSpace(scope) == "" {
		return nil
	}
	return []string{scope}
}

func sortManagementRuntimeRoutes(routes []portal.RuntimeRoute) {
	slices.SortFunc(routes, func(left, right portal.RuntimeRoute) int {
		if left.Route.Path == right.Route.Path {
			return strings.Compare(left.Route.Method, right.Route.Method)
		}
		return strings.Compare(left.Route.Path, right.Route.Path)
	})
}

func serviceName(cfg *baseconfig.Config) string {
	if cfg != nil && cfg.App.Name != "" {
		return cfg.App.Name
	}
	return "api-gateway"
}

func bootstrapConfigStore(ctx context.Context, store configstore.Store, gwCfg *gatewaycfg.Gateway, baseDir string, middlewareRegistry map[string]func() router.MiddlewareFunc) error {
	if _, err := store.Active(ctx); err == nil {
		return nil
	} else if !errors.Is(err, configstore.ErrNoActiveConfig) {
		return fmt.Errorf("check active gateway config: %w", err)
	}

	bootstrap := *gwCfg
	bootstrap.ConfigStore.SourceOfTruth = gatewaycfg.ConfigSourceOfTruthFile
	if err := bootstrap.LoadRoutes(baseDir, middlewareRegistry); err != nil {
		return fmt.Errorf("bootstrap config store from file: %w", err)
	}
	draft, err := store.SaveDraft(ctx, configstore.DraftInput{
		Routes:    bootstrap.Routes,
		CreatedBy: "system",
		Message:   "bootstrap from file",
	})
	if err != nil {
		return fmt.Errorf("save bootstrap gateway config draft: %w", err)
	}
	if _, err := store.Publish(ctx, draft.Version, configstore.PublishOptions{Actor: "system"}); err != nil {
		return fmt.Errorf("publish bootstrap gateway config: %w", err)
	}
	return nil
}

func managementSecurityMiddleware(authEnabled bool, validator auth.JWTValidator, scopes ...string) []router.MiddlewareFunc {
	if !authEnabled {
		return nil
	}
	return []router.MiddlewareFunc{
		authentication.Authenticate(validator),
		authorization.RequireScopes(scopes...),
	}
}

func validateAuthRequirements(cfg *baseconfig.Config, routeDefs gatewaycfg.Routing, validator auth.JWTValidator) error {
	service := serviceName(cfg)
	if cfg != nil && cfg.Management.AuthEnabled && validator == nil {
		return fmt.Errorf("%s requires auth.enabled=true when management.auth_enabled=true", service)
	}

	if validator != nil {
		return nil
	}

	for groupName, group := range routeDefs.Groups {
		if group.AuthEndpoints != nil && group.AuthEndpoints.Me {
			if required := firstAuthMiddleware(group.Middlewares); required != "" {
				return fmt.Errorf("%s auth/me in group %q requires auth.enabled=true because it uses middleware %q", service, groupName, required)
			}
		}

		for _, route := range group.Routes {
			routeMiddlewares := routing.ApplyMiddlewareDirectives(group.Middlewares, route.Middlewares, route.DisableMiddlewares)
			if required := firstAuthMiddleware(routeMiddlewares); required != "" {
				return fmt.Errorf("%s route %q in group %q requires auth.enabled=true because it uses middleware %q", service, route.PathPrefix, groupName, required)
			}
			for _, endpoint := range route.Endpoints {
				endpointMiddlewares := routing.ApplyMiddlewareDirectives(routeMiddlewares, endpoint.Middlewares, endpoint.DisableMiddlewares)
				for methodName, method := range endpoint.Methods {
					methodMiddlewares := routing.ApplyMiddlewareDirectives(endpointMiddlewares, method.Middlewares, method.DisableMiddlewares)
					if required := firstAuthMiddleware(methodMiddlewares); required != "" {
						return fmt.Errorf("%s route %q %s %s requires auth.enabled=true because it uses middleware %q", service, route.PathPrefix, methodName, endpoint.Path, required)
					}
					if len(method.Scopes) > 0 {
						return fmt.Errorf("%s route %q %s %s requires auth.enabled=true because it defines scopes", service, route.PathPrefix, methodName, endpoint.Path)
					}
				}
			}
		}

		for _, ws := range group.WebSockets {
			wsMiddlewares := routing.ApplyMiddlewareDirectives(group.Middlewares, ws.Middlewares, ws.DisableMiddlewares)
			if required := firstAuthMiddleware(wsMiddlewares); required != "" {
				return fmt.Errorf("%s websocket %q requires auth.enabled=true because it uses middleware %q", service, ws.Path, required)
			}
			if len(ws.Scopes) > 0 {
				return fmt.Errorf("%s websocket %q requires auth.enabled=true because it defines scopes", service, ws.Path)
			}
		}
	}

	return nil
}

func firstAuthMiddleware(names []string) string {
	for _, name := range names {
		switch name {
		case "Authenticate", "ClaimsGuardFromConfig":
			return name
		}
	}
	return ""
}

func securityFailureLoggingMiddleware(log logpkg.Logger, surface string) router.MiddlewareFunc {
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			err := next(c)
			if log == nil || !c.Response().Written() {
				return err
			}
			status := c.Response().Status()
			if status != http.StatusUnauthorized && status != http.StatusForbidden {
				return err
			}
			log.Warn(
				"security request denied",
				"surface", surface,
				"method", c.Request().Method,
				"path", c.Request().URL.Path,
				"status", status,
				"authorization_header_present", c.Request().Header.Get("Authorization") != "",
			)
			return err
		}
	}
}
