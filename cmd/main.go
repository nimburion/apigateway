package main

import (
	"context"
	"fmt"
	"os"

	openapicmd "github.com/nimburion/apigateway/cmd/openapi"
	"github.com/nimburion/apigateway/cmd/routes"
	appserver "github.com/nimburion/apigateway/cmd/server"
	gatewaycfg "github.com/nimburion/apigateway/internal/config"
	"github.com/nimburion/nimburion/pkg/cli"
	"github.com/nimburion/nimburion/pkg/config"
	"github.com/nimburion/nimburion/pkg/observability/logger"
)

func main() {
	var gwCfg = *gatewaycfg.NewDefaultConfig()
	var resolvedConfigPath string

	serviceName := "api-gateway"

	opts := cli.ServiceCommandOptions{
		Name:             serviceName,
		Description:      fmt.Sprintf("Run %s service", serviceName),
		ConfigExtensions: []any{&gwCfg},
		ConfigPathResolved: func(path string) {
			resolvedConfigPath = path
			if dir := routes.DetermineRoutesBaseDir(path, routes.DefaultConfigPath); dir != "" {
				gwCfg.ConfigDir = dir
			}
		},
		RunServer: func(ctx context.Context, cfg *config.Config, log logger.Logger) error {
			if gwCfg.ConfigDir == "" {
				gwCfg.ConfigDir = routes.DetermineRoutesBaseDir(resolvedConfigPath, routes.DefaultConfigPath)
			}
			if gwCfg.ConfigDir == "" {
				if cwd, err := os.Getwd(); err == nil {
					gwCfg.ConfigDir = cwd
				}
			}
			return appserver.RunServer(cfg, &gwCfg, log)
		},
	}
	opts.CustomCommands = append(opts.CustomCommands, routes.NewCommand(&opts, &gwCfg))
	opts.CustomCommands = append(opts.CustomCommands, openapicmd.NewCommand(&opts, &gwCfg))

	cmd := cli.NewServiceCommand(opts)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cli.Execute(cmd)
}
