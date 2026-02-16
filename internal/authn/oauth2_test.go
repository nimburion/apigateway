package authn

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/middleware/session"
	"github.com/nimburion/nimburion/pkg/observability/logger"
	"github.com/nimburion/nimburion/pkg/server/router"
)

func TestMapOAuth2TokenExchangeError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantError  string
	}{
		{
			name:       "provider returns 400 invalid grant",
			err:        errors.New("token endpoint returned 400: {\"error\":\"invalid_grant\"}"),
			wantStatus: http.StatusBadRequest,
			wantError:  "invalid oauth2 authorization code",
		},
		{
			name:       "provider rate limit",
			err:        errors.New("token endpoint returned 429: {\"error\":\"rate_limited\"}"),
			wantStatus: http.StatusServiceUnavailable,
			wantError:  "oauth2 provider is rate limiting requests",
		},
		{
			name:       "provider 500",
			err:        errors.New("token endpoint returned 500: internal error"),
			wantStatus: http.StatusBadGateway,
			wantError:  "oauth2 provider error",
		},
		{
			name:       "deadline exceeded",
			err:        context.DeadlineExceeded,
			wantStatus: http.StatusGatewayTimeout,
			wantError:  "oauth2 provider timeout",
		},
		{
			name: "dns unreachable",
			err: &url.Error{
				Op:  "Post",
				URL: "https://idp.example.com/oauth/token",
				Err: &net.DNSError{Err: "no such host", Name: "idp.example.com"},
			},
			wantStatus: http.StatusServiceUnavailable,
			wantError:  "oauth2 provider unreachable",
		},
		{
			name:       "invalid token url scheme",
			err:        errors.New("parse \"\": unsupported protocol scheme \"\""),
			wantStatus: http.StatusInternalServerError,
			wantError:  "oauth2 configuration error",
		},
		{
			name:       "generic fallback",
			err:        errors.New("boom"),
			wantStatus: http.StatusBadGateway,
			wantError:  "failed oauth2 token exchange",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotPayload := mapOAuth2TokenExchangeError(tt.err)
			if gotStatus != tt.wantStatus {
				t.Fatalf("status mismatch: got %d want %d", gotStatus, tt.wantStatus)
			}
			if gotPayload["error"] != tt.wantError {
				t.Fatalf("error payload mismatch: got %q want %q", gotPayload["error"], tt.wantError)
			}
		})
	}
}

func TestParseOAuth2TokenEndpointStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantOK     bool
	}{
		{
			name:       "valid status",
			err:        errors.New("token endpoint returned 401: unauthorized"),
			wantStatus: 401,
			wantOK:     true,
		},
		{
			name:       "invalid prefix",
			err:        errors.New("something else"),
			wantStatus: 0,
			wantOK:     false,
		},
		{
			name:       "invalid code",
			err:        errors.New("token endpoint returned abc: bad"),
			wantStatus: 0,
			wantOK:     false,
		},
		{
			name:       "invalid range",
			err:        errors.New("token endpoint returned 999: bad"),
			wantStatus: 0,
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotOK := parseOAuth2TokenEndpointStatus(tt.err)
			if gotStatus != tt.wantStatus || gotOK != tt.wantOK {
				t.Fatalf("got (%d, %t), want (%d, %t)", gotStatus, gotOK, tt.wantStatus, tt.wantOK)
			}
		})
	}
}

func TestSanitizeReturnTo(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: "/"},
		{in: "  /dashboard ", want: "/dashboard"},
		{in: "https://evil.example", want: "/"},
		{in: "//evil", want: "/"},
	}
	for _, tt := range tests {
		if got := sanitizeReturnTo(tt.in); got != tt.want {
			t.Fatalf("sanitizeReturnTo(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSanitizeTheme(t *testing.T) {
	if got := sanitizeTheme("light"); got != "light" {
		t.Fatalf("expected light, got %q", got)
	}
	if got := sanitizeTheme(" DARK "); got != "dark" {
		t.Fatalf("expected dark, got %q", got)
	}
	if got := sanitizeTheme("nope"); got != "system" {
		t.Fatalf("expected system fallback, got %q", got)
	}
}

func TestAppendAuthorizeQueryParam(t *testing.T) {
	got, err := appendAuthorizeQueryParam("https://idp.example.com/authorize?client_id=a", "theme", "dark")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "theme=dark") {
		t.Fatalf("expected theme query param in %q", got)
	}

	if _, err := appendAuthorizeQueryParam("://bad-url", "theme", "dark"); err == nil {
		t.Fatalf("expected parse error for invalid URL")
	}
}

func TestRandomState(t *testing.T) {
	got, err := randomState(16)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" {
		t.Fatalf("expected non-empty state")
	}
}

