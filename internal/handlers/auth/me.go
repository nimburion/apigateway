package auth

import (
	"net/http"
	"time"

	"github.com/nimburion/nimburion/pkg/auth"
	"github.com/nimburion/nimburion/pkg/http/router"
)

func MeHandler(c router.Context) error {
	claims := auth.GetClaims(c.Request().Context())
	if claims == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing authentication"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"subject":       claims.Subject,
		"tenant_id":     claims.TenantID,
		"scopes":        claims.Scopes,
		"roles":         claims.Roles,
		"issuer":        claims.Issuer,
		"audience":      claims.Audience,
		"expires_at":    claims.ExpiresAt.UTC().Format(time.RFC3339),
	})
}
