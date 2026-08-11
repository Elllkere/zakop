package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseInitialConfig(t *testing.T) {
	cfg, err := Parse(withTestOutbound(`
config main 'main'
	option enabled '1'
	option singbox_bin '/usr/libexec/neto/sing-box'
	option singbox_dns '127.0.0.1:15353'
	option tproxy_port '16001'
	option mark '0x101'
	option table '101'
	option fakeip_enabled '1'
	option fakeip_range '198.18.0.0/15'
	option default_action 'direct'

config client
	option name 'gaming_pc'
	option ip '192.168.8.50'
	option policy 'bypass'

config rule
	option name 'youtube'
	option enabled '1'
	option priority '100'
	option action 'proxy'
	option dns_mode 'fakeip'
	option outbound 'direct'
	list domain_equals 'youtube.com'
	list domain_ends_with '.youtube.com'
	list domain_equals 'googlevideo.com'
	list domain_ends_with '.googlevideo.com'

config rule
	option name 'cloudflare'
	option enabled '1'
	option priority '200'
	option action 'proxy'
	option dns_mode 'real_ip'
	option outbound 'direct'
	list file '/etc/neto/providers/cloudflare-v4.txt'
`))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Main.Enabled || cfg.Main.TProxyPort != 16001 || cfg.Main.Table != 101 {
		t.Fatalf("unexpected main config: %+v", cfg.Main)
	}
	if cfg.Main.RoutingMode != "custom" || cfg.Main.DefaultOutbound != "direct" {
		t.Fatalf("unexpected routing defaults: %+v", cfg.Main)
	}
	if cfg.Main.UpdateVia != "direct" || cfg.Main.UpdateOutbound != "" {
		t.Fatalf("unexpected update defaults: %+v", cfg.Main)
	}
	if len(cfg.Main.LANSubnets) != 1 || cfg.Main.LANSubnets[0] != "192.168.8.0/24" {
		t.Fatalf("unexpected LAN defaults: %+v", cfg.Main.LANSubnets)
	}
	if len(cfg.Clients) != 1 || cfg.Clients[0].Policy != "direct" {
		t.Fatalf("unexpected clients: %+v", cfg.Clients)
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %+v", cfg.Rules)
	}
	if cfg.Rules[0].Action != "proxy" || cfg.Rules[0].DNSMode != "fakeip" || len(cfg.Rules[0].DomainEndsWith) != 2 || len(cfg.Rules[0].DomainEquals) != 2 {
		t.Fatalf("unexpected domain rule: %+v", cfg.Rules[0])
	}
	if cfg.Rules[1].Action != "proxy" || cfg.Rules[1].DNSMode != "real_ip" || len(cfg.Rules[1].Files) != 1 {
		t.Fatalf("unexpected provider rule: %+v", cfg.Rules[1])
	}
	if len(cfg.Outbounds) != 1 || cfg.Rules[0].Outbound != "test_proxy" || cfg.Rules[1].Outbound != "test_proxy" {
		t.Fatalf("unexpected outbound: %+v", cfg.Outbounds)
	}
}

func TestParseRequiresCustomOutboundForProxyRule(t *testing.T) {
	_, err := Parse(`
config rule
	option name 'youtube'
	option action 'proxy'
	list domain_contains 'youtube'
`)
	if err == nil || !strings.Contains(err.Error(), "requires a custom outbound") {
		t.Fatalf("got %v, want custom outbound requirement", err)
	}
}