type oauthTestLogger struct{}

func (l oauthTestLogger) Debug(string, ...any)                      {}
func (l oauthTestLogger) Info(string, ...any)                       {}
func (l oauthTestLogger) Warn(string, ...any)                       {}
func (l oauthTestLogger) Error(string, ...any)                      {}
func (l oauthTestLogger) With(...any) logger.Logger                 { return l }
func (l oauthTestLogger) WithContext(context.Context) logger.Logger { return l }

type oauthTestRouter struct {
	getHandlers  map[string]router.HandlerFunc
	postHandlers map[string]router.HandlerFunc
}

func newOAuthTestRouter() *oauthTestRouter {
	return &oauthTestRouter{
		getHandlers:  map[string]router.HandlerFunc{},
		postHandlers: map[string]router.HandlerFunc{},
	}
}

func (r *oauthTestRouter) GET(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	r.getHandlers[path] = handler
}
func (r *oauthTestRouter) POST(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
	r.postHandlers[path] = handler
}
func (r *oauthTestRouter) PUT(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
}
func (r *oauthTestRouter) DELETE(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
}
func (r *oauthTestRouter) PATCH(path string, handler router.HandlerFunc, middleware ...router.MiddlewareFunc) {
}
func (r *oauthTestRouter) Group(prefix string, middleware ...router.MiddlewareFunc) router.Router {
	return r
}
func (r *oauthTestRouter) Use(middleware ...router.MiddlewareFunc)            {}
func (r *oauthTestRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {}

type oauthTestContext struct {
	req        *http.Request
	res        router.ResponseWriter
	statusCode int
	payload    any
	data       map[string]any
}

func (c *oauthTestContext) Request() *http.Request              { return c.req }
func (c *oauthTestContext) SetRequest(r *http.Request)          { c.req = r }
func (c *oauthTestContext) Response() router.ResponseWriter     { return c.res }
func (c *oauthTestContext) SetResponse(w router.ResponseWriter) { c.res = w }
func (c *oauthTestContext) Param(name string) string            { return "" }
func (c *oauthTestContext) Query(name string) string            { return c.req.URL.Query().Get(name) }
func (c *oauthTestContext) Bind(v interface{}) error            { return nil }
func (c *oauthTestContext) JSON(code int, v interface{}) error {
	c.statusCode = code
	c.payload = v
	return nil
}
func (c *oauthTestContext) String(code int, s string) error {
	c.statusCode = code
	c.payload = s
	return nil
}
func (c *oauthTestContext) Get(key string) interface{} {
	if c.data == nil {
		return nil
	}
	return c.data[key]
}
func (c *oauthTestContext) Set(key string, value interface{}) {
	if c.data == nil {
		c.data = map[string]any{}
	}
	c.data[key] = value
}

type oauthTestResponseWriter struct {
	header http.Header
	status int
}

func (w *oauthTestResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (w *oauthTestResponseWriter) WriteHeader(code int) { w.status = code }
func (w *oauthTestResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return len(b), nil
}
func (w *oauthTestResponseWriter) Status() int   { return w.status }
func (w *oauthTestResponseWriter) Written() bool { return w.status != 0 }

func TestRegisterOAuth2RoutesHandlers(t *testing.T) {
	r := newOAuthTestRouter()
	cfg := &gatewaycfg.OAuth2Config{PostLoginRedirectURL: "https://app.example.com"}
	RegisterOAuth2Routes(r, cfg, oauthTestLogger{})

	if _, ok := r.getHandlers["/auth/login"]; !ok {
		t.Fatalf("expected /auth/login handler")
	}
	if _, ok := r.getHandlers["/auth/callback"]; !ok {
		t.Fatalf("expected /auth/callback handler")
	}
	if _, ok := r.postHandlers["/auth/logout"]; !ok {
		t.Fatalf("expected /auth/logout handler")
	}
	if _, ok := r.postHandlers["/auth/refresh"]; !ok {
		t.Fatalf("expected /auth/refresh handler")
	}
}

func TestRegisterOAuth2Routes_LoginWithoutSession(t *testing.T) {
	r := newOAuthTestRouter()
	RegisterOAuth2Routes(r, &gatewaycfg.OAuth2Config{}, oauthTestLogger{})

	ctx := &oauthTestContext{
		req: httptest.NewRequest(http.MethodGet, "/auth/login?return_to=/dashboard", nil),
		res: &oauthTestResponseWriter{},
	}
	if err := r.getHandlers["/auth/login"](ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.statusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", ctx.statusCode)
	}
}

func TestRegisterOAuth2Routes_CallbackValidation(t *testing.T) {
	r := newOAuthTestRouter()
	RegisterOAuth2Routes(r, &gatewaycfg.OAuth2Config{}, oauthTestLogger{})

	errCtx := &oauthTestContext{
		req: httptest.NewRequest(http.MethodGet, "/auth/callback?error=access_denied&error_description=nope", nil),
		res: &oauthTestResponseWriter{},
	}
	if err := r.getHandlers["/auth/callback"](errCtx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errCtx.statusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for oauth error callback, got %d", errCtx.statusCode)
	}

	missingCtx := &oauthTestContext{
		req: httptest.NewRequest(http.MethodGet, "/auth/callback", nil),
		res: &oauthTestResponseWriter{},
	}
	if err := r.getHandlers["/auth/callback"](missingCtx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if missingCtx.statusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing params, got %d", missingCtx.statusCode)
	}

	noSessionCtx := &oauthTestContext{
		req: httptest.NewRequest(http.MethodGet, "/auth/callback?code=abc&state=xyz", nil),
		res: &oauthTestResponseWriter{},
	}
	if err := r.getHandlers["/auth/callback"](noSessionCtx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if noSessionCtx.statusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing session, got %d", noSessionCtx.statusCode)
	}
}

func TestRegisterOAuth2Routes_RefreshWithoutSession(t *testing.T) {
	r := newOAuthTestRouter()
	RegisterOAuth2Routes(r, &gatewaycfg.OAuth2Config{}, oauthTestLogger{})

	ctx := &oauthTestContext{
		req: httptest.NewRequest(http.MethodPost, "/auth/refresh", nil),
		res: &oauthTestResponseWriter{},
	}
	if err := r.postHandlers["/auth/refresh"](ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.statusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", ctx.statusCode)
	}
}

func TestRegisterOAuth2Routes_LogoutWithoutSession(t *testing.T) {
	r := newOAuthTestRouter()
	RegisterOAuth2Routes(r, &gatewaycfg.OAuth2Config{}, oauthTestLogger{})

	ctx := &oauthTestContext{
		req: httptest.NewRequest(http.MethodPost, "/auth/logout", nil),
		res: &oauthTestResponseWriter{},
	}
	if err := r.postHandlers["/auth/logout"](ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.statusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", ctx.statusCode)
	}
}

func TestRegisterOAuth2Routes_CallbackInvalidState(t *testing.T) {
	r := newOAuthTestRouter()
	RegisterOAuth2Routes(r, &gatewaycfg.OAuth2Config{}, oauthTestLogger{})

	ctx := &oauthTestContext{
		req:  httptest.NewRequest(http.MethodGet, "/auth/callback?code=abc&state=xyz", nil),
		res:  &oauthTestResponseWriter{},
		data: map[string]any{session.ContextKey: &session.Session{}},
	}
	if err := r.getHandlers["/auth/callback"](ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.statusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", ctx.statusCode)
	}
}
