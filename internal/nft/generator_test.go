package nft

import (
	"net"
	"strings"
	"testing"

	"github.com/elllkere/zakop/internal/config"
	"github.com/elllkere/zakop/internal/policy"
)

func TestGenerateOrder(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Clients = []config.Client{
		{Name: "direct", IP: "192.168.8.50", Policy: "direct"},
		{Name: "proxy", IP: "192.168.8.100", Policy: "proxy"},
	}
	cfg.Rules = []config.Rule{providerRule("cloudflare", 100, "proxy")}
	out, err := Generate(Input{
		Config:    cfg,
		RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("1.1.1.0/24")},
	})
	if err != nil {
		t.Fatal(err)
	}

	guard := strings.Index(out, "ip saddr @lan_subnets4 jump from_lan")
	fromLAN := strings.Index(out, "\tchain from_lan")
	dnat := strings.Index(out, "ct status dnat return")
	direct := strings.Index(out, "ip saddr @direct_clients4 return")
	reserved := strings.Index(out, "ip daddr @reserved4 return")
	forced := strings.Index(out, "ip saddr @proxy_clients4 meta l4proto { tcp, udp } jump to_proxy_0000")
	subnet := strings.Index(out, "ip daddr @rule4_0000 meta l4proto { tcp, udp } jump to_proxy_0000")
	def := strings.Index(out, "\t\treturn\n\t}\n\tchain to_proxy_0000")

	if !(guard >= 0 && guard < fromLAN && fromLAN < dnat && dnat < direct && direct < reserved && reserved < forced && forced < subnet && subnet < def) {
		t.Fatalf("unexpected rule order:\n%s", out)
	}
	if strings.Count(out, "1.1.1.0/24") != 1 {
		t.Fatalf("provider CIDR was not emitted once:\n%s", out)
	}
}

func TestGenerateReturnsDNATConnectionsBeforeProxyClientPolicy(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Clients = []config.Client{
		{Name: "rdp_host", IP: "192.168.8.100", Policy: "proxy"},
	}
	out, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}

	dnat := strings.Index(out, "ct status dnat return")
	forced := strings.Index(out, "ip saddr @proxy_clients4 meta l4proto { tcp, udp } jump to_proxy_0000")
	if dnat < 0 || forced < 0 || dnat > forced {
		t.Fatalf("DNAT-associated port-forward replies must return before proxy client policy:\n%s", out)
	}
}

func TestGenerateUsesSetForManyProviderCIDRs(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Rules = []config.Rule{providerRule("large", 100, "proxy")}
	cidrs := make([]string, 0, 7000)
	for i := 0; i < 7000; i++ {
		cidrs = append(cidrs, "100."+string(rune('0'+(i%10)))+".0.0/24")
	}
	// Use deterministic non-overlapping input after the cheap size check above.
	var provider []string
	for a := 1; a <= 28; a++ {
		for b := 0; b < 250; b++ {
			provider = append(provider, "11."+itoa(a)+"."+itoa(b)+".0/24")
		}
	}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs(provider...)}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "ip daddr @rule4_0000 meta l4proto { tcp, udp } jump to_proxy_0000") != 1 {
		t.Fatalf("expected one set-based proxy rule:\n%s", out)
	}
	if strings.Count(out, "set rule4_0000") != 1 {
		t.Fatalf("expected one rule4_0000 set:\n%s", out)
	}
	if strings.Count(out, "jump to_proxy_0000") > 3 {
		t.Fatalf("expected no per-CIDR proxy jump rules:\n%s", out)
	}
}

func TestGenerateInlineIPCIDRRule(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Rules = []config.Rule{{
		Name:     "inline_ip",
		Enabled:  true,
		Action:   "proxy",
		Outbound: "test",
		DNSMode:  "real_ip",
		IPCIDRs: []string{
			"8.8.8.8",
		},
	}}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("8.8.8.8")}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "8.8.8.8/32") || !strings.Contains(out, "ip daddr @rule4_0000 meta l4proto { tcp, udp } jump to_proxy_0000") {
		t.Fatalf("inline IP/CIDR rule was not emitted:\n%s", out)
	}
}

