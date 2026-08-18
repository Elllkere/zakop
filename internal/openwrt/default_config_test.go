package openwrt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedDefaultConfigHasNoSampleClientsOrRules(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/etc/config/zakop")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, forbidden := range []string{
		"config client",
		"config rule",
		"gaming_pc",
		"all_proxy",
		"youtube",
		"list lan_subnet '192.168.8.0/24'",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("default config should not include test fixture %q:\n%s", forbidden, s)
		}
	}
	for _, want := range []string{
		"option fakeip_enabled '1'",
		"option real_dns_mode 'direct'",
		"option real_dns_transport 'udp'",
		"option real_dns_server '1.1.1.1:53'",
		"option real_dns_server_name 'cloudflare-dns.com'",
		"option real_dns_path '/dns-query'",
		"option singbox_dns_fakeip '127.0.0.1:15353'",
		"option singbox_dns_real_direct '127.0.0.1:15354'",
		"option singbox_dns_real_proxy '127.0.0.1:15355'",
		"option dns_upstream_preset 'cloudflare'",
		"option dns_upstream_protocol 'udp'",
		"option dns_upstream_host '1.1.1.1'",
		"option dns_upstream_tls_name 'cloudflare-dns.com'",
		"option dns_upstream_path '/dns-query'",
		"option language 'en'",
		"option language_ru_installed '0'",
		"option update_via 'direct'",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("default config missing %q:\n%s", want, s)
		}
	}
}

func TestInstallerDetectsLANSubnetAndConfiguresLanguage(t *testing.T) {
	data, err := os.ReadFile("../../embedded/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"--language en|ru",
		"install Russian LuCI localization",
		"LANGUAGE_CHOICE=\"ru\"",
		"uci set zakop.main.language='ru'",
		"uci set zakop.main.language_ru_installed='1'",
		"ip -4 route show dev br-lan scope link",
		"ipcalc.sh \"$ipaddr\" \"$netmask\"",
		"network_from_ip_prefix",
		"normalized=\"$(network_from_ip_prefix \"$ipaddr\" \"$prefix\"",
		"ensure_lan_subnet_config",
		"chmod 0755 /usr/share/zakop/run-sing-box-log.sh",
		"chmod 0755 /usr/share/zakop/check-version.sh",
		"curl_usable()",
		"curl -fsSL --connect-timeout 10 --max-time 300 \"$url\" -o \"$tmp\"",
		"wget -T 20 -t 2 -O \"$tmp\" \"$url\"",
		"attempts=\"$attempts broken-curl\"",
		"check_runtime_curl",
		"warning: /usr/bin/curl is installed but cannot start",
		"atomic_install()",
		"mv -f \"$tmp\" \"$dest\"",
		"atomic_install \"$WORK_DIR/bin/$arch/zakopd\" /usr/bin/zakopd",
		"existing installation detected; preserving config and skipping package installation",
		"verify_installed_version",
		"restart_luci_deferred",
		"fix_luci_permissions",
		"/www/luci-static/resources/view/zakop/*.js",
		"/usr/share/rpcd/acl.d/luci-app-zakop.json",
		"chmod 0644 \"$path\"",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("installer missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "zakop.$section.enabled") {
		t.Fatalf("installer must not write provider enabled fields:\n%s", s)
	}
	for _, forbidden := range []string{
		"ensure_builtin_providers",
		"ensure_builtin_provider",
		"ensure_builtin_script_provider",
		"provider_url_exists",
		"provider_script_exists",
		"uci set \"zakop.$section=provider\"",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("installer must not auto-create provider sections %q:\n%s", forbidden, s)
		}
	}
}

