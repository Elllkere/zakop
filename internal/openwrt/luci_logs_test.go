package openwrt

import (
	"os"
	"strings"
	"testing"
)

func TestLuCILogsPageUsesZakopdLogCommand(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/www/luci-static/resources/view/zakop/logs.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"'require fs'",
		"'require ui'",
		"fs.exec('/usr/bin/zakopd', args)",
		"zakopd([ 'logs', 'sing-box' ])",
		"fs.exec('/usr/bin/zakopd', [ 'logs', 'sing-box', 'clear' ])",
		"handleRefresh: function()",
		"handleClear: function(button)",
		"sing-box Logs",
		"Refresh",
		"Clear",
		"No logs yet",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("logs.js missing %q:\n%s", want, s)
		}
	}
}
