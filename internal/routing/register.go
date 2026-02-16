package routing

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/apigateway/internal/proxy"
	"github.com/nimburion/nimburion/pkg/middleware/authz"
	openapimiddleware "github.com/nimburion/nimburion/pkg/middleware/openapivalidation"
	"github.com/nimburion/nimburion/pkg/middleware/ratelimit"
	logpkg "github.com/nimburion/nimburion/pkg/observability/logger"
	"github.com/nimburion/nimburion/pkg/server/router"
)

var proxySupportedMethods = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
}

var newOpenAPIRequestValidationMiddleware = openapimiddleware.NewRequestValidationMiddleware

func RegisterProxyRoute(r router.Router, route config.Route, groupRateLimit *config.RateLimit, keyFunc func(c router.Context) string, log logpkg.Logger) error {
	target, err := url.Parse(route.TargetURL)
	if err != nil {
		panic(fmt.Errorf("invalid target url for %s: %w", route.PathPrefix, err))
	}

	stripPrefix := strings.TrimSpace(route.StripPrefix)
	proxy := httputil.NewSingleHostReverseProxy(target)

	handler := func(c router.Context) error {
		if stripPrefix != "" {
			req := c.Request()
			if strings.HasPrefix(req.URL.Path, stripPrefix) {
				rewritten := strings.TrimPrefix(req.URL.Path, stripPrefix)
				if rewritten == "" {
					rewritten = "/"
				}
				req.URL.Path = rewritten
				if req.URL.RawPath != "" {
					req.URL.RawPath = rewritten
				}
			}
		}
		proxy.ServeHTTP(c.Response(), c.Request())
		return nil
	}

	effectiveRouteRateLimit := groupRateLimit
	if route.RateLimit != nil {
		effectiveRouteRateLimit = route.RateLimit
	}

	var routeRateLimitMiddleware router.MiddlewareFunc
	if effectiveRouteRateLimit != nil {
		routeLimiter := ratelimit.NewTokenBucketLimiter(effectiveRouteRateLimit.RequestsPerSecond, effectiveRouteRateLimit.Burst)
		routeRateLimitMiddleware = ratelimit.RateLimit(routeLimiter, ratelimit.RateLimitConfig{
			RequestsPerSecond: effectiveRouteRateLimit.RequestsPerSecond,
			Burst:             effectiveRouteRateLimit.Burst,
			KeyFunc:           keyFunc,
		})
	}

	var openAPIValidationMiddleware router.MiddlewareFunc
	if route.OpenAPI != nil {
		specPath := strings.TrimSpace(route.OpenAPI.ResolvedFile)
		if specPath == "" {
			specPath = strings.TrimSpace(route.OpenAPI.File)
		}
		openAPIValidationMiddleware, err = newOpenAPIRequestValidationMiddleware(openapimiddleware.Config{
			SpecPath:    specPath,
			StripPrefix: stripPrefix,
			Mode:        route.OpenAPI.Mode,
		}, log)
		if err != nil {
			return fmt.Errorf("configure openapi request validation for %s: %w", route.PathPrefix, err)
		}
	}

	allowedMethodsByPath := make(map[string]map[string]struct{})
	for _, endpoint := range route.Endpoints {
		routePath := joinRoutePath(route.PathPrefix, endpoint.Path)
		if _, exists := allowedMethodsByPath[routePath]; !exists {
			allowedMethodsByPath[routePath] = make(map[string]struct{})
		}
		for methodName, method := range endpoint.Methods {
			allowedMethodsByPath[routePath][methodName] = struct{}{}

			middlewares := []router.MiddlewareFunc{}

			if routeRateLimitMiddleware != nil {
				middlewares = append(middlewares, routeRateLimitMiddleware)
			}

			if method.RateLimit != nil {
				methodLimiter := ratelimit.NewTokenBucketLimiter(method.RateLimit.RequestsPerSecond, method.RateLimit.Burst)
				methodRateLimitMiddleware := ratelimit.RateLimit(methodLimiter, ratelimit.RateLimitConfig{
					RequestsPerSecond: method.RateLimit.RequestsPerSecond,
					Burst:             method.RateLimit.Burst,
					KeyFunc:           keyFunc,
				})
				middlewares = append(middlewares, methodRateLimitMiddleware)
			}

			if len(method.Scopes) > 0 {
				scopeMiddleware := authz.RequireScopes(method.Scopes...)
				middlewares = append(middlewares, scopeMiddleware)
			}

			if openAPIValidationMiddleware != nil {
				middlewares = append(middlewares, openAPIValidationMiddleware)
			}

			registerMethodRoute(r, methodName, routePath, handler, middlewares...)
		}
	}

	for path, allowedMethods := range allowedMethodsByPath {
		registerMethodNotAllowedRoutes(r, path, allowedMethods)
	}

	return nil
}

func RegisterWebSocketRoute(r router.Router, ws config.WebSocket, groupRateLimit *config.RateLimit, keyFunc func(c router.Context) string) {
	handler := proxy.ProxyWebSocket(ws.TargetURL, ws.StripPrefix)

	effectiveRateLimit := groupRateLimit
	if ws.RateLimit != nil {
		effectiveRateLimit = ws.RateLimit
	}

	middlewares := []router.MiddlewareFunc{}

	if effectiveRateLimit != nil {
		limiter := ratelimit.NewTokenBucketLimiter(effectiveRateLimit.RequestsPerSecond, effectiveRateLimit.Burst)
		rateLimitMiddleware := ratelimit.RateLimit(limiter, ratelimit.RateLimitConfig{
			RequestsPerSecond: effectiveRateLimit.RequestsPerSecond,
			Burst:             effectiveRateLimit.Burst,
			KeyFunc:           keyFunc,
		})
		middlewares = append(middlewares, rateLimitMiddleware)
	}

	if len(ws.Scopes) > 0 {
		scopeMiddleware := authz.RequireScopes(ws.Scopes...)
		middlewares = append(middlewares, scopeMiddleware)
	}

	r.GET(ws.Path, handler, middlewares...)
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

func registerMethodRoute(r router.Router, method, path string, handler router.HandlerFunc, middlewareChain ...router.MiddlewareFunc) {
	switch method {
	case http.MethodGet:
		r.GET(path, handler, middlewareChain...)
	case http.MethodPost:
		r.POST(path, handler, middlewareChain...)
	case http.MethodPut:
		r.PUT(path, handler, middlewareChain...)
	case http.MethodPatch:
		r.PATCH(path, handler, middlewareChain...)
	case http.MethodDelete:
		r.DELETE(path, handler, middlewareChain...)
	}
}

func registerMethodNotAllowedRoutes(r router.Router, path string, allowedMethods map[string]struct{}) {
	allowHeader := buildAllowHeaderValue(allowedMethods)
	handler := func(c router.Context) error {
		c.Response().Header().Set("Allow", allowHeader)
		return c.String(http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed))
	}

	for _, method := range proxySupportedMethods {
		if _, ok := allowedMethods[method]; ok {
			continue
		}
		registerMethodRoute(r, method, path, handler)
	}
}

func buildAllowHeaderValue(allowedMethods map[string]struct{}) string {
	allowValues := make([]string, 0, len(allowedMethods)+1)
	for _, method := range proxySupportedMethods {
		if _, ok := allowedMethods[method]; ok {
			allowValues = append(allowValues, method)
		}
	}
	allowValues = append(allowValues, http.MethodOptions)
	return strings.Join(allowValues, ", ")
}
