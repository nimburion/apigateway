package server

import (
	"errors"
	"fmt"
	"os"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	authHendler "github.com/nimburion/apigateway/internal/handlers/auth"
	"github.com/nimburion/apigateway/internal/handlers/portal"
	"github.com/nimburion/apigateway/internal/middleware"
	"github.com/nimburion/apigateway/internal/routing"
	baseconfig "github.com/nimburion/nimburion/pkg/config"
	logpkg "github.com/nimburion/nimburion/pkg/observability/logger"

	"github.com/nimburion/apigateway/internal/authn"
	"github.com/nimburion/nimburion/pkg/auth"
	"github.com/nimburion/nimburion/pkg/middleware/authz"
	"github.com/nimburion/nimburion/pkg/server"
	"github.com/nimburion/nimburion/pkg/server/router"
)

var ErrNilConfig = errors.New("nil config provided")
var runHTTPServersWithSignals = server.RunHTTPServersWithSignals

// RunServer starts the API gateway using the provided configs and log.
func RunServer(cfg *baseconfig.Config, gwCfg *gatewaycfg.Gateway, log logpkg.Logger) error {
	if cfg == nil || gwCfg == nil {
		return ErrNilConfig
	}

	if !cfg.Auth.Enabled {
		return fmt.Errorf("%s requires auth.enabled=true", serviceName(cfg))
	}

	jwksClient := auth.NewJWKSClient(cfg.Auth.JWKSUrl, cfg.Auth.JWKSCacheTTL, log)
	validator := auth.NewJWKSValidator(
		jwksClient,
		cfg.Auth.Issuer,
		cfg.Auth.Audience,
		log,
		auth.WithClaimMappings(cfg.Auth.Claims.Mappings),
	)

	middlewareRegistry := map[string]func() router.MiddlewareFunc{
		"Authenticate": func() router.MiddlewareFunc {
			return authz.Authenticate(validator)
		},
		"ForwardIdentityHeaders": func() router.MiddlewareFunc {
			return authz.ForwardIdentityHeaders(authz.IdentityHeaderConfig{})
		},
	}

	baseDir := gwCfg.ConfigDir
	if baseDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			baseDir = cwd
		}
	}
	if err := gwCfg.LoadRoutes(baseDir, middlewareRegistry); err != nil {
		return fmt.Errorf("failed to load gateway routes: %w", err)
	}
	routeDefs := gwCfg.Routes

	httpServerConfig := server.NewDefaultRunHTTPServersOptions()
	httpServerConfig.Config = cfg
	httpServerConfig.Logger = log
	httpServerConfig.ManagementJWTValidator = validator
	httpServers, err := server.BuildHTTPServers(httpServerConfig)
	if err != nil {
		log.Error(err.Error())
	}
	r := *httpServers.Public.Router()

	// Build route groups from configuration
	routeGroups := make(map[string]router.Router)

	for groupName, groupCfg := range routeDefs.Groups {
		group := r.Group(groupCfg.Prefix)
		authGroup := r.Group(groupCfg.Prefix)

		// Apply group middlewares
		for _, mwName := range groupCfg.Middlewares {
			mwFactory, ok := middlewareRegistry[mwName]
			if !ok {
				return fmt.Errorf("unknown middleware: %s", mwName)
			}
			group.Use(mwFactory())
		}

		// Register auth endpoints if configured
		if groupCfg.AuthEndpoints != nil {
			if groupCfg.AuthEndpoints.Me {
				group.GET("/auth/me", authHendler.MeHandler)
				log.Info("registered auth/me endpoint", "group", groupName, "endpoint", "/auth/me")
			}

			if groupCfg.AuthEndpoints != nil && groupCfg.AuthEndpoints.OAuth2 != nil && groupCfg.AuthEndpoints.OAuth2.Enabled {
				authn.RegisterOAuth2Routes(authGroup, groupCfg.AuthEndpoints.OAuth2, log)
				log.Info("registered oauth2 endpoints", "group", groupName, "prefix", "/auth")
			}
		}

		routeGroups[groupName] = group
	}

	// Register proxy routes
	for groupName, groupCfg := range routeDefs.Groups {
		group := routeGroups[groupName]
		for _, route := range groupCfg.Routes {
			if err := routing.RegisterProxyRoute(group, route, groupCfg.RateLimit, middleware.RateLimitKeyByTenantAndSubject, log); err != nil {
				return fmt.Errorf("register route %s: %w", route.PathPrefix, err)
			}
			log.Info(
				"registered route",
				"group", groupName,
				"path_prefix", route.PathPrefix,
				"target_url", route.TargetURL,
				"endpoints", len(route.Endpoints),
			)
		}
		for _, ws := range groupCfg.WebSockets {
			routing.RegisterWebSocketRoute(group, ws, groupCfg.RateLimit, middleware.RateLimitKeyByTenantAndSubject)
			log.Info(
				"registered websocket",
				"group", groupName,
				"path", ws.Path,
				"target_url", ws.TargetURL,
			)
		}
	}

	// Register developer portal on management server
	if httpServers.Management != nil {
		portalHandler, err := portal.NewPortalHandler(&routeDefs)
		if err != nil {
			return fmt.Errorf("initialize developer portal handler: %w", err)
		}
		mgmtRouter := httpServers.Management.Router()
		portalSecurity := managementSecurityMiddleware(cfg.Management.AuthEnabled, validator, "management:portal")
		mgmtRouter.GET("/portal", portalHandler.GetPortalHTML, portalSecurity...)
		securedAssetChain := append([]router.MiddlewareFunc{}, portalSecurity...)
		securedAssetChain = append(securedAssetChain, portalHandler.AssetCacheMiddleware(), portalHandler.StaticMiddleware())
		mgmtRouter.GET("/portal/*path", portalHandler.GetPortalHTML, securedAssetChain...)
		mgmtRouter.GET("/api/portal/routes", portalHandler.GetRoutes)
		mgmtRouter.GET("/api/portal/groups", portalHandler.GetGroups)
		log.Info("registered developer portal on management server", "path", "/portal")
	}

	return runHTTPServersWithSignals(httpServers, httpServerConfig)
}

func serviceName(cfg *baseconfig.Config) string {
	if cfg != nil && cfg.Service.Name != "" {
		return cfg.Service.Name
	}
	return "api-gateway"
}

func managementSecurityMiddleware(authEnabled bool, validator auth.JWTValidator, scopes ...string) []router.MiddlewareFunc {
	if !authEnabled {
		return nil
	}
	return []router.MiddlewareFunc{
		authz.Authenticate(validator),
		authz.RequireScopes(scopes...),
	}
}
