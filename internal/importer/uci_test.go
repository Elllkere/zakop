package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elllkere/zakop/internal/config"
)

const subscriptionPoolFixture = `
config subscription 'sub1'
	option url 'https://example.com/sub'

config outbound 'sub1_existing'
	option tag 'sub1_existing'
	option label 'Spotify'
	option type '%s'
	option server '198.51.100.1'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
	option password 'secret'
	option method 'aes-128-gcm'
	option subscription 'sub1'

config outbound 'backup'
	option type 'trojan'
	option server 'backup.example.com'
	option port '443'
	option password 'backup-secret'

config outbound_pool 'spotify'
	list outbound 'sub1_existing'
	list outbound 'backup'

config rule 'spotify'
	option action 'proxy'
	list domain_equals 'example.com'
	option outbound 'sub1_existing'
`

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

func TestApplyManualNodePreservesUnicodeFlagLabelInUCI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zakop")
	if err := os.WriteFile(path, []byte("config main 'main'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	nodes := []Node{{
		Raw: "trojan://secret@example.com:443#%F0%9F%87%AB%F0%9F%87%AE%D0%A4%D0%B8%D0%BD%D0%BB%D1%8F%D0%BD%D0%B4%D0%B8%D1%8F",
		Outbound: config.Outbound{
			Enabled:  true,
			Type:     "trojan",
			Label:    "🇫🇮Финляндия",
			Server:   "example.com",
			Port:     443,
			Password: "secret",
			TLS:      true,
		},
	}}
	if _, err := ApplyNodes(nodes, ApplyOptions{ConfigPath: path, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != 1 || cfg.Outbounds[0].Label != "🇫🇮Финляндия" {
		t.Fatalf("Unicode flag label was not preserved: %+v", cfg.Outbounds)
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

func TestApplySubscriptionAddressChangePreservesPoolAndRule(t *testing.T) {
	for _, tt := range []struct {
		typ  string
		link string
	}{
		{"vless", "vless://a3482e88-686a-4a58-8126-99c9df64b060@198.51.100.2:443#Spotify"},
		{"hysteria2", "hy2://secret@198.51.100.2:443#Spotify"},
		{"shadowsocks", "ss://aes-128-gcm:secret@198.51.100.2:443#Spotify"},
		{"trojan", "trojan://secret@198.51.100.2:443#Spotify"},
	} {
		t.Run(tt.typ, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "zakop")
			if err := os.WriteFile(path, []byte(fmt.Sprintf(subscriptionPoolFixture, tt.typ)), 0644); err != nil {
				t.Fatal(err)
			}
			nodes, err := ParseLinks(tt.link)
			if err != nil {
				t.Fatal(err)
			}
			// Refresh again to verify that the retained tag also survives the
			// next update after the address migration.
			for refresh := 0; refresh < 2; refresh++ {
				if _, err := ApplyNodes(nodes, ApplyOptions{ConfigPath: path, Source: "sub1", Subscription: true, Replace: true}); err != nil {
					t.Fatal(err)
				}
				cfg, err := config.LoadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if len(cfg.Outbounds) != 2 || cfg.Outbounds[1].Tag != "sub1_existing" || cfg.Outbounds[1].Server != "198.51.100.2" {
					t.Fatalf("address change did not update the existing node: %+v", cfg.Outbounds)
				}
				if len(cfg.OutboundPools) != 1 || strings.Join(cfg.OutboundPools[0].Outbounds, ",") != "sub1_existing,backup" {
					t.Fatalf("pool references or priority changed: %+v", cfg.OutboundPools)
				}
				if len(cfg.Rules) != 1 || cfg.Rules[0].Outbound != "sub1_existing" {
					t.Fatalf("rule reference changed: %+v", cfg.Rules)
				}
			}
		})
	}
}

func TestApplySubscriptionAddressChangeRejectsUncertainMatches(t *testing.T) {
	for _, name := range []string{
		"changed label", "missing label", "changed port", "changed credentials",
		"changed protocol", "ambiguous old nodes", "ambiguous new nodes",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "zakop")
			before := fmt.Sprintf(subscriptionPoolFixture, "vless")
			nodes, err := ParseLinks("vless://a3482e88-686a-4a58-8126-99c9df64b060@198.51.100.2:443#Spotify")
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "changed label":
				nodes[0].Outbound.Label = "Another server"
			case "missing label":
				nodes[0].Outbound.Label = ""
			case "changed port":
				nodes[0].Outbound.Port = 8443
			case "changed credentials":
				nodes[0].Outbound.UUID = "different-uuid"
			case "changed protocol":
				nodes[0].Outbound.Type = "trojan"
				nodes[0].Outbound.Password = "secret"
			case "ambiguous old nodes":
				before += `
config outbound 'sub1_duplicate'
	option label 'Spotify'
	option type 'vless'
	option server '198.51.100.3'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
	option subscription 'sub1'
`
			case "ambiguous new nodes":
				duplicate := nodes[0]
				duplicate.Raw = "vless://a3482e88-686a-4a58-8126-99c9df64b060@198.51.100.3:443#Spotify"
				duplicate.Outbound.Server = "198.51.100.3"
				nodes = append(nodes, duplicate)
			}
			if err := os.WriteFile(path, []byte(before), 0644); err != nil {
				t.Fatal(err)
			}
			_, err = ApplyNodes(nodes, ApplyOptions{ConfigPath: path, Source: "sub1", Subscription: true, Replace: true})
			if err == nil || !strings.Contains(err.Error(), "references unknown outbound") {
				t.Fatalf("expected unresolved reference instead of guessing a replacement, got %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != before {
				t.Fatal("rejected update changed the original configuration")
			}
		})
	}
}

