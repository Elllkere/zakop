package openwrt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNetworkObserverBuildFlavor(t *testing.T) {
	initPath, err := filepath.Abs("../../embedded/files/etc/init.d/zakop")
	if err != nil {
		t.Fatal(err)
	}
	for _, flavor := range []string{"production", "network-debug"} {
		render := exec.Command("awk", "-v", "flavor="+flavor, `/# BEGIN NETWORKDEBUG/ { skip = (flavor == "production"); next } /# END NETWORKDEBUG/ { skip = 0; next } !skip { print }`, initPath)
		data, err := render.Output()
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "zakop")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		script := `
. "$1"
ZAKOPD=true
config_load() { :; }
config_get_bool() { eval "$1=1"; }
config_get() { eval "$1=true"; }
procd_open_instance() { echo "instance:$1"; }
procd_set_param() { :; }
procd_close_instance() { :; }
start_service
`
		cmd := exec.Command("sh", "-c", script, "test", path)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err, string(out))
		}
		if strings.Contains(string(out), "instance:network-debug") != (flavor == "network-debug") {
			t.Fatal(flavor, string(out))
		}
	}
}
