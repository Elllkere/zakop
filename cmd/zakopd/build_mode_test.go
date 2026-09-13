package main

import (
	"github.com/elllkere/zakop/internal/buildmode"
	"testing"
)

func TestCommandExcludedFromProduction(t *testing.T) {
	handled, err := runBuildCommand([]string{"network-debug"})
	if handled != buildmode.NetworkDebug {
		t.Fatal("wrong command availability", handled)
	}
	if buildmode.NetworkDebug && err == nil {
		t.Fatal("expected usage validation")
	}
	if !buildmode.NetworkDebug && buildUsage != "" {
		t.Fatal("production debug usage")
	}
}
