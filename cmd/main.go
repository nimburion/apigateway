package main

import (
	"context"
	"fmt"
	"os"

	configcmd "github.com/nimburion/apigateway/cmd/config"
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

	serviceName := "api-gateway"

	opts := cli.AppCommandOptions{
		Name:             serviceName,
		Description:      fmt.Sprintf("Run %s service", serviceName),
		ConfigExtensions: []any{&gwCfg},
	}
	opts.Run = func(ctx context.Context, cfg *config.Config, log logger.Logger) error {
		gwCfg.ConfigDir = routes.DetermineRoutesBaseDir(opts.ConfigPath, routes.DefaultConfigPath)
		if gwCfg.ConfigDir == "" {
			if cwd, err := os.Getwd(); err == nil {
				gwCfg.ConfigDir = cwd
			}
		}
		return appserver.RunServer(cfg, &gwCfg, log)
	}
	opts.CustomCommands = append(opts.CustomCommands, routes.NewCommand(&opts, &gwCfg))
	opts.CustomCommands = append(opts.CustomCommands, openapicmd.NewCommand(&opts, &gwCfg))

	cmd := cli.NewAppCommand(opts)
	configcmd.AttachToRoot(cmd, &opts)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cli.Execute(cmd)
}
