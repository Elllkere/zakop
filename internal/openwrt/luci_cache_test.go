package openwrt

import (
	"os"
	"strings"
	"testing"
)

func TestEmbeddedArchiveVersionsLuCIModuleURLs(t *testing.T) {
	packData, err := os.ReadFile("../../embedded/pack.sh")
	if err != nil {
		t.Fatal(err)
	}
	pack := string(packData)
	for _, want := range []string{
		"UI_CACHE_KEY=",
		"UI_NAMESPACE=\"zakop_${UI_CACHE_KEY}\"",
		"zakop-ui-cache.txt",
		"s/'require zakop\\./'require $UI_NAMESPACE./g",
		"$UI_NAMESPACE/#g",
	} {
		if !strings.Contains(pack, want) {
			t.Fatalf("pack.sh missing LuCI browser cache buster %q:\n%s", want, pack)
		}
	}

	installData, err := os.ReadFile("../../embedded/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	install := string(installData)
	for _, want := range []string{
		"/www/luci-static/resources/zakop_*",
		"/www/luci-static/resources/view/zakop_*",
		"content-versioned paths",
	} {
		if !strings.Contains(install, want) {
			t.Fatalf("install.sh missing versioned LuCI cleanup/install behavior %q:\n%s", want, install)
		}
	}
}
