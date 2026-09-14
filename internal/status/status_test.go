package status

import (
	"reflect"
	"strings"
	"testing"

	"github.com/elllkere/zakop/internal/config"
)

func TestTProxyListenerStatus(t *testing.T) {
	cfg := config.Defaults()
	if got := tproxyListenerStatus(cfg, func(string) string {
		t.Fatal("empty configuration must not require a listener")
		return "missing"
	}); got != "not_required" {
		t.Fatalf("empty configuration: %s", got)
	}
	cfg.Outbounds = []config.Outbound{
		{Enabled: true, Tag: "one", Type: "trojan"},
		{Enabled: true, Tag: "two", Type: "trojan"},
	}
	cfg.OutboundPools = []config.OutboundPool{{Tag: "pool", Outbounds: []string{"one", "two"}}}
	for _, missing := range []string{"", "127.0.0.1:16001", "127.0.0.1:16002", "127.0.0.1:16003"} {
		var probed []string
		got := tproxyListenerStatus(cfg, func(addr string) string {
			probed = append(probed, addr)
			if addr == missing {
				return "missing"
			}
			return "present"
		})
		want := "present"
		if missing != "" {
			want = "missing"
		}
		if got != want {
			t.Fatalf("missing=%q: got %s, want %s", missing, got, want)
		}
		if missing == "" && !reflect.DeepEqual(probed, []string{"127.0.0.1:16001", "127.0.0.1:16002", "127.0.0.1:16003"}) {
			t.Fatalf("must check every outbound and pool listener: %v", probed)
		}
	}
}

func TestLocalRouteStatusMissingTable(t *testing.T) {
	got := localRouteStatusResult("Error: ipv4: FIB table does not exist.\nDump terminated\n", 2, true)
	if got != "missing" {
		t.Fatalf("got %q, want missing", got)
	}
}

func TestLocalRouteStatusPresent(t *testing.T) {
	got := localRouteStatusResult("local default dev lo scope host\n", 0, false)
	if got != "present" {
		t.Fatalf("got %q, want present", got)
	}
}

func TestListenerPresentBusyBoxNetstat(t *testing.T) {
	output := `
Active Internet connections (only servers)
Proto Recv-Q Send-Q Local Address           Foreign Address         State       PID/Program name
tcp        0      0 127.0.0.1:15353         0.0.0.0:*               LISTEN      123/sing-box
udp        0      0 127.0.0.1:5353          0.0.0.0:*                           124/zakopd
udp        0      0 127.0.0.1:16001         0.0.0.0:*                           123/sing-box
`
	for _, addr := range []string{"127.0.0.1:15353", "127.0.0.1:5353", "127.0.0.1:16001"} {
		if !listenerPresent(output, addr) {
			t.Fatalf("expected %s to be present", addr)
		}
	}
}

func TestListenerPresentSS(t *testing.T) {
	output := `
Netid State  Recv-Q Send-Q Local Address:Port Peer Address:Port
udp   UNCONN 0      0          127.0.0.1:5353      0.0.0.0:*
tcp   LISTEN 0      4096       127.0.0.1:15353     0.0.0.0:*
`
	if !listenerPresent(output, "127.0.0.1:5353") {
		t.Fatal("expected UDP listener")
	}
	if !listenerPresent(output, "127.0.0.1:15353") {
		t.Fatal("expected TCP listener")
	}
	if listenerPresent(output, "127.0.0.1:16001") {
		t.Fatal("unexpected listener")
	}
}

func TestOutboundSummaryRedactsSecrets(t *testing.T) {
	out := config.Outbound{
		Tag:              "my_vless",
		Label:            "Primary VLESS",
		Type:             "vless",
		Server:           "example.com",
		Port:             443,
		UUID:             "a3482e88-686a-4a58-8126-99c9df64b060",
		Password:         "secret-password",
		RealityPublicKey: "public-key",
		RealityShortID:   "0123456789abcdef",
		TLS:              true,
		Reality:          true,
		Transport:        "tcp",
	}
	got := OutboundSummary(out)
	if !strings.Contains(got, "my_vless(vless)") || !strings.Contains(got, "label=Primary VLESS") || !strings.Contains(got, "server=example.com:443") {
		t.Fatalf("summary missing safe fields: %s", got)
	}
	for _, secret := range []string{out.UUID, out.Password, out.RealityPublicKey, out.RealityShortID} {
		if strings.Contains(got, secret) {
			t.Fatalf("summary leaked secret %q: %s", secret, got)
		}
	}
}

func TestOutboundsSummaryIncludesBuiltins(t *testing.T) {
	got := OutboundsSummary(config.Defaults())
	if !strings.Contains(got, "direct(builtin)") || !strings.Contains(got, "blocked(builtin)") {
		t.Fatalf("summary missing builtins: %s", got)
	}
}
