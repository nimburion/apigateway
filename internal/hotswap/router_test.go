package hotswap

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimburion/nimburion/pkg/http/router"
)

func TestRouterActivateSwapsDynamicRoutesAndKeepsStaticRoutes(t *testing.T) {
	r := NewRouter()
	r.GET("/static", func(c router.Context) error {
		return c.String(http.StatusOK, "static")
	})
	if err := r.Activate(func(next router.Router) error {
		next.GET("/dynamic", func(c router.Context) error {
			return c.String(http.StatusOK, "v1")
		})
		return nil
	}); err != nil {
		t.Fatalf("activate v1: %v", err)
	}
	assertResponse(t, r, "/static", http.StatusOK, "static")
	assertResponse(t, r, "/dynamic", http.StatusOK, "v1")

	if err := r.Activate(func(next router.Router) error {
		next.GET("/replacement", func(c router.Context) error {
			return c.String(http.StatusOK, "v2")
		})
		return nil
	}); err != nil {
		t.Fatalf("activate v2: %v", err)
	}
	assertResponse(t, r, "/static", http.StatusOK, "static")
	assertResponse(t, r, "/dynamic", http.StatusNotFound, "404 page not found\n")
	assertResponse(t, r, "/replacement", http.StatusOK, "v2")
}

func assertResponse(t *testing.T, handler http.Handler, path string, status int, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != status {
		t.Fatalf("%s: expected status %d, got %d", path, status, w.Code)
	}
	if w.Body.String() != body {
		t.Fatalf("%s: expected body %q, got %q", path, body, w.Body.String())
	}
}
