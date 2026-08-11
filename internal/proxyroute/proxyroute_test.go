package proxyroute

import (
	"testing"

	"github.com/elllkere/neto/internal/config"
)

func TestTargetsFollowCustomOutboundOrder(t *testing.T) {
	cfg := config.Defaults()
	cfg.Main.TProxyPort = 16001
	cfg.Outbounds = []config.Outbound{
		{Enabled: true, Tag: "first"},
		{Enabled: true, Tag: "second"},
	}

	targets := Targets(cfg)
	if len(targets) != 2 {
		t.Fatalf("unexpected targets: %+v", targets)
	}
	if targets[0].Tag != "first" || targets[0].Chain != "to_proxy_0000" || targets[0].Inbound != "tproxy-0000-in" || targets[0].Port != 16001 {
		t.Fatalf("unexpected first target: %+v", targets[0])
	}
	if targets[1].Tag != "second" || targets[1].Chain != "to_proxy_0001" || targets[1].Inbound != "tproxy-0001-in" || targets[1].Port != 16002 {
		t.Fatalf("unexpected second target: %+v", targets[1])
	}
}

func TestTargetsAppendOutboundPoolsWithoutRenumberingOutbounds(t *testing.T) {
	cfg := config.Defaults()
	cfg.Main.TProxyPort = 16001
	cfg.Outbounds = []config.Outbound{
		{Enabled: true, Tag: "first"},
		{Enabled: true, Tag: "second"},
	}
	cfg.OutboundPools = []config.OutboundPool{
		{Tag: "preferred"},
		{Tag: "fallback"},
	}

	targets := Targets(cfg)
	if len(targets) != 4 {
		t.Fatalf("unexpected targets: %+v", targets)
	}
	if targets[0].Tag != "first" || targets[0].Port != 16001 || targets[1].Tag != "second" || targets[1].Port != 16002 {
		t.Fatalf("concrete targets were renumbered: %+v", targets)
	}
	if targets[2].Tag != "preferred" || targets[2].Port != 16003 || targets[3].Tag != "fallback" || targets[3].Port != 16004 {
		t.Fatalf("unexpected pool targets: %+v", targets)
	}
}
