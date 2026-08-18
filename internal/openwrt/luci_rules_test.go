package openwrt

import (
	"os"
	"strings"
	"testing"
)

func TestRulesLuCINoMatchAllOrLegacyMatchers(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/rules.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, forbidden := range []string{
		"match_all",
		"domain_exact",
		"domain_keyword",
		"domain_prefix",
		"domain_suffix",
		"exclude_domain_exact",
		"exclude_domain_keyword",
		"exclude_domain_prefix",
		"exclude_domain_suffix",
		"Keyword",
		"Prefix",
		"Suffix",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("rules.js must not contain %q:\n%s", forbidden, s)
		}
	}
}

func TestRulesLuCIExplicitEnabledAndPriorityRewrite(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/rules.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"form.Flag, 'enabled'",
		"o.enabled = '1'",
		"o.disabled = '0'",
		"o.rmempty = false",
		"uci.set('zakop', sid, 'enabled', '1')",
		"uci.set('zakop', sid, 'dns_mode', 'auto')",
		"uci.set('zakop', sid, 'priority', String(n * 100))",
		"this.map.save(rewriteRuleState)",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("rules.js missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "on_before_save") {
		t.Fatalf("rules.js must not use unsupported on_before_save hook:\n%s", s)
	}
}

func TestRulesLuCISortsRenderedRulesByPriority(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/rules.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"function rulePriority(section_id, fallback)",
		"function sortRuleSectionIDs(ids)",
		"return pa - pb",
		"1000 + order[a]",
		"s.cfgsections = function()",
		"sortRuleSectionIDs(form.GridSection.prototype.cfgsections.apply(this, arguments))",
		"function renderedRuleSectionIDs(ids)",
		"document.querySelectorAll('#cbi-zakop-rule tr.cbi-section-table-row[data-sid]')",
		"function orderedRuleSectionIDs()",
		"var ids = orderedRuleSectionIDs()",
		"function nextRulePriority()",
		"var priority = rulePriority(ids[i], 1000 + i)",
		"return max + 100",
		"function initializeNewRuleSection(section_id, priority, outbound)",
		"s.handleAdd = function(ev, name)",
		"var priority = nextRulePriority()",
		"data.add = function(configName, sectionType, sectionName)",
		"var sid = add.apply(this, arguments)",
		"initializeNewRuleSection(sid, priority, firstOutbound)",
		"Create an outbound before adding a proxy rule.",
		"return form.GridSection.prototype.handleAdd.apply(this, arguments)",
		"data.add = add",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("rules.js missing priority-backed table ordering behavior %q:\n%s", want, s)
		}
	}
	handleAddStart := strings.Index(s, "s.handleAdd = function(ev, name)")
	if handleAddStart < 0 {
		t.Fatalf("could not find add handler block:\n%s", s)
	}
	handleAddEnd := strings.Index(s[handleAddStart:], "s.modaltitle = _('Rule details')")
	if handleAddEnd < 0 {
		t.Fatalf("could not find add handler block:\n%s", s)
	}
	handleAddBlock := s[handleAddStart : handleAddStart+handleAddEnd]
	if strings.Contains(handleAddBlock, "this.map.save(null, true)") {
		t.Fatalf("rule add handler must preserve GridSection modal flow, not save immediately:\n%s", handleAddBlock)
	}
}

func TestRulesLuCIHiddenOutsideCustomMode(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/rules.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"if (String(uci.get('zakop', 'main', 'routing_mode') || 'custom').trim() != 'custom')",
		"routingMode = String(uci.get('zakop', 'main', 'routing_mode') || 'custom').trim()",
		"if (routingMode != 'custom')",
		"return m.render()",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("rules.js missing custom-mode visibility guard %q:\n%s", want, s)
		}
	}
}

