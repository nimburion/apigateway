package configstore

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/http/router"
)

type Handler struct {
	store                          Store
	runtime                        *Runtime
	requireValidationBeforePublish bool
	requireBaseVersionMatch        bool
}

type HandlerOptions struct {
	RequireValidationBeforePublish bool
	RequireBaseVersionMatch        bool
}

func NewHandler(store Store, runtime *Runtime, opts HandlerOptions) *Handler {
	return &Handler{
		store:                          store,
		runtime:                        runtime,
		requireValidationBeforePublish: opts.RequireValidationBeforePublish,
		requireBaseVersionMatch:        opts.RequireBaseVersionMatch,
	}
}

func (h *Handler) GetConfig(c router.Context) error {
	current := h.runtime.CurrentVersion()
	return c.JSON(http.StatusOK, current)
}

func (h *Handler) ListVersions(c router.Context) error {
	versions, err := h.store.ListVersions(c.Request().Context(), 50)
	if err != nil {
		return writeStoreError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"versions": versions})
}

func (h *Handler) ListDrafts(c router.Context) error {
	versions, err := h.store.ListDrafts(c.Request().Context(), 50)
	if err != nil {
		return writeStoreError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"drafts": versions})
}

func (h *Handler) GetVersion(c router.Context) error {
	version, ok := pathVersion(c)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid version"})
	}
	got, err := h.store.GetVersion(c.Request().Context(), version)
	if err != nil {
		return writeStoreError(c, err)
	}
	return c.JSON(http.StatusOK, got)
}

type draftRequest struct {
	Routes      gatewaycfg.Routing `json:"routes"`
	Message     string             `json:"message"`
	BaseVersion *int64             `json:"base_version"`
}

func (h *Handler) CreateDraft(c router.Context) error {
	var req draftRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if _, err := h.runtime.normalize(req.Routes); err != nil {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
	version, err := h.store.SaveDraft(c.Request().Context(), DraftInput{
		Routes:      req.Routes,
		CreatedBy:   requestSubject(c),
		Message:     strings.TrimSpace(req.Message),
		BaseVersion: req.BaseVersion,
	})
	if err != nil {
		return writeStoreError(c, err)
	}
	return c.JSON(http.StatusCreated, version)
}

func (h *Handler) UpdateDraft(c router.Context) error {
	version, ok := pathVersion(c)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid version"})
	}
	var req draftRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if _, err := h.runtime.normalize(req.Routes); err != nil {
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
	updated, err := h.store.UpdateDraft(c.Request().Context(), version, DraftInput{
		Routes:      req.Routes,
		CreatedBy:   requestSubject(c),
		Message:     strings.TrimSpace(req.Message),
		BaseVersion: req.BaseVersion,
	})
	if err != nil {
		return writeStoreError(c, err)
	}
	return c.JSON(http.StatusOK, updated)
}

func (h *Handler) ValidateDraft(c router.Context) error {
	version, ok := pathVersion(c)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid version"})
	}
	got, err := h.store.GetVersion(c.Request().Context(), version)
	if err != nil {
		return writeStoreError(c, err)
	}
	if _, err := h.runtime.normalize(got.Routes); err != nil {
		_ = h.store.MarkFailed(c.Request().Context(), version, err.Error())
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
	validated, err := h.store.ValidateDraft(c.Request().Context(), version, requestSubject(c))
	if err != nil {
		return writeStoreError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"valid": true, "version": validated})
}

type publishRequest struct {
	BaseVersion *int64 `json:"base_version"`
}

func (h *Handler) PublishDraft(c router.Context) error {
	version, ok := pathVersion(c)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid version"})
	}
	var req publishRequest
	if c.Request().Body != nil && c.Request().ContentLength != 0 {
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
	}
	published, err := h.store.Publish(c.Request().Context(), version, PublishOptions{
		BaseVersion:                    req.BaseVersion,
		Actor:                          requestSubject(c),
		RequireValidationBeforePublish: h.requireValidationBeforePublish,
		RequireBaseVersionMatch:        h.requireBaseVersionMatch,
	})
	if err != nil {
		return writeStoreError(c, err)
	}
	if err := h.runtime.Activate(c.Request().Context(), published); err != nil {
		_ = h.store.MarkFailed(c.Request().Context(), published.Version, err.Error())
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, published)
}

type rollbackRequest struct {
	Message string `json:"message"`
}

func (h *Handler) Rollback(c router.Context) error {
	version, ok := pathVersion(c)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid version"})
	}
	var req rollbackRequest
	if c.Request().Body != nil && c.Request().ContentLength != 0 {
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
	}
	published, err := h.store.Rollback(c.Request().Context(), version, requestSubject(c), strings.TrimSpace(req.Message))
	if err != nil {
		return writeStoreError(c, err)
	}
	if err := h.runtime.Activate(c.Request().Context(), published); err != nil {
		_ = h.store.MarkFailed(c.Request().Context(), published.Version, err.Error())
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, published)
}

func (h *Handler) ListAuditEvents(c router.Context) error {
	events, err := h.store.ListAuditEvents(c.Request().Context(), 50)
	if err != nil {
		return writeStoreError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"events": events})
}

func pathVersion(c router.Context) (int64, bool) {
	raw := strings.TrimSpace(c.Param("version"))
	version, err := strconv.ParseInt(raw, 10, 64)
	return version, err == nil && version > 0
}

func requestSubject(c router.Context) string {
	if c == nil || c.Request() == nil {
		return ""
	}
	if value := strings.TrimSpace(c.Request().Header.Get("X-User")); value != "" {
		return value
	}
	return strings.TrimSpace(c.Request().Header.Get("X-Subject"))
}

func writeStoreError(c router.Context, err error) error {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrNoActiveConfig):
		return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrStaleBase):
		return c.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrBaseVersionMissing):
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, ErrValidationRequired):
		return c.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}
