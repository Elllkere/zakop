package openwrt

import (
	"os"
	"strings"
	"testing"
)

func TestUninstallRestoresDNSMasqFallback(t *testing.T) {
	data, err := os.ReadFile("../../embedded/uninstall.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		`state_dir="/etc/zakop/dnsmasq-state"`,
		`legacy_state_dir="/var/lib/zakop"`,
		`server_state="$state_dir/dnsmasq-server.prev"`,
		"read_state()",
		"has_non_zakop_dnsmasq_server()",
		"uci -q del_list dhcp.@dnsmasq[0].server=\"$saved_server\"",
		"uci -q del_list dhcp.@dnsmasq[0].server=\"127.0.0.1#5353\"",
		"current_noresolv=\"$(uci -q get dhcp.@dnsmasq[0].noresolv || true)\"",
		"uci -q delete dhcp.@dnsmasq[0].noresolv || true",
		"rm -rf /etc/zakop/dnsmasq-state",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("uninstall.sh missing DNSMasq cleanup fallback %q:\n%s", want, s)
		}
	}
}
