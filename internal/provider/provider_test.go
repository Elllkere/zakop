package provider

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/elllkere/zakop/internal/config"
	"github.com/elllkere/zakop/internal/policy"
)

func TestLoadRuleCIDRsCombinesInlineAndFileCIDRs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cidrs.txt")
	if err := os.WriteFile(path, []byte("8.8.8.0/24\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Defaults()
	providerPath := filepath.Join(dir, "provider.txt")
	if err := os.WriteFile(providerPath, []byte("9.9.9.0/24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg.Rules = []config.Rule{{
		Name:        "ip",
		Enabled:     true,
		Action:      "proxy",
		DNSMode:     "real_ip",
		IPProviders: []string{"remote_ips"},
		IPCIDRs: []string{
			"1.1.1.1",
		},
		Files: []string{
			path,
		},
	}}
	cfg.Providers = []config.Provider{{
		Name:      "remote_ips",
		Enabled:   true,
		Type:      "ip",
		LocalPath: providerPath,
		URL:       "https://example.com/ip.txt",
	}}

	got, err := LoadRuleCIDRs(cfg)
	if err != nil {
		t.Fatal(err)
	}
	values := policy.CIDRStrings(got[0])
	want := []string{"1.1.1.1/32", "8.8.8.0/24", "9.9.9.0/24"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("got %v, want %v", values, want)
	}
}

func TestLoadRuleCIDRsUsesSimpleEffectiveRule(t *testing.T) {
	cfg := config.Defaults()
	cfg.Main.RoutingMode = "simple"
	cfg.Main.SimpleRule.IPCIDRs = []string{"1.1.1.1"}

	got, err := LoadRuleCIDRs(cfg)
	if err != nil {
		t.Fatal(err)
	}
	values := policy.CIDRStrings(got[0])
	want := []string{"1.1.1.1/32"}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("got %v, want %v", values, want)
	}
}

func TestLoadRuleCIDRsMissingProviderCacheIsSkipped(t *testing.T) {
	cfg := config.Defaults()
	cfg.Rules = []config.Rule{{
		Name:        "telegram",
		Enabled:     true,
		Action:      "proxy",
		DNSMode:     "real_ip",
		IPProviders: []string{"telegram_ipv4"},
	}}
	cfg.Providers = []config.Provider{{
		Name:      "telegram_ipv4",
		Enabled:   true,
		Type:      "ip",
		LocalPath: filepath.Join(t.TempDir(), "telegram_ipv4.txt"),
		URL:       "https://core.telegram.org/resources/cidr.txt",
	}}

	got, err := LoadRuleCIDRs(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0]) != 0 {
		t.Fatalf("got %v, want missing provider to compile as empty", got[0])
	}
}

