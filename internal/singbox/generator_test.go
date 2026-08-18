package singbox

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elllkere/zakop/internal/config"
)

func TestGenerateUsesModernFakeIPServer(t *testing.T) {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{{Enabled: true, Tag: "test", Type: "vless", Server: "example.com", Port: 443, UUID: "a3482e88-686a-4a58-8126-99c9df64b060"}}
	out, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	raw := string(out)
	if !strings.Contains(raw, `"type": "fakeip"`) || !strings.Contains(raw, `"tag": "fakeip"`) {
		t.Fatalf("fakeip server missing:\n%s", raw)
	}
	if strings.Contains(raw, `"fake-ip"`) || strings.Contains(raw, `"fake_ip"`) {
		t.Fatalf("legacy fake-ip config detected:\n%s", raw)
	}
	for _, want := range []string{`"listen_port": 15353`, `"listen_port": 15354`, `"listen_port": 15355`, `"listen_port": 16001`, `"tag": "tproxy-0000-in"`, `"udp_timeout": "10s"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected DNS and TProxy listeners %q:\n%s", want, raw)
		}
	}
	if count := strings.Count(raw, `"udp_timeout": "10s"`); count != 3 {
		t.Fatalf("expected a short UDP timeout on all three DNS inbounds, got %d:\n%s", count, raw)
	}
	if !strings.Contains(raw, `"default_domain_resolver": "real-direct"`) {
		t.Fatalf("expected route.default_domain_resolver:\n%s", raw)
	}
	for _, want := range []string{`"tag": "real-direct"`, `"tag": "real-proxy"`, `"tag": "fakeip"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected DNS server %q:\n%s", want, raw)
		}
	}
	if strings.Contains(raw, `"override_destination"`) || strings.Contains(raw, `"sniff": true`) {
		t.Fatalf("generated config contains sing-box 1.13-incompatible sniff fields:\n%s", raw)
	}
	if strings.Contains(raw, `"domain_strategy"`) {
		t.Fatalf("generated config contains deprecated legacy domain strategy field:\n%s", raw)
	}
	if strings.Contains(raw, `"detour": "direct"`) {
		t.Fatalf("DNS servers must not detour to empty direct outbound:\n%s", raw)
	}
	if strings.Contains(raw, "rule_set") || strings.Contains(raw, "rule-set") || strings.Contains(raw, "/tmp/sing-box/rulesets") {
		t.Fatalf("generated config must not reference external rule-set files:\n%s", raw)
	}
	if !strings.Contains(raw, `"level": "warn"`) {
		t.Fatalf("sing-box log level should default to warn to avoid INFO connection log spam:\n%s", raw)
	}
	if !strings.Contains(raw, `"store_fakeip": true`) {
		t.Fatalf("expected fakeip cache persistence:\n%s", raw)
	}
}

func TestGenerateBuiltinOutbounds(t *testing.T) {
	cfg := config.Defaults()
	direct := generatedOutbound(t, cfg, "direct")
	blocked := generatedOutbound(t, cfg, "blocked")
	if direct["type"] != "direct" {
		t.Fatalf("unexpected direct outbound: %+v", direct)
	}
	if blocked["type"] != "block" {
		t.Fatalf("unexpected blocked outbound: %+v", blocked)
	}
}