func TestInstallerMigratesLegacyNetoOnlyBeforeVerifiedCleanup(t *testing.T) {
	data, err := os.ReadFile("../../embedded/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"legacy_neto_detected()",
		"prepare_legacy_neto_migration()",
		"cleanup_legacy_neto_installation()",
		"/etc/rc.d/S*neto",
		"/etc/rc.d/K*neto",
		"/usr/libexec/neto",
		"/var/lib/neto",
		"/tmp/neto-import.txt",
		"/etc/init.d/neto stop",
		"cp /etc/config/neto /etc/config/zakop",
		"cp -R /etc/neto/. /etc/zakop/",
		"cp -R /usr/share/neto/. /usr/share/zakop/",
		"cp -R /var/lib/neto/providers/. /etc/zakop/provider-cache/",
		"s#/usr/libexec/neto/#/usr/libexec/zakop/#g",
		"s#/etc/neto/#/etc/zakop/#g",
		"s#/var/lib/neto/providers/#/etc/zakop/provider-cache/#g",
		"atomic_install \"$old_managed\" \"$MANAGED_SINGBOX\"",
		"/tmp/neto-install.*",
		"/tmp/neto-upgrade.*",
		"/tmp/neto-go-cache",
		"/etc/init.d/cron reload",
		"/etc/init.d/dnsmasq reload",
		"legacy neto files removed after successful zakop startup",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("installer legacy migration missing %q:\n%s", want, s)
		}
	}
	prepare := strings.Index(s, "prepare_legacy_neto_migration\n")
	restart := strings.LastIndex(s, "restart_zakop_safely\n")
	cleanup := strings.LastIndex(s, "cleanup_legacy_neto_installation\n")
	if prepare < 0 || restart < 0 || cleanup < 0 || prepare > restart || restart > cleanup {
		t.Fatalf("legacy cleanup must happen only after zakop startup verification:\n%s", s)
	}
	for _, want := range []string{
		`$0 ~ /\/usr\/bin\/netod([[:space:]]|$)/ { next }`,
		`$0 ~ /\/etc\/init\.d\/neto([[:space:]]|$)/ { next }`,
		`$0 ~ /\/usr\/share\/neto(\/|[[:space:]]|$)/ { next }`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("installer must remove standalone legacy cron entry %q:\n%s", want, s)
		}
	}
}

func TestVersionCheckWrapperCannotTriggerUpgrade(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/usr/share/zakop/check-version.sh")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat("../../embedded/files/usr/share/zakop/check-version.sh")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Fatal("check-version.sh is not executable")
	}
	s := string(data)
	if !strings.Contains(s, "exec /usr/share/zakop/upgrade.sh --check") {
		t.Fatalf("version wrapper must force check-only mode:\n%s", s)
	}
}

func TestUpgradeScriptFallsBackAroundBrokenCurl(t *testing.T) {
	data, err := os.ReadFile("../../embedded/upgrade.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"curl_usable()",
		"--check) MODE=\"check\"",
		"--luci) MODE=\"luci\"",
		"latest_version()",
		"zakop-version.txt",
		"RELEASE_API_URL",
		"status=\"available\"",
		"printf 'current=%s\\nlatest=%s\\nstatus=%s\\n'",
		"sh \"$TMP\"",
		"curl -fsSL --connect-timeout 10 --max-time 300 \"$url\" -o \"$tmp\"",
		"wget -T 20 -t 2 -O \"$tmp\" \"$url\"",
		"attempts=\"$attempts broken-curl\"",
		"download \"$INSTALL_URL\" \"$TMP\"",
		"ZAKOP_EXPECT_VERSION=\"$expected\" sh \"$TMP\"",
		"zakop upgrade: verified installed version $actual",
		"UPGRADE_LOG=\"${ZAKOP_UPGRADE_LOG:-/tmp/zakop/upgrade.log}\"",
		"UPDATE_VIA=\"${ZAKOP_UPDATE_VIA:-}\"",
		"UPDATE_OUTBOUND=\"${ZAKOP_UPDATE_OUTBOUND:-}\"",
		"\"$ZAKOPD_BIN\" download -url \"$url\" -output \"$dest\" -via proxy",
		"download \"$ARCHIVE_URL\" \"$ARCHIVE_TMP\"",
		"sh \"$TMP\" --local \"$ARCHIVE_TMP\"",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("upgrade script missing %q:\n%s", want, s)
		}
	}
}

