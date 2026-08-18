package openwrt

import (
	"os"
	"strings"
	"testing"
)

func TestOutboundsLuCIExposesNativeTypes(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/outbounds.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"form.GridSection, 'outbound'",
		"s.addremove = true",
		"s.modaltitle = _('Outbound details')",
		"s.sectiontitle = function(section_id)",
		"s.renderSectionAdd = function()",
		"ui.addValidator(nameEl, 'uciname'",
		"function outboundTagExists(tag)",
		"section.tag || sid || section['.name']",
		"addNamedSectionValidator(el, this, _('This tag is reserved'), true)",
		"return _('Expecting: %s').format(_('unique outbound tag'))",
		"form.Value, 'label'",
		"o.cfgvalue = function(section_id)",
		"o.write = function(section_id, formvalue)",
		"uci.set('zakop', section_id, 'label', label || section_id)",
		"uci.set('zakop', sid, 'tag', sid)",
		"o.value('vless'",
		"o.value('hysteria2'",
		"o.value('shadowsocks'",
		"o.value('trojan'",
		"o.default = 'vless'",
		"form.Value, 'server', _('Address')",
		"uci.get('zakop', section_id, 'server') || uci.get('zakop', section_id, 'address')",
		"uci.set('zakop', section_id, 'server', String(formvalue || '').trim())",
		"reality_public_key",
		"tls_min_version",
		"tls_max_version",
		"tls_cipher_suites",
		"ech_config",
		"utls_fingerprint",
		"grpc_service_name",
		"websocket_early_data",
		"packet_encoding",
		"hysteria_obfs_type",
		"method",
		"password",
		"handleOutboundLatencyTest: function()",
		"runOutboundLatencyTests: function(targets)",
		"fs.exec('/usr/bin/zakopd', [ 'outbounds', 'latency', target.tag ])",
		"outboundLatencyFailure: function(target, error)",
		"updateOutboundLatencyResults: function(report)",
		"setOutboundLatencyStatus: function(text, className, title, tag)",
		"setOutboundLatencyButton: function(testing, current, total)",
		"_('Testing %d/%d…').format(current, total)",
		"form.DummyValue, '_latency', _('URLTest delay')",
		"o.textvalue = function(section_id)",
		"'data-zakop-latency-tag': outboundTag(section_id)",
		"'style': 'font-size:1.1em;font-weight:600;white-space:nowrap'",
		"'data-zakop-latency-button': '1'",
		"self.setOutboundLatencyStatus(_('Testing…'), 'spinning', '', target.tag)",
		"_('Test latency')",
		"cellResult.latency_ms",
		"_('Not tested')",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("outbounds.js missing %q:\n%s", want, s)
		}
	}
	for _, forbidden := range []string{
		"o.value('socks'",
		"o.value('socks4'",
		"o.value('socks5'",
		"o.value('mixed'",
		"o.default = 'proxy_default'",
		"uci.set('zakop', 'proxy_default', 'outbound')",
		"form.value, 'tag'",
		"showoutboundlatencyresults",
		"ui.showmodal(_('outbound latency test')",
		"o.renderwidget = function(section_id)",
		"fs.exec('/usr/bin/zakopd', [ 'outbounds', 'latency' ])",
	} {
		if strings.Contains(strings.ToLower(s), forbidden) {
			t.Fatalf("outbounds.js must not expose %q:\n%s", forbidden, s)
		}
	}
	if strings.Count(s, "o.modalonly = true") < 10 {
		t.Fatalf("outbound detail fields should be modal-only:\n%s", s)
	}
	start := strings.Index(s, "form.ListValue, 'type'")
	end := strings.Index(s, "form.Value, 'server', _('Address')")
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("could not find outbound type block:\n%s", s)
	}
	typeBlock := s[start:end]
	if strings.Contains(typeBlock, "o.value('direct'") {
		t.Fatalf("outbound type dropdown must not expose direct:\n%s", typeBlock)
	}
	if strings.Contains(typeBlock, "o.editable = true") {
		t.Fatalf("outbound type should be read-only text in table and editable in modal:\n%s", typeBlock)
	}
}

func TestOutboundsLuCIExposesPriorityPools(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/outbounds.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"form.GridSection, 'outbound_pool'",
		"pool.nodescriptions = true",
		"form.DynamicList, 'outbound', _('Priority order')",
		"o.textvalue = function(section_id)",
		"return poolPriorityText(section_id)",
		"section.label || section.name || tag",
		"display:flex;width:100%;min-width:0;max-width:100%",
		"min-width:5ch;max-width:100%;overflow:hidden;text-overflow:ellipsis",
		"form.Value, 'check_url'",
		"form.Value, 'check_interval'",
		"concreteOutboundTagExists(members[i])",
		"The top outbound has the highest priority.",
		"uci.sections('zakop', 'outbound_pool'",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("outbounds.js missing pool behavior %q:\n%s", want, s)
		}
	}
}

