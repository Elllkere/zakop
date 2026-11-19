package openwrt

import (
	"os"
	"strings"
	"testing"
)

func TestClientsLuCIPolicyHelpIsSectionDescription(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/clients.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "form.GridSection, 'client', _('Clients'),") {
		t.Fatal("client policy help must be a section description")
	}
	for _, help := range []string{
		"Default — use the rules for this device first, then the shared settings.",
		"proxy — use the selected proxy for internet traffic. Local traffic goes directly.",
		"direct — use the internet directly, bypassing zakop.",
	} {
		if !strings.Contains(s, "_('"+help+"')") {
			t.Fatalf("missing client policy description %q", help)
		}
	}
	if strings.Contains(s, "form.ListValue, 'policy', _('Policy'),") {
		t.Fatalf("clients policy help must not render inside the table option:\n%s", s)
	}
}

func TestClientsLuCIProxyPolicyCanSelectOutbound(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/clients.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"function addOutboundChoices(option)",
		"uci.sections('zakop', 'outbound'",
		"tag == 'direct' || tag == 'blocked' || tag == 'block' || tag == 'proxy_default'",
		"form.ListValue, 'outbound', _('Outbound')",
		"o.depends('policy', 'proxy')",
		"function outboundTagExists(wanted)",
		"return outboundTagExists(value) ? value : firstOutbound",
		"o.forcewrite = true",
		"function rewriteClientState()",
		"this.map.save(rewriteClientState)",
		"uci.unset('zakop', sid, 'outbound')",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("clients.js missing proxy outbound UI behavior %q:\n%s", want, s)
		}
	}

	policyStart := strings.Index(s, "form.ListValue, 'policy', _('Policy')")
	outboundStart := strings.Index(s, "form.ListValue, 'outbound', _('Outbound')")
	if policyStart < 0 || outboundStart <= policyStart {
		t.Fatalf("clients.js policy/outbound option order is invalid:\n%s", s)
	}
	policyBlock := s[policyStart:outboundStart]
	if !strings.Contains(policyBlock, "o.rmempty = false;") || !strings.Contains(policyBlock, "o.editable = true;") {
		t.Fatalf("client policy must be an editable, persistent table selector:\n%s", policyBlock)
	}
}
