package approutes

import (
	"fmt"

	"github.com/nimburion/apigateway/internal/authn"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	authHendler "github.com/nimburion/apigateway/internal/handlers/auth"
	"github.com/nimburion/apigateway/internal/middleware"
	"github.com/nimburion/apigateway/internal/portalmeta"
	"github.com/nimburion/apigateway/internal/routing"
	httpopenapi "github.com/nimburion/nimburion/pkg/http/openapi"
	"github.com/nimburion/nimburion/pkg/http/router"
	logpkg "github.com/nimburion/nimburion/pkg/observability/logger"
)

func ValidateSupportedMethods(routeDefs gatewaycfg.Routing) error {
	for groupName, groupCfg := range routeDefs.Groups {
		for _, route := range groupCfg.Routes {
			for _, endpoint := range route.Endpoints {
				for method := range endpoint.Methods {
					switch method {
					case "GET", "POST", "PUT", "PATCH", "DELETE":
					default:
						return fmt.Errorf("group %q route %q endpoint %q uses unsupported HTTP method %q", groupName, route.PathPrefix, endpoint.Path, method)
					}
				}
			}
		}
	}
	return nil
}

// Register registers the gateway application routes on the provided router using
// the same route definitions and middleware resolution used by the runtime server.
func Register(r router.Router, routeDefs gatewaycfg.Routing, middlewareRegistry map[string]func() router.MiddlewareFunc, log logpkg.Logger) error {
	inheritedMiddlewareGroups := make(map[string]router.Router)

	for groupName, groupCfg := range routeDefs.Groups {
		// These two groups intentionally share the same URL prefix but not the same middleware role:
		// - inheritedMiddlewareGroup is the base router for proxied gateway traffic, where the
		//   configured group middleware chain is inherited later per route/endpoint/method.
		// - authEndpointGroup is used for built-in auth endpoints that should not inherit those
		//   user-declared gateway middleware chains implicitly.
		inheritedMiddlewareGroup := r.Group(groupCfg.Prefix)
		authEndpointGroup := r.Group(groupCfg.Prefix)

		groupMiddlewares, err := routing.BuildMiddlewareChain(groupCfg.Middlewares, middlewareRegistry)
		if err != nil {
			return fmt.Errorf("build middleware chain for group %q: %w", groupName, err)
		}

		if groupCfg.AuthEndpoints != nil {
			if groupCfg.AuthEndpoints.Me {
				meHandler := portalmeta.Annotate(
					httpopenapi.Annotate(authHendler.MeHandler, httpopenapi.EndpointAnnotations{
						Summary: "Authenticated user info",
						Tags:    []string{"auth"},
					}),
					portalmeta.AuthMe(),
				)
				inheritedMiddlewareGroup.GET("/auth/me", meHandler, groupMiddlewares...)
				if log != nil {
					log.Info("registered auth/me endpoint", "group", groupName, "endpoint", "/auth/me")
				}
			}

			if groupCfg.AuthEndpoints.OAuth2 != nil && groupCfg.AuthEndpoints.OAuth2.Enabled {
				authn.RegisterOAuth2Routes(authEndpointGroup, groupCfg.AuthEndpoints.OAuth2, log)
				if log != nil {
					log.Info("registered oauth2 endpoints", "group", groupName, "prefix", "/auth")
				}
			}
		}

		inheritedMiddlewareGroups[groupName] = inheritedMiddlewareGroup
	}

	for groupName, groupCfg := range routeDefs.Groups {
		inheritedMiddlewareGroup := inheritedMiddlewareGroups[groupName]

		for _, route := range groupCfg.Routes {
			if err := routing.RegisterProxyRouteWithMiddlewareRegistry(
				inheritedMiddlewareGroup,
				route,
				groupCfg.RateLimit,
				middleware.RateLimitKeyByTenantAndSubject,
				log,
				middlewareRegistry,
				groupCfg.Middlewares,
			); err != nil {
				return fmt.Errorf("register route %s: %w", route.PathPrefix, err)
			}

			if log != nil {
				log.Info(
					"registered route",
					"group", groupName,
					"path_prefix", route.PathPrefix,
					"target_url", route.TargetURL,
					"endpoints", len(route.Endpoints),
				)
			}
		}

		for _, ws := range groupCfg.WebSockets {
			routing.RegisterWebSocketRouteWithMiddlewareRegistry(
				inheritedMiddlewareGroup,
				ws,
				groupCfg.RateLimit,
				middleware.RateLimitKeyByTenantAndSubject,
				middlewareRegistry,
				groupCfg.Middlewares,
			)

			if log != nil {
				log.Info(
					"registered websocket",
					"group", groupName,
					"path", ws.Path,
					"target_url", ws.TargetURL,
				)
			}
		}
	}

	return nil
}
