package hotswap

import (
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/nimburion/nimburion/pkg/http/router"
	nethttprouter "github.com/nimburion/nimburion/pkg/http/router/nethttp"
)

type Router struct {
	active atomic.Value
	mu     sync.RWMutex
	mw     []router.MiddlewareFunc
	static []func(router.Router)
	build  func(router.Router) error
}

func NewRouter() *Router {
	r := &Router{}
	r.active.Store(router.Router(nethttprouter.NewRouter()))
	return r
}

func (r *Router) Activate(build func(router.Router) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.build = build
	return r.rebuildLocked()
}

func (r *Router) rebuildLocked() error {
	next := nethttprouter.NewRouter()
	mw := append([]router.MiddlewareFunc(nil), r.mw...)
	if len(mw) > 0 {
		next.Use(mw...)
	}
	for _, register := range r.static {
		register(next)
	}
	if r.build != nil {
		if err := r.build(next); err != nil {
			return err
		}
	}
	r.active.Store(router.Router(next))
	return nil
}

func (r *Router) addStatic(register func(router.Router)) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.static = append(r.static, register)
	return r.rebuildLocked()
}

func (r *Router) addMiddleware(middleware ...router.MiddlewareFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mw = append(r.mw, middleware...)
	_ = r.rebuildLocked()
}

func (r *Router) Use(middleware ...router.MiddlewareFunc) {
	r.addMiddleware(middleware...)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.active.Load().(router.Router).ServeHTTP(w, req)
}

func (r *Router) GET(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	_ = r.addStatic(func(next router.Router) {
		next.GET(path, handler, middleware...)
	})
}

func (r *Router) POST(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	_ = r.addStatic(func(next router.Router) {
		next.POST(path, handler, middleware...)
	})
}

func (r *Router) PUT(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	_ = r.addStatic(func(next router.Router) {
		next.PUT(path, handler, middleware...)
	})
}

func (r *Router) DELETE(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	_ = r.addStatic(func(next router.Router) {
		next.DELETE(path, handler, middleware...)
	})
}

func (r *Router) PATCH(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	_ = r.addStatic(func(next router.Router) {
		next.PATCH(path, handler, middleware...)
	})
}

func (r *Router) Group(prefix string, middleware ...router.MiddlewareFunc) router.Router {
	return &groupRouter{root: r, prefix: prefix, middleware: middleware}
}

type groupRouter struct {
	root       *Router
	prefix     string
	middleware []router.MiddlewareFunc
}

func (g *groupRouter) Use(middleware ...router.MiddlewareFunc) {
	g.middleware = append(g.middleware, middleware...)
}

func (g *groupRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	g.root.ServeHTTP(w, req)
}

func (g *groupRouter) GET(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	g.root.GET(g.prefix+path, handler, append(g.middleware, middleware...)...)
}

func (g *groupRouter) POST(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	g.root.POST(g.prefix+path, handler, append(g.middleware, middleware...)...)
}

func (g *groupRouter) PUT(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	g.root.PUT(g.prefix+path, handler, append(g.middleware, middleware...)...)
}

func (g *groupRouter) DELETE(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	g.root.DELETE(g.prefix+path, handler, append(g.middleware, middleware...)...)
}

func (g *groupRouter) PATCH(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	g.root.PATCH(g.prefix+path, handler, append(g.middleware, middleware...)...)
}

func (g *groupRouter) Group(prefix string, middleware ...router.MiddlewareFunc) router.Router {
	return &groupRouter{
		root:       g.root,
		prefix:     g.prefix + prefix,
		middleware: append(append([]router.MiddlewareFunc(nil), g.middleware...), middleware...),
	}
}

var _ router.Router = (*Router)(nil)
var _ router.Router = (*groupRouter)(nil)
