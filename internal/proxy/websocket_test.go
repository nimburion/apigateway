package proxy

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nimburion/nimburion/pkg/server/router"
)

func TestIsWebSocketRequest(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	if !isWebSocketRequest(req) {
		t.Fatalf("expected websocket request")
	}

	req.Header.Set("Connection", "keep-alive")
	if isWebSocketRequest(req) {
		t.Fatalf("expected non-websocket request")
	}
}

type wsTestResponseWriter struct {
	header http.Header
	status int
}

func (w *wsTestResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}
func (w *wsTestResponseWriter) WriteHeader(statusCode int) { w.status = statusCode }
func (w *wsTestResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return len(b), nil
}
func (w *wsTestResponseWriter) Status() int { return w.status }
func (w *wsTestResponseWriter) Written() bool {
	return w.status != 0
}

type wsHijackResponseWriter struct {
	wsTestResponseWriter
	conn net.Conn
	rw   *bufio.ReadWriter
}

func (w *wsHijackResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, w.rw, nil
}

type wsTestContext struct {
	req        *http.Request
	res        router.ResponseWriter
	jsonStatus int
}

func (c *wsTestContext) Request() *http.Request              { return c.req }
func (c *wsTestContext) SetRequest(r *http.Request)          { c.req = r }
func (c *wsTestContext) Response() router.ResponseWriter     { return c.res }
func (c *wsTestContext) SetResponse(w router.ResponseWriter) { c.res = w }
func (c *wsTestContext) Param(name string) string            { return "" }
func (c *wsTestContext) Query(name string) string            { return "" }
func (c *wsTestContext) Bind(v interface{}) error            { return nil }
func (c *wsTestContext) JSON(code int, v interface{}) error {
	c.jsonStatus = code
	return nil
}
func (c *wsTestContext) String(code int, s string) error   { return nil }
func (c *wsTestContext) Get(key string) interface{}        { return nil }
func (c *wsTestContext) Set(key string, value interface{}) {}

func TestProxyWebSocketPanicsOnInvalidTarget(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic for invalid target URL")
		}
	}()
	_ = ProxyWebSocket("://bad", "")
}

func TestProxyWebSocketHijackingNotSupported(t *testing.T) {
	handler := ProxyWebSocket("http://example.com", "/api")
	ctx := &wsTestContext{
		req: httptest.NewRequest(http.MethodGet, "http://gateway.local/api/ws?x=1", nil),
		res: &wsTestResponseWriter{},
	}
	if err := handler(ctx); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if ctx.jsonStatus != http.StatusInternalServerError {
		t.Fatalf("expected %d, got %d", http.StatusInternalServerError, ctx.jsonStatus)
	}
}

func TestProxyWebSocketBackendDialFailureWrites502(t *testing.T) {
	handler := ProxyWebSocket("http://127.0.0.1:1", "/api")
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	hijackWriter := &wsHijackResponseWriter{
		wsTestResponseWriter: wsTestResponseWriter{},
		conn:                 serverConn,
		rw:                   bufio.NewReadWriter(bufio.NewReader(serverConn), bufio.NewWriter(serverConn)),
	}
	ctx := &wsTestContext{
		req: httptest.NewRequest(http.MethodGet, "http://gateway.local/api/ws?x=1", nil),
		res: hijackWriter,
	}
	ctx.req.Header.Set("Sec-WebSocket-Version", "13")
	ctx.req.Header.Set("Sec-WebSocket-Key", "abc")
	ctx.req.Header.Set("Upgrade", "websocket")
	ctx.req.Header.Set("Connection", "Upgrade")

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = handler(ctx)
	}()

	buf := make([]byte, 64)
	n, err := clientConn.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("read from client side failed: %v", err)
	}
	if n == 0 {
		t.Fatalf("expected 502 response bytes")
	}
	got := string(buf[:n])
	if len(got) < 12 || got[:12] != "HTTP/1.1 502" {
		t.Fatalf("expected 502 response, got %q", got)
	}
	<-done
}