func TestGenerateBlockIPCIDRRuleDropsPacket(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Rules = []config.Rule{{
		Name:    "blocked_ip",
		Enabled: true,
		Action:  "block",
		DNSMode: "real_ip",
		IPCIDRs: []string{
			"8.8.8.8",
		},
	}}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("8.8.8.8")}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "8.8.8.8/32") || !strings.Contains(out, "ip daddr @rule4_0000 meta l4proto { tcp, udp } drop") {
		t.Fatalf("inline IP/CIDR block rule was not emitted as drop:\n%s", out)
	}
	if strings.Contains(out, "ip daddr @rule4_0000 meta l4proto { tcp, udp } return") {
		t.Fatalf("IP/CIDR block rule must not return like direct:\n%s", out)
	}
}

func TestGenerateIPCIDRRuleStopsDroppingWhenActionChangesFromBlock(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Rules = []config.Rule{{
		Name:    "changed_ip",
		Enabled: true,
		Action:  "block",
		DNSMode: "real_ip",
		IPCIDRs: []string{
			"8.8.8.8",
		},
	}}
	ruleCIDRs := map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("8.8.8.8")}

	blocked, err := Generate(Input{Config: cfg, RuleCIDRs: ruleCIDRs})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(blocked, "ip daddr @rule4_0000 meta l4proto { tcp, udp } drop") {
		t.Fatalf("initial block rule did not drop:\n%s", blocked)
	}

	cfg.Rules[0].Action = "direct"
	direct, err := Generate(Input{Config: cfg, RuleCIDRs: ruleCIDRs})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(direct, "ip daddr @rule4_0000 meta l4proto { tcp, udp } drop") {
		t.Fatalf("direct rule must remove previous block drop:\n%s", direct)
	}
	if !strings.Contains(direct, "ip daddr @rule4_0000 meta l4proto { tcp, udp } return") {
		t.Fatalf("direct rule must return after block is removed:\n%s", direct)
	}

	cfg.Rules[0].Action = "proxy"
	cfg.Rules[0].Outbound = "test"
	proxy, err := Generate(Input{Config: cfg, RuleCIDRs: ruleCIDRs})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(proxy, "ip daddr @rule4_0000 meta l4proto { tcp, udp } drop") {
		t.Fatalf("proxy rule must remove previous block drop:\n%s", proxy)
	}
	if !strings.Contains(proxy, "ip daddr @rule4_0000 meta l4proto { tcp, udp } jump to_proxy_0000") {
		t.Fatalf("proxy rule must jump after block is removed:\n%s", proxy)
	}
}

func TestGenerateMixedDomainAndProviderIPRule(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Rules = []config.Rule{{
		Name:           "mixed",
		Enabled:        true,
		Action:         "proxy",
		Outbound:       "test",
		DNSMode:        "auto",
		DomainContains: []string{"youtube"},
		IPProviders:    []string{"cloudflare"},
	}}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("1.1.1.0/24")}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "set rule4_0000") || !strings.Contains(out, "1.1.1.0/24") {
		t.Fatalf("mixed rule provider CIDR set was not emitted:\n%s", out)
	}
	if !strings.Contains(out, "ip daddr @rule4_0000 meta l4proto { tcp, udp } jump to_proxy_0000") {
		t.Fatalf("mixed rule provider packet rule was not emitted:\n%s", out)
	}
}

func TestGenerateProviderRuleTCPDstPort(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	rule := providerRule("cloudflare", 100, "proxy")
	rule.Proto = []string{"tcp"}
	rule.DstPorts = []string{"443"}
	cfg.Rules = []config.Rule{rule}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("1.1.1.0/24")}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ip daddr @rule4_0000 tcp dport 443 jump to_proxy_0000") {
		t.Fatalf("tcp dst port rule missing:\n%s", out)
	}
	if strings.Contains(out, "udp dport 443") {
		t.Fatalf("tcp-only rule must not emit udp match:\n%s", out)
	}
}

