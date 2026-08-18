package openwrt

import (
	"os"
	"strings"
	"testing"
)

func TestLuCII18nModuleUsesBaseclassConstructor(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/zakop/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"'require baseclass'",
		"return baseclass.extend({",
		"translate: translate",
		"ruAvailable: ruAvailable",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("i18n.js missing %q:\n%s", want, s)
		}
	}
}
