package openwrt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestInitScriptStartsTwoProcdInstances(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/etc/init.d/neto")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Count(s, "procd_open_instance ") < 2 {
		t.Fatalf("expected at least two procd instances:\n%s", s)
	}
	if !strings.Contains(s, "procd_open_instance netod") {
		t.Fatalf("missing netod procd instance:\n%s", s)
	}
	if !strings.Contains(s, "procd_open_instance sing-box") {
		t.Fatalf("missing sing-box procd instance:\n%s", s)
	}
	if !strings.Contains(s, `procd_set_param command /usr/share/neto/run-sing-box-log.sh "$singbox_bin" /tmp/neto/sing-box.json`) {
		t.Fatalf("sing-box must run through neto log wrapper:\n%s", s)
	}
	if strings.Count(s, "procd_set_param term_timeout 3") != 2 {
		t.Fatalf("both procd instances must have a bounded shutdown timeout:\n%s", s)
	}
	if !strings.Contains(s, `"$singbox_bin" check -c /tmp/neto/sing-box.json`) {
		t.Fatalf("missing sing-box check before procd start:\n%s", s)
	}
	startService := strings.Index(s, "start_service()")
	lastInstance := strings.LastIndex(s, "procd_close_instance")
	startedHook := strings.Index(s, "service_started()")
	ready := strings.Index(s[startService:], `"$NETOD" ready -timeout 30s`) + startService
	apply := strings.Index(s[startService:], `"$NETOD" apply`) + startService
	dnsmasq := strings.Index(s[startService:], "neto_write_dnsmasq") + startService
	if !(lastInstance >= 0 && startedHook > lastInstance && ready > startedHook && apply > ready && dnsmasq > apply) {
		t.Fatalf("DNS services must become ready before nft and dnsmasq are enabled:\n%s", s)
	}
	start := strings.Index(s, "procd_open_instance sing-box")
	if start < 0 {
		t.Fatalf("missing sing-box procd block:\n%s", s)
	}
	end := strings.Index(s[start:], "procd_close_instance")
	if end < 0 {
		t.Fatalf("missing sing-box procd close:\n%s", s[start:])
	}
	block := s[start : start+end]
	if strings.Contains(block, "procd_set_param stdout 1") || strings.Contains(block, "procd_set_param stderr 1") {
		t.Fatalf("sing-box stdout/stderr must not be forwarded to system log:\n%s", block)
	}
}

func TestSingBoxWrapperRestartsAfterSustainedDNSFailure(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/usr/share/neto/run-sing-box-log.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		`health_failures_max="${NETO_DNS_HEALTH_FAILURES:-3}"`,
		`"$netod_bin" ready -timeout "${health_timeout}s"`,
		`real DNS health check failed $health_failures times; restarting sing-box`,
		`kill "$child_pid"`,
		`trap stop_child INT TERM`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("sing-box DNS watchdog is missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, `exec "$bin" run`) {
		t.Fatalf("wrapper cannot supervise an exec-replaced sing-box process:\n%s", s)
	}
}

func TestSingBoxWrapperDNSWatchdogRestartsChild(t *testing.T) {
	tempDir := t.TempDir()
	startsPath := filepath.Join(tempDir, "starts")
	stopsPath := filepath.Join(tempDir, "stops")
	logPath := filepath.Join(tempDir, "sing-box.log")
	singBoxPath := filepath.Join(tempDir, "sing-box")
	netodPath := filepath.Join(tempDir, "netod")

	singBoxScript := `#!/bin/sh
printf 'start\n' >> "$NETO_TEST_STARTS"
trap 'printf "stop\n" >> "$NETO_TEST_STOPS"; exit 0' TERM INT
while :; do
	sleep 1
done
`
	if err := os.WriteFile(singBoxPath, []byte(singBoxScript), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(netodPath, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}

	wrapper := "../../embedded/files/usr/share/neto/run-sing-box-log.sh"
	cmd := exec.Command("sh", wrapper, singBoxPath, filepath.Join(tempDir, "config.json"))
	cmd.Env = append(os.Environ(),
		"NETO_NETOD="+netodPath,
		"NETO_SINGBOX_LOG="+logPath,
		"NETO_DNS_HEALTH_GRACE_SECONDS=1",
		"NETO_DNS_HEALTH_INTERVAL_SECONDS=1",
		"NETO_DNS_HEALTH_FAILURES=2",
		"NETO_DNS_HEALTH_TIMEOUT_SECONDS=1",
		"NETO_TEST_STARTS="+startsPath,
		"NETO_TEST_STOPS="+stopsPath,
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	deadline := time.Now().Add(8 * time.Second)
	for {
		data, _ := os.ReadFile(startsPath)
		if strings.Count(string(data), "start\n") >= 2 {
			break
		}
		if time.Now().After(deadline) {
			logData, _ := os.ReadFile(logPath)
			t.Fatalf("watchdog did not restart sing-box; starts=%q log=%q", data, logData)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("wrapper did not stop cleanly: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("wrapper did not terminate its sing-box child")
	}

	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logData), "real DNS health check failed 2 times; restarting sing-box") {
		t.Fatalf("watchdog restart was not logged: %s", logData)
	}
	stopData, err := os.ReadFile(stopsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(stopData), "stop\n") < 2 {
		t.Fatalf("watchdog or wrapper left a sing-box child running: %q", stopData)
	}
}

func TestInitScriptReloadsAfterNetworkInterfaceChanges(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/etc/init.d/neto")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"neto_add_interface_trigger()",
		`[ "$section" = "loopback" ] && return 0`,
		`procd_add_reload_interface_trigger "$section"`,
		`procd_add_reload_trigger "$CONFIG_NAME" network`,
		"config_load network",
		"config_foreach neto_add_interface_trigger interface",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in init script:\n%s", want, s)
		}
	}
	if strings.Contains(s, "reload_service()") {
		t.Fatalf("reload must use rc.common's procd-aware start wrapper:\n%s", s)
	}
	if !strings.Contains(s, `if [ "$enabled" -ne 1 ]; then`) || !strings.Contains(s, "\t\tstop_service\n") {
		t.Fatalf("disabled reload must clean neto-owned runtime state:\n%s", s)
	}
}