func TestGenerateProviderRuleUDPDstPort(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	rule := providerRule("cloudflare", 100, "proxy")
	rule.Proto = []string{"udp"}
	rule.DstPorts = []string{"443"}
	cfg.Rules = []config.Rule{rule}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("1.1.1.0/24")}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ip daddr @rule4_0000 udp dport 443 jump to_proxy_0000") {
		t.Fatalf("udp dst port rule missing:\n%s", out)
	}
	if strings.Contains(out, "tcp dport 443") {
		t.Fatalf("udp-only rule must not emit tcp match:\n%s", out)
	}
}

func TestGenerateProviderRuleTCPUDPDstPort(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	rule := providerRule("cloudflare", 100, "proxy")
	rule.Proto = []string{"tcp", "udp"}
	rule.DstPorts = []string{"443"}
	cfg.Rules = []config.Rule{rule}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("1.1.1.0/24")}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, " dport 443 jump to_proxy_0000") != 2 ||
		!strings.Contains(out, "ip daddr @rule4_0000 tcp dport 443 jump to_proxy_0000") ||
		!strings.Contains(out, "ip daddr @rule4_0000 udp dport 443 jump to_proxy_0000") {
		t.Fatalf("tcp+udp dst port rules missing:\n%s", out)
	}
}

func TestGenerateProviderRuleDefaultsPortsToTCPUDP(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	rule := providerRule("cloudflare", 100, "proxy")
	rule.DstPorts = []string{"443"}
	cfg.Rules = []config.Rule{rule}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("1.1.1.0/24")}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "tcp dport 443 jump to_proxy_0000") || !strings.Contains(out, "udp dport 443 jump to_proxy_0000") {
		t.Fatalf("port rule without proto must default to tcp+udp:\n%s", out)
	}
}

func TestGenerateProviderRulePortRange(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	rule := providerRule("cloudflare", 100, "proxy")
	rule.Proto = []string{"tcp"}
	rule.DstPorts = []string{"1000-2000"}
	cfg.Rules = []config.Rule{rule}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("1.1.1.0/24")}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ip daddr @rule4_0000 tcp dport 1000-2000 jump to_proxy_0000") {
		t.Fatalf("tcp dst port range rule missing:\n%s", out)
	}
}

func TestGenerateMixedDomainAndProviderPortRule(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Rules = []config.Rule{{
		Name:           "mixed",
		Enabled:        true,
		Action:         "proxy",
		Outbound:       "test",
		DNSMode:        "auto",
		DomainContains: []string{"youtube"},
		IPProviders:    []string{"cloudflare"},
		Proto:          []string{"tcp"},
		DstPorts:       []string{"443"},
	}}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("1.1.1.0/24")}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ip daddr @rule4_0000 tcp dport 443 jump to_proxy_0000") {
		t.Fatalf("mixed rule provider port match missing:\n%s", out)
	}
}

func TestGenerateCounters(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = true
	cfg.Rules = []config.Rule{providerRule("cloudflare", 100, "proxy")}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("1.1.1.0/24")}})
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{
		"ip saddr @direct_clients4 counter return",
		"ip daddr @reserved4 counter return",
		"ip saddr @proxy_clients4 meta l4proto { tcp, udp } counter jump to_proxy_0000",
		"ip daddr @rule4_0000 meta l4proto { tcp, udp } counter jump to_proxy_0000",
		"meta l4proto { tcp, udp } counter meta mark set",
	} {
		if !strings.Contains(out, rule) {
			t.Fatalf("missing counter rule %q:\n%s", rule, out)
		}
	}
}

func TestGenerateProviderPortRuleCounters(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = true
	rule := providerRule("cloudflare", 100, "proxy")
	rule.Proto = []string{"tcp"}
	rule.DstPorts = []string{"443"}
	cfg.Rules = []config.Rule{rule}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("1.1.1.0/24")}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ip daddr @rule4_0000 tcp dport 443 counter jump to_proxy_0000") {
		t.Fatalf("port rule counter missing:\n%s", out)
	}
}