func TestApplySubscriptionAddressChangeIgnoresOtherSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zakop")
	before := fmt.Sprintf(subscriptionPoolFixture, "vless") + `
config subscription 'sub2'
	option url 'https://example.com/other-sub'
config outbound 'sub2_existing'
	option label 'Spotify'
	option type 'vless'
	option server '198.51.100.2'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
	option subscription 'sub2'
config outbound 'manual'
	option label 'Spotify'
	option type 'vless'
	option server '198.51.100.2'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
`
	if err := os.WriteFile(path, []byte(before), 0644); err != nil {
		t.Fatal(err)
	}
	nodes, err := ParseLinks("vless://a3482e88-686a-4a58-8126-99c9df64b060@198.51.100.2:443#Spotify")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyNodes(nodes, ApplyOptions{ConfigPath: path, Source: "sub1", Subscription: true, Replace: true}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != 4 || cfg.Outbounds[3].Tag != "sub1_existing" || cfg.Outbounds[3].Server != "198.51.100.2" {
		t.Fatalf("matching reused another source's tag or lost a node: %+v", cfg.Outbounds)
	}
	if cfg.Outbounds[1].Tag != "sub2_existing" || cfg.Outbounds[2].Tag != "manual" {
		t.Fatalf("nodes outside the subscription changed: %+v", cfg.Outbounds)
	}
}

func TestApplySubscriptionMatchesAllEndpointsBeforeAddressChanges(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%v", reverse), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "zakop")
			before := fmt.Sprintf(subscriptionPoolFixture, "vless") + `
config outbound 'sub1_other'
	option label 'Spotify'
	option type 'vless'
	option server '198.51.100.3'
	option port '443'
	option uuid 'a3482e88-686a-4a58-8126-99c9df64b060'
	option subscription 'sub1'
`
			if err := os.WriteFile(path, []byte(before), 0644); err != nil {
				t.Fatal(err)
			}
			links := []string{
				"vless://a3482e88-686a-4a58-8126-99c9df64b060@198.51.100.2:443#Spotify",
				"vless://a3482e88-686a-4a58-8126-99c9df64b060@198.51.100.1:443#Renamed",
			}
			if reverse {
				links[0], links[1] = links[1], links[0]
			}
			nodes, err := ParseLinks(strings.Join(links, "\n"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ApplyNodes(nodes, ApplyOptions{ConfigPath: path, Source: "sub1", Subscription: true, Replace: true}); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.LoadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.Outbounds) != 3 {
				t.Fatalf("unexpected outbounds: %+v", cfg.Outbounds)
			}
			wantServers := map[string]string{
				"backup":        "backup.example.com",
				"sub1_existing": "198.51.100.1",
				"sub1_other":    "198.51.100.2",
			}
			for _, outbound := range cfg.Outbounds {
				if wantServers[outbound.Tag] != outbound.Server {
					t.Fatalf("endpoint matching and address migration assigned the wrong tag: %+v", outbound)
				}
				if outbound.Tag == "sub1_existing" && outbound.Label != "Renamed" {
					t.Fatal("the unchanged endpoint did not receive the updated label")
				}
			}
		})
	}
}
