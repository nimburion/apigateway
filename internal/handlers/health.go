package handlers

import (
	"net/http"

	"github.com/nimburion/nimburion/pkg/http/router"
)

func HealthHandler(serviceName string) router.HandlerFunc {
	return func(c router.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok", "service": serviceName})
	}
}