func TestUpgradeScriptProxyDownloadsInstallerAndArchive(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "version-state")
	versionSource := filepath.Join(dir, "latest-version")
	installerSource := filepath.Join(dir, "installer.sh")
	archiveSource := filepath.Join(dir, "archive.tar.gz")
	zakopdPath := filepath.Join(dir, "zakopd")
	downloadArgsPath := filepath.Join(dir, "download.args")
	installerArgsPath := filepath.Join(dir, "installer.args")
	upgradeLogPath := filepath.Join(dir, "upgrade.log")

	for path, data := range map[string]string{
		statePath:     "v1.0.0\n",
		versionSource: "v1.0.1\n",
		archiveSource: "fake archive\n",
	} {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	installer := `#!/bin/sh
printf '%s\n' "$@" > "$ZAKOP_FAKE_INSTALLER_ARGS"
[ "$1" = "--local" ] || exit 20
[ -s "$2" ] || exit 21
printf 'v1.0.1\n' > "$ZAKOP_FAKE_VERSION_STATE"
`
	if err := os.WriteFile(installerSource, []byte(installer), 0755); err != nil {
		t.Fatal(err)
	}
	fakeZakopd := `#!/bin/sh
case "$1" in
version)
	printf 'zakopd %s\n' "$(cat "$ZAKOP_FAKE_VERSION_STATE")"
	;;
download)
	shift
	printf '%s\n' "$@" >> "$ZAKOP_FAKE_DOWNLOAD_ARGS"
	url=''
	output=''
	while [ "$#" -gt 0 ]; do
		case "$1" in
		-url) url="$2"; shift 2 ;;
		-output) output="$2"; shift 2 ;;
		*) shift ;;
		esac
	done
	case "$url" in
	*zakop-version.txt) cp "$ZAKOP_FAKE_VERSION_SOURCE" "$output" ;;
	*install.sh) cp "$ZAKOP_FAKE_INSTALLER_SOURCE" "$output" ;;
	*archive.tar.gz) cp "$ZAKOP_FAKE_ARCHIVE_SOURCE" "$output" ;;
	*) exit 22 ;;
	esac
	;;
*) exit 23 ;;
esac
`
	if err := os.WriteFile(zakopdPath, []byte(fakeZakopd), 0755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sh", "../../embedded/upgrade.sh")
	cmd.Env = append(os.Environ(),
		"TMPDIR="+dir,
		"ZAKOP_ZAKOPD_BIN="+zakopdPath,
		"ZAKOP_INSTALL_URL=https://example.test/install.sh",
		"ZAKOP_VERSION_URL=https://example.test/zakop-version.txt",
		"ZAKOP_RELEASE_API_URL=https://example.test/release-api",
		"ZAKOP_ARCHIVE_URL=https://example.test/archive.tar.gz",
		"ZAKOP_UPDATE_VIA=proxy",
		"ZAKOP_UPDATE_OUTBOUND=proxy1",
		"ZAKOP_UPGRADE_LOG="+upgradeLogPath,
		"ZAKOP_FAKE_VERSION_STATE="+statePath,
		"ZAKOP_FAKE_VERSION_SOURCE="+versionSource,
		"ZAKOP_FAKE_INSTALLER_SOURCE="+installerSource,
		"ZAKOP_FAKE_ARCHIVE_SOURCE="+archiveSource,
		"ZAKOP_FAKE_DOWNLOAD_ARGS="+downloadArgsPath,
		"ZAKOP_FAKE_INSTALLER_ARGS="+installerArgsPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("proxy upgrade failed: %v\n%s", err, out)
	}
	downloadArgs, err := os.ReadFile(downloadArgsPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(downloadArgs)
	if strings.Count(args, "-via\nproxy\n") != 3 || strings.Count(args, "-outbound\nproxy1\n") != 3 {
		t.Fatalf("version, installer, and archive must use selected proxy outbound:\n%s", args)
	}
	installerArgs, err := os.ReadFile(installerArgsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(installerArgs), "--local\n") {
		t.Fatalf("proxy upgrade must pass the predownloaded archive to installer:\n%s", installerArgs)
	}
	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(state)) != "v1.0.1" {
		t.Fatalf("unexpected installed version state: %s", state)
	}
}

func TestInstallerRefreshesLuCIAfterUpdate(t *testing.T) {
	data, err := os.ReadFile("../../embedded/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"--no-ui-restart)",
		"clear_luci_cache()",
		"rm -f /tmp/luci-indexcache /var/run/luci-indexcache",
		"rm -rf /tmp/luci-modulecache",
		"/etc/init.d/rpcd restart",
		"/etc/init.d/uhttpd restart",
		"sleep 2",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("installer missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "RESTART_UI=0") {
		t.Fatalf("installer must not skip LuCI restart after replacing views and ACLs:\n%s", s)
	}
	uhttpdRestart := strings.Index(s, "/etc/init.d/uhttpd restart")
	rpcdRestart := strings.Index(s, "/etc/init.d/rpcd restart")
	if uhttpdRestart < 0 || rpcdRestart < 0 || uhttpdRestart > rpcdRestart {
		t.Fatalf("installer must restart uhttpd before rpcd because rpcd may terminate its file.exec child:\n%s", s)
	}
}

func TestInstallerWaitsForCleanServiceRestartAfterUpdate(t *testing.T) {
	data, err := os.ReadFile("../../embedded/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"zakop_service_pids()",
		`ubus call service list '{"name":"zakop"}'`,
		"restart_zakop_safely()",
		`/etc/init.d/zakop stop`,
		`kill -0 "$pid"`,
		`/etc/init.d/zakop start`,
		`/etc/init.d/zakop running`,
		"zakop_runtime_ready()",
		`"dns_listener: present"`,
		`"tproxy_listener: present"`,
		"networking was returned to direct mode",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("installer safe restart missing %q:\n%s", want, s)
		}
	}
	if strings.Count(s, `/etc/init.d/zakop stop >/dev/null 2>&1 || true`) < 3 {
		t.Fatalf("installer must return networking to direct mode on every restart failure path:\n%s", s)
	}
	if strings.Contains(s, "/etc/init.d/zakop restart\n") {
		t.Fatalf("installer must not race old and new procd instances with a single restart action:\n%s", s)
	}
	updateRestart := `restart_zakop_safely
if [ "$EXISTING_INSTALL" -eq 1 ]; then
	# The first start replaces the procd instance definitions`
	if !strings.Contains(s, updateRestart) ||
		!strings.Contains(s, `log "performing final clean service restart after update"`) {
		t.Fatalf("existing installations must get a final clean restart after registering new procd instances:\n%s", s)
	}
}

