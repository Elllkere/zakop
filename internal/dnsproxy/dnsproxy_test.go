package dnsproxy

import (
	"context"
	"encoding/binary"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/elllkere/neto/internal/config"
)

func testQuery(qtype uint16) []byte {
	msg := []byte{
		0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x07, 'y', 'o', 'u', 't', 'u', 'b', 'e',
		0x03, 'c', 'o', 'm',
		0x00, 0x00, 0x01, 0x00, 0x01,
	}
	binary.BigEndian.PutUint16(msg[len(msg)-4:len(msg)-2], qtype)
	return msg
}

func TestQueryName(t *testing.T) {
	msg := testQuery(qTypeA)
	got, ok := QueryName(msg)
	if !ok || got != "youtube.com" {
		t.Fatalf("got %q, %t", got, ok)
	}
}

func TestParseQuery(t *testing.T) {
	query, ok := ParseQuery(testQuery(qTypeAAAA))
	if !ok {
		t.Fatal("query did not parse")
	}
	if query.Name != "youtube.com" || query.Type != qTypeAAAA || query.QuestionEnd != len(testQuery(qTypeAAAA)) {
		t.Fatalf("unexpected query: %+v", query)
	}
}

func TestMatchesFakeIP(t *testing.T) {
	p := Proxy{Rules: []config.Rule{
		fakeRule("youtube.com"),
		fakeRule("googlevideo.com"),
	}}
	for _, name := range []string{"youtube.com", "www.youtube.com", "r1.googlevideo.com"} {
		decision := p.domainDecision(name, "192.168.8.10")
		if decision.Action != "proxy" || decision.DNSMode != "fakeip" {
			t.Fatalf("expected %s to match", name)
		}
	}
	if decision := p.domainDecision("notyoutube.com", "192.168.8.10"); decision.Action == "proxy" && decision.DNSMode == "fakeip" {
		t.Fatal("unexpected root/subdomain match")
	}
}

func TestFakeIPAAAAGetsLocalNODATA(t *testing.T) {
	p := Proxy{
		Rules:               []config.Rule{fakeRule("youtube.com")},
		RoutingMode:         "custom",
		FilterAAAAForFakeIP: true,
	}
	resp, ok := p.localResponse(testQuery(qTypeAAAA), "192.168.8.10")
	if !ok {
		t.Fatal("expected local AAAA response")
	}
	if len(resp) != len(testQuery(qTypeAAAA)) {
		t.Fatalf("unexpected response length %d", len(resp))
	}
	if resp[2]&0x80 == 0 {
		t.Fatal("response bit is not set")
	}
	if rcode := resp[3] & 0x0f; rcode != 0 {
		t.Fatalf("rcode=%d, want NOERROR", rcode)
	}
	if answers := binary.BigEndian.Uint16(resp[6:8]); answers != 0 {
		t.Fatalf("answers=%d, want 0", answers)
	}
}

func TestFakeIPAStillForwards(t *testing.T) {
	p := Proxy{
		Rules:               []config.Rule{fakeRule("youtube.com")},
		RoutingMode:         "custom",
		FilterAAAAForFakeIP: true,
	}
	if _, ok := p.localResponse(testQuery(qTypeA), "192.168.8.10"); ok {
		t.Fatal("A query must be forwarded to sing-box FakeIP DNS")
	}
}

func TestDNSPolicyCustomDefaultFakeIP(t *testing.T) {
	p := Proxy{
		Rules:              []config.Rule{fakeRule("youtube.com")},
		RoutingMode:        "custom",
		FakeUpstream:       "127.0.0.1:15353",
		RealDirectUpstream: "127.0.0.1:15354",
	}
	if got := p.upstreamFor(testQuery(qTypeA), "192.168.8.10"); got != p.FakeUpstream {
		t.Fatalf("got %s, want fake upstream", got)
	}
	if got := p.upstreamFor(testQueryName(qTypeA, "example.org"), "192.168.8.10"); got != p.RealDirectUpstream {
		t.Fatalf("got %s, want real upstream", got)
	}
}

