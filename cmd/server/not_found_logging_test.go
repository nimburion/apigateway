package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimburion/nimburion/pkg/http/middleware/testutil"
	"github.com/nimburion/nimburion/pkg/http/router/nethttp"
)

func TestNotFoundLoggingRouterLogs404(t *testing.T) {
	t.Parallel()

	log := &testutil.MockLogger{}
	baseRouter := nethttp.NewRouter()
	r := newNotFoundLoggingRouter(baseRouter, log, "public")

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)

	r.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}
	if len(log.Logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(log.Logs))
	}
	entry := log.Logs[0]
	if entry.Level != "warn" {
		t.Fatalf("expected warn log, got %s", entry.Level)
	}
	if entry.Msg != "route not found" {
		t.Fatalf("expected route not found message, got %q", entry.Msg)
	}
	if got := entry.Fields["surface"]; got != "public" {
		t.Fatalf("expected surface public, got %#v", got)
	}
	if got := entry.Fields["method"]; got != http.MethodGet {
		t.Fatalf("expected method GET, got %#v", got)
	}
	if got := entry.Fields["path"]; got != "/missing" {
		t.Fatalf("expected path /missing, got %#v", got)
	}
	if got := entry.Fields["status"]; got != http.StatusNotFound {
		t.Fatalf("expected status 404, got %#v", got)
	}
}
