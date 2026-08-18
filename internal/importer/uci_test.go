package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elllkere/zakop/internal/config"
)

func TestApplyManualNodesAppendsOutbounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zakop")
	if err := os.WriteFile(path, []byte(`
config main 'main'
	option enabled '1'
`), 0644); err != nil {
		t.Fatal(err)
	}
	nodes := []Node{{
		Raw: "trojan://secret@example.com:443#Trojan",
		Outbound: config.Outbound{
			Enabled:  true,
			Type:     "trojan",
			Label:    "Trojan",
			Server:   "example.com",
			Port:     443,
			Password: "secret",
			TLS:      true,
		},
	}}
	count, err := ApplyNodes(nodes, ApplyOptions{ConfigPath: path, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("got count %d, want 1", count)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != 1 || cfg.Outbounds[0].Type != "trojan" || cfg.Outbounds[0].Tag == "" {
		t.Fatalf("unexpected outbounds: %+v", cfg.Outbounds)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "option imported \"1\"") || !strings.Contains(string(raw), "option import_source \"manual\"") {
		t.Fatalf("manual import metadata missing:\n%s", raw)
	}
}

func TestApplyManualNodesUsesUniqueTags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zakop")
	if err := os.WriteFile(path, []byte(`
config main 'main'
	option enabled '1'
`), 0644); err != nil {
		t.Fatal(err)
	}
	nodes := []Node{
		{
			Raw: "trojan://secret@example.com:443#Same",
			Outbound: config.Outbound{
				Enabled:  true,
				Type:     "trojan",
				Label:    "Same",
				Server:   "example.com",
				Port:     443,
				Password: "secret",
				TLS:      true,
			},
		},
		{
			Raw: "trojan://secret@example.com:443#Same",
			Outbound: config.Outbound{
				Enabled:  true,
				Type:     "trojan",
				Label:    "Same",
				Server:   "example.com",
				Port:     443,
				Password: "secret",
				TLS:      true,
			},
		},
	}
	count, err := ApplyNodes(nodes, ApplyOptions{ConfigPath: path, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("got count %d, want 2", count)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != 2 || cfg.Outbounds[0].Tag == cfg.Outbounds[1].Tag {
		t.Fatalf("expected unique tags, got %+v", cfg.Outbounds)
	}
	if cfg.Outbounds[0].Tag != "Same" || cfg.Outbounds[1].Tag != "Same_2" {
		t.Fatalf("unexpected tags: %+v", cfg.Outbounds)
	}
}

func TestApplySubscriptionNodesReplacesOnlyMatchingSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zakop")
	if err := os.WriteFile(path, []byte(`
config subscription 'sub1'
	option url 'https://example.com/sub'

config outbound 'manual'
	option tag 'manual'
	option type 'trojan'
	option server 'manual.example.com'
	option port '443'
	option password 'secret'

config outbound 'old'
	option tag 'old'
	option type 'trojan'
	option server 'old.example.com'
	option port '443'
	option password 'secret'
	option imported '1'
	option subscription 'sub1'
`), 0644); err != nil {
		t.Fatal(err)
	}
	nodes := []Node{{
		Raw: "vless://a3482e88-686a-4a58-8126-99c9df64b060@new.example.com:443#VLESS",
		Outbound: config.Outbound{
			Enabled: true,
			Type:    "vless",
			Label:   "VLESS",
			Server:  "new.example.com",
			Port:    443,
			UUID:    "a3482e88-686a-4a58-8126-99c9df64b060",
		},
	}}
	_, err := ApplyNodes(nodes, ApplyOptions{ConfigPath: path, Source: "sub1", Subscription: true, Replace: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	text := string(raw)
	if strings.Contains(text, "old.example.com") {
		t.Fatalf("old subscription node was not removed:\n%s", text)
	}
	if !strings.Contains(text, "manual.example.com") || !strings.Contains(text, "new.example.com") {
		t.Fatalf("expected manual and new node:\n%s", text)
	}
	if !strings.Contains(text, "option node_count \"1\"") ||
		!strings.Contains(text, "option last_update \"") ||
		!strings.Contains(text, "option subscription \"sub1\"") {
		t.Fatalf("subscription metadata missing:\n%s", text)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != 2 {
		t.Fatalf("unexpected outbounds: %+v", cfg.Outbounds)
	}
}

func TestApplySubscriptionNodesKeepsStableTagOnRepeatUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zakop")
	if err := os.WriteFile(path, []byte(`
config subscription 'sub1'
	option url 'https://example.com/sub'
`), 0644); err != nil {
		t.Fatal(err)
	}
	nodes := []Node{{
		Raw: "vless://a3482e88-686a-4a58-8126-99c9df64b060@new.example.com:443#VLESS",
		Outbound: config.Outbound{
			Enabled: true,
			Type:    "vless",
			Label:   "VLESS",
			Server:  "new.example.com",
			Port:    443,
			UUID:    "a3482e88-686a-4a58-8126-99c9df64b060",
		},
	}}
	if _, err := ApplyNodes(nodes, ApplyOptions{ConfigPath: path, Source: "sub1", Subscription: true, Replace: true}); err != nil {
		t.Fatal(err)
	}
	first, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Outbounds) != 1 {
		t.Fatalf("unexpected first outbounds: %+v", first.Outbounds)
	}
	firstTag := first.Outbounds[0].Tag

	if _, err := ApplyNodes(nodes, ApplyOptions{ConfigPath: path, Source: "sub1", Subscription: true, Replace: true}); err != nil {
		t.Fatal(err)
	}
	second, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Outbounds) != 1 || second.Outbounds[0].Tag != firstTag {
		t.Fatalf("expected repeated subscription update to replace with stable tag %q, got %+v", firstTag, second.Outbounds)
	}
}

func TestApplySubscriptionNodesPreservesTagUsedByRule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zakop")
	if err := os.WriteFile(path, []byte(`
config subscription 'sub1'
	option url 'https://example.com/sub'

config rule 'cf'
	option action 'proxy'
	option dns_mode 'auto'
	list domain_equals 'example.com'
	option outbound 'sub1_existing'

config outbound 'sub1_existing'
	option tag 'sub1_existing'
	option type 'vless'
	option server 'node.example.com'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
	option subscription 'sub1'
`), 0644); err != nil {
		t.Fatal(err)
	}
	nodes := []Node{{
		// The changed link label and TLS setting must update the node without
		// invalidating the rule that selects it by its stable tag.
		Raw: "vless://a3482e88-686a-4a58-8126-99c9df64b060@node.example.com:443?security=tls#Updated",
		Outbound: config.Outbound{
			Enabled: true,
			Type:    "vless",
			Label:   "Updated",
			Server:  "node.example.com",
			Port:    443,
			UUID:    "a3482e88-686a-4a58-8126-99c9df64b060",
			TLS:     true,
		},
	}}
	if _, err := ApplyNodes(nodes, ApplyOptions{ConfigPath: path, Source: "sub1", Subscription: true, Replace: true}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != 1 || cfg.Outbounds[0].Tag != "sub1_existing" || !cfg.Outbounds[0].TLS {
		t.Fatalf("subscription node was not updated in place: %+v", cfg.Outbounds)
	}
	if len(cfg.Rules) != 1 || cfg.Rules[0].Outbound != "sub1_existing" {
		t.Fatalf("rule reference was not preserved: %+v", cfg.Rules)
	}
}
