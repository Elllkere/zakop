package status

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/elllkere/zakop/internal/config"
	"github.com/elllkere/zakop/internal/tproxy"
)

func Summary(cfg config.Config) string {
	realDNSOutbound := strings.TrimSpace(cfg.Main.RealDNSOutbound)
	if realDNSOutbound == "" {
		realDNSOutbound = "-"
	}
	lines := []string{
		fmt.Sprintf("enabled: %t", cfg.Main.Enabled),
		fmt.Sprintf("singbox_bin: %s", cfg.Main.SingBoxBin),
		fmt.Sprintf("real_dns_mode: %s", cfg.Main.RealDNSMode),
		fmt.Sprintf("real_dns_outbound: %s", realDNSOutbound),
		fmt.Sprintf("real_dns_transport: %s", cfg.Main.DNSUpstream().Protocol),
		fmt.Sprintf("real_dns_server: %s", cfg.Main.DNSUpstream().Address()),
		fmt.Sprintf("singbox_dns_fakeip: %s", cfg.Main.SingBoxDNSFakeIPAddr()),
		fmt.Sprintf("singbox_dns_real_direct: %s", cfg.Main.SingBoxDNSRealDirectAddr()),
		fmt.Sprintf("singbox_dns_real_proxy: %s", cfg.Main.SingBoxDNSRealProxyAddr()),
		fmt.Sprintf("tproxy_port: %d", cfg.Main.TProxyPort),
		fmt.Sprintf("mark: %s", cfg.Main.Mark),
		fmt.Sprintf("table: %d", cfg.Main.Table),
		fmt.Sprintf("routing_mode: %s", cfg.Main.RoutingMode),
		fmt.Sprintf("default_outbound: %s", cfg.Main.DefaultOutbound),
		fmt.Sprintf("outbounds: %s", OutboundsSummary(cfg)),
		fmt.Sprintf("outbound_pools: %s", OutboundPoolsSummary(cfg)),
		fmt.Sprintf("lan_subnets4: %s", listOrDash(cfg.Main.LANSubnets)),
		fmt.Sprintf("lan_ifaces: %s", listOrDash(cfg.Main.LANIfaces)),
		fmt.Sprintf("nft_table: %s", nftTableStatus()),
		fmt.Sprintf("ip_rule: %s", ipRuleStatus(cfg)),
		fmt.Sprintf("local_route: %s", localRouteStatus(cfg)),
		fmt.Sprintf("dns_listener: %s", listenerStatus(cfg.Main.DNSListen)),
		fmt.Sprintf("fakeip_dns_listener: %s", listenerStatus(cfg.Main.SingBoxDNSFakeIPAddr())),
		fmt.Sprintf("real_direct_dns_listener: %s", listenerStatus(cfg.Main.SingBoxDNSRealDirectAddr())),
		fmt.Sprintf("real_proxy_dns_listener: %s", listenerStatus(cfg.Main.SingBoxDNSRealProxyAddr())),
		fmt.Sprintf("tproxy_listener: %s", listenerStatus("127.0.0.1:"+strconv.Itoa(cfg.Main.TProxyPort))),
	}
	if len(cfg.OutboundPools) > 0 {
		lines = append(lines, fmt.Sprintf("pool_controller: %s", listenerStatus(cfg.Main.PoolController)))
	}
	return strings.Join(lines, "\n")
}

func OutboundsSummary(cfg config.Config) string {
	parts := []string{
		config.BuiltinDirectOutbound + "(builtin)",
		config.BuiltinBlockedOutbound + "(builtin)",
	}
	for _, outbound := range cfg.EnabledCustomOutbounds() {
		parts = append(parts, OutboundSummary(outbound))
	}
	return strings.Join(parts, ", ")
}