func TestRulesLuCIImportExportForcesDirectAction(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/rules.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"function exportRulesJSON()",
		"function parseImportedRules(text)",
		"showExportRules: function()",
		"showImportRules: function()",
		"handleImportRules: function(text)",
		"s.renderSectionAdd = function()",
		"function downloadTextFile(filename, text)",
		"new Blob([ text ], { type: 'application/json' })",
		"'download': filename",
		"downloadTextFile('zakop-rules.json', exportRulesJSON())",
		"function pickTextFile(accept)",
		"'type': 'file'",
		"'accept': accept",
		"new FileReader()",
		"reader.readAsText(file)",
		"pickTextFile('.json,application/json')",
		"action: 'direct'",
		"outbound: 'direct'",
		"dns_mode: 'auto'",
		"function isProviderRuleOption(option)",
		"option == 'domain_provider' || option == 'ip_provider' || option == 'provider'",
		"if (domainInput != '' && domainInput != 'provider')",
		"if (ipInput != '' && ipInput != 'provider')",
		"if (isProviderRuleOption(option))",
		"if (domainInput == 'provider')",
		"if (ipInput == 'provider')",
		"uci.set('zakop', section_id, 'action', 'direct')",
		"uci.set('zakop', section_id, 'outbound', 'direct')",
		"uci.set('zakop', section_id, 'dns_mode', 'auto')",
		"uci.remove('zakop', sections[i])",
		"return fs.exec('/sbin/uci', [ 'commit', 'zakop' ])",
		"fs.exec('/etc/init.d/zakop', [ 'restart' ])",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("rules.js missing import/export safety behavior %q:\n%s", want, s)
		}
	}
	for _, forbidden := range []string{
		"ui.showModal(_('Export rules')",
		"ui.showModal(_('Import rules')",
		"'class': 'cbi-input-textarea'",
		"textarea.focus()",
		"textarea.select()",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("rules.js must use file import/export, not textbox modal %q:\n%s", forbidden, s)
		}
	}
}

func TestRulesLuCIDNSModeHiddenAndAutomatic(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/rules.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Contains(s, "s.option(form.ListValue, 'dns_mode'") || strings.Contains(s, "DNS mode") {
		t.Fatalf("rules.js must not expose dns_mode in LuCI:\n%s", s)
	}
	for _, want := range []string{
		"uci.set('zakop', sid, 'dns_mode', 'auto')",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("rules.js missing automatic dns_mode behavior %q:\n%s", want, s)
		}
	}
}

func TestRulesLuCICompactTableFields(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/rules.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, field := range []string{
		"domain_equals",
		"domain_contains",
		"domain_starts_with",
		"domain_ends_with",
		"exclude_domain_equals",
		"exclude_domain_contains",
		"exclude_domain_starts_with",
		"exclude_domain_ends_with",
		"ip_cidr",
		"domain_provider",
		"ip_provider",
		"domain_file",
		"ip_file",
		"src_port",
		"dst_port",
	} {
		needle := "'" + field + "'"
		if !strings.Contains(s, needle) {
			t.Fatalf("rules.js missing detail field %q:\n%s", field, s)
		}
	}
	for _, want := range []string{
		"form.ListValue, 'domain_input'",
		"form.ListValue, 'ip_input'",
		"o.value('file', _('File paths'))",
		"form.TextValue, option",
		"addTextList(s, '_domain_equals_text'",
		"addTextList(s, '_ip_cidr_text'",
		"addProviderList(s, 'domain_provider'",
		"addProviderList(s, 'ip_provider'",
		"addDynamicList(s, 'domain_file', _('Domain file paths'), 'domain_input', 'file'",
		"addDynamicList(s, 'ip_file', _('IP/CIDR file paths'), 'ip_input', 'file'",
		"form.DummyValue, '_packet_match'",
		"form.ListValue, '_packet_proto'",
		"o.value('any', _('Any'))",
		"o.value('tcp', _('TCP'))",
		"o.value('udp', _('UDP'))",
		"uci.unset('zakop', section_id, 'proto')",
		"form.DynamicList, 'src_port'",
		"form.DynamicList, 'dst_port'",
		"o.validate = validatePortMatch",
		"function validatePortMatch(section_id, value)",
		"Port must be a number or range, for example 443 or 1000-2000",
		"Port must be between 1 and 65535",
		"Port range start must be less than or equal to range end",
		"Source ports are client-side ports chosen by the LAN device. Usually leave empty. Syntax: 443 or 1000-2000.",
		"Destination ports are service ports on the remote IP, for example 443 for HTTPS or 53 for DNS. Syntax: 443 or 1000-2000.",
		"Port matching is packet-level. It applies only to provider/CIDR/IP matches, not to DNS/FakeIP domain matching.",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("rules.js missing input mode UI %q:\n%s", want, s)
		}
	}
	if strings.Count(s, "o.modalonly = true") < 8 {
		t.Fatalf("matcher/provider fields should be modal-only:\n%s", s)
	}
	if strings.Contains(s, "form.Value, 'priority'") {
		t.Fatalf("priority must not be user-editable:\n%s", s)
	}
	if strings.Contains(s, "form.GridSection, 'provider'") {
		t.Fatalf("rules.js must not create provider sections:\n%s", s)
	}
	for _, forbidden := range []string{
		"tcp_udp",
		"TCP+UDP",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("rules.js must not expose redundant protocol option %q:\n%s", forbidden, s)
		}
	}
}

