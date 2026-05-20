package approutes

import (
	"net/http"
	"strings"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/apigateway/internal/portalmeta"
	"github.com/nimburion/nimburion/pkg/http/router"
)

type RuntimeRoute struct {
	Method   string
	Path     string
	Metadata gatewaycfg.ResourceMetadata
	AuthRequired bool
	Scopes       []string
	HasRateLimit bool
}

func CollectRuntimeRoutes(register func(router.Router)) []RuntimeRoute {
	if register == nil {
		return []RuntimeRoute{}
	}
	c := newRuntimeRouteCollector()
	register(c)
	cloned := make([]RuntimeRoute, len(*c.routes))
	copy(cloned, *c.routes)
	return cloned
}

type runtimeRouteCollector struct {
	routes *[]RuntimeRoute
	prefix string
}

func newRuntimeRouteCollector() *runtimeRouteCollector {
	routes := make([]RuntimeRoute, 0)
	return &runtimeRouteCollector{routes: &routes}
}

func (r *runtimeRouteCollector) GET(path string, handler router.HandlerFunc, _ ...router.MiddlewareFunc) {
	r.add(http.MethodGet, path, handler)
}

func (r *runtimeRouteCollector) POST(path string, handler router.HandlerFunc, _ ...router.MiddlewareFunc) {
	r.add(http.MethodPost, path, handler)
}

func (r *runtimeRouteCollector) PUT(path string, handler router.HandlerFunc, _ ...router.MiddlewareFunc) {
	r.add(http.MethodPut, path, handler)
}

func (r *runtimeRouteCollector) DELETE(path string, handler router.HandlerFunc, _ ...router.MiddlewareFunc) {
	r.add(http.MethodDelete, path, handler)
}

func (r *runtimeRouteCollector) PATCH(path string, handler router.HandlerFunc, _ ...router.MiddlewareFunc) {
	r.add(http.MethodPatch, path, handler)
}

func (r *runtimeRouteCollector) Group(prefix string, _ ...router.MiddlewareFunc) router.Router {
	return &runtimeRouteCollector{
		routes:  r.routes,
		prefix: joinPaths(r.prefix, prefix),
	}
}

func (r *runtimeRouteCollector) Use(_ ...router.MiddlewareFunc) {}

func (r *runtimeRouteCollector) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {}

func (r *runtimeRouteCollector) add(method, path string, handler router.HandlerFunc) {
	fullPath := joinPaths(r.prefix, path)
	meta, _ := portalmeta.MetadataForHandler(handler)
	*r.routes = append(*r.routes, RuntimeRoute{
		Method:      strings.ToUpper(strings.TrimSpace(method)),
		Path:        fullPath,
		Metadata:    meta.Resource,
		AuthRequired: meta.AuthRequired,
		Scopes:       append([]string(nil), meta.Scopes...),
		HasRateLimit: meta.HasRateLimit,
	})
}

func joinPaths(prefix, path string) string {
	prefix = strings.TrimSpace(prefix)
	path = strings.TrimSpace(path)
	switch {
	case prefix == "" && path == "":
		return "/"
	case prefix == "":
		if strings.HasPrefix(path, "/") {
			return path
		}
		return "/" + path
	case path == "":
		if strings.HasPrefix(prefix, "/") {
			return prefix
		}
		return "/" + prefix
	}
	joined := strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(path, "/")
	if strings.HasPrefix(joined, "/") {
		return joined
	}
	return "/" + joined
}