func TestLoadRuleCIDRsRestoresDefaultProviderCacheMirror(t *testing.T) {
	dir := t.TempDir()
	oldRuntimeDir := config.ProviderCacheDir
	oldPersistentDir := config.ProviderPersistentCacheDir
	config.ProviderCacheDir = filepath.Join(dir, "run")
	config.ProviderPersistentCacheDir = filepath.Join(dir, "persist")
	t.Cleanup(func() {
		config.ProviderCacheDir = oldRuntimeDir
		config.ProviderPersistentCacheDir = oldPersistentDir
	})

	providerCfg := config.Provider{
		Name:      "telegram_ipv4",
		Enabled:   true,
		Type:      "ip",
		LocalPath: filepath.Join(config.ProviderCacheDir, "telegram_ipv4.txt"),
		URL:       "https://core.telegram.org/resources/cidr.txt",
	}
	if err := os.MkdirAll(filepath.Dir(providerCfg.PersistentCachePath()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(providerCfg.PersistentCachePath(), []byte("91.108.4.0/22\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Defaults()
	cfg.Rules = []config.Rule{{
		Name:        "telegram",
		Enabled:     true,
		Action:      "proxy",
		DNSMode:     "real_ip",
		IPProviders: []string{"telegram_ipv4"},
	}}
	cfg.Providers = []config.Provider{providerCfg}

	got, err := LoadRuleCIDRs(cfg)
	if err != nil {
		t.Fatal(err)
	}
	values := policy.CIDRStrings(got[0])
	if !reflect.DeepEqual(values, []string{"91.108.4.0/22"}) {
		t.Fatalf("got %v, want restored CIDR", values)
	}
	if _, err := os.Stat(providerCfg.CachePath()); err != nil {
		t.Fatalf("runtime cache was not restored: %v", err)
	}
}

func TestLoadRuleCIDRsMirrorsExistingDefaultProviderCache(t *testing.T) {
	dir := t.TempDir()
	oldRuntimeDir := config.ProviderCacheDir
	oldPersistentDir := config.ProviderPersistentCacheDir
	config.ProviderCacheDir = filepath.Join(dir, "run")
	config.ProviderPersistentCacheDir = filepath.Join(dir, "persist")
	t.Cleanup(func() {
		config.ProviderCacheDir = oldRuntimeDir
		config.ProviderPersistentCacheDir = oldPersistentDir
	})

	providerCfg := config.Provider{
		Name:      "cloudflare_ipv4",
		Enabled:   true,
		Type:      "ip",
		LocalPath: filepath.Join(config.ProviderCacheDir, "cloudflare_ipv4.txt"),
		URL:       "https://www.cloudflare.com/ips-v4/",
	}
	if err := os.MkdirAll(filepath.Dir(providerCfg.CachePath()), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(providerCfg.CachePath(), []byte("1.1.1.0/24\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Defaults()
	cfg.Rules = []config.Rule{{
		Name:        "cloudflare",
		Enabled:     true,
		Action:      "direct",
		DNSMode:     "real_ip",
		IPProviders: []string{"cloudflare_ipv4"},
	}}
	cfg.Providers = []config.Provider{providerCfg}

	if _, err := LoadRuleCIDRs(cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(providerCfg.PersistentCachePath())
	if err != nil {
		t.Fatalf("persistent mirror was not created: %v", err)
	}
	if string(raw) != "1.1.1.0/24\n" {
		t.Fatalf("unexpected persistent mirror contents: %q", raw)
	}
}

func TestWriteCacheMirrorsDefaultProviderCache(t *testing.T) {
	dir := t.TempDir()
	oldRuntimeDir := config.ProviderCacheDir
	oldPersistentDir := config.ProviderPersistentCacheDir
	config.ProviderCacheDir = filepath.Join(dir, "run")
	config.ProviderPersistentCacheDir = filepath.Join(dir, "persist")
	t.Cleanup(func() {
		config.ProviderCacheDir = oldRuntimeDir
		config.ProviderPersistentCacheDir = oldPersistentDir
	})

	providerCfg := config.Provider{Name: "cloudflare_ipv4", Type: "ip"}
	cachePath, err := WriteCache(providerCfg, []string{"1.1.1.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	if cachePath != providerCfg.CachePath() {
		t.Fatalf("got cache path %q, want %q", cachePath, providerCfg.CachePath())
	}
	for _, path := range []string{providerCfg.CachePath(), providerCfg.PersistentCachePath()} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("cache %q was not written: %v", path, err)
		}
		if string(raw) != "1.1.1.0/24\n" {
			t.Fatalf("unexpected cache %q contents: %q", path, raw)
		}
	}
}

func TestNormalizeDownloadedProviderLists(t *testing.T) {
	domains, err := NormalizeDownloadedList(config.Provider{Name: "domains", Type: "domain"}, []byte("Example.COM. example.net\n*.cdn.example.org .assets.example.org # comment\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(domains, []string{"example.com", "example.net", "cdn.example.org", "assets.example.org"}) {
		t.Fatalf("unexpected domains: %v", domains)
	}

	cidrs, err := NormalizeDownloadedList(config.Provider{Name: "ips", Type: "ip"}, []byte("1.1.1.1 8.8.8.0/24\n2001:db8::/32\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cidrs, []string{"1.1.1.1/32", "8.8.8.0/24"}) {
		t.Fatalf("unexpected cidrs: %v", cidrs)
	}
}

func TestNormalizeDownloadedIPProviderRejectsInvalidNonIP(t *testing.T) {
	_, err := NormalizeDownloadedList(config.Provider{Name: "ips", Type: "ip"}, []byte("not-an-ip\n"))
	if err == nil {
		t.Fatal("expected invalid non-IP provider line to fail")
	}
}