func TestEmbeddedSingBoxLogWrapperIsInstalledAsset(t *testing.T) {
	path := "../../embedded/files/usr/share/zakop/run-sing-box-log.sh"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Fatalf("sing-box log wrapper is not executable: %s", path)
	}
	s := string(data)
	for _, want := range []string{
		"#!/bin/sh",
		"/tmp/zakop/sing-box.log",
		"tail -c \"$log_keep_bytes\"",
		"\"$bin\" run -c \"$config\" >> \"$log_file\" 2>&1 &",
		`"$zakopd_bin" ready`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("%s missing %q:\n%s", path, want, s)
		}
	}
}

func TestEmbeddedProviderScriptsAreInstalledAssets(t *testing.T) {
	for _, path := range []string{
		"../../embedded/files/usr/share/zakop/providers/akamai-ipv4.sh",
		"../../embedded/files/usr/share/zakop/providers/aws-ipv4.sh",
		"../../embedded/files/usr/share/zakop/providers/aws-full-ipv4.sh",
		"../../embedded/files/usr/share/zakop/providers/aws-full-eu-ipv4.sh",
		"../../embedded/files/usr/share/zakop/providers/google-cloud-eu-ipv4.sh",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0111 == 0 {
			t.Fatalf("provider script is not executable: %s", path)
		}
		s := string(data)
		for _, want := range []string{
			"#!/bin/sh",
			"ZAKOP_PROVIDER_OUTPUT",
			"curl -fsSL",
			"command -v jq",
		} {
			if !strings.Contains(s, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, s)
			}
		}
		if strings.Contains(s, "select(test(") {
			t.Fatalf("%s must not use jq regex functions; OpenWrt jq may be built without ONIGURUMA:\n%s", path, s)
		}
	}
}

func TestEmbeddedGoogleCloudProviderScriptFiltersEuropeanIPv4Ranges(t *testing.T) {
	data, err := os.ReadFile("../../embedded/files/usr/share/zakop/providers/google-cloud-eu-ipv4.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		"https://www.gstatic.com/ipranges/cloud.json",
		"startswith(\"europe-\")",
		"/^europe-/",
		".ipv4Prefix // empty",
		"Google Cloud IPv4 ranges for Europe",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("Google Cloud Europe provider missing %q:\n%s", want, s)
		}
	}
}

func TestEmbeddedAWSProviderScriptsAreSplitByService(t *testing.T) {
	cdnData, err := os.ReadFile("../../embedded/files/usr/share/zakop/providers/aws-ipv4.sh")
	if err != nil {
		t.Fatal(err)
	}
	fullData, err := os.ReadFile("../../embedded/files/usr/share/zakop/providers/aws-full-ipv4.sh")
	if err != nil {
		t.Fatal(err)
	}
	cdn := string(cdnData)
	full := string(fullData)

	for _, want := range []string{"CLOUDFRONT", "S3", "AWS CDN IPv4"} {
		if !strings.Contains(cdn, want) {
			t.Fatalf("AWS CDN provider missing %q:\n%s", want, cdn)
		}
	}
	for _, want := range []string{"AMAZON", "EC2", "GLOBALACCELERATOR", "AWS Full IPv4", "may affect ping to games hosted on Amazon/AWS servers"} {
		if !strings.Contains(full, want) {
			t.Fatalf("AWS Full provider missing %q:\n%s", want, full)
		}
	}
}
