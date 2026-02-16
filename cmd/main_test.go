package main

import (
	"os"
	"testing"

	openapicmd "github.com/nimburion/apigateway/cmd/openapi"
	"github.com/nimburion/apigateway/cmd/routes"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/cli"
)

func TestCommandConstruction(t *testing.T) {
	gwCfg := *gatewaycfg.NewDefaultConfig()
	opts := cli.ServiceCommandOptions{
		Name:             "api-gateway",
		ConfigExtensions: []any{&gwCfg},
	}
	opts.CustomCommands = append(opts.CustomCommands, routes.NewCommand(&opts, &gwCfg))
	opts.CustomCommands = append(opts.CustomCommands, openapicmd.NewCommand(&opts, &gwCfg))

	cmd := cli.NewServiceCommand(opts)
	if cmd == nil {
		t.Fatalf("expected command to be constructed")
	}
}

func TestMainHelp(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })
	os.Args = []string{"api-gateway", "--help"}
	main()
}
