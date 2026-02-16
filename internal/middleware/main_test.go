package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimburion/nimburion/pkg/auth"
	"github.com/nimburion/nimburion/pkg/server/router"
)

type testContext struct {
	req *http.Request
}

func (c *testContext) Request() *http.Request              { return c.req }
func (c *testContext) SetRequest(r *http.Request)          { c.req = r }
func (c *testContext) Response() router.ResponseWriter     { return nil }
func (c *testContext) SetResponse(w router.ResponseWriter) {}
func (c *testContext) Param(name string) string            { return "" }
func (c *testContext) Query(name string) string            { return "" }
func (c *testContext) Bind(v interface{}) error            { return nil }
func (c *testContext) JSON(code int, v interface{}) error  { return nil }
func (c *testContext) String(code int, s string) error     { return nil }
func (c *testContext) Get(key string) interface{}          { return nil }
func (c *testContext) Set(key string, value interface{})   {}

func TestRateLimitKeyByTenantAndSubject(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := &testContext{req: req}

	key := RateLimitKeyByTenantAndSubject(ctx)
	if key == "" {
		t.Fatalf("expected fallback key")
	}

	claims := &auth.Claims{TenantID: "tenant", Subject: "user"}
	req = req.WithContext(auth.WithClaims(req.Context(), claims))
	ctx.req = req

	key = RateLimitKeyByTenantAndSubject(ctx)
	if key != "tenant:user" {
		t.Fatalf("unexpected key: %s", key)
	}
}