func OutboundSummary(outbound config.Outbound) string {
	parts := []string{
		fmt.Sprintf("%s(%s)", outbound.Tag, outbound.Type),
	}
	if outbound.Label != "" && outbound.Label != outbound.Tag {
		parts = append(parts, fmt.Sprintf("label=%s", outbound.Label))
	}
	if outbound.Server != "" && outbound.Port > 0 {
		parts = append(parts, fmt.Sprintf("server=%s:%d", outbound.Server, outbound.Port))
	}
	if outbound.Method != "" {
		parts = append(parts, fmt.Sprintf("method=%s", outbound.Method))
	}
	if outbound.TLS || outbound.Reality || outbound.Type == "hysteria2" {
		parts = append(parts, fmt.Sprintf("tls=%t", outbound.TLS || outbound.Reality || outbound.Type == "hysteria2"))
	}
	if outbound.Reality {
		parts = append(parts, "reality=true")
	}
	if outbound.Transport != "" {
		parts = append(parts, fmt.Sprintf("transport=%s", outbound.Transport))
	}
	return strings.Join(parts, " ")
}

func OutboundPoolsSummary(cfg config.Config) string {
	if len(cfg.OutboundPools) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(cfg.OutboundPools))
	for _, pool := range cfg.OutboundPools {
		parts = append(parts, fmt.Sprintf("%s[%s]", pool.Tag, strings.Join(pool.Outbounds, ">")))
	}
	return strings.Join(parts, ", ")
}

func listOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ",")
}

func nftTableStatus() string {
	err := exec.Command("nft", "list", "table", "inet", "zakop").Run()
	if err != nil {
		return "missing"
	}
	return "present"
}

func ipRuleStatus(cfg config.Config) string {
	out, err := exec.Command("ip", "-4", "rule", "show").Output()
	if err != nil {
		return "unknown"
	}
	if tproxy.RulePresent(string(out), tproxy.Config{Mark: cfg.Main.Mark, Table: cfg.Main.Table}) {
		return "present"
	}
	return "missing"
}

func localRouteStatus(cfg config.Config) string {
	out, err := exec.Command("ip", "-4", "route", "show", "table", strconv.Itoa(cfg.Main.Table)).CombinedOutput()
	return localRouteStatusResult(string(out), exitCode(err), err != nil)
}

func localRouteStatusResult(output string, code int, failed bool) string {
	if failed {
		if tproxy.RouteTableMissing(output, code) {
			return "missing"
		}
		return "unknown"
	}
	if tproxy.RoutePresent(output) {
		return "present"
	}
	return "missing"
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func listenerStatus(addr string) string {
	if out, err := exec.Command("ss", "-ln").CombinedOutput(); err == nil && listenerPresent(string(out), addr) {
		return "present"
	}
	if out, err := exec.Command("netstat", "-lnp").CombinedOutput(); err == nil && listenerPresent(string(out), addr) {
		return "present"
	}
	if out, err := exec.Command("netstat", "-ln").CombinedOutput(); err == nil && listenerPresent(string(out), addr) {
		return "present"
	}
	return "missing"
}

func listenerPresent(output string, addr string) bool {
	wantHost, wantPort, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	for _, raw := range strings.Split(output, "\n") {
		fields := strings.Fields(raw)
		if len(fields) < 4 {
			continue
		}
		proto := fields[0]
		if !strings.HasPrefix(proto, "tcp") && !strings.HasPrefix(proto, "udp") {
			continue
		}
		for _, field := range fields[1:] {
			host, port, ok := splitListenAddress(field)
			if !ok || port != wantPort {
				continue
			}
			if host == wantHost || host == "0.0.0.0" || host == "*" || host == "::" || host == "[::]" {
				return true
			}
		}
	}
	return false
}

func splitListenAddress(s string) (string, string, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") {
		if i := strings.LastIndex(s, "]:"); i >= 0 {
			return s[:i+1], s[i+2:], true
		}
	}
	i := strings.LastIndex(s, ":")
	if i < 0 || i == len(s)-1 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}
