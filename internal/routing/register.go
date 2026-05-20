package routing

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/apigateway/internal/proxy"
	"github.com/nimburion/nimburion/pkg/http/authorization"
	openapimiddleware "github.com/nimburion/nimburion/pkg/http/contract/openapi"
	"github.com/nimburion/nimburion/pkg/http/ratelimit"
	"github.com/nimburion/nimburion/pkg/http/router"
	logpkg "github.com/nimburion/nimburion/pkg/observability/logger"
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
	return RegisterProxyRouteWithMiddlewareRegistry(r, route, groupRateLimit, keyFunc, log, nil, nil)
}

func RegisterProxyRouteWithMiddlewareRegistry(r router.Router, route config.Route, groupRateLimit *config.RateLimit, keyFunc func(c router.Context) string, log logpkg.Logger, middlewareRegistry map[string]func() router.MiddlewareFunc, inheritedMiddlewares []string) error {
	target, err := url.Parse(route.TargetURL)
	if err != nil {
		panic(fmt.Errorf("invalid target url for %s: %w", route.PathPrefix, err))
	}

	stripPrefix := strings.TrimSpace(route.StripPrefix)

	handler := func(c router.Context) error {
		if err := forwardHTTPRequest(c.Response(), c.Request(), target, stripPrefix); err != nil {
			return err
		}
		return nil
	}

	effectiveRouteRateLimit := groupRateLimit
	if route.RateLimit != nil {
		effectiveRouteRateLimit = route.RateLimit
	}

	var routeRateLimitMiddleware router.MiddlewareFunc
	if effectiveRouteRateLimit != nil {
		routeLimiter := ratelimit.NewTokenBucketLimiter(effectiveRouteRateLimit.RequestsPerSecond, effectiveRouteRateLimit.Burst)
		routeRateLimitMiddleware = ratelimit.RateLimit(routeLimiter, ratelimit.Config{
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
		endpointMiddlewareNames := ApplyMiddlewareDirectives(
			ApplyMiddlewareDirectives(inheritedMiddlewares, route.Middlewares, route.DisableMiddlewares),
			endpoint.Middlewares,
			endpoint.DisableMiddlewares,
		)
		for methodName, method := range endpoint.Methods {
			allowedMethodsByPath[routePath][methodName] = struct{}{}

			methodMiddlewareNames := ApplyMiddlewareDirectives(endpointMiddlewareNames, method.Middlewares, method.DisableMiddlewares)
			middlewares, err := BuildMiddlewareChain(methodMiddlewareNames, middlewareRegistry)
			if err != nil {
				return err
			}

			if routeRateLimitMiddleware != nil {
				middlewares = append(middlewares, routeRateLimitMiddleware)
			}

			if method.RateLimit != nil {
				methodLimiter := ratelimit.NewTokenBucketLimiter(method.RateLimit.RequestsPerSecond, method.RateLimit.Burst)
				methodRateLimitMiddleware := ratelimit.RateLimit(methodLimiter, ratelimit.Config{
					RequestsPerSecond: method.RateLimit.RequestsPerSecond,
					Burst:             method.RateLimit.Burst,
					KeyFunc:           keyFunc,
				})
				middlewares = append(middlewares, methodRateLimitMiddleware)
			}

			if len(method.Scopes) > 0 {
				scopeMiddleware := authorization.RequireScopes(method.Scopes...)
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
	RegisterWebSocketRouteWithMiddlewareRegistry(r, ws, groupRateLimit, keyFunc, nil, nil)
}

func RegisterWebSocketRouteWithMiddlewareRegistry(r router.Router, ws config.WebSocket, groupRateLimit *config.RateLimit, keyFunc func(c router.Context) string, middlewareRegistry map[string]func() router.MiddlewareFunc, inheritedMiddlewares []string) {
	handler := proxy.ProxyWebSocket(ws.TargetURL, ws.StripPrefix)

	effectiveRateLimit := groupRateLimit
	if ws.RateLimit != nil {
		effectiveRateLimit = ws.RateLimit
	}

	middlewareNames := ApplyMiddlewareDirectives(inheritedMiddlewares, ws.Middlewares, ws.DisableMiddlewares)
	middlewares, err := BuildMiddlewareChain(middlewareNames, middlewareRegistry)
	if err != nil {
		panic(err)
	}

	if effectiveRateLimit != nil {
		limiter := ratelimit.NewTokenBucketLimiter(effectiveRateLimit.RequestsPerSecond, effectiveRateLimit.Burst)
		rateLimitMiddleware := ratelimit.RateLimit(limiter, ratelimit.Config{
			RequestsPerSecond: effectiveRateLimit.RequestsPerSecond,
			Burst:             effectiveRateLimit.Burst,
			KeyFunc:           keyFunc,
		})
		middlewares = append(middlewares, rateLimitMiddleware)
	}

	if len(ws.Scopes) > 0 {
		scopeMiddleware := authorization.RequireScopes(ws.Scopes...)
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

func forwardHTTPRequest(w http.ResponseWriter, incoming *http.Request, target *url.URL, stripPrefix string) error {
	upstreamURL := buildUpstreamURL(incoming.URL, target, stripPrefix)

	var bodyBytes []byte
	if incoming.Body != nil {
		defer func() { _ = incoming.Body.Close() }()
		var err error
		bodyBytes, err = io.ReadAll(incoming.Body)
		if err != nil {
			return fmt.Errorf("read upstream request body: %w", err)
		}
	}

	// #nosec G704 -- upstream URL comes from trusted gateway routing configuration.
	outReq, err := http.NewRequestWithContext(incoming.Context(), incoming.Method, upstreamURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("build upstream request: %w", err)
	}
	if bodyBytes != nil {
		outReq.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
		outReq.ContentLength = int64(len(bodyBytes))
	}

	copyRequestHeaders(outReq.Header, incoming.Header)
	outReq.Host = ""
	appendForwardedHeaders(outReq, incoming)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after %d redirects", len(via))
			}
			copyRequestHeaders(req.Header, via[0].Header)
			return nil
		},
	}

	// #nosec G704 -- request target is the configured upstream service for this route.
	resp, err := client.Do(outReq)
	if err != nil {
		return fmt.Errorf("forward upstream request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("copy upstream response body: %w", err)
	}
	return nil
}

func buildUpstreamURL(source *url.URL, target *url.URL, stripPrefix string) *url.URL {
	rewrittenPath := source.Path
	rewrittenRawPath := source.RawPath
	if stripPrefix != "" && strings.HasPrefix(rewrittenPath, stripPrefix) {
		rewrittenPath = strings.TrimPrefix(rewrittenPath, stripPrefix)
		if rewrittenPath == "" {
			rewrittenPath = "/"
		}
		if rewrittenRawPath != "" && strings.HasPrefix(rewrittenRawPath, stripPrefix) {
			rewrittenRawPath = strings.TrimPrefix(rewrittenRawPath, stripPrefix)
			if rewrittenRawPath == "" {
				rewrittenRawPath = "/"
			}
		}
	}

	upstream := *target
	upstream.Path = joinProxyURLPath(target.Path, rewrittenPath)
	upstream.RawPath = joinProxyURLPath(target.EscapedPath(), rewrittenRawPath)
	upstream.RawQuery = source.RawQuery
	upstream.Fragment = ""
	return &upstream
}

func joinProxyURLPath(basePath, requestPath string) string {
	switch {
	case basePath == "":
		if requestPath == "" {
			return "/"
		}
		return requestPath
	case requestPath == "":
		return basePath
	case strings.HasSuffix(basePath, "/") && strings.HasPrefix(requestPath, "/"):
		return basePath + strings.TrimPrefix(requestPath, "/")
	case !strings.HasSuffix(basePath, "/") && !strings.HasPrefix(requestPath, "/"):
		return basePath + "/" + requestPath
	default:
		return basePath + requestPath
	}
}

func copyRequestHeaders(dst, src http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) {
			continue
		}
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func appendForwardedHeaders(outReq, incoming *http.Request) {
	if ip := clientIPFromRemoteAddr(incoming.RemoteAddr); ip != "" {
		prior := incoming.Header.Get("X-Forwarded-For")
		if prior != "" {
			outReq.Header.Set("X-Forwarded-For", prior+", "+ip)
		} else {
			outReq.Header.Set("X-Forwarded-For", ip)
		}
	}
	if incoming.TLS != nil {
		outReq.Header.Set("X-Forwarded-Proto", "https")
	} else {
		outReq.Header.Set("X-Forwarded-Proto", "http")
	}
	if incoming.Host != "" {
		outReq.Header.Set("X-Forwarded-Host", incoming.Host)
	}
}

func clientIPFromRemoteAddr(remoteAddr string) string {
	if remoteAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}

func isHopByHopHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}
