package openwrt

import (
	"os"
	"strings"
	"testing"
)

func TestLuCIMenuOrderAndDebugPage(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/usr/share/luci/menu.d/luci-app-zakop.json")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		`"admin/services/zakop/general"`,
		`"title": "General"`,
		`"order": 10`,
		`"admin/services/zakop/rules"`,
		`"title": "Rules"`,
		`"order": 30`,
		`"admin/services/zakop/clients"`,
		`"title": "Clients"`,
		`"order": 40`,
		`"admin/services/zakop/outbounds"`,
		`"title": "Outbounds"`,
		`"order": 20`,
		`"admin/services/zakop/advanced"`,
		`"title": "Advanced"`,
		`"path": "zakop/advanced"`,
		`"admin/services/zakop/logs"`,
		`"title": "Logs"`,
		`"order": 80`,
		`"path": "zakop/logs"`,
		`"admin/services/zakop/debug"`,
		`"title": "Debug"`,
		`"order": 90`,
		`"path": "zakop/debug"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("menu missing %q:\n%s", want, s)
		}
	}
	for _, forbidden := range []string{
		`"admin/services/zakop/overview"`,
		`"title": "Overview"`,
		`"path": "zakop/overview"`,
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("overview menu entry should be replaced by Debug, found %q:\n%s", forbidden, s)
		}
	}

	if _, err := os.Stat("../../embedded/files/www/luci-static/resources/view/zakop/debug.js"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("../../embedded/files/www/luci-static/resources/view/zakop/logs.js"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("../../embedded/files/www/luci-static/resources/view/zakop/overview.js"); !os.IsNotExist(err) {
		t.Fatalf("overview.js should be removed, stat err=%v", err)
	}
}

func TestLuCIACLAllowsStatusAndVersionCommands(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/usr/share/rpcd/acl.d/luci-app-zakop.json")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		`"/bin/pidof": [ "exec" ]`,
		`"/etc/init.d/zakop": [ "exec" ]`,
		`"/usr/bin/zakopd": [ "exec" ]`,
		`"/usr/bin/sing-box": [ "exec" ]`,
		`"/usr/libexec/zakop/sing-box": [ "exec" ]`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("ACL missing %q:\n%s", want, s)
		}
	}
}

func TestLuCIHidesRulesTabOutsideCustomMode(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/zakop/ui.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"routing_mode",
		"rulesTabVisible()",
		"document.querySelectorAll('a[href]')",
		"/admin/services/zakop/rules",
		"/zakop/rules",
		"hideElement(tabContainer(links[i]), hidden)",
		"syncRulesTab: syncRulesTab",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("zakop/ui.js missing rules tab visibility behavior %q:\n%s", want, s)
		}
	}

	for _, path := range []string{
		"advanced.js",
		"clients.js",
		"debug.js",
		"general.js",
		"logs.js",
		"outbounds.js",
		"providers.js",
		"rules.js",
	} {
		view, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/" + path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(view), "'require zakop.ui as zakopUI'") || !strings.Contains(string(view), "zakopUI.syncRulesTab()") {
			t.Fatalf("%s must sync Rules tab visibility:\n%s", path, string(view))
		}
	}
}
