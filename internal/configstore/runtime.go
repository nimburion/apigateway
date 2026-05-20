package configstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nimburion/apigateway/internal/approutes"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/apigateway/internal/hotswap"
	"github.com/nimburion/nimburion/pkg/http/router"
	logpkg "github.com/nimburion/nimburion/pkg/observability/logger"
)

type Runtime struct {
	store              Store
	router             *hotswap.Router
	middlewareRegistry map[string]func() router.MiddlewareFunc
	baseDir            string
	cachePath          string
	log                logpkg.Logger

	mu      sync.RWMutex
	current Version
}

func NewRuntime(store Store, router *hotswap.Router, middlewareRegistry map[string]func() router.MiddlewareFunc, baseDir, cachePath string, log logpkg.Logger) *Runtime {
	return &Runtime{
		store:              store,
		router:             router,
		middlewareRegistry: middlewareRegistry,
		baseDir:            baseDir,
		cachePath:          cachePath,
		log:                log,
	}
}

func (r *Runtime) CurrentRoutes() gatewaycfg.Routing {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current.Routes
}

func (r *Runtime) CurrentVersion() Version {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.current
}

func (r *Runtime) Activate(ctx context.Context, version Version) error {
	normalized, err := r.normalize(version.Routes)
	if err != nil {
		return err
	}
	version.Routes = normalized
	if err := r.router.Activate(func(next router.Router) error {
		if err := approutes.ValidateSupportedMethods(normalized); err != nil {
			return err
		}
		return approutes.Register(next, normalized, r.middlewareRegistry, r.log)
	}); err != nil {
		return fmt.Errorf("activate routes version %d: %w", version.Version, err)
	}
	r.mu.Lock()
	r.current = version
	r.mu.Unlock()
	if r.cachePath != "" {
		if err := writeLastGoodCache(r.cachePath, version); err != nil && r.log != nil {
			r.log.Warn("failed to write last-good config cache", "path", r.cachePath, "error", err)
		}
	}
	if r.log != nil {
		r.log.Info("activated gateway config version", "version", version.Version, "checksum", version.Checksum)
	}
	_ = ctx
	return nil
}

func (r *Runtime) LoadAndActivate(ctx context.Context) error {
	version, err := r.store.Active(ctx)
	if err == nil {
		return r.Activate(ctx, version)
	}
	if r.cachePath == "" {
		return err
	}
	cached, cacheErr := readLastGoodCache(r.cachePath)
	if cacheErr != nil {
		return err
	}
	if r.log != nil {
		r.log.Warn("using last-good config cache", "path", r.cachePath, "store_error", err)
	}
	return r.Activate(ctx, cached)
}

func (r *Runtime) StartPolling(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return errors.New("poll interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			active, err := r.store.Active(ctx)
			if err != nil {
				if r.log != nil {
					r.log.Warn("failed to poll active gateway config", "error", err)
				}
				continue
			}
			current := r.CurrentVersion()
			if active.Version == current.Version && active.Checksum == current.Checksum {
				continue
			}
			if err := r.Activate(ctx, active); err != nil {
				if r.log != nil {
					r.log.Error("failed to activate polled gateway config", "version", active.Version, "error", err)
				}
			}
		}
	}
}

func (r *Runtime) normalize(routes gatewaycfg.Routing) (gatewaycfg.Routing, error) {
	cfg := gatewaycfg.NewDefaultConfig()
	cfg.Routes = routes
	cfg.ConfigStore.SourceOfTruth = gatewaycfg.ConfigSourceOfTruthDatabase
	cfg.ConfigDir = r.baseDir
	if err := cfg.LoadRoutes(r.baseDir, r.middlewareRegistry); err != nil {
		return gatewaycfg.Routing{}, fmt.Errorf("validate stored routes: %w", err)
	}
	return cfg.Routes, nil
}

func writeLastGoodCache(path string, version Version) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(version, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o600)
}

func readLastGoodCache(path string) (Version, error) {
	// #nosec G304 -- last-good cache path is an operator-configured local file.
	payload, err := os.ReadFile(path)
	if err != nil {
		return Version{}, err
	}
	var version Version
	if err := json.Unmarshal(payload, &version); err != nil {
		return Version{}, err
	}
	return version, nil
}
