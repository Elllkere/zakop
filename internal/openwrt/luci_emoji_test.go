package openwrt

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestLuCICountryFlagEmojiPolyfillIsLocal(t *testing.T) {
	uiData, err := os.ReadFile("../../embedded/files/www/luci-static/resources/zakop/ui.js")
	if err != nil {
		t.Fatal(err)
	}
	ui := string(uiData)
	for _, want := range []string{
		"function enableCountryFlagEmoji()",
		"supportsColorEmoji('🇨🇭')",
		"/luci-static/resources/zakop-assets/TwemojiCountryFlags-0.1.8.woff2",
		"unicode-range:U+1F1E6-1F1FF",
		"document.documentElement.classList.add('zakop-country-flags')",
		"enableCountryFlagEmoji();",
	} {
		if !strings.Contains(ui, want) {
			t.Fatalf("zakop/ui.js missing country flag support %q:\n%s", want, ui)
		}
	}
	if strings.Contains(ui, "cdn.jsdelivr.net") || strings.Contains(ui, "cdn.skypack.dev") {
		t.Fatalf("country flag font must not be fetched from a CDN:\n%s", ui)
	}

	font, err := os.ReadFile("../../embedded/files/www/luci-static/resources/zakop-assets/TwemojiCountryFlags-0.1.8.woff2")
	if err != nil {
		t.Fatal(err)
	}
	if len(font) < 4 || string(font[:4]) != "wOF2" {
		t.Fatalf("country flag asset is not a WOFF2 font")
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(font)); got != "9f04f14429bb6a9f415c7a4dd902a918d7e81a4f7526c415496fdb063954e3b8" {
		t.Fatalf("unexpected country flag font checksum %s", got)
	}

	license, err := os.ReadFile("../../embedded/files/www/luci-static/resources/zakop-assets/LICENSE.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(license), "MIT") || !strings.Contains(string(license), "CC BY 4.0") {
		t.Fatalf("country flag font attribution is incomplete:\n%s", license)
	}

	for _, path := range []string{"../../embedded/install.sh", "../../embedded/uninstall.sh"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "/www/luci-static/resources/zakop-assets") {
			t.Fatalf("%s does not manage the country flag assets directory", path)
		}
	}
}
