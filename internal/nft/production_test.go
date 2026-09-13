//go:build !networkdebug

package nft

import (
	"github.com/elllkere/zakop/internal/config"
	"strings"
	"testing"
)

func TestProductionIgnoresLegacyDebugFlag(t *testing.T) {
	cfg, err := config.Parse("config main 'main'\n option debug_network '1'\n")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "debug_") {
		t.Fatal("production diagnostics enabled")
	}
}
