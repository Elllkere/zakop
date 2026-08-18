package openwrt

import (
	"os"
	"strings"
	"testing"
)

func TestOutboundsLuCIContainsImportAndSubscriptions(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/outbounds.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"showImportModal: function()",
		"var importButton, cancelButton;",
		"var status = E('span'",
		"importButton.disabled = true;",
		"cancelButton.disabled = true;",
		"textarea.disabled = true;",
		"importButton.textContent = _('Importing...')",
		"status.style.display = '';",
		"status.style.display = 'none';",
		"handleManualImport: function(value)",
		"fs.write(importPath",
		"fs.exec('/usr/bin/zakopd', [ 'import-uri', '-file', importPath ])",
		"el.appendChild(E('button'",
		"}, _('Import')))",
		"form.GridSection, 'subscription', _('Subscriptions')",
		"handleSubscriptionUpdate: function(section_id)",
		"handleSaveCommitConfig: function()",
		"return this.handleSaveCommitConfig()",
		"fs.exec('/sbin/uci', [ 'commit', 'zakop' ])",
		"throw new Error(res.stderr || res.stdout || _('Commit failed'))",
		"fs.exec('/usr/bin/zakopd', [ 'subscriptions', 'update', section_id ])",
		"handleSubscriptionUpdateAll: function()",
		"runSubscriptionUpdates: function(names)",
		"setSubscriptionUpdateAllButton: function(running, current, total)",
		"fs.exec('/usr/bin/zakopd', [ 'subscriptions', 'update', name ])",
		"failures.push({ name: name, error:",
		"showSubscriptionUpdateFailures: function(failures)",
		"Updating %d/%d…",
		"'data-zakop-subscriptions-update-all': '1'",
		"_('Updating all…')",
		"_('Update all')",
		"form.Value, 'url'",
		"form.Flag, 'auto_update'",
		"form.ListValue, 'update_schedule'",
		"form.ListValue, 'update_hour'",
		"form.ListValue, 'update_interval_minutes'",
		"addUpdateIntervalChoices(o)",
		"for (var hour = 0; hour < 24; hour++)",
		"o.value(String(hour), _('%d:00').format(hour))",
		"o.depends('auto_update', '1')",
		"o.depends({ 'auto_update': '1', 'update_schedule': 'time' })",
		"o.depends({ 'auto_update': '1', 'update_schedule': 'interval' })",
		"option.value('360', _('Every 6 hours'))",
		"form.ListValue, 'update_via'",
		"o.value('direct', 'direct')",
		"o.value('proxy', 'proxy')",
		"form.ListValue, 'update_outbound'",
		"o.depends('update_via', 'proxy')",
		"form.Button, '_update'",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("outbounds.js missing import/subscription UI %q:\n%s", want, s)
		}
	}
	for _, forbidden := range []string{
		"form.GridSection, 'outbound', _('Subscription nodes')",
		"uci.get('zakop', section_id, 'subscription') != null",
		"uci.get('zakop', section_id, 'subscription') == null",
		"uci.commit(",
		"fs.exec('/usr/bin/zakopd', [ 'subscriptions', 'update' ])",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("subscription nodes must remain editable in the regular Outbounds table, found %q:\n%s", forbidden, s)
		}
	}
	for _, want := range []string{
		"vless://",
		"hysteria2://",
		"ss://",
		"trojan://",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("import modal should mention %q:\n%s", want, s)
		}
	}
}

func TestOutboundsLuCIShowsSubscriptionUpdatedInTable(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/outbounds.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	needle := "sub.option(form.DummyValue, 'last_update'"
	start := strings.Index(s, needle)
	if start < 0 {
		t.Fatalf("outbounds.js missing subscription updated field %q:\n%s", needle, s)
	}
	end := strings.Index(s[start+len(needle):], "\n\t\to = sub.option(")
	block := s[start:]
	if end >= 0 {
		block = s[start : start+len(needle)+end]
	}
	if strings.Contains(block, "o.modalonly = true") {
		t.Fatalf("subscription updated field should remain visible in table:\n%s", block)
	}
}

func TestOutboundsLuCISubscriptionUpdateButtonIsModalOnly(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/outbounds.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	needle := "sub.option(form.Button, '_update'"
	start := strings.Index(s, needle)
	if start < 0 {
		t.Fatalf("outbounds.js missing subscription update button %q:\n%s", needle, s)
	}
	end := strings.Index(s[start+len(needle):], "\n\t\to = sub.option(")
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
			t.Fatalf("subscription update button missing %q:\n%s", want, block)
		}
	}
}

func TestNoSeparateImportsLuCIMenu(t *testing.T) {
	menu, err := os.ReadFile("../../embedded/files/usr/share/luci/menu.d/luci-app-zakop.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(menu), `"admin/services/zakop/imports"`) || strings.Contains(string(menu), `"path": "zakop/imports"`) {
		t.Fatalf("imports must be integrated into Outbounds, not a separate menu page:\n%s", menu)
	}

	acl, err := os.ReadFile("../../embedded/files/usr/share/rpcd/acl.d/luci-app-zakop.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"/tmp/zakop-import.txt": [ "read", "write" ]`,
		`"/sbin/uci": [ "exec" ]`,
		`"/usr/bin/zakopd": [ "exec" ]`,
		`"/etc/init.d/zakop": [ "exec" ]`,
	} {
		if !strings.Contains(string(acl), want) {
			t.Fatalf("ACL missing %q:\n%s", want, acl)
		}
	}
}
