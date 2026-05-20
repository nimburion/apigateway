package authn

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/apigateway/internal/portalmeta"
	"github.com/nimburion/nimburion/pkg/auth"
	httpopenapi "github.com/nimburion/nimburion/pkg/http/openapi"
	"github.com/nimburion/nimburion/pkg/http/router"
	"github.com/nimburion/nimburion/pkg/http/session"
	logpkg "github.com/nimburion/nimburion/pkg/observability/logger"
)

func RegisterOAuth2Routes(r router.Router, cfg *gatewaycfg.OAuth2Config, logger logpkg.Logger) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	authCfg := cfg.ToAuthConfig()

	r.GET("/auth/login", portalmeta.Annotate(httpopenapi.Annotate(func(c router.Context) error {
		state, err := randomState(32)
		if err != nil {
			logger.Error("failed generating oauth state", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to start oauth2 login"})
		}

		// Store state and return_to in session
		sess, ok := session.FromContext(c)
		if !ok || sess == nil {
			logger.Error("session not available")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "session error"})
		}

		sess.Set("oauth2_state", state)
		sess.Set("oauth2_return_to", sanitizeReturnTo(c.Request().URL.Query().Get("return_to")))

		theme := sanitizeTheme(c.Request().URL.Query().Get("theme"))
		authorizeURL, err := auth.BuildAuthorizeURL(authCfg, state)
		if err != nil {
			logger.Error("failed building authorize url", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to start oauth2 login"})
		}

		authorizeURL, err = appendAuthorizeQueryParam(authorizeURL, "theme", theme)
		if err != nil {
			logger.Error("failed adding authorize query param", "error", err, "param", "theme")
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to start oauth2 login"})
		}

		http.Redirect(c.Response(), c.Request(), authorizeURL, http.StatusFound)
		return nil
	}, httpopenapi.EndpointAnnotations{
		Summary: "OAuth2 login",
		Tags:    []string{"auth", "oauth2"},
	}), portalmeta.OAuth2Login(cfg.AuthorizeURL)))

	r.GET("/auth/callback", portalmeta.Annotate(httpopenapi.Annotate(func(c router.Context) error {
		query := c.Request().URL.Query()
		oauthError := strings.TrimSpace(query.Get("error"))
		if oauthError != "" {
			oauthErrorDescription := strings.TrimSpace(query.Get("error_description"))
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error":                      "oauth2_authorization_failed",
				"provider_error":             oauthError,
				"provider_error_description": oauthErrorDescription,
			})
		}

		code := strings.TrimSpace(query.Get("code"))
		state := strings.TrimSpace(query.Get("state"))
		if code == "" || state == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing oauth2 callback parameters"})
		}

		// Validate state from session
		sess, ok := session.FromContext(c)
		if !ok || sess == nil {
			logger.Error("session not available")
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "session error"})
		}

		storedState, ok := sess.Get("oauth2_state")
		if !ok || storedState == "" || storedState != state {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid oauth2 state"})
		}

		token, err := auth.ExchangeAuthorizationCode(c.Request().Context(), httpClient, authCfg, code)
		if err != nil {
			logger.Error("oauth2 token exchange failed", "error", err)
			statusCode, payload := mapOAuth2TokenExchangeError(err)
			return c.JSON(statusCode, payload)
		}

		// Store tokens in session
		if err := session.SetOAuthTokens(c, token.AccessToken, token.RefreshToken); err != nil {
			logger.Error("failed to store tokens in session", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to store tokens"})
		}

		// Get return_to from session
		redirectTo := cfg.GetPostLoginRedirectURL()
		if returnTo, ok := sess.Get("oauth2_return_to"); ok && returnTo != "" {
			baseURL, parseErr := url.Parse(cfg.GetPostLoginRedirectURL())
			if parseErr == nil {
				redirectTo = baseURL.Scheme + "://" + baseURL.Host + returnTo
			}
		}

		// Clear temporary session data
		sess.Delete("oauth2_state")
		sess.Delete("oauth2_return_to")

		if redirectTo != "" {
			http.Redirect(c.Response(), c.Request(), redirectTo, http.StatusFound)
			return nil
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"status":      "authenticated",
			"token_type":  token.TokenType,
			"expires_in":  token.ExpiresIn,
			"scope":       token.Scope,
			"redirect_to": redirectTo,
		})
	}, httpopenapi.EndpointAnnotations{
		Summary: "OAuth2 callback",
		Tags:    []string{"auth", "oauth2"},
	}), portalmeta.OAuth2Callback(cfg.AuthorizeURL)))

	r.POST("/auth/logout", portalmeta.Annotate(httpopenapi.Annotate(func(c router.Context) error {
		if err := session.ClearOAuthTokens(c); err != nil {
			logger.Error("failed to clear tokens", "error", err)
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "logged_out"})
	}, httpopenapi.EndpointAnnotations{
		Summary: "OAuth2 logout",
		Tags:    []string{"auth", "oauth2"},
	}), portalmeta.OAuth2Logout(cfg.AuthorizeURL)))

	r.POST("/auth/refresh", portalmeta.Annotate(httpopenapi.Annotate(func(c router.Context) error {
		_, refreshToken, ok := session.GetOAuthTokens(c)
		if !ok || refreshToken == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing refresh token"})
		}

		token, err := auth.ExchangeRefreshToken(c.Request().Context(), httpClient, authCfg, refreshToken)
		if err != nil {
			logger.Error("oauth2 token refresh failed", "error", err)
			return c.JSON(http.StatusBadGateway, map[string]string{"error": "failed oauth2 token refresh"})
		}

		if err := session.SetOAuthTokens(c, token.AccessToken, token.RefreshToken); err != nil {
			logger.Error("failed to store refreshed tokens", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to store tokens"})
		}

		return c.JSON(http.StatusOK, map[string]interface{}{
			"status":     "refreshed",
			"token_type": token.TokenType,
			"expires_in": token.ExpiresIn,
			"scope":      token.Scope,
		})
	}, httpopenapi.EndpointAnnotations{
		Summary: "OAuth2 refresh",
		Tags:    []string{"auth", "oauth2"},
	}), portalmeta.OAuth2Refresh(cfg.AuthorizeURL)))
}

