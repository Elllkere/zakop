package openwrt

import (
	"os"
	"strings"
	"testing"
)

func TestLuCIConfigPagesRestartZakopAfterCommit(t *testing.T) {
	helperData, err := os.ReadFile("../../embedded/files/www/luci-static/resources/zakop/ui.js")
	if err != nil {
		t.Fatal(err)
	}
	helper := string(helperData)
	for _, want := range []string{
		"applyAndRestart: applyAndRestart",
		"showApplyProgress();",
		"ui.showModal(_('Save & Apply')",
		"_('Applying configuration changes…')",
		"_('Configuration changes applied.')",
		"_('Failed to apply configuration changes.')",
		"showApplyError(err);",
		"return uci.apply()",
		"window.setTimeout(resolve, 2500)",
		"fs.exec('/etc/init.d/zakop', [ 'restart' ])",
		"return ui.changes.init();",
		"window.location.reload();",
	} {
		if !strings.Contains(helper, want) {
			t.Fatalf("zakop/ui.js missing %q:\n%s", want, helper)
		}
	}
	if strings.Contains(helper, "fs.exec('/sbin/uci', [ 'commit', 'zakop' ])") {
		t.Fatalf("Save & Apply must not commit outside the LuCI UCI session:\n%s", helper)
	}

	for _, path := range []string{
		"../../embedded/files/www/luci-static/resources/view/zakop/general.js",
		"../../embedded/files/www/luci-static/resources/view/zakop/advanced.js",
		"../../embedded/files/www/luci-static/resources/view/zakop/clients.js",
		"../../embedded/files/www/luci-static/resources/view/zakop/outbounds.js",
		"../../embedded/files/www/luci-static/resources/view/zakop/rules.js",
		"../../embedded/files/www/luci-static/resources/view/zakop/providers.js",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := string(data)
		for _, want := range []string{
			"handleSaveApply: function(ev)",
			"return zakopUI.applyAndRestart();",
		} {
			if !strings.Contains(s, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, s)
			}
		}
		for _, forbidden := range []string{
			"on_after_commit",
			"on_before_commit",
			"return zakopUI.commitAndRestart();",
		} {
			if strings.Contains(s, forbidden) {
				t.Fatalf("%s must not use unsupported hook %q:\n%s", path, forbidden, s)
			}
		}
	}
}

func TestLuCIACLAllowsZakopRestart(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/usr/share/rpcd/acl.d/luci-app-zakop.json")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"/etc/init.d/zakop": [ "exec" ]`) {
		t.Fatalf("ACL must allow zakop restart from LuCI:\n%s", s)
	}
	if strings.Count(s, `"/usr/share/zakop/check-version.sh": [ "exec" ]`) != 1 ||
		strings.Count(s, `"/usr/share/zakop/upgrade.sh": [ "exec" ]`) != 1 {
		t.Fatalf("ACL must separate read-only version checks from authenticated zakop upgrades:\n%s", s)
	}
}