func TestEveryProxyOutboundSelectorIncludesPools(t *testing.T) {
	paths := []string{
		"../../embedded/files/www/luci-static/resources/view/zakop/general.js",
		"../../embedded/files/www/luci-static/resources/view/zakop/clients.js",
		"../../embedded/files/www/luci-static/resources/view/zakop/rules.js",
		"../../embedded/files/www/luci-static/resources/view/zakop/providers.js",
		"../../embedded/files/www/luci-static/resources/view/zakop/outbounds.js",
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "uci.sections('zakop', 'outbound_pool'") {
			t.Fatalf("%s does not include outbound pools", path)
		}
	}
}

func TestOutboundActionsDoNotCreateSyntheticPoolTagChanges(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/outbounds.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, forbidden := range []string{
		"function normalizePools()",
		"uci.set('zakop', sid, 'tag', sid);\n\n\t\tif (String(uci.get('zakop', sid, 'label')",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("outbounds.js must not create synthetic pool changes %q:\n%s", forbidden, s)
		}
	}
	for _, want := range []string{
		"uci.unload('zakop')",
		"return ui.changes.init()",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("outbounds.js missing post-commit refresh %q:\n%s", want, s)
		}
	}
}

func TestOutboundsLuCITableOnlySectionNameTypeAddressPortAndLatency(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/outbounds.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "s.sectiontitle = function(section_id)") {
		t.Fatalf("outbounds table should show name through section title:\n%s", s)
	}
	visible := []string{
		"form.ListValue, 'type'",
		"form.Value, 'server', _('Address')",
		"form.Value, 'port'",
		"form.DummyValue, '_latency', _('URLTest delay')",
	}
	for _, needle := range visible {
		start := strings.Index(s, needle)
		if start < 0 {
			t.Fatalf("outbounds.js missing visible field %q:\n%s", needle, s)
		}
		end := strings.Index(s[start+len(needle):], "\n\t\to = s.option(")
		block := s[start:]
		if end >= 0 {
			block = s[start : start+len(needle)+end]
		}
		if strings.Contains(block, "o.modalonly = true") {
			t.Fatalf("field %q should be visible in table:\n%s", needle, block)
		}
		if strings.Contains(block, "o.depends(") {
			t.Fatalf("field %q should not depend on the modal type control in the table:\n%s", needle, block)
		}
		if strings.Contains(block, "o.editable = true") {
			t.Fatalf("field %q should be read-only text in the table and editable through the modal:\n%s", needle, block)
		}
	}
	for _, needle := range []string{
		"form.Value, 'label'",
		"form.Value, 'uuid'",
		"form.ListValue, 'flow'",
		"form.Flag, 'tls'",
		"form.Value, 'server_name'",
		"form.Flag, 'reality'",
		"form.Value, 'reality_public_key'",
		"form.Value, 'password'",
		"form.ListValue, 'method'",
	} {
		start := strings.Index(s, needle)
		if start < 0 {
			t.Fatalf("outbounds.js missing detail field %q:\n%s", needle, s)
		}
		end := strings.Index(s[start+len(needle):], "\n\t\to = s.option(")
		block := s[start:]
		if end >= 0 {
			block = s[start : start+len(needle)+end]
		}
		if !strings.Contains(block, "o.modalonly = true") {
			t.Fatalf("field %q should be modal-only:\n%s", needle, block)
		}
	}
}

func TestOutboundsLuCIHomeProxyLikeControlsAndDependencies(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/outbounds.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"form.ListValue, 'flow'",
		"o.value('xtls-rprx-vision')",
		"form.ListValue, 'tls_min_version'",
		"form.ListValue, 'tls_max_version'",
		"form.DynamicList, 'tls_cipher_suites'",
		"form.Flag, 'insecure', _('Allow insecure')",
		"form.Flag, 'ech', _('Enable ECH')",
		"form.DynamicList, 'ech_config'",
		"form.Value, 'ech_config_path'",
		"form.ListValue, 'utls_fingerprint'",
		"o.value('chrome')",
		"form.Flag, 'reality'",
		"o.depends({ 'type': 'vless', 'tls': '1' })",
		"form.Value, 'reality_public_key'",
		"function dependsReality(option)",
		"option.depends({ 'type': 'vless', 'tls': '1', 'reality': '1' })",
		"form.ListValue, 'transport'",
		"o.value('grpc', _('gRPC'))",
		"o.value('ws', _('WebSocket'))",
		"form.ListValue, 'packet_encoding'",
		"form.ListValue, 'method', _('Encrypt method')",
		"o.value('2022-blake3-aes-128-gcm')",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("outbounds.js missing homeproxy-like control/dependency %q:\n%s", want, s)
		}
	}
}

func TestOutboundsLuCIMenuEntry(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/usr/share/luci/menu.d/luci-app-zakop.json")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"admin/services/zakop/outbounds"`) || !strings.Contains(s, `"path": "zakop/outbounds"`) {
		t.Fatalf("menu missing outbounds page:\n%s", s)
	}
}