func sanitizeReturnTo(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "//") {
		return "/"
	}
	return trimmed
}

func sanitizeTheme(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "light":
		return "light"
	case "dark":
		return "dark"
	case "system":
		return "system"
	default:
		return "system"
	}
}

func appendAuthorizeQueryParam(rawURL, key, value string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func randomState(byteLen int) (string, error) {
	raw := make([]byte, byteLen)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func mapOAuth2TokenExchangeError(err error) (int, map[string]string) {
	if err == nil {
		return http.StatusBadGateway, map[string]string{"error": "failed oauth2 token exchange"}
	}

	if status, ok := parseOAuth2TokenEndpointStatus(err); ok {
		switch status {
		case http.StatusBadRequest:
			return http.StatusBadRequest, map[string]string{
				"error":                "invalid oauth2 authorization code",
				"provider_status_code": strconv.Itoa(status),
			}
		case http.StatusTooManyRequests:
			return http.StatusServiceUnavailable, map[string]string{
				"error":                "oauth2 provider is rate limiting requests",
				"provider_status_code": strconv.Itoa(status),
			}
		default:
			if status >= http.StatusInternalServerError {
				return http.StatusBadGateway, map[string]string{
					"error":                "oauth2 provider error",
					"provider_status_code": strconv.Itoa(status),
				}
			}
			return http.StatusBadGateway, map[string]string{
				"error":                "failed oauth2 token exchange",
				"provider_status_code": strconv.Itoa(status),
			}
		}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, map[string]string{"error": "oauth2 provider timeout"}
	}

	if errors.Is(err, context.Canceled) {
		return http.StatusRequestTimeout, map[string]string{"error": "request canceled"}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return http.StatusGatewayTimeout, map[string]string{"error": "oauth2 provider timeout"}
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return http.StatusServiceUnavailable, map[string]string{"error": "oauth2 provider unreachable"}
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return http.StatusServiceUnavailable, map[string]string{"error": "oauth2 provider unreachable"}
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") || strings.Contains(msg, "tls:") {
		return http.StatusServiceUnavailable, map[string]string{"error": "oauth2 provider unreachable"}
	}
	if strings.Contains(msg, "unsupported protocol scheme") {
		return http.StatusInternalServerError, map[string]string{"error": "oauth2 configuration error"}
	}

	return http.StatusBadGateway, map[string]string{"error": "failed oauth2 token exchange"}
}

func parseOAuth2TokenEndpointStatus(err error) (int, bool) {
	const prefix = "token endpoint returned "
	if err == nil {
		return 0, false
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, prefix) {
		return 0, false
	}
	rest := strings.TrimPrefix(msg, prefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) == 0 {
		return 0, false
	}
	status, convErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	if convErr != nil {
		return 0, false
	}
	if status < 100 || status > 599 {
		return 0, false
	}
	return status, true
}
