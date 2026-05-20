package middleware

import (
	"github.com/nimburion/nimburion/pkg/auth"
	"github.com/nimburion/nimburion/pkg/http/ratelimit"
	"github.com/nimburion/nimburion/pkg/http/router"
)

func RateLimitKeyByTenantAndSubject(c router.Context) string {
	claims := auth.GetClaims(c.Request().Context())
	if claims != nil && claims.TenantID != "" && claims.Subject != "" {
		return claims.TenantID + ":" + claims.Subject
	}
	return ratelimit.ExtractIPFromRequest(c.Request())
}
