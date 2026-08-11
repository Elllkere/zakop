package openwrt

import (
	"os"
	"strings"
	"testing"
)

func TestGeneralLuCIShowsStatusControlsAndOnlyCoreSettings(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/neto/general.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"'require neto.i18n as netoI18n'",
		"commandResult('/etc/init.d/neto', [ 'status' ])",
		"commandResult('/etc/init.d/neto', [ 'enabled' ])",
		"commandResult('/bin/pidof', [ 'netod' ])",
		"commandResult('/bin/pidof', [ 'sing-box' ])",
		"commandResult('/usr/bin/netod', [ 'version' ])",
		"commandResult(singboxBin, [ 'version' ])",
		"commandResult('/usr/share/neto/check-version.sh', [])",
		"refreshUpdateState: function()",
		"status: 'checking'",
		"this.updateState.status = update.status",
		"window.setTimeout(L.bind(this.refreshUpdateState, this), 0)",
		"latestOutput.textContent = update.latest || '-'",
		"updateButton.disabled = !available",
		"form.DummyValue, '_neto_status'",
		"form.DummyValue, '_singbox_status'",
		"form.DummyValue, '_netod_version'",
		"form.DummyValue, '_singbox_version'",
		"form.DummyValue, '_latest_version'",
		"form.DummyValue, '_update_status'",
		"form.ListValue, 'update_via'",
		"form.ListValue, 'update_outbound'",
		"o.depends('update_via', 'proxy')",
		"form.Button, '_neto_update'",
		"fs.exec('/usr/share/neto/upgrade.sh', [ '--luci' ])",
		"Update installed. Reconnecting to LuCI...",
		"Connection interrupted. Reconnecting to verify the installed version...",
		"window.setTimeout(function()",
		"form.Button, '_service'",
		"form.Button, '_autostart'",
		"fs.exec('/etc/init.d/neto', [ action ])",
		"waitForServiceState(action == 'start', 40)",
		"window.setTimeout(resolve, 250)",
		"Service did not start in time",
		"Service did not stop in time",
		"fs.exec('/sbin/uci', [ 'set', 'neto.main.enabled=1' ])",
		"form.ListValue, 'dns_upstream_preset'",
		"o.value('cloudflare', _('Cloudflare'))",
		"o.value('google', _('Google'))",
		"o.value('custom', _('Custom'))",
		"form.ListValue, 'real_dns_mode'",
		"o.value('direct', 'direct')",
		"o.value('proxy', 'proxy')",
		"form.ListValue, 'real_dns_outbound'",
		"addProxyOutboundChoices(o)",
		"o.depends('real_dns_mode', 'proxy')",
		"form.ListValue, 'real_dns_transport'",
		"form.Value, 'real_dns_server'",
		"form.Value, 'real_dns_server_name'",
		"form.Value, '_real_dns_doh'",
		"splitDoHValue(formvalue",
		"port = defaultDNSPort(protocol)",
		"uci.set('neto', 'main', 'real_dns_server', host + ':' + port)",
		"uci.set('neto', 'main', 'real_dns_transport', protocol)",
		"uci.set('neto', 'main', 'real_dns_upstream', host + ':' + port)",
		"form.ListValue, 'routing_mode'",
		"o.value('simple', _('Simple'))",
		"form.ListValue, 'default_outbound'",
		"o.value('direct', 'direct')",
		"form.ListValue, 'simple_action'",
		"form.ListValue, 'simple_outbound'",
		"form.ListValue, 'simple_domain_input'",
		"form.ListValue, 'simple_ip_input'",
		"addSimpleProviderList(s, 'simple_domain_provider'",
		"addSimpleProviderList(s, 'simple_ip_provider'",
		"addSimpleDynamicList(s, 'simple_domain_equals'",
		"addSimpleDynamicList(s, 'simple_domain_ends_with'",
		"addSimpleTextList(s, '_simple_domain_equals_text'",
		"addSimpleTextList(s, '_simple_domain_ends_with_text'",
		"addSimpleDynamicList(s, 'simple_domain_file'",
		"addSimpleDynamicList(s, 'simple_ip_cidr'",
		"addSimpleTextList(s, '_simple_ip_cidr_text'",
		"addSimpleDynamicList(s, 'simple_ip_file'",
		"setListOption(section_id, target, splitTextValues(formvalue))",
		"uci.set('neto', 'main', 'simple_domain_input', domainInput)",
		"uci.set('neto', 'main', 'simple_ip_input', ipInput)",
		"normalizeSimpleRuleState()",
		"if (routingMode != 'simple')",
		"o.retain = true",
		"form.ListValue, 'language'",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("general.js missing %q:\n%s", want, s)
		}
	}

	loadStart := strings.Index(s, "load: function()")
	if loadStart < 0 {
		t.Fatalf("general.js load block not found:\n%s", s)
	}
	loadEnd := strings.Index(s[loadStart:], "handleSave: function()")
	if loadEnd < 0 {
		t.Fatalf("general.js load block end not found:\n%s", s)
	}
	loadBlock := s[loadStart : loadStart+loadEnd]
	if strings.Contains(loadBlock, "check-version.sh") {
		t.Fatalf("version check must not block the LuCI load path:\n%s", loadBlock)
	}
	for _, forbidden := range []string{
		"form.Flag, 'enabled'",
		"form.Flag, 'fakeip_enabled'",
		"form.Value, 'dns_listen'",
		"form.Value, 'fakeip_range'",
		"form.Flag, 'filter_aaaa_for_fakeip'",
		"form.DynamicList, 'lan_subnet'",
		"form.Value, 'singbox_bin'",
		"form.Value, 'tproxy_port'",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("general.js should not expose %q:\n%s", forbidden, s)
		}
	}

	simpleEnd := strings.Index(s, "addSimpleDynamicList(s, 'simple_ip_file'")
	defaultOutbound := strings.LastIndex(s, "form.ListValue, 'default_outbound'")
	if simpleEnd < 0 || defaultOutbound < 0 || defaultOutbound < simpleEnd {
		t.Fatalf("default_outbound should be rendered after simple rule fields:\n%s", s)
	}
}