func TestRouterSelfNeverReceivesFakeIP(t *testing.T) {
	cfg := config.Defaults()
	cfg.Main.RoutingMode = "custom"
	cfg.Rules = []config.Rule{fakeRule("youtube.com")}
	p := New(cfg)
	p.LocalIPs["192.168.8.1"] = struct{}{}

	for _, clientIP := range []string{"127.0.0.1", "192.168.8.1", "", "203.0.113.10"} {
		if got := p.upstreamFor(testQuery(qTypeA), clientIP); got != p.RealDirectUpstream {
			t.Fatalf("router/non-LAN IP %q got %s, want real DNS", clientIP, got)
		}
		if _, ok := p.localResponse(testQuery(qTypeAAAA), clientIP); ok {
			t.Fatalf("router/non-LAN IP %q must not receive FakeIP AAAA filtering", clientIP)
		}
	}
	if got := p.upstreamFor(testQuery(qTypeA), "192.168.8.10"); got != p.FakeUpstream {
		t.Fatalf("LAN client got %s, want FakeIP upstream", got)
	}
}

func TestDNSPolicySimpleUsesSimpleRule(t *testing.T) {
	cfg := config.Defaults()
	cfg.Main.RoutingMode = "simple"
	cfg.Main.SimpleRule = fakeRule("youtube.com")
	cfg.Main.SimpleRule.Name = ""
	p := New(cfg)
	if got := p.upstreamFor(testQuery(qTypeA), "192.168.8.10"); got != p.FakeUpstream {
		t.Fatalf("got %s, want fake upstream", got)
	}
	if len(p.Rules) != 1 || p.Rules[0].Name != "simple" {
		t.Fatalf("unexpected effective rules: %+v", p.Rules)
	}
}

func TestDNSPolicyCustomDirectNoFakeIP(t *testing.T) {
	p := Proxy{
		Rules:              []config.Rule{fakeRule("youtube.com")},
		RoutingMode:        "custom",
		FakeUpstream:       "127.0.0.1:15353",
		RealDirectUpstream: "127.0.0.1:15354",
		ClientPolicies:     map[string]string{"192.168.8.50": "direct"},
	}
	if got := p.upstreamFor(testQuery(qTypeA), "192.168.8.50"); got != p.RealDirectUpstream {
		t.Fatalf("got %s, want real upstream for direct client", got)
	}
	if _, ok := p.localResponse(testQuery(qTypeAAAA), "192.168.8.50"); ok {
		t.Fatal("direct client AAAA must not receive fakeip local response")
	}
}

func TestDNSPolicyGlobalDefaultReal(t *testing.T) {
	p := Proxy{
		Rules:              []config.Rule{fakeRule("youtube.com")},
		RoutingMode:        "global",
		FakeUpstream:       "127.0.0.1:15353",
		RealDirectUpstream: "127.0.0.1:15354",
	}
	if got := p.upstreamFor(testQuery(qTypeA), "192.168.8.10"); got != p.RealDirectUpstream {
		t.Fatalf("got %s, want real upstream in global default", got)
	}
}

func TestDNSPolicyGlobalProxyUsesRealDNS(t *testing.T) {
	p := Proxy{
		Rules:              []config.Rule{fakeRule("youtube.com")},
		RoutingMode:        "global",
		FakeUpstream:       "127.0.0.1:15353",
		RealDirectUpstream: "127.0.0.1:15354",
		ClientPolicies:     map[string]string{"192.168.8.100": "proxy"},
	}
	if got := p.upstreamFor(testQuery(qTypeA), "192.168.8.100"); got != p.RealDirectUpstream {
		t.Fatalf("got %s, want real upstream in global mode", got)
	}
}

func TestDNSPolicyIPProviderRuleUsesRealDNS(t *testing.T) {
	p := Proxy{
		Rules: []config.Rule{{
			Name:        "cloudflare",
			Enabled:     true,
			Action:      "proxy",
			DNSMode:     "auto",
			IPProviders: []string{"cloudflare"},
		}},
		RoutingMode:        "custom",
		FakeUpstream:       "127.0.0.1:15353",
		RealDirectUpstream: "127.0.0.1:15354",
	}
	if got := p.upstreamFor(testQuery(qTypeA), "192.168.8.10"); got != p.RealDirectUpstream {
		t.Fatalf("got %s, want real DNS for provider/CIDR rule", got)
	}
}

