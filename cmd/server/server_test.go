package server

import (
	"context"
	"errors"
	"os"
	"testing"

	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	baseconfig "github.com/nimburion/nimburion/pkg/config"
	logpkg "github.com/nimburion/nimburion/pkg/observability/logger"
	"github.com/nimburion/nimburion/pkg/server"
)

func TestServiceName(t *testing.T) {
	cfg := &baseconfig.Config{}
	if got := serviceName(cfg); got != "api-gateway" {
		t.Fatalf("expected fallback name, got %s", got)
	}
	cfg.Service.Name = "Gateway"
	if got := serviceName(cfg); got != "Gateway" {
		t.Fatalf("expected service name, got %s", got)
	}
}

func TestManagementSecurityMiddleware(t *testing.T) {
	if got := managementSecurityMiddleware(false, nil, "scope"); got != nil {
		t.Fatalf("expected nil middleware when auth disabled")
	}
	if got := managementSecurityMiddleware(true, nil, "scope"); len(got) != 2 {
		t.Fatalf("expected 2 middleware when auth enabled, got %d", len(got))
	}
}

func TestRunServerNilConfig(t *testing.T) {
	err := RunServer(nil, nil, nil)
	if !errors.Is(err, ErrNilConfig) {
		t.Fatalf("expected ErrNilConfig, got %v", err)
	}
}

func TestRunServerAuthDisabled(t *testing.T) {
	cfg := &baseconfig.Config{}
	gwCfg := gatewaycfg.NewDefaultConfig()
	gwCfg.Routes = gatewaycfg.Routing{
		Groups: map[string]gatewaycfg.Group{
			"default": {
				Prefix: "/",
				Routes: []gatewaycfg.Route{
					{
						PathPrefix: "/users",
						TargetURL:  "http://example.com",
						Endpoints: []gatewaycfg.Endpoint{
							{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
						},
					},
				},
			},
		},
	}
	if err := RunServer(cfg, gwCfg, nil); err == nil {
		t.Fatalf("expected auth enabled validation error")
	}
}

func TestRunServerLoadRoutesError(t *testing.T) {
	cfg := &baseconfig.Config{}
	cfg.Auth.Enabled = true
	gwCfg := gatewaycfg.NewDefaultConfig()

	err := RunServer(cfg, gwCfg, nil)
	if err == nil {
		t.Fatalf("expected load routes error")
	}
}

type noopLogger struct{}

func (l noopLogger) Debug(string, ...any)                      {}
func (l noopLogger) Info(string, ...any)                       {}
func (l noopLogger) Warn(string, ...any)                       {}
func (l noopLogger) Error(string, ...any)                      {}
func (l noopLogger) With(...any) logpkg.Logger                 { return l }
func (l noopLogger) WithContext(context.Context) logpkg.Logger { return l }

func TestRunServerSuccessWithStubbedRun(t *testing.T) {
	originalRun := runHTTPServersWithSignals
	t.Cleanup(func() { runHTTPServersWithSignals = originalRun })
	called := false
	runHTTPServersWithSignals = func(servers *server.HTTPServers, opts *server.RunHTTPServersOptions, signals ...os.Signal) error {
		called = true
		if servers == nil || servers.Public == nil {
			t.Fatalf("expected public server to be built")
		}
		return nil
	}

	cfg := &baseconfig.Config{}
	cfg.Auth.Enabled = true
	gwCfg := &gatewaycfg.Gateway{
		Routes: gatewaycfg.Routing{
			Groups: map[string]gatewaycfg.Group{
				"default": {
					Prefix: "/",
					Routes: []gatewaycfg.Route{
						{
							PathPrefix: "/users",
							TargetURL:  "http://example.com",
							Endpoints: []gatewaycfg.Endpoint{
								{Path: "/", Methods: map[string]*gatewaycfg.Method{"GET": {}}},
							},
						},
					},
				},
			},
		},
	}
	if err := RunServer(cfg, gwCfg, noopLogger{}); err != nil {
		t.Fatalf("RunServer error: %v", err)
	}
	if !called {
		t.Fatalf("expected runHTTPServersWithSignals to be called")
	}
}
