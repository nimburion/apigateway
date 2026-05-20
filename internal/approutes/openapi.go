package approutes

import (
	"fmt"
	"strings"

	baseconfig "github.com/nimburion/nimburion/pkg/config"
	httpopenapi "github.com/nimburion/nimburion/pkg/http/openapi"
	"github.com/nimburion/nimburion/pkg/http/router"
	"github.com/nimburion/nimburion/pkg/version"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
)

func BuildOpenAPISpec(cfg *baseconfig.Config, routeDefs gatewaycfg.Routing, middlewareRegistry map[string]func() router.MiddlewareFunc) (*httpopenapi.Spec, error) {
	var registerErr error
	configuredMethods := collectConfiguredMethods(routeDefs)
	routes := httpopenapi.CollectRoutes(func(r router.Router) {
		registerErr = Register(r, routeDefs, middlewareRegistry, nil)
	})
	if registerErr != nil {
		return nil, fmt.Errorf("register routes for openapi collection: %w", registerErr)
	}
	filtered := make([]httpopenapi.Route, 0, len(routes))
	for _, route := range routes {
		key := strings.ToUpper(strings.TrimSpace(route.Method)) + " " + normalizeConfiguredPath(route.Path)
		if configuredMethods[key] {
			filtered = append(filtered, route)
		}
	}
	return httpopenapi.BuildSpec(serviceName(cfg), version.AppVersion, filtered), nil
}

func serviceName(cfg *baseconfig.Config) string {
	if cfg != nil && cfg.App.Name != "" {
		return cfg.App.Name
	}
	return "api-gateway"
}

func collectConfiguredMethods(routeDefs gatewaycfg.Routing) map[string]bool {
	result := make(map[string]bool)
	for _, group := range routeDefs.Groups {
		groupPrefix := strings.TrimSpace(group.Prefix)
		if groupPrefix == "" {
			groupPrefix = "/"
		}

		if group.AuthEndpoints != nil && group.AuthEndpoints.Me {
			result["GET "+normalizeConfiguredPath(joinConfiguredPath(groupPrefix, "/auth/me"))] = true
		}

		if group.AuthEndpoints != nil && group.AuthEndpoints.OAuth2 != nil && group.AuthEndpoints.OAuth2.Enabled {
			for _, route := range []struct {
				method string
				path   string
			}{
				{method: "GET", path: "/auth/login"},
				{method: "GET", path: "/auth/callback"},
				{method: "POST", path: "/auth/logout"},
				{method: "POST", path: "/auth/refresh"},
			} {
				result[route.method+" "+normalizeConfiguredPath(joinConfiguredPath(groupPrefix, route.path))] = true
			}
		}

		for _, route := range group.Routes {
			for _, endpoint := range route.Endpoints {
				fullPath := joinConfiguredPath(groupPrefix, joinConfiguredPath(route.PathPrefix, endpoint.Path))
				for method := range endpoint.Methods {
					result[strings.ToUpper(strings.TrimSpace(method))+" "+normalizeConfiguredPath(fullPath)] = true
				}
			}
		}
	}
	return result
}

func joinConfiguredPath(prefix, suffix string) string {
	normalizedSuffix := strings.TrimSpace(suffix)
	if normalizedSuffix == "" || normalizedSuffix == "/" {
		return normalizeConfiguredPath(prefix)
	}
	normalizedPrefix := normalizeConfiguredPath(prefix)
	if normalizedPrefix == "/" {
		return normalizeConfiguredPath(normalizedSuffix)
	}
	return normalizeConfiguredPath(strings.TrimRight(normalizedPrefix, "/") + "/" + strings.TrimLeft(normalizedSuffix, "/"))
}

func normalizeConfiguredPath(path string) string {
	normalized := strings.TrimSpace(path)
	if normalized == "" {
		return "/"
	}
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	if len(normalized) > 1 {
		normalized = strings.TrimSuffix(normalized, "/")
	}
	if normalized == "" {
		return "/"
	}
	return normalized
}
