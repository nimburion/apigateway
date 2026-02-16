package proxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nimburion/nimburion/pkg/server/router"
)

// ProxyWebSocket proxies WebSocket connections to a backend service.
// It handles the upgrade on both client and backend sides and copies frames bidirectionally.
func ProxyWebSocket(targetURL string, stripPrefix string) router.HandlerFunc {
	target, err := url.Parse(targetURL)
	if err != nil {
		panic(err)
	}

	// Convert http(s) to ws(s)
	wsScheme := "ws"
	if target.Scheme == "https" {
		wsScheme = "wss"
	}

	return func(c router.Context) error {
		req := c.Request()

		// Rewrite path if strip_prefix is configured
		targetPath := req.URL.Path
		if stripPrefix != "" && strings.HasPrefix(targetPath, stripPrefix) {
			targetPath = strings.TrimPrefix(targetPath, stripPrefix)
			if targetPath == "" {
				targetPath = "/"
			}
		}

		// Build backend WebSocket URL
		backendURL := wsScheme + "://" + target.Host + targetPath
		if req.URL.RawQuery != "" {
			backendURL += "?" + req.URL.RawQuery
		}

		// Hijack client connection
		hijacker, ok := c.Response().(http.Hijacker)
		if !ok {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "hijacking not supported"})
		}

		clientConn, clientBuf, err := hijacker.Hijack()
		if err != nil {
			return err
		}
		defer clientConn.Close()

		// Connect to backend WebSocket
		backendReq, err := http.NewRequestWithContext(req.Context(), "GET", backendURL, nil)
		if err != nil {
			return err
		}

		// Forward WebSocket headers
		backendReq.Header.Set("Upgrade", "websocket")
		backendReq.Header.Set("Connection", "Upgrade")
		backendReq.Header.Set("Sec-WebSocket-Version", req.Header.Get("Sec-WebSocket-Version"))
		backendReq.Header.Set("Sec-WebSocket-Key", req.Header.Get("Sec-WebSocket-Key"))
		if protocol := req.Header.Get("Sec-WebSocket-Protocol"); protocol != "" {
			backendReq.Header.Set("Sec-WebSocket-Protocol", protocol)
		}
		if extensions := req.Header.Get("Sec-WebSocket-Extensions"); extensions != "" {
			backendReq.Header.Set("Sec-WebSocket-Extensions", extensions)
		}

		// Forward identity headers (set by authz middleware)
		for _, header := range []string{"X-Tenant-ID", "X-User-ID", "X-User-Scopes", "X-User-Roles", "Authorization"} {
			if value := req.Header.Get(header); value != "" {
				backendReq.Header.Set(header, value)
			}
		}

		// Dial backend
		dialer := &net.Dialer{
			Timeout: 10 * time.Second,
		}
		backendConn, err := dialer.DialContext(req.Context(), "tcp", target.Host)
		if err != nil {
			clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return nil
		}
		defer backendConn.Close()

		// Send upgrade request to backend
		if err := backendReq.Write(backendConn); err != nil {
			clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return nil
		}

		// Read backend upgrade response
		backendResp, err := http.ReadResponse(clientBuf.Reader, backendReq)
		if err != nil {
			clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return nil
		}

		// Forward upgrade response to client
		if err := backendResp.Write(clientConn); err != nil {
			return nil
		}

		// If upgrade failed, stop here
		if backendResp.StatusCode != http.StatusSwitchingProtocols {
			return nil
		}

		// Bidirectional copy
		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()

		go func() {
			io.Copy(backendConn, clientConn)
			cancel()
		}()

		go func() {
			io.Copy(clientConn, backendConn)
			cancel()
		}()

		<-ctx.Done()
		return nil
	}
}

// isWebSocketRequest checks if the request is a WebSocket upgrade request
func isWebSocketRequest(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}
