package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimburion/nimburion/pkg/server/router"
)

type testResponseWriter struct {
	*httptest.ResponseRecorder
}

func (w testResponseWriter) Status() int  { return w.Code }
func (w testResponseWriter) Written() bool { return w.Code != 0 }

type testContext struct {
	req  *http.Request
	resp testResponseWriter
	data map[string]interface{}
}

func newTestContext(req *http.Request) *testContext {
	return &testContext{req: req, resp: testResponseWriter{httptest.NewRecorder()}, data: map[string]interface{}{}}
}

func (c *testContext) Request() *http.Request                 { return c.req }
func (c *testContext) SetRequest(r *http.Request)             { c.req = r }
func (c *testContext) Response() router.ResponseWriter        { return c.resp }
func (c *testContext) SetResponse(w router.ResponseWriter)    { c.resp = w.(testResponseWriter) }
func (c *testContext) Param(name string) string               { return "" }
func (c *testContext) Query(name string) string               { return "" }
func (c *testContext) Bind(v interface{}) error               { return json.NewDecoder(c.req.Body).Decode(v) }
func (c *testContext) Get(key string) interface{}             { return c.data[key] }
func (c *testContext) Set(key string, value interface{})      { c.data[key] = value }
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

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", bytes.NewBuffer(nil))
	ctx := newTestContext(req)

	handler := HealthHandler("gateway")
	if err := handler(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", ctx.resp.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(ctx.resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["service"] != "gateway" || body["status"] != "ok" {
		t.Fatalf("unexpected body: %#v", body)
	}
}