func TestGenerateCustomModeNoGlobalCatchAll(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Main.RoutingMode = "custom"
	out, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\t\tmeta l4proto { tcp, udp } jump to_proxy_0000\n") {
		t.Fatalf("custom mode must not include global catch-all:\n%s", out)
	}
	if strings.Contains(out, "ip daddr 198.18.0.0/15 meta l4proto") {
		t.Fatalf("custom mode without domain proxy rules must not capture FakeIP:\n%s", out)
	}
}

func TestGenerateSimpleModeUsesSimpleIPRule(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Main.RoutingMode = "simple"
	cfg.Main.SimpleRule = providerRule("simple_ips", 100, "proxy")
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{0: policy.MustIPv4CIDRs("1.1.1.0/24")}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "ip daddr 198.18.0.0/15 meta l4proto") {
		t.Fatalf("simple IP-only mode must not capture FakeIP:\n%s", out)
	}
	if !strings.Contains(out, "ip daddr @rule4_0000 meta l4proto { tcp, udp } jump to_proxy_0000") {
		t.Fatalf("simple IP provider rule missing:\n%s", out)
	}
	if strings.Contains(out, "\t\tmeta l4proto { tcp, udp } jump to_proxy_0000\n") {
		t.Fatalf("simple mode must not include global catch-all:\n%s", out)
	}
}

func TestGenerateGlobalModeCatchAll(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Main.RoutingMode = "global"
	cfg.Clients = []config.Client{
		{Name: "direct", IP: "192.168.8.50", Policy: "direct"},
		{Name: "default", IP: "192.168.8.60", Policy: "default"},
	}
	out, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	guard := strings.Index(out, "ip saddr @lan_subnets4 jump from_lan")
	nonLANReturn := strings.Index(out, "ip saddr @lan_subnets4 jump from_lan\n\t\treturn\n\t}\n\tchain from_lan")
	direct := strings.Index(out, "ip saddr @direct_clients4 return")
	reserved := strings.Index(out, "ip daddr @reserved4 return")
	global := strings.Index(out, "\t\tmeta l4proto { tcp, udp } jump to_proxy_0000\n")
	if !(guard >= 0 && nonLANReturn >= 0 && guard < direct && direct < reserved && reserved < global) {
		t.Fatalf("global rule order is wrong:\n%s", out)
	}
	if strings.Contains(out, "ip daddr 198.18.0.0/15 meta l4proto") {
		t.Fatalf("global mode should not need fakeip destination rule:\n%s", out)
	}
}

func TestCustomClientDirectReturnsBeforeProxy(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Main.RoutingMode = "custom"
	cfg.Clients = []config.Client{{Name: "pc", IP: "192.168.8.50", Policy: "direct"}}
	cfg.Rules = []config.Rule{{
		Name:           "domain_proxy",
		Enabled:        true,
		Action:         "proxy",
		Outbound:       "test",
		DNSMode:        "auto",
		DomainContains: []string{"example"},
	}}
	out, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	directRule := strings.Index(out, "ip saddr @direct_clients4 return")
	fakeRule := strings.Index(out, "ip daddr 198.18.0.0/15 meta l4proto { tcp, udp } jump to_proxy_0000")
	if !(directRule >= 0 && fakeRule >= 0 && directRule < fakeRule) {
		t.Fatalf("direct client must return before fakeip proxy:\n%s", out)
	}
}

func TestCustomClientProxySourceRule(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Main.RoutingMode = "custom"
	cfg.Clients = []config.Client{{Name: "pc", IP: "192.168.8.100", Policy: "proxy"}}
	out, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "set proxy_clients4") || !strings.Contains(out, "192.168.8.100") {
		t.Fatalf("proxy client set missing:\n%s", out)
	}
	if !strings.Contains(out, "ip saddr @proxy_clients4 meta l4proto { tcp, udp } jump to_proxy_0000") {
		t.Fatalf("proxy client source rule missing:\n%s", out)
	}
}

