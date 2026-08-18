package openwrt

import (
	"os"
	"strings"
	"testing"
)

func TestProvidersLuCIUsesProviderSections(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/providers.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"form.GridSection, 'provider'",
		"uci.unset('zakop', sid, 'enabled')",
		"form.Value, 'label'",
		"form.ListValue, 'type'",
		"form.ListValue, 'source'",
		"form.Value, 'url'",
		"form.Value, 'script_path'",
		"ZAKOP_PROVIDER_OUTPUT",
		"form.Flag, 'auto_update'",
		"form.ListValue, 'update_schedule'",
		"form.ListValue, 'update_hour'",
		"form.ListValue, 'update_minute'",
		"form.ListValue, 'update_interval_minutes'",
		"addUpdateIntervalChoices(o)",
		"o.depends({ 'auto_update': '1', 'update_schedule': 'time' })",
		"o.depends({ 'auto_update': '1', 'update_schedule': 'interval' })",
		"option.value('360', _('Every 6 hours'))",
		"form.ListValue, 'update_via'",
		"o.value('direct', 'direct')",
		"o.value('proxy', 'proxy')",
		"form.ListValue, 'update_outbound'",
		"firstProxyOutboundTag()",
		"proxyOutboundTagExists(value)",
		"o.forcewrite = true",
		"return _('Create an outbound before selecting proxy update.')",
		"form.DummyValue, 'item_count'",
		"form.DummyValue, 'last_update'",
		"validateProviderReferences()",
		"referencedProviders(section_id)",
		"handleSaveCommitConfig: function()",
		"return this.handleSaveCommitConfig()",
		"fs.exec('/sbin/uci', [ 'commit', 'zakop' ])",
		"throw new Error(res.stderr || res.stdout || _('Commit failed'))",
		"domain_provider",
		"ip_provider",
		"Rule \"%s\" references missing provider \"%s\"",
		"form.Button, '_update'",
		"function(ev, section_id)",
		"ZAKOP_PROVIDER_PROXY",
		"fs.exec('/usr/bin/zakopd', [ 'providers', 'update', section_id ])",
		"handleProviderUpdateAll: function()",
		"runProviderUpdates: function(names)",
		"setProviderUpdateAllButton: function(running, current, total)",
		"fs.exec('/usr/bin/zakopd', [ 'providers', 'update', name ])",
		"failures.push({ name: name, error:",
		"showProviderUpdateFailures: function(failures)",
		"Updating %d/%d…",
		"'data-zakop-providers-update-all': '1'",
		"_('Updating all…')",
		"_('Update all')",
		"handleImportProviderPresets: function()",
		"Import provider presets",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("providers.js missing %q:\n%s", want, s)
		}
	}
	for _, forbidden := range []string{
		"form.GridSection, 'rule'",
		"form.DynamicList, 'file'",
		"form.Value, 'priority'",
		"form.Value, 'description'",
		"form.ListValue, 'action'",
		"form.ListValue, 'dns_mode'",
		"form.ListValue, 'outbound'",
		"sortable = true",
		"uci.commit(",
		"form.Flag, 'enabled'",
		"missing or disabled provider",
		"fs.exec('/usr/bin/zakopd', [ 'providers', 'update' ])",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("providers.js must not contain policy field %q:\n%s", forbidden, s)
		}
	}
	for _, forbidden := range []string{
		"option.value('', _('Auto'))",
		"option.value('', _('Select outbound'))",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("providers.js must not expose empty outbound choice %q:\n%s", forbidden, s)
		}
	}
}

