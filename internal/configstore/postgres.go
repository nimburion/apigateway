package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	baseconfig "github.com/nimburion/nimburion/pkg/config"
)

type PostgresStore struct {
	db           *sql.DB
	queryTimeout time.Duration
}

func NewPostgresStore(cfg baseconfig.Config) (*PostgresStore, error) {
	if strings.TrimSpace(cfg.Database.URL) == "" {
		return nil, errors.New("database.url is required for postgres config store")
	}
	db, err := sql.Open("postgres", cfg.Database.URL)
	if err != nil {
		return nil, fmt.Errorf("open postgres config store: %w", err)
	}
	db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.Database.ConnMaxIdleTime)
	return &PostgresStore{db: db, queryTimeout: cfg.Database.QueryTimeout}, nil
}

func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS gateway_config_versions (
  version BIGSERIAL PRIMARY KEY,
  status TEXT NOT NULL CHECK (status IN ('draft', 'validated', 'active', 'archived', 'failed')),
  routes JSONB NOT NULL,
  checksum TEXT NOT NULL,
  created_by TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  base_version BIGINT NULL,
  validated_at TIMESTAMPTZ NULL,
  failed_at TIMESTAMPTZ NULL,
  failure TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  published_at TIMESTAMPTZ NULL
);
ALTER TABLE gateway_config_versions
  DROP CONSTRAINT IF EXISTS gateway_config_versions_status_check;
ALTER TABLE gateway_config_versions
  ADD CONSTRAINT gateway_config_versions_status_check
  CHECK (status IN ('draft', 'validated', 'active', 'archived', 'failed'));
ALTER TABLE gateway_config_versions
  ADD COLUMN IF NOT EXISTS validated_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS failed_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS failure TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS gateway_config_versions_one_active
  ON gateway_config_versions ((status))
  WHERE status = 'active';
CREATE INDEX IF NOT EXISTS gateway_config_versions_created_at_idx
  ON gateway_config_versions (created_at DESC);
CREATE TABLE IF NOT EXISTS gateway_config_audit_events (
  id BIGSERIAL PRIMARY KEY,
  version BIGINT NULL REFERENCES gateway_config_versions(version),
  action TEXT NOT NULL,
  actor TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS gateway_config_audit_events_created_at_idx
  ON gateway_config_audit_events (created_at DESC);
`)
	if err != nil {
		return fmt.Errorf("ensure config store schema: %w", err)
	}
	return nil
}

func (s *PostgresStore) Active(ctx context.Context) (Version, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	row := s.db.QueryRowContext(ctx, versionSelectSQL()+` WHERE status = $1 ORDER BY version DESC LIMIT 1`, StatusActive)
	version, err := scanVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, ErrNoActiveConfig
	}
	return version, err
}

func (s *PostgresStore) ListVersions(ctx context.Context, limit int) ([]Version, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, versionSelectSQL()+` ORDER BY version DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list config versions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var versions []Version
	for rows.Next() {
		version, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate config versions: %w", err)
	}
	return versions, nil
}

