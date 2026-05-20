package main

import (
	"os"
	"testing"

	configcmd "github.com/nimburion/apigateway/cmd/config"
	openapicmd "github.com/nimburion/apigateway/cmd/openapi"
	"github.com/nimburion/apigateway/cmd/routes"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/cli"
)

func TestCommandConstruction(t *testing.T) {
	gwCfg := *gatewaycfg.NewDefaultConfig()
	opts := cli.AppCommandOptions{
		Name:             "api-gateway",
		ConfigExtensions: []any{&gwCfg},
	}
	opts.CustomCommands = append(opts.CustomCommands, routes.NewCommand(&opts, &gwCfg))
	opts.CustomCommands = append(opts.CustomCommands, openapicmd.NewCommand(&opts, &gwCfg))

	cmd := cli.NewAppCommand(opts)
	configcmd.AttachToRoot(cmd, &opts)
	if cmd == nil {
		t.Fatalf("expected command to be constructed")
	}
	var foundGenerate bool
	var foundShow bool
	for _, subcmd := range cmd.Commands() {
		if subcmd.Name() != "config" {
			continue
		}
		for _, nested := range subcmd.Commands() {
			if nested.Name() == "generate" {
				foundGenerate = true
			}
			if nested.Name() == "show" {
				foundShow = true
			}
		}
	}
	if !foundGenerate {
		t.Fatalf("expected config generate command to be registered")
	}
	if !foundShow {
		t.Fatalf("expected config show command to be registered")
	}
}

func TestMainHelp(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })
	os.Args = []string{"api-gateway", "--help"}
	main()
}