func TestGlobalClientDefaultUsesCatchAll(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Main.RoutingMode = "global"
	cfg.Clients = []config.Client{{Name: "pc", IP: "192.168.8.60", Policy: "default"}}
	out, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "192.168.8.60") {
		t.Fatalf("default client must not be placed in direct/proxy sets:\n%s", out)
	}
	if !strings.Contains(out, "ip saddr @lan_subnets4 jump from_lan") {
		t.Fatalf("LAN guard missing:\n%s", out)
	}
	if !strings.Contains(out, "\t\tmeta l4proto { tcp, udp } jump to_proxy_0000\n") {
		t.Fatalf("global catch-all missing:\n%s", out)
	}
}

func TestGenerateLANSubnetsSet(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Main.LANSubnets = []string{"192.168.10.0/24", "192.168.8.0/24", "192.168.8.0/24"}
	out, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "set lan_subnets4") {
		t.Fatalf("lan_subnets4 set missing:\n%s", out)
	}
	if strings.Count(out, "192.168.8.0/24") != 1 || strings.Count(out, "192.168.10.0/24") != 1 {
		t.Fatalf("LAN subnets must be normalized and deduplicated:\n%s", out)
	}
	if !strings.Contains(out, "ip saddr @lan_subnets4 jump from_lan\n\t\treturn\n\t}\n\tchain from_lan") {
		t.Fatalf("non-LAN traffic must return before policy rules:\n%s", out)
	}
}

func TestGenerateLANIfaceGuard(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Main.LANIfaces = []string{"br-lan", "lan0"}
	out, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `iifname { "br-lan", "lan0" } ip saddr @lan_subnets4 jump from_lan`) {
		t.Fatalf("LAN iface guard missing:\n%s", out)
	}
}

func TestGenerateRedirectsLANPlainDNSIntoDNSMasq(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Main.LANIfaces = []string{"br-lan", "lan0"}
	out, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"chain dns_prerouting",
		"type nat hook prerouting priority dstnat; policy accept;",
		`iifname { "br-lan", "lan0" } ip saddr @lan_subnets4 udp dport 53 redirect to :53`,
		`iifname { "br-lan", "lan0" } ip saddr @lan_subnets4 tcp dport 53 redirect to :53`,
		"udp dport 53 return",
		"tcp dport 53 return",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing LAN DNS interception %q:\n%s", want, out)
		}
	}
	dnsReturn := strings.Index(out, "udp dport 53 return")
	proxyPolicy := strings.Index(out, "ip saddr @proxy_clients4")
	if dnsReturn < 0 || proxyPolicy < 0 || dnsReturn > proxyPolicy {
		t.Fatalf("plain DNS must bypass TProxy before the later nat redirect:\n%s", out)
	}
}

func TestGenerateDoesNotRedirectDNSWhenDNSMasqManagementDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.Main.ManageDNSMasq = false
	out, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "chain dns_prerouting") || strings.Contains(out, "redirect to :53") || strings.Contains(out, "dport 53") {
		t.Fatalf("DNS interception requires manage_dnsmasq=1:\n%s", out)
	}
}

func TestGenerateGlobalNonLANReturnsBeforeCatchAll(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Main.RoutingMode = "global"
	out, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	nonLANReturn := strings.Index(out, "\t\treturn\n\t}\n\tchain from_lan")
	global := strings.Index(out, "\t\tmeta l4proto { tcp, udp } jump to_proxy_0000\n")
	if !(nonLANReturn >= 0 && global >= 0 && nonLANReturn < global) {
		t.Fatalf("non-LAN return must be before global proxy rule:\n%s", out)
	}
}