func TestParseAssignsFirstCustomOutboundWhenLegacySelectionIsMissing(t *testing.T) {
	cfg, err := Parse(withTestOutbound(`
config rule
	option name 'youtube'
	option action 'proxy'
	list domain_contains 'youtube'
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rules[0].Outbound != "test_proxy" {
		t.Fatalf("got outbound %q, want first custom outbound", cfg.Rules[0].Outbound)
	}
}

func TestParsePreservesDifferentExplicitRuleOutbounds(t *testing.T) {
	cfg, err := Parse(`
config outbound 'first'
	option type 'vless'
	option server 'first.example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'

config outbound 'second'
	option type 'trojan'
	option server 'second.example.com'
	option port '443'
	option password 'secret'

config rule
	option name 'youtube'
	option action 'proxy'
	option outbound 'first'
	list domain_contains 'youtube'

config rule
	option name 'telegram'
	option action 'proxy'
	option outbound 'second'
	list domain_contains 'telegram'
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rules[0].Outbound != "first" || cfg.Rules[1].Outbound != "second" {
		t.Fatalf("rule outbounds collapsed: %+v", cfg.Rules)
	}
}

func TestParseMainProxyUpdateTransport(t *testing.T) {
	cfg, err := Parse(`
config main 'main'
	option update_via 'proxy'
	option update_outbound 'updater'

config outbound 'updater'
	option type 'trojan'
	option server 'example.com'
	option port '443'
	option password 'secret'
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Main.UpdateVia != "proxy" || cfg.Main.UpdateOutbound != "updater" {
		t.Fatalf("unexpected main update transport: %+v", cfg.Main)
	}
}

func TestParseRejectsInvalidMainUpdateTransport(t *testing.T) {
	for _, raw := range []string{
		"config main 'main'\n option update_via 'vpn'\n",
		"config main 'main'\n option update_via 'proxy'\n option update_outbound 'direct'\n",
		"config main 'main'\n option update_via 'proxy'\n option update_outbound 'missing'\n",
	} {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("expected invalid main update transport to fail:\n%s", raw)
		}
	}
}

func TestLoadRuleDomainFilesRestoresDefaultProviderCacheMirror(t *testing.T) {
	dir := t.TempDir()
	oldRuntimeDir := ProviderCacheDir
	oldPersistentDir := ProviderPersistentCacheDir
	ProviderCacheDir = filepath.Join(dir, "run")
	ProviderPersistentCacheDir = filepath.Join(dir, "persist")
	t.Cleanup(func() {
		ProviderCacheDir = oldRuntimeDir
		ProviderPersistentCacheDir = oldPersistentDir
	})

	provider := Provider{
		Name:      "domains",
		Enabled:   true,
		Type:      "domain",
		LocalPath: filepath.Join(ProviderCacheDir, "domains.txt"),
		URL:       "https://example.com/domains.txt",
	}
	if err := os.MkdirAll(filepath.Dir(provider.PersistentCachePath()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(provider.PersistentCachePath(), []byte("Example.COM.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Defaults()
	cfg.Rules = []Rule{{
		Name:            "domain-provider",
		Enabled:         true,
		Action:          "proxy",
		DomainProviders: []string{"domains"},
	}}
	cfg.Providers = []Provider{provider}

	if err := LoadRuleDomainFiles(&cfg); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Rules[0].DomainEquals, []string{"example.com"}) {
		t.Fatalf("unexpected restored domains: %+v", cfg.Rules[0].DomainEquals)
	}
	if !reflect.DeepEqual(cfg.Rules[0].DomainEndsWith, []string{".example.com"}) {
		t.Fatalf("unexpected restored domain suffixes: %+v", cfg.Rules[0].DomainEndsWith)
	}
	if _, err := os.Stat(provider.CachePath()); err != nil {
		t.Fatalf("runtime cache was not restored: %v", err)
	}
}

func TestLoadRuleDomainFilesMissingProviderCacheIsSkipped(t *testing.T) {
	cfg := Defaults()
	cfg.Rules = []Rule{{
		Name:            "domain-provider",
		Enabled:         true,
		Action:          "proxy",
		DomainProviders: []string{"domains"},
	}}
	cfg.Providers = []Provider{{
		Name:      "domains",
		Enabled:   true,
		Type:      "domain",
		LocalPath: filepath.Join(t.TempDir(), "domains.txt"),
		URL:       "https://example.com/domains.txt",
	}}

	if err := LoadRuleDomainFiles(&cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules[0].DomainEquals) != 0 {
		t.Fatalf("got %+v, want missing provider to compile as empty", cfg.Rules[0].DomainEquals)
	}
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "skipping provider") {
		t.Fatalf("unexpected warnings: %+v", cfg.Warnings)
	}
}

func TestProviderCachePathTreatsLegacyVarPathAsDefault(t *testing.T) {
	dir := t.TempDir()
	oldCacheDir := ProviderCacheDir
	oldPersistentDir := ProviderPersistentCacheDir
	oldLegacyDir := ProviderLegacyCacheDir
	ProviderCacheDir = filepath.Join(dir, "etc-cache")
	ProviderPersistentCacheDir = filepath.Join(dir, "etc-cache")
	ProviderLegacyCacheDir = filepath.Join(dir, "var-cache")
	t.Cleanup(func() {
		ProviderCacheDir = oldCacheDir
		ProviderPersistentCacheDir = oldPersistentDir
		ProviderLegacyCacheDir = oldLegacyDir
	})

	p := Provider{
		Name:      "cloudflare_ipv4",
		LocalPath: filepath.Join(ProviderLegacyCacheDir, "cloudflare_ipv4.txt"),
	}
	if got, want := p.CachePath(), filepath.Join(ProviderCacheDir, "cloudflare_ipv4.txt"); got != want {
		t.Fatalf("got cache path %q, want persistent default %q", got, want)
	}
	if !p.UsesDefaultCachePath() {
		t.Fatal("legacy /var cache path should be treated as default")
	}
}

func TestParseNormalizesLANSubnetHostCIDR(t *testing.T) {
	cfg, err := Parse(`
config main 'main'
	list lan_subnet '192.168.8.1/24'
	list lan_subnet '192.168.8.42/24'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Main.LANSubnets) != 1 || cfg.Main.LANSubnets[0] != "192.168.8.0/24" {
		t.Fatalf("unexpected LAN subnets: %+v", cfg.Main.LANSubnets)
	}
}

func TestClientPolicyAliases(t *testing.T) {
	cfg, err := Parse(withTestOutbound(`
config client
	option ip '192.168.8.10'
	option policy 'bypass'

config client
	option ip '192.168.8.11'
	option policy 'proxy_default'

config client
	option ip '192.168.8.12'
	option policy 'direct'

config client
	option ip '192.168.8.13'
	option policy 'default'
`))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{cfg.Clients[0].Policy, cfg.Clients[1].Policy, cfg.Clients[2].Policy, cfg.Clients[3].Policy}
	want := []string{"direct", "proxy", "direct", "default"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if len(cfg.Warnings) != 2 {
		t.Fatalf("got warnings %v, want 2 alias warnings", cfg.Warnings)
	}
}

func TestParseClientProxyOutbound(t *testing.T) {
	cfg, err := Parse(`
config outbound 'my_vless'
	option type 'vless'
	option server 'example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'

config client
	option name 'desktop'
	option ip '192.168.8.100'
	option policy 'proxy'
	option outbound 'my_vless'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Clients) != 1 || cfg.Clients[0].Outbound != "my_vless" {
		t.Fatalf("unexpected client outbound: %+v", cfg.Clients)
	}
}

func TestParseRejectsUnknownClientProxyOutbound(t *testing.T) {
	_, err := Parse(`
config client
	option name 'desktop'
	option ip '192.168.8.100'
	option policy 'proxy'
	option outbound 'missing_proxy'
`)
	if err == nil || !strings.Contains(err.Error(), "unsupported outbound") {
		t.Fatalf("got error %v, want unsupported outbound", err)
	}
}

func TestParseRejectsBuiltinClientProxyOutbound(t *testing.T) {
	_, err := Parse(`
config client
	option name 'desktop'
	option ip '192.168.8.100'
	option policy 'proxy'
	option outbound 'direct'
`)
	if err == nil || !strings.Contains(err.Error(), "must be a custom outbound") {
		t.Fatalf("got error %v, want custom outbound requirement", err)
	}
}

func TestRoutingModeOptions(t *testing.T) {
	cfg, err := Parse(withTestOutbound(`
config main 'main'
	option routing_mode 'global'
	option default_outbound 'direct'
	list lan_subnet '192.168.8.0/24'
	list lan_subnet '192.168.9.0/24'
	list lan_iface 'br-lan'
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Main.RoutingMode != "global" || cfg.Main.DefaultOutbound != "direct" {
		t.Fatalf("unexpected main: %+v", cfg.Main)
	}
	if len(cfg.Main.LANSubnets) != 2 || cfg.Main.LANSubnets[1] != "192.168.9.0/24" {
		t.Fatalf("unexpected LAN subnets: %+v", cfg.Main.LANSubnets)
	}
	if len(cfg.Main.LANIfaces) != 1 || cfg.Main.LANIfaces[0] != "br-lan" {
		t.Fatalf("unexpected LAN ifaces: %+v", cfg.Main.LANIfaces)
	}
}

func TestParseSimpleRoutingMode(t *testing.T) {
	cfg, err := Parse(`
config main 'main'
	option routing_mode 'simple'
	option simple_action 'proxy'
	option simple_outbound 'my_vless'
	list simple_domain_provider 'telegram'
	list simple_ip_provider 'cloudflare'
	list simple_domain_equals 'Example.COM.'
	list simple_domain_ends_with '.Example.ORG.'
	list simple_domain_file '/etc/neto/domains.txt'
	list simple_ip_cidr '1.1.1.1'
	list simple_ip_file '/etc/neto/ips.txt'

config provider 'telegram'
	option enabled '1'
	option type 'domain'
	option url 'https://example.com/domains.txt'

config provider 'cloudflare'
	option enabled '1'
	option type 'ip'
	option url 'https://example.com/ips.txt'

config outbound 'my_vless'
	option type 'vless'
	option server 'example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) != 0 {
		t.Fatalf("simple mode must not create config rule sections: %+v", cfg.Rules)
	}
	rules := cfg.EffectiveRules()
	if len(rules) != 1 {
		t.Fatalf("got %d effective rules, want 1", len(rules))
	}
	r := rules[0]
	if r.Name != "simple" || r.Action != "proxy" || r.Outbound != "my_vless" || r.DNSMode != "auto" {
		t.Fatalf("unexpected simple rule: %+v", r)
	}
	if strings.Join(r.DomainProviders, ",") != "telegram" || strings.Join(r.IPProviders, ",") != "cloudflare" {
		t.Fatalf("unexpected simple providers: %+v", r)
	}
	if strings.Join(r.DomainEquals, ",") != "example.com" || strings.Join(r.DomainEndsWith, ",") != ".example.org" {
		t.Fatalf("unexpected simple domains: %+v", r)
	}
	if strings.Join(r.IPCIDRs, ",") != "1.1.1.1" {
		t.Fatalf("unexpected simple CIDRs: %+v", r)
	}
	if strings.Join(r.DomainFiles, ",") != "/etc/neto/domains.txt" || strings.Join(r.Files, ",") != "/etc/neto/ips.txt" {
		t.Fatalf("unexpected simple files: %+v", r)
	}
}

func TestParseOrderedRules(t *testing.T) {
	cfg, err := Parse(withTestOutbound(`
config rule
	option name 'proxy_youtube'
	option enabled '1'
	option priority '200'
	option action 'proxy'
	option dns_mode 'fakeip'
	list domain_contains 'YouTube'
	list exclude_domain_ends_with '.youtube.kz'

config rule
	option name 'direct_kz'
	option priority '100'
	option action 'direct'
	option dns_mode 'real_ip'
	list domain_equals 'youtube.kz'
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("unexpected rules: %+v", cfg.Rules)
	}
	if cfg.Rules[0].Name != "direct_kz" || cfg.Rules[1].Name != "proxy_youtube" {
		t.Fatalf("rules were not sorted by priority: %+v", cfg.Rules)
	}
	if cfg.Rules[1].DomainContains[0] != "youtube" || cfg.Rules[1].ExcludeDomainEndsWith[0] != ".youtube.kz" {
		t.Fatalf("domain lists were not normalized: %+v", cfg.Rules[1])
	}
}

func TestParseRuleAliases(t *testing.T) {
	cfg, err := Parse(withTestOutbound(`
config rule
	option name 'aliases'
	option action 'proxy'
	option dns_mode 'fakeip'
	list domain_exact 'Example.COM.'
	list domain_keyword 'tube'
	list domain_prefix 'www'
	list domain_suffix '.example.com.'
	list exclude_domain_exact 'bad.example.com'
	list exclude_domain_keyword 'ads'
	list exclude_domain_prefix 'old'
	list exclude_domain_suffix '.bad.example.com'
`))
	if err != nil {
		t.Fatal(err)
	}
	r := cfg.Rules[0]
	if r.DomainEquals[0] != "example.com" {
		t.Fatalf("domain_exact alias failed: %+v", r)
	}
	if r.DomainContains[0] != "tube" {
		t.Fatalf("domain_keyword alias failed: %+v", r)
	}
	if r.DomainStartsWith[0] != "www" {
		t.Fatalf("domain_prefix alias failed: %+v", r)
	}
	if r.DomainEndsWith[0] != ".example.com" {
		t.Fatalf("domain_suffix alias failed: %+v", r)
	}
	if r.ExcludeDomainEquals[0] != "bad.example.com" ||
		r.ExcludeDomainContains[0] != "ads" ||
		r.ExcludeDomainStartsWith[0] != "old" ||
		r.ExcludeDomainEndsWith[0] != ".bad.example.com" {
		t.Fatalf("exclude aliases failed: %+v", r)
	}
}

func TestParseIgnoresDeprecatedMatchAll(t *testing.T) {
	cfg, err := Parse(withTestOutbound(`
config rule
	option name 'old_match_all'
	option enabled '1'
	option action 'proxy'
	option dns_mode 'fakeip'
	option match_all '1'
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("unexpected rules: %+v", cfg.Rules)
	}
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "match_all is deprecated and ignored") {
		t.Fatalf("expected match_all warning, got %+v", cfg.Warnings)
	}
	if len(cfg.Rules[0].DomainEquals)+len(cfg.Rules[0].DomainContains)+len(cfg.Rules[0].DomainStartsWith)+len(cfg.Rules[0].DomainEndsWith)+len(cfg.Rules[0].DomainFiles)+len(cfg.Rules[0].IPCIDRs)+len(cfg.Rules[0].Files)+len(cfg.Rules[0].DomainProviders)+len(cfg.Rules[0].IPProviders)+len(cfg.Rules[0].Providers) != 0 {
		t.Fatalf("match_all should not create include conditions: %+v", cfg.Rules[0])
	}
}

func TestParseRuleAlternateDomainAndIPInputs(t *testing.T) {
	cfg, err := Parse(withTestOutbound(`
config rule
	option name 'alternate_domains'
	option action 'proxy'
	option dns_mode 'auto'
	list domain_file '/etc/neto/domains/youtube.txt'
	list domain_provider 'youtube_domains'

config rule
	option name 'alternate_ips'
	option action 'proxy'
	option dns_mode 'auto'
	list ip_cidr '1.1.1.1'
	list ip_cidr '8.8.8.0/24'
	list ip_file '/etc/neto/providers/google.txt'
	list file '/etc/neto/providers/legacy.txt'
	list ip_provider 'google_ips'
	list provider 'legacy_provider'

config provider 'youtube_domains'
	option type 'domain'
	option url 'https://example.com/youtube.txt'

config provider 'google_ips'
	option type 'ip'
	option url 'https://example.com/google.txt'

config provider 'legacy_provider'
	option type 'ip'
	option url 'https://example.com/legacy.txt'
`))
	if err != nil {
		t.Fatal(err)
	}
	domainRule := cfg.Rules[0]
	if len(domainRule.DomainFiles) != 1 || domainRule.DomainFiles[0] != "/etc/neto/domains/youtube.txt" {
		t.Fatalf("domain files were not parsed: %+v", domainRule)
	}
	if len(domainRule.DomainProviders) != 1 || domainRule.DomainProviders[0] != "youtube_domains" {
		t.Fatalf("domain provider refs were not parsed: %+v", domainRule)
	}
	ipRule := cfg.Rules[1]
	if len(ipRule.IPCIDRs) != 2 || ipRule.IPCIDRs[0] != "1.1.1.1" || ipRule.IPCIDRs[1] != "8.8.8.0/24" {
		t.Fatalf("inline IP CIDRs were not parsed: %+v", ipRule)
	}
	if len(ipRule.Files) != 2 || ipRule.Files[0] != "/etc/neto/providers/google.txt" || ipRule.Files[1] != "/etc/neto/providers/legacy.txt" {
		t.Fatalf("ip_file/file aliases were not parsed: %+v", ipRule)
	}
	if len(ipRule.IPProviders) != 1 || ipRule.IPProviders[0] != "google_ips" || len(ipRule.Providers) != 1 || ipRule.Providers[0] != "legacy_provider" {
		t.Fatalf("provider refs were not parsed: %+v", ipRule)
	}
}

func TestLoadFileExpandsDomainFilesAsEquals(t *testing.T) {
	dir := t.TempDir()
	domains := filepath.Join(dir, "domains.txt")
	cfgPath := filepath.Join(dir, "neto")

	if err := os.WriteFile(domains, []byte("Example.COM.\n# comment\nexample.org\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(withTestOutbound(`
config rule
	option name 'domains'
	option action 'proxy'
	option dns_mode 'fakeip'
	list domain_file '`+domains+`'
`)), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(cfg.Rules[0].DomainEquals, ",")
	if got != "example.com,example.org" {
		t.Fatalf("unexpected domain file expansion: %q", got)
	}
}

func TestLoadFileExpandsDomainProviderCacheAsRootAndSubdomains(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "provider.txt")
	cfgPath := filepath.Join(dir, "neto")

	if err := os.WriteFile(cache, []byte("YouTube.COM. www.youtube.com\nx.com twimg.com # comment\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(withTestOutbound(`
config provider 'youtube'
	option type 'domain'
	option url 'https://example.com/youtube.txt'
	option local_path '`+cache+`'

config rule
	option name 'youtube'
	option action 'proxy'
	option dns_mode 'fakeip'
	list domain_provider 'youtube'
`)), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(cfg.Rules[0].DomainEquals, ",")
	if got != "youtube.com,www.youtube.com,x.com,twimg.com" {
		t.Fatalf("unexpected provider domain expansion: %q", got)
	}
	gotSuffixes := strings.Join(cfg.Rules[0].DomainEndsWith, ",")
	if gotSuffixes != ".youtube.com,.www.youtube.com,.x.com,.twimg.com" {
		t.Fatalf("unexpected provider domain suffix expansion: %q", gotSuffixes)
	}
}

func TestParseRejectsInvalidInlineIPCIDR(t *testing.T) {
	if _, err := Parse(`
config rule
	option name 'bad_ip'
	option action 'proxy'
	list ip_cidr '2001:db8::/32'
`); err == nil {
		t.Fatal("expected invalid inline IPv4 CIDR to be rejected")
	}
}

func TestParseRejectsProviderCIDRFakeIP(t *testing.T) {
	_, err := Parse(`
config provider 'cloudflare'
	option type 'ip'
	option url 'https://example.com/cloudflare.txt'

config rule
	option name 'cloudflare'
	option action 'proxy'
	option dns_mode 'fakeip'
	list ip_provider 'cloudflare'
`)
	if err == nil || !strings.Contains(err.Error(), "provider/CIDR rules require real DNS because FakeIP hides destination IP") {
		t.Fatalf("got error %v, want provider/CIDR fakeip rejection", err)
	}
}

func TestParseAllowsMixedDomainAndProviderCIDRRule(t *testing.T) {
	cfg, err := Parse(withTestOutbound(`
config provider 'cloudflare'
	option type 'ip'
	option url 'https://example.com/cloudflare.txt'

config rule
	option name 'mixed'
	option action 'proxy'
	option dns_mode 'auto'
	list domain_contains 'youtube'
	list ip_provider 'cloudflare'
`))
	if err != nil {
		t.Fatalf("mixed domain+provider rule should validate: %v", err)
	}
	if len(cfg.Rules) != 1 || len(cfg.Rules[0].DomainContains) != 1 || len(cfg.Rules[0].IPProviders) != 1 {
		t.Fatalf("unexpected mixed rule parse: %+v", cfg.Rules)
	}
}

func TestParseAllowsDomainOnlyRuleWithoutProvider(t *testing.T) {
	cfg, err := Parse(withTestOutbound(`
config rule
	option name 'youtube'
	option action 'proxy'
	list domain_contains 'youtube'
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) != 1 || len(cfg.Rules[0].DomainContains) != 1 || cfg.Rules[0].DomainContains[0] != "youtube" {
		t.Fatalf("unexpected domain-only rule: %+v", cfg.Rules)
	}
}

func TestParseValidatesPacketPortRange(t *testing.T) {
	cfg, err := Parse(withTestOutbound(`
config provider 'cloudflare'
	option type 'ip'
	option url 'https://example.com/cloudflare.txt'

config rule
	option name 'cloudflare_https'
	option action 'proxy'
	list ip_provider 'cloudflare'
	list proto 'tcp'
	list dst_port '1000-2000'
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) != 1 || len(cfg.Rules[0].DstPorts) != 1 || cfg.Rules[0].DstPorts[0] != "1000-2000" {
		t.Fatalf("unexpected port rule parse: %+v", cfg.Rules)
	}
}

func TestParseRejectsInvalidPacketPort(t *testing.T) {
	_, err := Parse(`
config provider 'cloudflare'
	option type 'ip'
	option url 'https://example.com/cloudflare.txt'

config rule
	option name 'bad_port'
	option action 'proxy'
	list ip_provider 'cloudflare'
	list dst_port '70000'
`)
	if err == nil || !strings.Contains(err.Error(), "invalid port match") {
		t.Fatalf("got error %v, want invalid port rejection", err)
	}
}

func TestParseRejectsInvalidPacketProto(t *testing.T) {
	_, err := Parse(`
config provider 'cloudflare'
	option type 'ip'
	option url 'https://example.com/cloudflare.txt'

config rule
	option name 'bad_proto'
	option action 'proxy'
	list ip_provider 'cloudflare'
	list proto 'icmp'
`)
	if err == nil || !strings.Contains(err.Error(), "unsupported proto") {
		t.Fatalf("got error %v, want invalid proto rejection", err)
	}
}

func TestParseRejectsDomainOnlyRuleWithPortMatch(t *testing.T) {
	_, err := Parse(`
config rule
	option name 'youtube_https'
	option action 'proxy'
	list domain_contains 'youtube'
	list dst_port '443'
`)
	if err == nil || !strings.Contains(err.Error(), "Port matching is packet-level and requires provider/CIDR/IP matchers in v1") {
		t.Fatalf("got error %v, want domain-only port rejection", err)
	}
}

func TestParseRejectsDomainOnlyRuleWithProtoMatch(t *testing.T) {
	_, err := Parse(`
config rule
	option name 'youtube_tcp'
	option action 'proxy'
	list domain_contains 'youtube'
	list proto 'tcp'
`)
	if err == nil || !strings.Contains(err.Error(), "Protocol matching is packet-level and requires provider/CIDR/IP matchers in v1") {
		t.Fatalf("got error %v, want domain-only proto rejection", err)
	}
}

func TestParseAllowsMixedDomainProviderRuleWithPortMatch(t *testing.T) {
	cfg, err := Parse(withTestOutbound(`
config provider 'cloudflare'
	option type 'ip'
	option url 'https://example.com/cloudflare.txt'

config rule
	option name 'mixed_https'
	option action 'proxy'
	list domain_contains 'youtube'
	list ip_provider 'cloudflare'
	list dst_port '443'
`))
	if err != nil {
		t.Fatalf("mixed rule with port should validate: %v", err)
	}
	if len(cfg.Rules) != 1 || len(cfg.Rules[0].DomainContains) != 1 || len(cfg.Rules[0].IPProviders) != 1 || len(cfg.Rules[0].DstPorts) != 1 {
		t.Fatalf("unexpected mixed port rule parse: %+v", cfg.Rules)
	}
}

func TestParseProvider(t *testing.T) {
	cfg, err := Parse(`
config outbound 'updater'
	option type 'trojan'
	option server 'example.com'
	option port '443'
	option password 'secret'

config provider 'youtube'
	option label 'YouTube'
	option type 'domain'
	option source 'url'
	option url 'https://example.com/youtube.txt'
	option auto_update '1'
	option update_hour '3'
	option update_minute '17'
	option update_via 'proxy'
	option update_outbound 'updater'
	option local_path '/var/lib/neto/providers/youtube.txt'
	option last_update '1710000000'
	option item_count '2'

config rule
	option name 'youtube'
	option action 'proxy'
	list domain_provider 'youtube'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("unexpected providers: %+v", cfg.Providers)
	}
	p := cfg.Providers[0]
	if p.Name != "youtube" || p.Label != "YouTube" || p.Type != "domain" || p.Source != "url" || p.URL == "" || !p.AutoUpdate || p.UpdateSchedule != UpdateScheduleTime || p.UpdateHour != 3 || p.UpdateMinute != 17 || p.UpdateIntervalMinutes != DefaultUpdateIntervalMinutes || p.UpdateVia != "proxy" || p.UpdateOutbound != "updater" || p.ItemCount != 2 {
		t.Fatalf("unexpected provider: %+v", p)
	}
}

func TestParseProviderEnabledOptionIsIgnored(t *testing.T) {
	cfg, err := Parse(`
config provider 'domains'
	option enabled '0'
	option type 'domain'
	option url 'https://example.com/domains.txt'

config rule
	option name 'domains'
	option action 'direct'
	list domain_provider 'domains'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 1 || !cfg.Providers[0].Enabled {
		t.Fatalf("provider enabled option should be ignored: %+v", cfg.Providers)
	}
}

func TestParseProviderIntervalSchedule(t *testing.T) {
	cfg, err := Parse(`
config provider 'telegram'
	option type 'ip'
	option url 'https://example.com/telegram.txt'
	option auto_update '1'
	option update_schedule 'interval'
	option update_interval_minutes '720'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("unexpected providers: %+v", cfg.Providers)
	}
	p := cfg.Providers[0]
	if p.UpdateSchedule != UpdateScheduleInterval || p.UpdateIntervalMinutes != 720 {
		t.Fatalf("unexpected provider interval schedule: %+v", p)
	}
}

func TestParseScriptProvider(t *testing.T) {
	cfg, err := Parse(`
config provider 'json_ips'
	option label 'JSON IPs'
	option type 'ip'
	option source 'script'
	option script_path '/usr/share/neto/providers/json-ips.sh'
	option url 'https://example.com/ips.json'
	option auto_update '1'
	option update_hour '4'
	option update_minute '42'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("unexpected providers: %+v", cfg.Providers)
	}
	p := cfg.Providers[0]
	if p.Name != "json_ips" || p.Type != "ip" || p.Source != "script" || p.ScriptPath != "/usr/share/neto/providers/json-ips.sh" || p.UpdateHour != 4 || p.UpdateMinute != 42 {
		t.Fatalf("unexpected provider: %+v", p)
	}
}

func TestParseRejectsInvalidProvider(t *testing.T) {
	tests := []struct {
		name string
		uci  string
		want string
	}{
		{
			name: "missing_url",
			uci: `
config provider 'p'
`,
			want: "url is required",
		},
		{
			name: "type",
			uci: `
config provider 'p'
option type 'cidr'
option url 'https://example.com/list.txt'
`,
			want: "unsupported type",
		},
		{
			name: "source",
			uci: `
config provider 'p'
	option type 'ip'
	option source 'database'
	option url 'https://example.com/list.txt'
`,
			want: "unsupported source",
		},
		{
			name: "missing_script_path",
			uci: `
config provider 'p'
	option type 'ip'
	option source 'script'
`,
			want: "script_path is required",
		},
		{
			name: "relative_script_path",
			uci: `
config provider 'p'
	option type 'ip'
	option source 'script'
	option script_path 'filter.sh'
`,
			want: "script_path must be absolute",
		},
		{
			name: "invalid_update_minute",
			uci: `
config provider 'p'
	option type 'ip'
	option url 'https://example.com/list.txt'
	option auto_update '1'
	option update_minute '60'
`,
			want: "invalid update_minute",
		},
		{
			name: "invalid_update_schedule",
			uci: `
config provider 'p'
	option type 'ip'
	option url 'https://example.com/list.txt'
	option auto_update '1'
	option update_schedule 'random'
`,
			want: "unsupported update_schedule",
		},
		{
			name: "invalid_update_interval_minutes",
			uci: `
config provider 'p'
	option type 'ip'
	option url 'https://example.com/list.txt'
	option auto_update '1'
	option update_schedule 'interval'
	option update_interval_minutes '90'
`,
			want: "invalid update_interval_minutes",
		},
		{
			name: "unknown_ref",
			uci: `
config rule
	option name 'unknown'
	list domain_provider 'missing'
`,
			want: "unknown provider",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.uci)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseRejectsInvalidLANSubnet(t *testing.T) {
	if _, err := Parse(`
config main 'main'
	list lan_subnet 'not-a-cidr'
`); err == nil {
		t.Fatal("expected invalid lan_subnet to be rejected")
	}
}

func TestParseRejectsDNSLoops(t *testing.T) {
	if _, err := Parse(`
config main 'main'
	option dns_listen '127.0.0.1:5353'
	option real_dns_upstream '127.0.0.1:5353'
`); err == nil {
		t.Fatal("expected real_dns_upstream loop to be rejected")
	}

	if _, err := Parse(`
config main 'main'
	option dns_listen '127.0.0.1:5353'
	option singbox_dns '127.0.0.1:5353'
`); err == nil {
		t.Fatal("expected singbox_dns loop to be rejected")
	}
}

func TestParseDNSUpstreamPresetAndProtocol(t *testing.T) {
	cfg, err := Parse(`
config main 'main'
	option dns_upstream_preset 'google'
	option dns_upstream_protocol 'doh'
`)
	if err != nil {
		t.Fatal(err)
	}
	upstream := cfg.Main.DNSUpstream()
	if upstream.Protocol != "https" || upstream.Host != "dns.google" || upstream.Port != 443 || upstream.TLSName != "dns.google" || upstream.Path != "/dns-query" {
		t.Fatalf("unexpected DNS upstream: %+v", upstream)
	}
}

func TestParseGoogleUDPPresetUsesIPAddress(t *testing.T) {
	cfg, err := Parse(`
config main 'main'
	option dns_upstream_preset 'google'
	option dns_upstream_protocol 'udp'
`)
	if err != nil {
		t.Fatal(err)
	}
	upstream := cfg.Main.DNSUpstream()
	if upstream.Protocol != "udp" || upstream.Host != "8.8.8.8" || upstream.Port != 53 || upstream.TLSName != "dns.google" {
		t.Fatalf("unexpected DNS upstream: %+v", upstream)
	}
}

func TestParsePresetOverridesStaleRealDNSFields(t *testing.T) {
	cfg, err := Parse(`
config main 'main'
	option dns_upstream_preset 'google'
	option dns_upstream_protocol 'https'
	option real_dns_transport 'https'
	option real_dns_server '1.1.1.1:443'
	option real_dns_server_name 'cloudflare-dns.com'
	option real_dns_path '/dns-query'
`)
	if err != nil {
		t.Fatal(err)
	}
	upstream := cfg.Main.DNSUpstream()
	if upstream.Protocol != "https" || upstream.Host != "dns.google" || upstream.Port != 443 || upstream.TLSName != "dns.google" {
		t.Fatalf("preset should override stale real DNS fields: %+v", upstream)
	}
}

func TestParseRealDNSTransportDefaultsPort(t *testing.T) {
	cfg, err := Parse(`
config main 'main'
	option real_dns_transport 'https'
	option real_dns_server '1.1.1.1'
	option real_dns_server_name 'cloudflare-dns.com'
	option real_dns_path '/dns-query'
`)
	if err != nil {
		t.Fatal(err)
	}
	upstream := cfg.Main.DNSUpstream()
	if upstream.Protocol != "https" || upstream.Port != 443 {
		t.Fatalf("unexpected real DNS upstream: %+v", upstream)
	}
}

func TestParseRealDNSServerHostPortOverridesDefaultPort(t *testing.T) {
	cfg, err := Parse(`
config main 'main'
	option real_dns_transport 'https'
	option real_dns_server '1.1.1.1:443'
	option real_dns_server_name 'cloudflare-dns.com'
	option real_dns_path '/dns-query'
`)
	if err != nil {
		t.Fatal(err)
	}
	upstream := cfg.Main.DNSUpstream()
	if upstream.Host != "1.1.1.1" || upstream.Port != 443 {
		t.Fatalf("unexpected real DNS upstream: %+v", upstream)
	}
}

func TestParseLegacyRealDNSUpstreamAsCustomUDP(t *testing.T) {
	cfg, err := Parse(`
config main 'main'
	option real_dns_upstream '9.9.9.9:53'
`)
	if err != nil {
		t.Fatal(err)
	}
	upstream := cfg.Main.DNSUpstream()
	if upstream.Preset != "custom" || upstream.Protocol != "udp" || upstream.Host != "9.9.9.9" || upstream.Port != 53 {
		t.Fatalf("unexpected legacy DNS upstream migration: %+v", upstream)
	}
}

func TestParseRejectsInvalidDNSUpstream(t *testing.T) {
	if _, err := Parse(`
config main 'main'
	option dns_upstream_protocol 'https'
	option dns_upstream_preset 'custom'
	option dns_upstream_host '1.1.1.1'
	option dns_upstream_port '443'
	option dns_upstream_path 'dns-query'
`); err == nil {
		t.Fatal("expected invalid DoH path to be rejected")
	}
}

func TestParseRealDNSProxyRequiresSelectedOutbound(t *testing.T) {
	_, err := Parse(`
config main 'main'
	option real_dns_mode 'proxy'
`)
	if err == nil || !strings.Contains(err.Error(), "real_dns_outbound") {
		t.Fatalf("got error %v, want real_dns_outbound requirement", err)
	}
}

func TestParseRealDNSProxyOutbound(t *testing.T) {
	cfg, err := Parse(`
config main 'main'
	option real_dns_mode 'proxy'
	option real_dns_outbound 'dns_proxy'

config outbound 'dns_proxy'
	option type 'vless'
	option server 'example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Main.RealDNSOutbound != "dns_proxy" {
		t.Fatalf("unexpected real_dns_outbound: %+v", cfg.Main)
	}
}

func TestParseRejectsDeprecatedRealDNSProxyDefault(t *testing.T) {
	_, err := Parse(`
config main 'main'
	option real_dns_mode 'proxy'
	option real_dns_outbound 'proxy_default'

config outbound 'dns_proxy'
	option type 'vless'
	option server 'example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
`)
	if err == nil || !strings.Contains(err.Error(), "proxy_default") {
		t.Fatalf("got error %v, want proxy_default rejection", err)
	}
}

func TestParseAllowsMissingCustomOutboundWhenEnabled(t *testing.T) {
	cfg, err := Parse(`
config main 'main'
	option enabled '1'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != 0 {
		t.Fatalf("unexpected custom outbounds: %+v", cfg.Outbounds)
	}
}

func TestParseDisabledAllowsMissingOutbound(t *testing.T) {
	cfg, err := Parse(`
config main 'main'
	option enabled '0'
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Main.Enabled {
		t.Fatalf("expected disabled config: %+v", cfg.Main)
	}
	if len(cfg.Outbounds) != 0 {
		t.Fatalf("disabled config should not synthesize outbounds: %+v", cfg.Outbounds)
	}
}

func TestParseOutboundVLESSReality(t *testing.T) {
	cfg, err := Parse(`
config outbound 'my_vless'
	option enabled '1'
	option tag 'my_vless'
	option label 'My VLESS'
	option type 'vless'
	option server 'example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
	option flow 'xtls-rprx-vision'
	option tls '1'
	option server_name 'www.example.com'
	option reality '1'
	option reality_public_key 'public-key'
	option reality_short_id '0123456789abcdef'
	option alpn 'h2,http/1.1'
	option transport 'tcp'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != 1 {
		t.Fatalf("unexpected outbounds: %+v", cfg.Outbounds)
	}
	out := cfg.Outbounds[0]
	if out.Tag != "my_vless" || out.Label != "My VLESS" || out.Type != "vless" || !out.TLS || !out.Reality || out.RealityPublicKey != "public-key" {
		t.Fatalf("unexpected outbound: %+v", out)
	}
	if len(out.ALPN) != 2 || out.ALPN[0] != "h2" || out.ALPN[1] != "http/1.1" {
		t.Fatalf("unexpected alpn: %+v", out.ALPN)
	}
}

func TestParseOutboundWarnings(t *testing.T) {
	cfg, err := Parse(`
config outbound 'my_trojan'
	option type 'trojan'
	option server 'example.com'
	option port '443'
	option password 'secret'
	option tls '1'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 1 || !strings.Contains(cfg.Warnings[0], "server_name is recommended") {
		t.Fatalf("unexpected warnings: %+v", cfg.Warnings)
	}
}

func TestParseOutboundHomeProxyAliasesAndAdvancedTLS(t *testing.T) {
	cfg, err := Parse(`
config outbound 'my_vless'
	option label 'Alias VLESS'
	option type 'vless'
	option address 'example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
	option vless_flow 'xtls-rprx-vision'
	option tls '1'
	option tls_sni 'www.example.com'
	list tls_alpn 'h2'
	list tls_alpn 'http/1.1'
	option tls_min_version '1.2'
	option tls_max_version '1.3'
	list tls_cipher_suites 'TLS_AES_128_GCM_SHA256'
	option tls_ech '1'
	list tls_ech_config 'ech-config'
	option tls_ech_config_path '/etc/neto/ech.pem'
	option tls_utls 'chrome'
	option tls_reality '1'
	option tls_reality_public_key 'public-key'
	option tls_reality_short_id 'short-id'
	option transport 'ws'
	option ws_host 'front.example.com'
	option ws_path '/ws'
	option websocket_early_data '2048'
	option websocket_early_data_header 'Sec-WebSocket-Protocol'
	option packet_encoding 'xudp'
`)
	if err != nil {
		t.Fatal(err)
	}
	out := cfg.Outbounds[0]
	if out.Server != "example.com" || out.Flow != "xtls-rprx-vision" || out.ServerName != "www.example.com" || !out.Reality || out.RealityPublicKey != "public-key" {
		t.Fatalf("aliases not parsed: %+v", out)
	}
	if len(out.ALPN) != 2 || out.TLSMinVersion != "1.2" || out.TLSMaxVersion != "1.3" || len(out.TLSCipherSuites) != 1 {
		t.Fatalf("advanced TLS fields not parsed: %+v", out)
	}
	if !out.ECH || len(out.ECHConfig) != 1 || out.ECHConfigPath != "/etc/neto/ech.pem" || out.UTLSFingerprint != "chrome" {
		t.Fatalf("ECH/uTLS fields not parsed: %+v", out)
	}
	if out.Transport != "ws" || out.WSHost != "front.example.com" || out.WSPath != "/ws" || out.WSEarlyData != 2048 || out.PacketEncoding != "xudp" {
		t.Fatalf("transport fields not parsed: %+v", out)
	}
}

func TestParseDeprecatedProxyDefaultOutboundIsIgnored(t *testing.T) {
	cfg, err := Parse(`
config outbound 'proxy_default'
	option type 'direct'

config outbound 'replacement'
	option type 'vless'
	option server 'example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'

config rule
	option name 'old_rule'
	option action 'proxy'
	option outbound 'proxy_default'
	list domain_equals 'example.com'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != 2 || cfg.Outbounds[0].Enabled || !cfg.Outbounds[1].Enabled {
		t.Fatalf("deprecated proxy_default outbound should be ignored: %+v", cfg.Outbounds)
	}
	if cfg.Rules[0].Outbound != "replacement" {
		t.Fatalf("deprecated proxy_default rule outbound should use first custom outbound: %+v", cfg.Rules[0])
	}
	if len(cfg.Warnings) < 2 {
		t.Fatalf("expected deprecation warnings, got %+v", cfg.Warnings)
	}
}

func TestParseRejectsCustomDirectOutbound(t *testing.T) {
	_, err := Parse(`
config outbound 'debug_direct'
	option type 'direct'
`)
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("got error %v, want unsupported direct outbound", err)
	}
}

func TestParseRejectsReservedOutboundTag(t *testing.T) {
	_, err := Parse(`
config outbound 'direct'
	option type 'vless'
	option server 'example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
`)
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("got error %v, want reserved tag", err)
	}
}

func TestParseRejectsDuplicateOutboundTag(t *testing.T) {
	_, err := Parse(`
config outbound 'first'
	option tag 'same_tag'
	option type 'vless'
	option server 'one.example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'

config outbound 'second'
	option tag 'same_tag'
	option type 'trojan'
	option server 'two.example.com'
	option port '443'
	option password 'secret'
`)
	if err == nil || !strings.Contains(err.Error(), "duplicate enabled outbound tag") {
		t.Fatalf("got error %v, want duplicate outbound tag", err)
	}
}

func TestParseOutboundPoolAndAllowItAsRoutingTarget(t *testing.T) {
	cfg, err := Parse(`
config outbound 'primary'
	option type 'vless'
	option server 'primary.example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'

config outbound 'backup'
	option type 'trojan'
	option server 'backup.example.com'
	option port '443'
	option password 'secret'

config outbound_pool 'priority_pool'
	option label 'Priority pool'
	list outbound 'primary'
	list outbound 'backup'
	option check_url 'https://example.com/generate_204'
	option check_interval '30'

config rule 'pooled_rule'
	option action 'proxy'
	option outbound 'priority_pool'
	list domain_equals 'example.com'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.OutboundPools) != 1 {
		t.Fatalf("unexpected pools: %+v", cfg.OutboundPools)
	}
	pool := cfg.OutboundPools[0]
	if pool.Tag != "priority_pool" || pool.Label != "Priority pool" || pool.CheckInterval != 30 {
		t.Fatalf("unexpected pool: %+v", pool)
	}
	if !reflect.DeepEqual(pool.Outbounds, []string{"primary", "backup"}) {
		t.Fatalf("unexpected pool priority: %+v", pool.Outbounds)
	}
	if cfg.Rules[0].Outbound != "priority_pool" {
		t.Fatalf("unexpected rule outbound: %+v", cfg.Rules[0])
	}
	if got := cfg.OutboundCandidateTags("priority_pool"); !reflect.DeepEqual(got, []string{"primary", "backup"}) {
		t.Fatalf("unexpected candidates: %+v", got)
	}
	if got := cfg.ProxyTargetTags(); !reflect.DeepEqual(got, []string{"primary", "backup", "priority_pool"}) {
		t.Fatalf("unexpected proxy targets: %+v", got)
	}
}

func TestParseRejectsInvalidOutboundPools(t *testing.T) {
	const outbounds = `
config outbound 'primary'
	option type 'vless'
	option server 'primary.example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'

config outbound 'backup'
	option type 'trojan'
	option server 'backup.example.com'
	option port '443'
	option password 'secret'
`
	tests := []struct {
		name string
		pool string
		want string
	}{
		{"one member", "config outbound_pool 'pool'\n\tlist outbound 'primary'\n", "at least two"},
		{"unknown member", "config outbound_pool 'pool'\n\tlist outbound 'primary'\n\tlist outbound 'missing'\n", "unknown outbound"},
		{"duplicate member", "config outbound_pool 'pool'\n\tlist outbound 'primary'\n\tlist outbound 'primary'\n", "duplicate outbound"},
		{"duplicate tag", "config outbound_pool 'primary'\n\tlist outbound 'primary'\n\tlist outbound 'backup'\n", "duplicate outbound or pool tag"},
		{"invalid URL", "config outbound_pool 'pool'\n\tlist outbound 'primary'\n\tlist outbound 'backup'\n\toption check_url 'not-a-url'\n", "invalid check_url"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(outbounds + test.pool)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}

func TestParseRuleCustomOutbound(t *testing.T) {
	cfg, err := Parse(`
config outbound 'my_vless'
	option type 'vless'
	option server 'example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'

config rule
	option name 'custom_proxy'
	option action 'proxy'
	option outbound 'my_vless'
	list domain_equals 'example.com'
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rules[0].Outbound != "my_vless" {
		t.Fatalf("unexpected rule outbound: %+v", cfg.Rules[0])
	}
}

func TestParseIgnoresCustomOutboundEnabledOption(t *testing.T) {
	cfg, err := Parse(`
config outbound 'my_vless'
	option enabled '0'
	option type 'vless'
	option server 'example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'

config rule
	option name 'custom_proxy'
	option action 'proxy'
	option outbound 'my_vless'
	list domain_equals 'example.com'
`)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Outbounds[0].Enabled || cfg.Rules[0].Outbound != "my_vless" {
		t.Fatalf("custom outbound enabled option should be ignored: %+v %+v", cfg.Outbounds[0], cfg.Rules[0])
	}
}

func TestParseSubscription(t *testing.T) {
	cfg, err := Parse(`
config outbound 'updater'
	option type 'trojan'
	option server 'example.com'
	option port '443'
	option password 'secret'

config subscription 'my_sub'
	option label 'My subscription'
	option url 'https://example.com/sub'
	option auto_update '1'
	option update_hour '12'
	option update_via 'proxy'
	option update_outbound 'updater'
	option last_update '1710000000'
	option node_count '2'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Subscriptions) != 1 {
		t.Fatalf("unexpected subscriptions: %+v", cfg.Subscriptions)
	}
	sub := cfg.Subscriptions[0]
	if sub.Name != "my_sub" || sub.Label != "My subscription" || !sub.AutoUpdate || sub.UpdateSchedule != UpdateScheduleTime || sub.UpdateHour != 12 || sub.UpdateMinute != 0 || sub.UpdateIntervalMinutes != DefaultUpdateIntervalMinutes || sub.UpdateVia != "proxy" || sub.UpdateOutbound != "updater" || sub.NodeCount != 2 {
		t.Fatalf("unexpected subscription: %+v", sub)
	}
}

func TestParseSubscriptionIntervalSchedule(t *testing.T) {
	cfg, err := Parse(`
config subscription 'my_sub'
	option label 'My subscription'
	option url 'https://example.com/sub'
	option auto_update '1'
	option update_schedule 'interval'
	option update_interval_minutes '6h'
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Subscriptions) != 1 {
		t.Fatalf("unexpected subscriptions: %+v", cfg.Subscriptions)
	}
	sub := cfg.Subscriptions[0]
	if sub.UpdateSchedule != UpdateScheduleInterval || sub.UpdateIntervalMinutes != 360 {
		t.Fatalf("unexpected subscription interval schedule: %+v", sub)
	}
}

func TestParseRejectsInvalidSubscription(t *testing.T) {
	tests := []struct {
		name string
		uci  string
		want string
	}{
		{
			name: "missing_url",
			uci: `
config subscription 'sub'
`,
			want: "url is required",
		},
		{
			name: "update_via",
			uci: `
config subscription 'sub'
	option url 'https://example.com/sub'
	option update_via 'vpn'
`,
			want: "unsupported update_via",
		},
		{
			name: "missing_update_outbound",
			uci: `
config subscription 'sub'
	option url 'https://example.com/sub'
	option update_via 'proxy'
	option update_outbound 'missing'
`,
			want: "unsupported update_outbound",
		},
		{
			name: "invalid_update_hour",
			uci: `
config subscription 'sub'
	option url 'https://example.com/sub'
	option auto_update '1'
	option update_hour '24'
`,
			want: "invalid update_hour",
		},
		{
			name: "invalid_update_interval_minutes",
			uci: `
config subscription 'sub'
	option url 'https://example.com/sub'
	option auto_update '1'
	option update_schedule 'interval'
	option update_interval_minutes '90'
`,
			want: "invalid update_interval_minutes",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.uci)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseRejectsUnknownRuleOutbound(t *testing.T) {
	_, err := Parse(`
config rule
	option name 'unknown_outbound'
	option action 'proxy'
	option outbound 'missing_proxy'
	list domain_equals 'example.com'
`)
	if err == nil || !strings.Contains(err.Error(), "unsupported outbound") {
		t.Fatalf("got error %v, want unsupported outbound", err)
	}
}

func TestParseOutboundMissingFields(t *testing.T) {
	tests := []struct {
		name string
		uci  string
		want string
	}{
		{
			name: "vless_uuid",
			uci: `
config outbound 'my_vless'
	option type 'vless'
	option server 'example.com'
	option port '443'
`,
			want: "uuid is required",
		},
		{
			name: "vless_reality_public_key",
			uci: `
config outbound 'my_vless'
	option type 'vless'
	option server 'example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
	option reality '1'
`,
			want: "reality_public_key is required",
		},
		{
			name: "hysteria2_password",
			uci: `
config outbound 'my_hy2'
	option type 'hysteria2'
	option server 'example.com'
	option port '443'
`,
			want: "password is required",
		},
		{
			name: "shadowsocks_method",
			uci: `
config outbound 'my_ss'
	option type 'shadowsocks'
	option server 'example.com'
	option port '8388'
	option password 'secret'
`,
			want: "method is required",
		},
		{
			name: "trojan_server",
			uci: `
config outbound 'my_trojan'
	option type 'trojan'
	option port '443'
	option password 'secret'
`,
			want: "server is required",
		},
		{
			name: "unsupported",
			uci: `
config outbound 'my_socks'
	option type 'socks'
`,
			want: "unsupported type",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.uci)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got error %v, want %q", err, tc.want)
			}
		})
	}
}

func withTestOutbound(uci string) string {
	return uci + `

config outbound 'test_proxy'
	option type 'vless'
	option server 'example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
`
}