func TestRulesLuCIOutboundVisibleInTable(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/rules.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	start := strings.Index(s, "form.ListValue, 'outbound'")
	end := strings.Index(s, "form.ListValue, 'domain_input'")
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("could not find outbound option block:\n%s", s)
	}
	block := s[start:end]
	if strings.Contains(block, "modalonly") {
		t.Fatalf("outbound must remain visible in rules table:\n%s", block)
	}
	if !strings.Contains(block, "o.editable = true") {
		t.Fatalf("outbound should be editable in rules table:\n%s", block)
	}
	if !strings.Contains(block, "addOutboundChoices(o)") || !strings.Contains(block, "o.depends('action', 'proxy')") || !strings.Contains(block, "o.rmempty = false") {
		t.Fatalf("outbound should use dynamic custom choices only for proxy action:\n%s", block)
	}
	for _, want := range []string{
		"o.forcewrite = true",
		"return outboundTagExists(value) ? value : firstOutbound",
		"if (!outboundTagExists(value))",
		"return _('Create an outbound before adding a proxy rule.')",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("rule outbound must be populated and validated before saving; missing %q:\n%s", want, block)
		}
	}
	for _, forbidden := range []string{
		"option.value('direct'",
		"option.value('blocked'",
		"option.value('', _('Auto'))",
		"option.value('', _('Select outbound'))",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("rules.js must not expose builtin outbound choice %q:\n%s", forbidden, s)
		}
	}
	if strings.Contains(block, "proxy_default") {
		t.Fatalf("rule outbound dropdown must not add proxy_default:\n%s", block)
	}
	if strings.Contains(s, "section.enabled") {
		t.Fatalf("rules.js must not filter custom outbounds by removed enabled option:\n%s", s)
	}
	for _, want := range []string{
		"firstOutbound = firstOutboundTag()",
		"var tag = String(section.tag || sid || section['.name'] || '').trim()",
		"var label = String(section.label || section.name || tag).trim()",
		"option.value(tag, label || tag)",
		"option.default = first",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("rules.js should support pending custom outbounds; missing %q:\n%s", want, s)
		}
	}
}

func TestRulesLuCIInputModesNormalizePersistence(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/rules.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"uci.set('zakop', sid, 'domain_input', domainInput)",
		"uci.set('zakop', sid, 'ip_input', ipInput)",
		"uci.unset('zakop', sid, 'domain_provider')",
		"uci.unset('zakop', sid, 'ip_provider')",
		"uci.unset('zakop', sid, 'domain_file')",
		"uci.unset('zakop', sid, 'ip_file')",
		"else if (optionValues(sid, 'domain_file').length > 0)",
		"else if (optionValues(sid, 'ip_file').length > 0 || optionValues(sid, 'file').length > 0)",
		"else if (domainInput == 'file')",
		"else if (ipInput == 'file')",
		"addProviderChoices(o, providerType)",
		"setListOption(section_id, target, splitTextValues(formvalue))",
		"uci.set('zakop', section_id, option, values)",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("rules.js missing persistence behavior %q:\n%s", want, s)
		}
	}
}