func TestDNSPolicyMixedRuleUsesDomainPartOnlyForFakeIP(t *testing.T) {
	p := Proxy{
		Rules: []config.Rule{{
			Name:           "mixed",
			Enabled:        true,
			Action:         "proxy",
			DNSMode:        "auto",
			DomainContains: []string{"youtube"},
			IPProviders:    []string{"cloudflare"},
			Proto:          []string{"tcp"},
			DstPorts:       []string{"443"},
		}},
		RoutingMode:        "custom",
		FakeUpstream:       "127.0.0.1:15353",
		RealDirectUpstream: "127.0.0.1:15354",
		ClientPolicies:     map[string]string{"192.168.8.50": "direct"},
	}
	if got := p.upstreamFor(testQueryName(qTypeA, "www.youtube.com"), "192.168.8.10"); got != p.FakeUpstream {
		t.Fatalf("got %s, want FakeIP upstream for mixed rule domain match", got)
	}
	if got := p.upstreamFor(testQueryName(qTypeA, "random-cloudflare-site.com"), "192.168.8.10"); got != p.RealDirectUpstream {
		t.Fatalf("got %s, want real DNS because provider part is packet phase only", got)
	}
	if got := p.upstreamFor(testQueryName(qTypeA, "www.youtube.com"), "192.168.8.50"); got != p.RealDirectUpstream {
		t.Fatalf("got %s, want real DNS for direct client", got)
	}
}

func TestDNSPolicyRealDNSModeProxyUsesProxyListener(t *testing.T) {
	p := Proxy{
		RoutingMode:        "custom",
		RealDNSMode:        "proxy",
		RealDirectUpstream: "127.0.0.1:15354",
		RealProxyUpstream:  "127.0.0.1:15355",
	}
	if got := p.upstreamFor(testQueryName(qTypeA, "example.org"), "192.168.8.10"); got != p.RealProxyUpstream {
		t.Fatalf("got %s, want real-proxy upstream", got)
	}
}

func TestDNSProxyDoesNotUseGoHTTPDoHPath(t *testing.T) {
	data, err := os.ReadFile("dnsproxy.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, forbidden := range []string{`"net/http"`, "forwardHTTPS", "DoH upstream"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("dnsproxy normal path must not contain Go DoH client code %q", forbidden)
		}
	}
}

func TestDNSPolicyBlockReturnsNXDOMAIN(t *testing.T) {
	p := Proxy{
		Rules: []config.Rule{{
			Name:         "blocked",
			Enabled:      true,
			Action:       "block",
			DNSMode:      "real_ip",
			DomainEquals: []string{"blocked.example"},
		}},
		RoutingMode: "custom",
	}
	resp, ok := p.localResponse(testQueryName(qTypeA, "blocked.example"), "192.168.8.10")
	if !ok {
		t.Fatal("expected local block response")
	}
	if rcode := resp[3] & 0x0f; rcode != 3 {
		t.Fatalf("rcode=%d, want NXDOMAIN", rcode)
	}
}

func TestForwardFailureReturnsSERVFAIL(t *testing.T) {
	upstream, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()

	p := Proxy{
		FakeUpstream:       "127.0.0.1:15353",
		RealDirectUpstream: upstream.LocalAddr().String(),
		Timeout:            20 * time.Millisecond,
	}
	resp, err := p.handleUDP(context.Background(), testQueryName(qTypeA, "example.org"), "192.168.8.10")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp) == 0 || resp[2]&0x80 == 0 {
		t.Fatalf("expected DNS error response, got %x", resp)
	}
	if rcode := resp[3] & 0x0f; rcode != 2 {
		t.Fatalf("rcode=%d, want SERVFAIL", rcode)
	}
}

func TestForwardLimitReturnsImmediateSERVFAIL(t *testing.T) {
	p := Proxy{
		FakeUpstream:       "127.0.0.1:15353",
		RealDirectUpstream: "127.0.0.1:15354",
		realForwardSlots:   make(chan struct{}, 1),
	}
	p.realForwardSlots <- struct{}{}

	start := time.Now()
	resp, err := p.handleUDP(context.Background(), testQueryName(qTypeA, "example.org"), "192.168.8.10")
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("overloaded forward waited %v instead of failing immediately", elapsed)
	}
	if len(resp) == 0 || resp[3]&0x0f != 2 {
		t.Fatalf("expected SERVFAIL response, got %x", resp)
	}
}