func TestInitScriptManagesDNSMasqUCI(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/etc/init.d/neto")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		`DNSMASQ_STATE_DIR="/etc/neto/dnsmasq-state"`,
		`DNSMASQ_LEGACY_STATE_DIR="/var/lib/neto"`,
		"DNSMASQ_SERVER_STATE",
		"uci -q del_list dhcp.@dnsmasq[0].server=\"$server\"",
		"uci -q del_list dhcp.@dnsmasq[0].server=\"$saved_server\"",
		"uci -q del_list dhcp.@dnsmasq[0].server=\"127.0.0.1#5353\"",
		"uci add_list dhcp.@dnsmasq[0].server=\"$server\"",
		"uci set dhcp.@dnsmasq[0].noresolv='1'",
		"uci set dhcp.@dnsmasq[0].addsubnet='32'",
		"uci commit dhcp",
		"neto_dnsmasq_has_non_neto_server()",
		"current_noresolv=\"$(uci -q get dhcp.@dnsmasq[0].noresolv || true)\"",
		"uci -q delete dhcp.@dnsmasq[0].noresolv || true",
		"DNSMASQ_NORESOLV_STATE",
		"DNSMASQ_ADDSUBNET_STATE",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in init script:\n%s", want, s)
		}
	}
}

func TestInitScriptManagesSubscriptionCron(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/etc/init.d/neto")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		`CRON_FILE="/etc/crontabs/root"`,
		`CRON_BEGIN="# neto subscriptions begin"`,
		"config_get_bool auto_update \"$section\" auto_update 0",
		"neto_interval_minutes()",
		"neto_interval_cron_spec()",
		"config_get schedule \"$section\" update_schedule \"time\"",
		"config_get interval \"$section\" update_interval_minutes \"\"",
		"config_get interval \"$section\" update_interval \"360\"",
		"cron_spec=\"$(neto_interval_cron_spec \"$interval\"",
		"*/%s * * * *",
		"0 */6 * * *",
		"config_get hour \"$section\" update_hour \"0\"",
		"config_get minute \"$section\" update_minute \"0\"",
		"config_get minute \"$section\" update_minute \"5\"",
		"config_foreach neto_append_subscription_cron subscription",
		"config_foreach neto_append_provider_cron provider",
		"/usr/bin/netod subscriptions update %s",
		"/usr/bin/netod providers update %s",
		"/etc/init.d/neto restart",
		"neto_write_cron",
		"neto_remove_cron",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in init script:\n%s", want, s)
		}
	}

	start := strings.Index(s, "neto_append_provider_cron()")
	if start < 0 {
		t.Fatalf("could not find provider cron block:\n%s", s)
	}
	end := strings.Index(s[start:], "config_foreach neto_append_subscription_cron subscription")
	if end < 0 {
		t.Fatalf("could not find provider cron block end:\n%s", s[start:])
	}
	providerBlock := s[start : start+end]
	if strings.Contains(providerBlock, "config_get_bool enabled") || strings.Contains(providerBlock, "$enabled") {
		t.Fatalf("provider cron must depend only on auto_update, not provider enabled:\n%s", providerBlock)
	}
}