func TestGenerateProviderRulePriorityDirectBeforeProxy(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Rules = []config.Rule{
		providerRule("direct_google", 10, "direct"),
		providerRule("proxy_all", 20, "proxy"),
	}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{
		0: policy.MustIPv4CIDRs("8.8.8.0/24"),
		1: policy.MustIPv4CIDRs("0.0.0.0/0"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	direct := strings.Index(out, "ip daddr @rule4_0000 meta l4proto { tcp, udp } return")
	proxy := strings.Index(out, "ip daddr @rule4_0001 meta l4proto { tcp, udp } jump to_proxy_0000")
	if !(direct >= 0 && proxy >= 0 && direct < proxy) {
		t.Fatalf("direct provider rule must be before proxy rule:\n%s", out)
	}
}

func TestGenerateProviderRulePriorityProxyBeforeDirect(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Rules = []config.Rule{
		providerRule("proxy_youtube", 10, "proxy"),
		providerRule("direct_google", 20, "direct"),
	}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{
		0: policy.MustIPv4CIDRs("8.8.8.0/24"),
		1: policy.MustIPv4CIDRs("8.8.8.0/24"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	proxy := strings.Index(out, "ip daddr @rule4_0000 meta l4proto { tcp, udp } jump to_proxy_0000")
	direct := strings.Index(out, "ip daddr @rule4_0001 meta l4proto { tcp, udp } return")
	if !(proxy >= 0 && direct >= 0 && proxy < direct) {
		t.Fatalf("proxy provider rule must be before direct rule:\n%s", out)
	}
}

func TestGenerateRoutesDifferentIPRulesToDifferentTProxyPorts(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	cfg.Outbounds = append(cfg.Outbounds, config.Outbound{
		Enabled:  true,
		Tag:      "second",
		Type:     "trojan",
		Server:   "second.example.com",
		Port:     443,
		Password: "secret",
	})
	cfg.Rules = []config.Rule{
		{Name: "first", Enabled: true, Priority: 100, Action: "proxy", Outbound: "test", DNSMode: "real_ip", IPCIDRs: []string{"1.1.1.1"}},
		{Name: "second", Enabled: true, Priority: 200, Action: "proxy", Outbound: "second", DNSMode: "real_ip", IPCIDRs: []string{"8.8.8.8"}},
	}

	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{
		0: policy.MustIPv4CIDRs("1.1.1.1"),
		1: policy.MustIPv4CIDRs("8.8.8.8"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ip daddr @rule4_0000 meta l4proto { tcp, udp } jump to_proxy_0000",
		"ip daddr @rule4_0001 meta l4proto { tcp, udp } jump to_proxy_0001",
		"chain to_proxy_0000",
		"tproxy ip to 127.0.0.1:16001",
		"chain to_proxy_0001",
		"tproxy ip to 127.0.0.1:16002",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing per-outbound TProxy routing %q:\n%s", want, out)
		}
	}
}

func TestGenerateProviderPortRulePriorityStable(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	first := providerRule("first_https", 10, "proxy")
	first.Proto = []string{"tcp", "udp"}
	first.DstPorts = []string{"443"}
	second := providerRule("second_plain", 20, "direct")
	cfg.Rules = []config.Rule{first, second}
	out, err := Generate(Input{Config: cfg, RuleCIDRs: map[int][]*net.IPNet{
		0: policy.MustIPv4CIDRs("1.1.1.0/24"),
		1: policy.MustIPv4CIDRs("8.8.8.0/24"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	firstTCP := strings.Index(out, "ip daddr @rule4_0000 tcp dport 443 jump to_proxy_0000")
	firstUDP := strings.Index(out, "ip daddr @rule4_0000 udp dport 443 jump to_proxy_0000")
	secondRule := strings.Index(out, "ip daddr @rule4_0001 meta l4proto { tcp, udp } return")
	if !(firstTCP >= 0 && firstUDP >= 0 && secondRule >= 0 && firstTCP < firstUDP && firstUDP < secondRule) {
		t.Fatalf("port-expanded rule priority is unstable:\n%s", out)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func providerRule(name string, priority int, action string) config.Rule {
	return config.Rule{
		Name:     name,
		Enabled:  true,
		Priority: priority,
		Action:   action,
		Outbound: "test",
		DNSMode:  "real_ip",
		Files:    []string{"/tmp/" + name + ".txt"},
	}
}

func testConfig() config.Config {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{{
		Enabled: true,
		Tag:     "test",
		Type:    "vless",
		Server:  "example.com",
		Port:    443,
		UUID:    "a3482e88-686a-4a58-8126-99c9df64b060",
	}}
	return cfg
}
