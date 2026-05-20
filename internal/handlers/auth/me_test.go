package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nimburion/nimburion/pkg/auth"
	"github.com/nimburion/nimburion/pkg/http/router"
)

type testResponseWriter struct {
	*httptest.ResponseRecorder
}

func (w testResponseWriter) Status() int   { return w.Code }
func (w testResponseWriter) Written() bool { return w.Code != 0 }

type testContext struct {
	req  *http.Request
	resp testResponseWriter
	data map[string]interface{}
}

func newTestContext(req *http.Request) *testContext {
	return &testContext{req: req, resp: testResponseWriter{httptest.NewRecorder()}, data: map[string]interface{}{}}
}

func (c *testContext) Request() *http.Request              { return c.req }
func (c *testContext) SetRequest(r *http.Request)          { c.req = r }
func (c *testContext) Response() router.ResponseWriter     { return c.resp }
func (c *testContext) SetResponse(w router.ResponseWriter) { c.resp = w.(testResponseWriter) }
func (c *testContext) Param(name string) string            { return "" }
func (c *testContext) Query(name string) string            { return "" }
func (c *testContext) Bind(v interface{}) error            { return json.NewDecoder(c.req.Body).Decode(v) }
func (c *testContext) Get(key string) interface{}          { return c.data[key] }
func (c *testContext) Set(key string, value interface{})   { c.data[key] = value }
func (c *testContext) JSON(code int, v interface{}) error {
	c.resp.Header().Set("Content-Type", "application/json")
	c.resp.WriteHeader(code)
	return json.NewEncoder(c.resp).Encode(v)
}
func (c *testContext) String(code int, s string) error {
	c.resp.WriteHeader(code)
	_, err := c.resp.Write([]byte(s))
	return err
}

func TestMeHandlerUnauthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	ctx := newTestContext(req)

	if err := MeHandler(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", ctx.resp.Code)
	}
}

func TestMeHandlerAuthorized(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	claims := &auth.Claims{
		Subject:   "user-1",
		TenantID:  "tenant-1",
		Scopes:    []string{"read"},
		Roles:     []string{"admin"},
		Issuer:    "issuer",
		Audience:  []string{"aud"},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	req = req.WithContext(auth.WithClaims(req.Context(), claims))
	ctx := newTestContext(req)

	if err := MeHandler(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", ctx.resp.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(ctx.resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["subject"] != "user-1" {
		t.Fatalf("unexpected subject: %#v", body["subject"])
	}
}