func TestAdvancedLuCIContainsMovedSettingsButNoFakeIPToggle(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/neto/advanced.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"form.NamedSection, 'main', 'main', _('Advanced')",
		"form.Flag, 'manage_dnsmasq'",
		"form.Value, 'dns_listen'",
		"form.Flag, 'filter_aaaa_for_fakeip'",
		"form.DynamicList, 'lan_subnet'",
		"form.DynamicList, 'lan_iface'",
		"form.Value, 'singbox_bin'",
		"form.Value, 'singbox_dns_fakeip'",
		"form.Value, 'singbox_dns_real_direct'",
		"form.Value, 'singbox_dns_real_proxy'",
		"form.Value, 'tproxy_port'",
		"form.Value, 'mark'",
		"form.Value, 'table'",
		"form.Value, 'fakeip_range'",
		"form.Flag, 'resolve_for_subnet_rules'",
		"form.Flag, 'nft_counters'",
		"uci.set('neto', 'main', 'fakeip_enabled', '1')",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("advanced.js missing %q:\n%s", want, s)
		}
	}
	for _, forbidden := range []string{
		"form.Flag, 'enabled'",
		"form.Flag, 'fakeip_enabled'",
		"form.Value, 'real_dns_upstream'",
		"form.Value, 'singbox_dns'",
		"form.ListValue, 'dns_upstream_protocol'",
		"form.ListValue, 'real_dns_mode'",
		"form.ListValue, 'real_dns_transport'",
		"form.ListValue, 'routing_mode'",
		"form.ListValue, 'default_outbound'",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("advanced.js should not expose %q:\n%s", forbidden, s)
		}
	}
}