func TestGenerateOrderedOutboundPoolSelector(t *testing.T) {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{
		{Enabled: true, Tag: "primary", Type: "trojan", Server: "primary.example", Port: 443, Password: "one", TLS: true},
		{Enabled: true, Tag: "backup", Type: "shadowsocks", Server: "backup.example", Port: 8388, Method: "aes-128-gcm", Password: "two"},
	}
	cfg.OutboundPools = []config.OutboundPool{{
		Tag: "priority_pool", Outbounds: []string{"primary", "backup"}, CheckURL: "https://example.com/generate_204", CheckInterval: 60,
	}}

	raw, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	selector := generatedOutbound(t, cfg, "priority_pool")
	if selector["type"] != "selector" || selector["default"] != "primary" || selector["interrupt_exist_connections"] != true {
		t.Fatalf("unexpected selector: %+v", selector)
	}
	wantMembers := []any{"primary", "backup"}
	if got := selector["outbounds"].([]any); len(got) != 2 || got[0] != wantMembers[0] || got[1] != wantMembers[1] {
		t.Fatalf("unexpected selector priority: %+v", got)
	}
	text := string(raw)
	for _, want := range []string{`"external_controller": "127.0.0.1:19090"`, `"tag": "tproxy-0002-in"`, `"outbound": "priority_pool"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("pool config missing %s:\n%s", want, text)
		}
	}
}

func TestGenerateDoTDNSServer(t *testing.T) {
	cfg := config.Defaults()
	cfg.Main.DNSUpstreamPreset = "custom"
	cfg.Main.RealDNSTransport = "tls"
	cfg.Main.RealDNSServer = "8.8.8.8"
	cfg.Main.RealDNSServerPort = 853
	cfg.Main.RealDNSServerName = "dns.google"
	local := generatedDNSServer(t, cfg, "real-direct")
	if local["type"] != "tls" || local["server"] != "8.8.8.8" || local["server_port"].(float64) != 853 {
		t.Fatalf("unexpected DoT DNS server: %+v", local)
	}
	if _, ok := local["detour"]; ok {
		t.Fatalf("real-direct DNS should not detour to direct outbound: %+v", local)
	}
	tls := local["tls"].(map[string]any)
	if tls["server_name"] != "dns.google" {
		t.Fatalf("unexpected DoT TLS config: %+v", tls)
	}
}

func TestGenerateDoHDNSServer(t *testing.T) {
	cfg := config.Defaults()
	cfg.Main.DNSUpstreamPreset = "custom"
	cfg.Main.RealDNSTransport = "https"
	cfg.Main.RealDNSServer = "1.1.1.1"
	cfg.Main.RealDNSServerPort = 443
	cfg.Main.RealDNSServerName = "cloudflare-dns.com"
	cfg.Main.RealDNSPath = "/dns-query"
	local := generatedDNSServer(t, cfg, "real-direct")
	if local["type"] != "https" || local["server"] != "1.1.1.1" || local["server_port"].(float64) != 443 || local["path"] != "/dns-query" {
		t.Fatalf("unexpected DoH DNS server: %+v", local)
	}
	tls := local["tls"].(map[string]any)
	if tls["server_name"] != "cloudflare-dns.com" {
		t.Fatalf("unexpected DoH TLS config: %+v", tls)
	}
	if _, ok := local["detour"]; ok {
		t.Fatalf("real-direct DNS should not detour to direct outbound: %+v", local)
	}
}

func TestGenerateBootstrapDNSServerDoesNotDetourToDirect(t *testing.T) {
	cfg := config.Defaults()
	cfg.Main.DNSUpstreamPreset = "custom"
	cfg.Main.RealDNSTransport = "https"
	cfg.Main.RealDNSServer = "dns.example.com"
	cfg.Main.RealDNSServerPort = 443
	cfg.Main.RealDNSServerName = "dns.example.com"
	cfg.Main.RealDNSPath = "/dns-query"
	bootstrap := generatedDNSServer(t, cfg, "bootstrap")
	if bootstrap["type"] != "udp" || bootstrap["server"] != "1.1.1.1" {
		t.Fatalf("unexpected bootstrap DNS server: %+v", bootstrap)
	}
	if _, ok := bootstrap["detour"]; ok {
		t.Fatalf("bootstrap DNS should not detour to direct outbound: %+v", bootstrap)
	}
}

func TestGenerateGoogleDoHUsesDomainResolver(t *testing.T) {
	cfg := config.Defaults()
	cfg.Main.DNSUpstreamPreset = "google"
	cfg.Main.RealDNSTransport = "https"
	local := generatedDNSServer(t, cfg, "real-direct")
	if local["type"] != "https" || local["server"] != "dns.google" || local["server_port"].(float64) != 443 {
		t.Fatalf("unexpected Google DoH DNS server: %+v", local)
	}
	if local["domain_resolver"] != "bootstrap" {
		t.Fatalf("Google DoH hostname should use bootstrap resolver tag: %+v", local)
	}
	if _, ok := local["domain_strategy"]; ok {
		t.Fatalf("Google DoH DNS server must not use deprecated domain_strategy: %+v", local)
	}
	if _, ok := local["detour"]; ok {
		t.Fatalf("Google DoH direct DNS should not detour to direct outbound: %+v", local)
	}
}

func TestGenerateRealProxyDNSServerDetoursThroughSelectedDNSOutbound(t *testing.T) {
	cfg := config.Defaults()
	cfg.Main.RealDNSOutbound = "dns_proxy"
	cfg.Outbounds = []config.Outbound{
		{
			Enabled: true,
			Tag:     "dns_proxy",
			Type:    "vless",
			Server:  "dns.example.com",
			Port:    443,
			UUID:    "a3482e88-686a-4a58-8126-99c9df64b060",
			TLS:     true,
		},
		{
			Enabled: true,
			Tag:     "rule_proxy",
			Type:    "vless",
			Server:  "rule.example.com",
			Port:    443,
			UUID:    "a3482e88-686a-4a58-8126-99c9df64b061",
			TLS:     true,
		},
	}
	cfg.Rules = []config.Rule{{
		Name:           "test",
		Enabled:        true,
		Priority:       100,
		Action:         "proxy",
		Outbound:       "rule_proxy",
		DNSMode:        "auto",
		DomainContains: []string{"check-host"},
	}}
	realProxy := generatedDNSServer(t, cfg, "real-proxy")
	if realProxy["detour"] != "dns_proxy" {
		t.Fatalf("real-proxy DNS should detour through selected DNS outbound: %+v", realProxy)
	}
}

func TestGenerateRoutesCapturedTrafficToReferencedProxyOutbound(t *testing.T) {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{{
		Enabled: true,
		Tag:     "test",
		Type:    "vless",
		Server:  "example.com",
		Port:    443,
		UUID:    "a3482e88-686a-4a58-8126-99c9df64b060",
		TLS:     true,
	}}
	cfg.Rules = []config.Rule{{
		Name:           "test",
		Enabled:        true,
		Priority:       100,
		Action:         "proxy",
		Outbound:       "test",
		DNSMode:        "fakeip",
		DomainContains: []string{"check-host"},
	}}
	route := generatedRoute(t, cfg)
	if route["final"] != "direct" {
		t.Fatalf("unmatched traffic should stay direct, got route %+v", route)
	}
	rules := route["rules"].([]any)
	sniff := rules[0].(map[string]any)
	if sniff["action"] != "sniff" {
		t.Fatalf("sniff action missing before proxy outbound, got %+v", sniff)
	}
	if _, ok := sniff["override_destination"]; ok {
		t.Fatalf("override_destination is not accepted by sing-box 1.13 route action, got %+v", sniff)
	}
	domainRule := rules[2].(map[string]any)
	if domainRule["outbound"] != "test" || domainRule["action"] != "route" {
		t.Fatalf("domain rule should select its own outbound, got %+v", domainRule)
	}
}

func TestGenerateRoutesSimpleModeToReferencedProxyOutbound(t *testing.T) {
	cfg := config.Defaults()
	cfg.Main.RoutingMode = "simple"
	cfg.Outbounds = []config.Outbound{{
		Enabled: true,
		Tag:     "test",
		Type:    "vless",
		Server:  "example.com",
		Port:    443,
		UUID:    "a3482e88-686a-4a58-8126-99c9df64b060",
		TLS:     true,
	}}
	cfg.Main.SimpleRule.Outbound = "test"
	cfg.Main.SimpleRule.DomainContains = []string{"check-host"}
	route := generatedRoute(t, cfg)
	rules := route["rules"].([]any)
	domainRule := rules[2].(map[string]any)
	if domainRule["outbound"] != "test" {
		t.Fatalf("simple mode domain rule should use referenced proxy outbound, got route %+v", route)
	}
}

func TestGenerateRoutesSimpleModeUsesSelectedFirstCustomOutbound(t *testing.T) {
	cfg := config.Defaults()
	cfg.Main.RoutingMode = "simple"
	cfg.Outbounds = []config.Outbound{{
		Enabled: true,
		Tag:     "first",
		Type:    "vless",
		Server:  "example.com",
		Port:    443,
		UUID:    "a3482e88-686a-4a58-8126-99c9df64b060",
		TLS:     true,
	}}
	cfg.Main.SimpleRule.Outbound = "first"
	cfg.Main.SimpleRule.DomainContains = []string{"check-host"}
	route := generatedRoute(t, cfg)
	rules := route["rules"].([]any)
	if rules[2].(map[string]any)["outbound"] != "first" {
		t.Fatalf("simple mode should use its selected first outbound, got route %+v", route)
	}
}

func TestGenerateRoutesDifferentDomainRulesToDifferentOutbounds(t *testing.T) {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{
		{Enabled: true, Tag: "first", Type: "vless", Server: "first.example.com", Port: 443, UUID: "a3482e88-686a-4a58-8126-99c9df64b060"},
		{Enabled: true, Tag: "second", Type: "trojan", Server: "second.example.com", Port: 443, Password: "secret"},
	}
	cfg.Rules = []config.Rule{
		{Name: "youtube", Enabled: true, Priority: 100, Action: "proxy", Outbound: "first", DNSMode: "auto", DomainContains: []string{"youtube"}},
		{Name: "telegram", Enabled: true, Priority: 200, Action: "proxy", Outbound: "second", DNSMode: "auto", DomainContains: []string{"telegram"}},
	}
	route := generatedRoute(t, cfg)
	rules := route["rules"].([]any)
	if rules[2].(map[string]any)["outbound"] != "first" || rules[3].(map[string]any)["outbound"] != "second" {
		t.Fatalf("domain rules should keep independent outbounds, got route %+v", route)
	}
}

func TestGenerateDomainOutboundRulePreservesLiteralMatchersAndExcludes(t *testing.T) {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{{
		Enabled: true, Tag: "proxy", Type: "vless", Server: "example.com", Port: 443,
		UUID: "a3482e88-686a-4a58-8126-99c9df64b060",
	}}
	cfg.Rules = []config.Rule{{
		Name:                    "domains",
		Enabled:                 true,
		Action:                  "proxy",
		Outbound:                "proxy",
		DNSMode:                 "auto",
		DomainEquals:            []string{"example.com"},
		DomainContains:          []string{"video+cdn"},
		DomainStartsWith:        []string{"www."},
		DomainEndsWith:          []string{".example.org"},
		ExcludeDomainEndsWith:   []string{".ads.example.org"},
		ExcludeDomainContains:   []string{"tracking"},
		ExcludeDomainEquals:     []string{"blocked.example.com"},
		ExcludeDomainStartsWith: []string{"old."},
	}}

	route := generatedRoute(t, cfg)
	rule := route["rules"].([]any)[2].(map[string]any)
	if rule["type"] != "logical" || rule["mode"] != "and" || rule["outbound"] != "proxy" {
		t.Fatalf("unexpected logical domain route rule: %+v", rule)
	}
	nested := rule["rules"].([]any)
	include := nested[0].(map[string]any)["domain_regex"].([]any)
	excludeRule := nested[1].(map[string]any)
	exclude := excludeRule["domain_regex"].([]any)
	if excludeRule["invert"] != true {
		t.Fatalf("domain exclusions must invert the exclusion matcher: %+v", excludeRule)
	}
	for _, want := range []string{`^example\.com$`, `video\+cdn`, `^www\.`, `\.example\.org$`} {
		if !containsAnyString(include, want) {
			t.Fatalf("missing include regexp %q in %+v", want, include)
		}
	}
	for _, want := range []string{`^blocked\.example\.com$`, `tracking`, `^old\.`, `\.ads\.example\.org$`} {
		if !containsAnyString(exclude, want) {
			t.Fatalf("missing exclude regexp %q in %+v", want, exclude)
		}
	}
}

func TestGenerateGlobalModeUsesFirstCustomOutbound(t *testing.T) {
	cfg := config.Defaults()
	cfg.Main.RoutingMode = "global"
	cfg.Outbounds = []config.Outbound{{
		Enabled: true,
		Tag:     "test",
		Type:    "vless",
		Server:  "example.com",
		Port:    443,
		UUID:    "a3482e88-686a-4a58-8126-99c9df64b060",
		TLS:     true,
	}}
	route := generatedRoute(t, cfg)
	rules := route["rules"].([]any)
	if rules[2].(map[string]any)["outbound"] != "test" {
		t.Fatalf("global inbound should use first custom outbound, got route %+v", route)
	}
}

func TestGenerateRoutesProxyClientToSelectedOutbound(t *testing.T) {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{
		{
			Enabled: true,
			Tag:     "first",
			Type:    "vless",
			Server:  "first.example.com",
			Port:    443,
			UUID:    "a3482e88-686a-4a58-8126-99c9df64b060",
			TLS:     true,
		},
		{
			Enabled:  true,
			Tag:      "desktop_proxy",
			Type:     "trojan",
			Server:   "desktop.example.com",
			Port:     443,
			Password: "secret",
			TLS:      true,
		},
	}
	cfg.Clients = []config.Client{{
		Name:     "desktop",
		IP:       "192.168.8.100",
		Policy:   "proxy",
		Outbound: "desktop_proxy",
	}}
	route := generatedRoute(t, cfg)
	rules := route["rules"].([]any)
	if len(rules) < 3 {
		t.Fatalf("expected client route rule, got %+v", route)
	}
	rule := rules[2].(map[string]any)
	if rule["outbound"] != "desktop_proxy" || rule["action"] != "route" {
		t.Fatalf("unexpected client outbound route rule: %+v", rule)
	}
	inbounds := rule["inbound"].([]any)
	if len(inbounds) != 2 || inbounds[0] != "tproxy-0000-in" || inbounds[1] != "tproxy-0001-in" {
		t.Fatalf("client outbound route must be scoped to tproxy inbound: %+v", rule)
	}
	sourceCIDRs := rule["source_ip_cidr"].([]any)
	if len(sourceCIDRs) != 1 || sourceCIDRs[0] != "192.168.8.100/32" {
		t.Fatalf("client outbound route must match client source IP: %+v", rule)
	}
}

func TestGenerateVLESSOutbound(t *testing.T) {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{{
		Enabled:    true,
		Tag:        "my_vless",
		Type:       "vless",
		Server:     "example.com",
		Port:       443,
		UUID:       "a3482e88-686a-4a58-8126-99c9df64b060",
		Flow:       "xtls-rprx-vision",
		TLS:        true,
		ServerName: "example.com",
		Transport:  "tcp",
	}}
	proxy := generatedOutbound(t, cfg, "my_vless")
	if proxy["type"] != "vless" || proxy["server"] != "example.com" || int(proxy["server_port"].(float64)) != 443 {
		t.Fatalf("unexpected vless outbound: %+v", proxy)
	}
	if proxy["uuid"] != "a3482e88-686a-4a58-8126-99c9df64b060" || proxy["flow"] != "xtls-rprx-vision" || proxy["network"] != "tcp" {
		t.Fatalf("missing vless fields: %+v", proxy)
	}
	tls := proxy["tls"].(map[string]any)
	if tls["enabled"] != true || tls["server_name"] != "example.com" {
		t.Fatalf("unexpected tls: %+v", tls)
	}
}

func TestGenerateVLESSRealityOutbound(t *testing.T) {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{{
		Enabled:          true,
		Tag:              "my_vless",
		Type:             "vless",
		Server:           "example.com",
		Port:             443,
		UUID:             "a3482e88-686a-4a58-8126-99c9df64b060",
		Flow:             "xtls-rprx-vision",
		ServerName:       "www.example.com",
		Reality:          true,
		RealityPublicKey: "public-key",
		RealityShortID:   "0123456789abcdef",
		ALPN:             []string{"h2", "http/1.1"},
	}}
	proxy := generatedOutbound(t, cfg, "my_vless")
	tls := proxy["tls"].(map[string]any)
	reality := tls["reality"].(map[string]any)
	if tls["enabled"] != true || tls["server_name"] != "www.example.com" {
		t.Fatalf("unexpected tls: %+v", tls)
	}
	if reality["enabled"] != true || reality["public_key"] != "public-key" || reality["short_id"] != "0123456789abcdef" {
		t.Fatalf("unexpected reality: %+v", reality)
	}
	alpn := tls["alpn"].([]any)
	if len(alpn) != 2 || alpn[0] != "h2" || alpn[1] != "http/1.1" {
		t.Fatalf("unexpected alpn: %+v", alpn)
	}
}

func TestGenerateVLESSAdvancedTLSAndTransport(t *testing.T) {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{{
		Enabled:           true,
		Tag:               "my_vless",
		Type:              "vless",
		Server:            "example.com",
		Port:              443,
		UUID:              "a3482e88-686a-4a58-8126-99c9df64b060",
		Flow:              "xtls-rprx-vision",
		TLS:               true,
		ServerName:        "www.example.com",
		ALPN:              []string{"h2", "http/1.1"},
		TLSMinVersion:     "1.2",
		TLSMaxVersion:     "1.3",
		TLSCipherSuites:   []string{"TLS_AES_128_GCM_SHA256"},
		ECH:               true,
		ECHConfig:         []string{"ech-config"},
		ECHConfigPath:     "/etc/zakop/ech.pem",
		UTLSFingerprint:   "chrome",
		Reality:           true,
		RealityPublicKey:  "public-key",
		Transport:         "ws",
		WSHost:            "front.example.com",
		WSPath:            "/ws",
		WSEarlyData:       2048,
		WSEarlyDataHeader: "Sec-WebSocket-Protocol",
		PacketEncoding:    "xudp",
	}}
	proxy := generatedOutbound(t, cfg, "my_vless")
	if proxy["packet_encoding"] != "xudp" {
		t.Fatalf("missing packet encoding: %+v", proxy)
	}
	tls := proxy["tls"].(map[string]any)
	if tls["min_version"] != "1.2" || tls["max_version"] != "1.3" {
		t.Fatalf("missing TLS versions: %+v", tls)
	}
	if tls["utls"].(map[string]any)["fingerprint"] != "chrome" {
		t.Fatalf("missing uTLS: %+v", tls)
	}
	if tls["ech"].(map[string]any)["config_path"] != "/etc/zakop/ech.pem" {
		t.Fatalf("missing ECH: %+v", tls)
	}
	ciphers := tls["cipher_suites"].([]any)
	if len(ciphers) != 1 || ciphers[0] != "TLS_AES_128_GCM_SHA256" {
		t.Fatalf("missing ciphers: %+v", tls)
	}
	transport := proxy["transport"].(map[string]any)
	if transport["type"] != "ws" || transport["path"] != "/ws" || int(transport["max_early_data"].(float64)) != 2048 {
		t.Fatalf("unexpected transport: %+v", transport)
	}
	headers := transport["headers"].(map[string]any)
	if headers["Host"] != "front.example.com" {
		t.Fatalf("missing websocket host header: %+v", transport)
	}
}

func TestGenerateHysteria2Outbound(t *testing.T) {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{{
		Enabled:    true,
		Tag:        "my_hy2",
		Type:       "hysteria2",
		Server:     "example.com",
		Port:       443,
		Password:   "hy2-secret",
		ServerName: "example.com",
		Insecure:   true,
	}}
	proxy := generatedOutbound(t, cfg, "my_hy2")
	if proxy["type"] != "hysteria2" || proxy["password"] != "hy2-secret" {
		t.Fatalf("unexpected hysteria2 outbound: %+v", proxy)
	}
	tls := proxy["tls"].(map[string]any)
	if tls["enabled"] != true || tls["server_name"] != "example.com" || tls["insecure"] != true {
		t.Fatalf("unexpected hysteria2 tls: %+v", tls)
	}
}

func TestGenerateHysteria2AdvancedFields(t *testing.T) {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{{
		Enabled:              true,
		Tag:                  "my_hy2",
		Type:                 "hysteria2",
		Server:               "example.com",
		Port:                 443,
		Password:             "hy2-secret",
		HysteriaObfsType:     "salamander",
		HysteriaObfsPassword: "obfs-secret",
		HysteriaUpMbps:       20,
		HysteriaDownMbps:     100,
	}}
	proxy := generatedOutbound(t, cfg, "my_hy2")
	obfs := proxy["obfs"].(map[string]any)
	if obfs["type"] != "salamander" || obfs["password"] != "obfs-secret" || int(proxy["up_mbps"].(float64)) != 20 || int(proxy["down_mbps"].(float64)) != 100 {
		t.Fatalf("unexpected hysteria2 advanced fields: %+v", proxy)
	}
}

func TestGenerateShadowsocksOutbound(t *testing.T) {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{{
		Enabled:  true,
		Tag:      "my_ss",
		Type:     "shadowsocks",
		Server:   "example.com",
		Port:     8388,
		Method:   "2022-blake3-aes-128-gcm",
		Password: "ss-secret",
	}}
	proxy := generatedOutbound(t, cfg, "my_ss")
	if proxy["type"] != "shadowsocks" || proxy["method"] != "2022-blake3-aes-128-gcm" || proxy["password"] != "ss-secret" {
		t.Fatalf("unexpected shadowsocks outbound: %+v", proxy)
	}
}

func TestGenerateTrojanOutbound(t *testing.T) {
	cfg := config.Defaults()
	cfg.Outbounds = []config.Outbound{{
		Enabled:    true,
		Tag:        "my_trojan",
		Type:       "trojan",
		Server:     "example.com",
		Port:       443,
		Password:   "trojan-secret",
		TLS:        true,
		ServerName: "example.com",
	}}
	proxy := generatedOutbound(t, cfg, "my_trojan")
	if proxy["type"] != "trojan" || proxy["password"] != "trojan-secret" {
		t.Fatalf("unexpected trojan outbound: %+v", proxy)
	}
	tls := proxy["tls"].(map[string]any)
	if tls["enabled"] != true || tls["server_name"] != "example.com" {
		t.Fatalf("unexpected trojan tls: %+v", tls)
	}
}

func TestGenerateLatencyClientExposesOutboundsThroughLocalClashAPI(t *testing.T) {
	cfg := config.Config{Outbounds: []config.Outbound{
		{Enabled: true, Tag: "first", Type: "trojan", Server: "first.example", Port: 443, Password: "one", TLS: true},
		{Enabled: true, Tag: "second", Type: "shadowsocks", Server: "second.example", Port: 8388, Method: "aes-128-gcm", Password: "two"},
	}}

	raw, err := GenerateLatencyClient(cfg, []string{"first", "second"}, 21001)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		`"tag": "first"`,
		`"tag": "second"`,
		`"external_controller": "127.0.0.1:21001"`,
		`"final": "direct"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("latency client missing %s:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{`"inbounds"`, `"type": "mixed"`, `latency-0000-in`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("latency client must not contain %s:\n%s", forbidden, text)
		}
	}
}

