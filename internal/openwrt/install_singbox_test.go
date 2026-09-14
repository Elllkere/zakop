package openwrt

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func installerFunctions(t *testing.T, names ...string) string {
	t.Helper()
	data, err := os.ReadFile("../../embedded/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	var functions strings.Builder
	for _, name := range names {
		start := strings.Index(string(data), name+"() {")
		if start < 0 {
			t.Fatalf("missing function %s", name)
		}
		end := strings.Index(string(data[start:]), "\n}\n\n")
		if end < 0 {
			t.Fatalf("unterminated function %s", name)
		}
		functions.Write(data[start : start+end+3])
	}
	return functions.String()
}

func TestInstallerSingBoxCompatibility(t *testing.T) {
	functions := installerFunctions(t, "log", "version_ge", "first_version", "write_singbox_check_config", "singbox_compatible")
	for _, fail := range []bool{false, true} {
		dir := t.TempDir()
		bin := filepath.Join(dir, "sing-box")
		fake := "#!/bin/sh\nif [ \"$1\" = version ]; then echo 'sing-box version 1.13.21'; exit 0; fi\n"
		if fail {
			fake += "echo 'unsupported configuration fixture' >&2\nexit 1\n"
		} else {
			fake += "exit 0\n"
		}
		if err := os.WriteFile(bin, []byte(fake), 0755); err != nil {
			t.Fatal(err)
		}
		script := functions + "\nMIN_SINGBOX_VERSION=1.12.0\nWORK_DIR=$1\nsingbox_compatible \"$2\"\n"
		cmd := exec.Command("sh", "-c", script, "check", dir, bin)
		output, err := cmd.CombinedOutput()
		if (err != nil) != fail {
			t.Fatalf("fail=%v: err=%v, output=%s", fail, err, output)
		}
		if fail && !strings.Contains(string(output), "unsupported configuration fixture") {
			t.Fatalf("check failure diagnostic hidden: %s", output)
		}
		path := filepath.Join(dir, "sing-box-check.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var cfg struct{ Outbounds []struct{ Type string } }
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatal(err)
		}
		for _, outbound := range cfg.Outbounds {
			if outbound.Type == "block" || outbound.Type == "dns" {
				t.Fatalf("installer uses removed outbound type %q", outbound.Type)
			}
		}
		if realBin := os.Getenv("ZAKOP_TEST_SINGBOX"); realBin != "" && !fail {
			cmd := exec.Command("sh", "-c", script, "check", dir, realBin)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("real sing-box compatibility check: %v\n%s", err, output)
			}
		}
	}
}

func TestInstallerRepairsSingBoxPath(t *testing.T) {
	functions := installerFunctions(t, "log", "set_selected_singbox_path")
	for _, tc := range []struct{ name, configured, selected, want string }{
		{"retry", "/usr/libexec/zakop/sing-box", "/usr/bin/sing-box", "/usr/bin/sing-box"},
		{"managed fallback", "/usr/bin/sing-box", "/usr/libexec/zakop/sing-box", "/usr/libexec/zakop/sing-box"},
		{"missing option", "", "/usr/bin/sing-box", "/usr/bin/sing-box"},
		{"custom", "/custom/working", "/usr/bin/sing-box", "/custom/working"},
		{"missing custom", "/custom/missing", "/usr/bin/sing-box", "/usr/bin/sing-box"},
		{"unchanged", "/usr/bin/sing-box", "/usr/bin/sing-box", "/usr/bin/sing-box"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := functions + `
set -eu
MANAGED_SINGBOX=/usr/libexec/zakop/sing-box
fixture_state=$1
uci() {
	case "$1" in
		-q) echo "$fixture_state" ;;
		set) fixture_state=${2#zakop.main.singbox_bin=} ;;
		commit) ;;
		*) return 1 ;;
	esac
}

singbox_compatible() { [ "$1" = /custom/working ]; }
set_selected_singbox_path "$2"
[ "$fixture_state" = "$3" ]
`
			output, err := exec.Command("sh", "-c", script, "test", tc.configured, tc.selected, tc.want).CombinedOutput()
			if err != nil {
				t.Fatalf("path selection: %v\n%s", err, output)
			}
		})
	}
}

func TestInstallerRuntimeReadiness(t *testing.T) {
	function := strings.ReplaceAll(installerFunctions(t, "zakop_runtime_ready"), "/usr/bin/zakopd status", "fixture_status")
	for _, tc := range []struct {
		name, tproxy, dns string
		ready             bool
	}{
		{"fresh install", "not_required", "present", true},
		{"proxy configured", "present", "present", true},
		{"proxy missing", "missing", "present", false},
		{"dns missing", "not_required", "missing", false},
		{"unknown proxy state", "", "present", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status := "nft_table: present\nip_rule: present\nlocal_route: present\ndns_listener: " + tc.dns + "\nfakeip_dns_listener: present\nreal_direct_dns_listener: present\nreal_proxy_dns_listener: present\ntproxy_listener: " + tc.tproxy
			script := function + "\nfixture_status() { printf '%s\\n' \"$fixture_output\"; }\nfixture_output=$1\nzakop_runtime_ready\n"
			output, err := exec.Command("sh", "-c", script, "test", status).CombinedOutput()
			if (err == nil) != tc.ready {
				t.Fatalf("ready=%v, err=%v\n%s", tc.ready, err, output)
			}
		})
	}
}