func fakeRule(suffix string) config.Rule {
	return config.Rule{
		Name:           suffix,
		Enabled:        true,
		Action:         "proxy",
		Outbound:       "proxy_default",
		DNSMode:        "fakeip",
		DomainEquals:   []string{suffix},
		DomainEndsWith: []string{"." + suffix},
	}
}

func testQueryName(qtype uint16, name string) []byte {
	msg := []byte{0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	for _, label := range strings.Split(name, ".") {
		msg = append(msg, byte(len(label)))
		msg = append(msg, []byte(label)...)
	}
	msg = append(msg, 0, 0, 0, 0, 1)
	binary.BigEndian.PutUint16(msg[len(msg)-4:len(msg)-2], qtype)
	return msg
}

func testQueryWithOptions(options ...[]byte) []byte {
	msg := testQuery(qTypeA)
	var rdata []byte
	for _, option := range options {
		rdata = append(rdata, option...)
	}
	binary.BigEndian.PutUint16(msg[10:12], 1)
	msg = append(msg,
		0x00,       // root name
		0x00, 0x29, // OPT
		0x04, 0xd0, // UDP payload size
		0x00, 0x00, 0x00, 0x00, // extended rcode/version/flags
	)
	var rdLen [2]byte
	binary.BigEndian.PutUint16(rdLen[:], uint16(len(rdata)))
	msg = append(msg, rdLen[:]...)
	msg = append(msg, rdata...)
	return msg
}

func ecsOption(ip [4]byte) []byte {
	return []byte{
		0x00, 0x08, // ECS option
		0x00, 0x08, // option len
		0x00, 0x01, // family IPv4
		0x20, // source prefix 32
		0x00, // scope prefix
		ip[0], ip[1], ip[2], ip[3],
	}
}

func ednsOption(code uint16, data ...byte) []byte {
	option := make([]byte, 4, 4+len(data))
	binary.BigEndian.PutUint16(option[0:2], code)
	binary.BigEndian.PutUint16(option[2:4], uint16(len(data)))
	option = append(option, data...)
	return option
}

func TestClientSubnetIPv4(t *testing.T) {
	msg := testQueryWithOptions(ecsOption([4]byte{192, 168, 8, 50}))
	if got := clientSubnetIPv4(msg); got != "192.168.8.50" {
		t.Fatalf("got %q, want 192.168.8.50", got)
	}
	p := Proxy{}
	if got := p.clientIP(msg, "127.0.0.1"); got != "192.168.8.50" {
		t.Fatalf("got client IP %q", got)
	}
}

func TestStripClientSubnetOptionBeforeForwarding(t *testing.T) {
	msg := testQueryWithOptions(ecsOption([4]byte{192, 168, 8, 50}))
	if got := clientSubnetIPv4(msg); got == "" {
		t.Fatal("test query must carry ECS before stripping")
	}

	stripped := stripClientSubnetOption(msg)
	if string(stripped) == string(msg) {
		t.Fatal("expected DNS query to change after stripping ECS")
	}
	if got := clientSubnetIPv4(stripped); got != "" {
		t.Fatalf("ECS leaked after strip: %q", got)
	}
	if _, ok := ParseQuery(stripped); !ok {
		t.Fatal("stripped query no longer parses")
	}
	if ar := binary.BigEndian.Uint16(stripped[10:12]); ar != 1 {
		t.Fatalf("additional count changed to %d, want 1", ar)
	}
	if got := len(stripped); got != len(msg)-12 {
		t.Fatalf("unexpected stripped length %d, want %d", got, len(msg)-12)
	}
}

func TestStripClientSubnetOptionPreservesOtherEDNSOptions(t *testing.T) {
	padding := ednsOption(12, 0xaa, 0xbb)
	msg := testQueryWithOptions(ecsOption([4]byte{192, 168, 8, 50}), padding)

	stripped := stripClientSubnetOption(msg)
	if got := clientSubnetIPv4(stripped); got != "" {
		t.Fatalf("ECS leaked after strip: %q", got)
	}
	if !strings.Contains(string(stripped), string([]byte{0x00, 0x0c, 0x00, 0x02, 0xaa, 0xbb})) {
		t.Fatalf("non-ECS EDNS option was not preserved: %x", stripped)
	}
}