func TestProvidersLuCIImportsProviderPresets(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/providers.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"var communityDomainProviders = [",
		"community_telegram_domains",
		"community_tiktok_domains",
		"community_twitter_domains",
		"community_youtube_domains",
		"community_meta_domains",
		"community_discord_domains",
		"community_anime_domains",
		"var builtinIPProviders = [",
		"cloudflare_ipv4",
		"telegram_ipv4",
		"akamai_ipv4",
		"aws_ipv4",
		"aws_full_ipv4",
		"aws_full_eu_ipv4",
		"google_cloud_eu_ipv4",
		"Cloudflare IPv4",
		"Telegram IPv4",
		"Akamai IPv4",
		"AWS CDN IPv4",
		"AWS Full IPv4 (may affect game ping)",
		"AWS Full EU IPv4",
		"Google Cloud Europe IPv4",
		"https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Services/telegram.lst",
		"https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Services/tiktok.lst",
		"https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Services/twitter.lst",
		"https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Services/youtube.lst",
		"https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Services/meta.lst",
		"https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Services/discord.lst",
		"https://raw.githubusercontent.com/itdoginfo/allow-domains/refs/heads/main/Categories/anime.lst",
		"https://www.cloudflare.com/ips-v4/",
		"https://core.telegram.org/resources/cidr.txt",
		"/usr/share/zakop/providers/akamai-ipv4.sh",
		"/usr/share/zakop/providers/aws-ipv4.sh",
		"/usr/share/zakop/providers/aws-full-ipv4.sh",
		"/usr/share/zakop/providers/aws-full-eu-ipv4.sh",
		"/usr/share/zakop/providers/google-cloud-eu-ipv4.sh",
		"providerURLExists(def.url)",
		"providerScriptExists(def.script_path)",
		"uniqueProviderSection(def.section)",
		"function addProviderPreset(def)",
		"uci.add('zakop', 'provider', section)",
		"uci.set('zakop', section, 'type', def.type || 'domain')",
		"uci.set('zakop', section, 'source', source)",
		"uci.set('zakop', section, 'script_path', def.script_path)",
		"uci.set('zakop', section, 'auto_update', '0')",
		"uci.set('zakop', section, 'update_minute', def.update_minute || '5')",
		"return this.map.save(normalizeProviders)",
		"for (var j = 0; j < builtinIPProviders.length; j++)",
		"return uci.save('zakop')",
		"throw new Error(_('Save failed'))",
		"return fs.exec('/sbin/uci', [ 'commit', 'zakop' ])",
		"return self.handleImportProviderPresets().catch(function(err) {",
		"fs.exec('/etc/init.d/zakop', [ 'restart' ])",
		"Provider presets already exist",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("providers.js missing community provider preset %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "uci.set('zakop', section, 'enabled'") {
		t.Fatalf("community provider presets must not write enabled:\n%s", s)
	}
}

func TestProvidersLuCIShowsUpdatedInTable(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/providers.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	needle := "form.DummyValue, 'last_update'"
	start := strings.Index(s, needle)
	if start < 0 {
		t.Fatalf("providers.js missing field %q:\n%s", needle, s)
	}
	end := strings.Index(s[start+len(needle):], "\n\t\to = s.option(")
	block := s[start:]
	if end >= 0 {
		block = s[start : start+len(needle)+end]
	}
	if strings.Contains(block, "o.modalonly = true") {
		t.Fatalf("provider updated field should remain visible in table:\n%s", block)
	}
}

func TestProvidersLuCIUpdateButtonIsModalOnly(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/providers.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	needle := "form.Button, '_update'"
	start := strings.Index(s, needle)
	if start < 0 {
		t.Fatalf("providers.js missing field %q:\n%s", needle, s)
	}
	end := strings.Index(s[start+len(needle):], "\n\t\to = s.option(")
	block := s[start:]
	if end >= 0 {
		block = s[start : start+len(needle)+end]
	}
	for _, want := range []string{
		"o.inputtitle = _('Update')",
		"return true;",
		"o.modalonly = true",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("provider update button missing %q:\n%s", want, block)
		}
	}
}

func TestProvidersLuCITableEditsOnlyAutoUpdateFlag(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/providers.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, needle := range []string{
		"form.ListValue, 'type'",
		"form.ListValue, 'source'",
		"form.Value, 'url'",
		"form.ListValue, 'update_via'",
	} {
		start := strings.Index(s, needle)
		if start < 0 {
			t.Fatalf("providers.js missing field %q:\n%s", needle, s)
		}
		end := strings.Index(s[start+len(needle):], "\n\t\to = s.option(")
		block := s[start:]
		if end >= 0 {
			block = s[start : start+len(needle)+end]
		}
		if strings.Contains(block, "o.editable = true") {
			t.Fatalf("provider table field %q should be read-only text:\n%s", needle, block)
		}
		if strings.Contains(block, "o.modalonly = true") {
			t.Fatalf("provider table field %q should remain visible:\n%s", needle, block)
		}
		for _, forbidden := range []string{
			"plain text provider list",
			"custom filtering",
			"ZAKOP_PROVIDER_PROXY",
		} {
			if strings.Contains(block, forbidden) {
				t.Fatalf("provider table field %q should not contain help text %q:\n%s", needle, forbidden, block)
			}
		}
	}
	if strings.Contains(s, "form.Flag, 'enabled'") {
		t.Fatalf("provider enabled flag should not be exposed:\n%s", s)
	}
	for _, needle := range []string{
		"form.Flag, 'auto_update'",
	} {
		start := strings.Index(s, needle)
		if start < 0 {
			t.Fatalf("providers.js missing flag %q:\n%s", needle, s)
		}
		end := strings.Index(s[start+len(needle):], "\n\t\to = s.option(")
		block := s[start:]
		if end >= 0 {
			block = s[start : start+len(needle)+end]
		}
		if !strings.Contains(block, "o.editable = true") {
			t.Fatalf("provider flag %q should remain editable in table:\n%s", needle, block)
		}
	}
}

func TestProvidersLuCIURLOnlyAppliesToURLSource(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/providers.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	needle := "form.Value, 'url'"
	start := strings.Index(s, needle)
	if start < 0 {
		t.Fatalf("providers.js missing field %q:\n%s", needle, s)
	}
	end := strings.Index(s[start+len(needle):], "\n\t\to = s.option(")
	block := s[start:]
	if end >= 0 {
		block = s[start : start+len(needle)+end]
	}
	if !strings.Contains(block, "o.depends('source', 'url')") {
		t.Fatalf("provider URL should only be active for source=url:\n%s", block)
	}
}