func (s *PostgresStore) ListDrafts(ctx context.Context, limit int) ([]Version, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, versionSelectSQL()+` WHERE status IN ($1, $2) ORDER BY version DESC LIMIT $3`, StatusDraft, StatusValidated, limit)
	if err != nil {
		return nil, fmt.Errorf("list config drafts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var versions []Version
	for rows.Next() {
		version, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *PostgresStore) SaveDraft(ctx context.Context, input DraftInput) (Version, error) {
	checksum, payload, err := Checksum(input.Routes)
	if err != nil {
		return Version{}, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Version{}, fmt.Errorf("begin save draft transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx, `
INSERT INTO gateway_config_versions (status, routes, checksum, created_by, message, base_version)
VALUES ($1, $2::jsonb, $3, $4, $5, $6)
RETURNING version, status, routes, checksum, created_by, message, base_version, validated_at, failed_at, failure, created_at, published_at
`, StatusDraft, string(payload), checksum, input.CreatedBy, input.Message, input.BaseVersion)
	version, err := scanVersion(row)
	if err != nil {
		return Version{}, err
	}
	if err := insertAuditEvent(ctx, tx, &version.Version, "create", input.CreatedBy, input.Message, nil); err != nil {
		return Version{}, err
	}
	if err := tx.Commit(); err != nil {
		return Version{}, fmt.Errorf("commit save draft transaction: %w", err)
	}
	return version, nil
}

func (s *PostgresStore) UpdateDraft(ctx context.Context, version int64, input DraftInput) (Version, error) {
	checksum, payload, err := Checksum(input.Routes)
	if err != nil {
		return Version{}, err
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Version{}, fmt.Errorf("begin update draft transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx, `
UPDATE gateway_config_versions
SET status = $1, routes = $2::jsonb, checksum = $3, message = $4, base_version = $5,
    validated_at = NULL, failed_at = NULL, failure = ''
WHERE version = $6 AND status IN ($1, $7)
RETURNING version, status, routes, checksum, created_by, message, base_version, validated_at, failed_at, failure, created_at, published_at
`, StatusDraft, string(payload), checksum, input.Message, input.BaseVersion, version, StatusValidated)
	updated, err := scanVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, err
	}
	if err := insertAuditEvent(ctx, tx, &version, "update", input.CreatedBy, input.Message, nil); err != nil {
		return Version{}, err
	}
	if err := tx.Commit(); err != nil {
		return Version{}, fmt.Errorf("commit update draft transaction: %w", err)
	}
	return updated, nil
}

func (s *PostgresStore) ValidateDraft(ctx context.Context, version int64, actor string) (Version, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Version{}, fmt.Errorf("begin validate draft transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx, `
UPDATE gateway_config_versions
SET status = $1, validated_at = NOW()
WHERE version = $2 AND status IN ($3, $1)
RETURNING version, status, routes, checksum, created_by, message, base_version, validated_at, failed_at, failure, created_at, published_at
`, StatusValidated, version, StatusDraft)
	validated, err := scanVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, err
	}
	if err := insertAuditEvent(ctx, tx, &version, "validate", actor, "", nil); err != nil {
		return Version{}, err
	}
	if err := tx.Commit(); err != nil {
		return Version{}, fmt.Errorf("commit validate draft transaction: %w", err)
	}
	return validated, nil
}

func (s *PostgresStore) Publish(ctx context.Context, version int64, opts PublishOptions) (Version, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Version{}, fmt.Errorf("begin publish config transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('nimburion.gateway_config_publish'))`); err != nil {
		return Version{}, fmt.Errorf("acquire config publish lock: %w", err)
	}

	active, activeErr := activeVersionTx(ctx, tx)
	if opts.RequireBaseVersionMatch && opts.BaseVersion == nil {
		return Version{}, ErrBaseVersionMissing
	}
	if opts.BaseVersion != nil && (activeErr != nil || active.Version != *opts.BaseVersion) {
		return Version{}, ErrStaleBase
	}
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM gateway_config_versions WHERE version = $1 FOR UPDATE`, version).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Version{}, ErrNotFound
		}
		return Version{}, fmt.Errorf("load draft config version: %w", err)
	}
	if opts.RequireValidationBeforePublish && status != StatusValidated {
		return Version{}, ErrValidationRequired
	}
	if status != StatusDraft && status != StatusValidated {
		return Version{}, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE gateway_config_versions SET status = $1 WHERE status = $2`, StatusArchived, StatusActive); err != nil {
		return Version{}, fmt.Errorf("archive active config version: %w", err)
	}
	row := tx.QueryRowContext(ctx, `
UPDATE gateway_config_versions
SET status = $1, published_at = NOW()
WHERE version = $2
RETURNING version, status, routes, checksum, created_by, message, base_version, validated_at, failed_at, failure, created_at, published_at
`, StatusActive, version)
	published, err := scanVersion(row)
	if err != nil {
		return Version{}, err
	}
	if err := insertAuditEvent(ctx, tx, &version, "publish", opts.Actor, "", nil); err != nil {
		return Version{}, err
	}
	if err := tx.Commit(); err != nil {
		return Version{}, fmt.Errorf("commit publish config transaction: %w", err)
	}
	return published, nil
}

func (s *PostgresStore) Rollback(ctx context.Context, version int64, createdBy, message string) (Version, error) {
	target, err := s.GetVersion(ctx, version)
	if err != nil {
		return Version{}, err
	}
	draft, err := s.SaveDraft(ctx, DraftInput{
		Routes:    target.Routes,
		CreatedBy: createdBy,
		Message:   message,
	})
	if err != nil {
		return Version{}, err
	}
	return s.Publish(ctx, draft.Version, PublishOptions{Actor: createdBy})
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) GetVersion(ctx context.Context, version int64) (Version, error) {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	row := s.db.QueryRowContext(ctx, versionSelectSQL()+` WHERE version = $1`, version)
	got, err := scanVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	return got, err
}

func (s *PostgresStore) MarkFailed(ctx context.Context, version int64, failure string) error {
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin mark failed transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
UPDATE gateway_config_versions
SET status = $1, failed_at = NOW(), failure = $2
WHERE version = $3
`, StatusFailed, failure, version); err != nil {
		return fmt.Errorf("mark config version failed: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE gateway_config_versions
SET status = $1
WHERE version = (
  SELECT version FROM gateway_config_versions
  WHERE status = $2
  ORDER BY published_at DESC NULLS LAST, version DESC
  LIMIT 1
)
`, StatusActive, StatusArchived); err != nil {
		return fmt.Errorf("restore previous active config version: %w", err)
	}
	if err := insertAuditEvent(ctx, tx, &version, "failure", "", failure, nil); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) ListAuditEvents(ctx context.Context, limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	ctx, cancel := s.withTimeout(ctx)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
SELECT id, version, action, actor, message, metadata, created_at
FROM gateway_config_audit_events
ORDER BY created_at DESC
LIMIT $1
`, limit)
	if err != nil {
		return nil, fmt.Errorf("list config audit events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var events []AuditEvent
	for rows.Next() {
		var event AuditEvent
		var metadata []byte
		if err := rows.Scan(&event.ID, &event.Version, &event.Action, &event.Actor, &event.Message, &metadata, &event.CreatedAt); err != nil {
			return nil, err
		}
		event.Metadata = append([]byte(nil), metadata...)
		events = append(events, event)
	}
	return events, rows.Err()
}

func activeVersionTx(ctx context.Context, tx *sql.Tx) (Version, error) {
	row := tx.QueryRowContext(ctx, versionSelectSQL()+` WHERE status = $1 ORDER BY version DESC LIMIT 1 FOR UPDATE`, StatusActive)
	version, err := scanVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, ErrNoActiveConfig
	}
	return version, err
}

func versionSelectSQL() string {
	return `SELECT version, status, routes, checksum, created_by, message, base_version, validated_at, failed_at, failure, created_at, published_at FROM gateway_config_versions`
}

func insertAuditEvent(ctx context.Context, tx *sql.Tx, version *int64, action, actor, message string, metadata []byte) error {
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO gateway_config_audit_events (version, action, actor, message, metadata)
VALUES ($1, $2, $3, $4, $5::jsonb)
`, version, action, actor, message, string(metadata))
	if err != nil {
		return fmt.Errorf("insert config audit event: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanVersion(row rowScanner) (Version, error) {
	var version Version
	var payload []byte
	if err := row.Scan(
		&version.Version,
		&version.Status,
		&payload,
		&version.Checksum,
		&version.CreatedBy,
		&version.Message,
		&version.BaseVersion,
		&version.ValidatedAt,
		&version.FailedAt,
		&version.Failure,
		&version.CreatedAt,
		&version.PublishedAt,
	); err != nil {
		return Version{}, err
	}
	if err := json.Unmarshal(payload, &version.Routes); err != nil {
		return Version{}, fmt.Errorf("decode config version %d routes: %w", version.Version, err)
	}
	return version, nil
}

func (s *PostgresStore) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.queryTimeout <= 0 {
		return ctx, func() {}
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, s.queryTimeout)
}

var _ Store = (*PostgresStore)(nil)
var _ = gatewaycfg.Routing{}
