package server

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/nimburion/nimburion/pkg/http/router"
	logpkg "github.com/nimburion/nimburion/pkg/observability/logger"
	frameworkmetrics "github.com/nimburion/nimburion/pkg/observability/metrics"
)

type notFoundLoggingRouter struct {
	router.Router
	log     logpkg.Logger
	surface string
	metrics *frameworkmetrics.Registry
}

func newNotFoundLoggingRouter(r router.Router, log logpkg.Logger, surface string) router.Router {
	return newNotFoundLoggingRouterWithMetrics(r, log, surface, nil)
}

func newNotFoundLoggingRouterWithMetrics(r router.Router, log logpkg.Logger, surface string, metrics *frameworkmetrics.Registry) router.Router {
	if r == nil || log == nil {
		return r
	}
	return &notFoundLoggingRouter{
		Router:  r,
		log:     log,
		surface: surface,
		metrics: metrics,
	}
}

func (r *notFoundLoggingRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	recorder := &statusRecorder{ResponseWriter: w}
	start := time.Now()

	r.Router.ServeHTTP(recorder, req)

	if recorder.status == http.StatusNotFound {
		if r.metrics != nil {
			frameworkmetrics.RecordHTTPMetrics(req.Method, req.URL.Path, recorder.status, time.Since(start))
		}
		r.log.Warn(
			"route not found",
			"surface", r.surface,
			"method", req.Method,
			"path", req.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}

func (r *notFoundLoggingRouter) GET(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	r.Router.GET(path, handler, middleware...)
}

func (r *notFoundLoggingRouter) POST(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	r.Router.POST(path, handler, middleware...)
}

func (r *notFoundLoggingRouter) PUT(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	r.Router.PUT(path, handler, middleware...)
}

func (r *notFoundLoggingRouter) DELETE(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	r.Router.DELETE(path, handler, middleware...)
}

func (r *notFoundLoggingRouter) PATCH(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	r.Router.PATCH(path, handler, middleware...)
}

func (r *notFoundLoggingRouter) Group(prefix string, middleware ...router.MiddlewareFunc) router.Router {
	return newNotFoundLoggingRouter(r.Router.Group(prefix, middleware...), r.log, r.surface)
}

func (r *notFoundLoggingRouter) Use(middleware ...router.MiddlewareFunc) {
	r.Router.Use(middleware...)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacker not supported")
	}
	return hijacker.Hijack()
}

func (r *statusRecorder) Push(target string, opts *http.PushOptions) error {
	pusher, ok := r.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}
