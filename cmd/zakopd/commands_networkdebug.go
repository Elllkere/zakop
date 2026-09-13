//go:build networkdebug

package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/elllkere/zakop/internal/config"
	"github.com/elllkere/zakop/internal/netdebug"
)

const buildUsage = "|network-debug"

func runBuildCommand(args []string) (bool, error) {
	if args[0] != "network-debug" {
		return false, nil
	}
	if len(args) != 2 || (args[1] != "watch" && args[1] != "manual" && args[1] != "sample" && args[1] != "ifdown" && args[1] != "singbox-exit" && args[1] != "dns-health") {
		return true, fmt.Errorf("usage: zakopd network-debug <manual|sample|watch|ifdown|singbox-exit|dns-health>")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if args[1] == "watch" {
		return true, netdebug.Watch(ctx, config.DefaultPath)
	}
	cfg, err := config.LoadFile(config.DefaultPath)
	if err != nil {
		return true, err
	}
	path, err := netdebug.Collect(ctx, cfg, netdebug.Directory, args[1], args[1] == "sample")
	if path != "" {
		fmt.Println(path)
	}
	return true, err
}