func TestGenerateLatencyClientRejectsInvalidTargets(t *testing.T) {
	cfg := config.Config{Outbounds: []config.Outbound{{Enabled: true, Tag: "first", Type: "trojan", Server: "example.com", Port: 443, Password: "secret"}}}

	for _, test := range []struct {
		tags []string
		port int
	}{
		{tags: nil, port: 21001},
		{tags: []string{"first"}, port: 0},
		{tags: []string{"missing"}, port: 21001},
		{tags: []string{"first", "first"}, port: 21001},
	} {
		if _, err := GenerateLatencyClient(cfg, test.tags, test.port); err == nil {
			t.Fatalf("expected latency target error for %+v", test)
		}
	}
}

func TestCompareVersion(t *testing.T) {
	if compareVersion("1.11.9", MinimumVersion) >= 0 {
		t.Fatal("1.11.9 should be unsupported")
	}
	if compareVersion("1.12.0", MinimumVersion) < 0 {
		t.Fatal("1.12.0 should be supported")
	}
	if compareVersion("1.13.12", MinimumVersion) < 0 {
		t.Fatal("1.13.12 should be supported")
	}
}

func generatedOutbound(t *testing.T, cfg config.Config, tag string) map[string]any {
	t.Helper()
	out, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, outbound := range decoded.Outbounds {
		if outbound["tag"] == tag {
			return outbound
		}
	}
	t.Fatalf("outbound %q not found in %s", tag, string(out))
	return nil
}

func containsAnyString(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func generatedRoute(t *testing.T, cfg config.Config) map[string]any {
	t.Helper()
	out, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Route map[string]any `json:"route"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded.Route
}

func generatedDNSServer(t *testing.T, cfg config.Config, tag string) map[string]any {
	t.Helper()
	out, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		DNS struct {
			Servers []map[string]any `json:"servers"`
		} `json:"dns"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, server := range decoded.DNS.Servers {
		if server["tag"] == tag {
			return server
		}
	}
	t.Fatalf("DNS server %q not found in %s", tag, string(out))
	return nil
}
