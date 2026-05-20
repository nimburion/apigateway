package configstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
)

const (
	StatusDraft     = "draft"
	StatusValidated = "validated"
	StatusActive    = "active"
	StatusArchived  = "archived"
	StatusFailed    = "failed"
)

var (
	ErrNotFound           = errors.New("config version not found")
	ErrNoActiveConfig     = errors.New("active config version not found")
	ErrStaleBase          = errors.New("base version does not match active version")
	ErrBaseVersionMissing = errors.New("base_version is required")
	ErrValidationRequired = errors.New("draft must be validated before publish")
)

type Version struct {
	Version     int64              `json:"version"`
	Status      string             `json:"status"`
	Routes      gatewaycfg.Routing `json:"routes"`
	Checksum    string             `json:"checksum"`
	CreatedBy   string             `json:"created_by,omitempty"`
	Message     string             `json:"message,omitempty"`
	BaseVersion *int64             `json:"base_version,omitempty"`
	ValidatedAt *time.Time         `json:"validated_at,omitempty"`
	FailedAt    *time.Time         `json:"failed_at,omitempty"`
	Failure     string             `json:"failure,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	PublishedAt *time.Time         `json:"published_at,omitempty"`
}

type AuditEvent struct {
	ID        int64           `json:"id"`
	Version   *int64          `json:"version,omitempty"`
	Action    string          `json:"action"`
	Actor     string          `json:"actor,omitempty"`
	Message   string          `json:"message,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type DraftInput struct {
	Routes      gatewaycfg.Routing
	CreatedBy   string
	Message     string
	BaseVersion *int64
}

type Store interface {
	EnsureSchema(context.Context) error
	Active(context.Context) (Version, error)
	GetVersion(context.Context, int64) (Version, error)
	ListVersions(context.Context, int) ([]Version, error)
	ListDrafts(context.Context, int) ([]Version, error)
	SaveDraft(context.Context, DraftInput) (Version, error)
	UpdateDraft(context.Context, int64, DraftInput) (Version, error)
	ValidateDraft(context.Context, int64, string) (Version, error)
	Publish(context.Context, int64, PublishOptions) (Version, error)
	Rollback(context.Context, int64, string, string) (Version, error)
	MarkFailed(context.Context, int64, string) error
	ListAuditEvents(context.Context, int) ([]AuditEvent, error)
	Close() error
}

type PublishOptions struct {
	BaseVersion                    *int64
	Actor                          string
	RequireValidationBeforePublish bool
	RequireBaseVersionMatch        bool
}

func Checksum(routes gatewaycfg.Routing) (string, []byte, error) {
	payload, err := json.Marshal(routes)
	if err != nil {
		return "", nil, fmt.Errorf("marshal routes: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), payload, nil
}
