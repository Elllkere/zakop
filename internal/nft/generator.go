package nft

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/elllkere/zakop/internal/config"
	"github.com/elllkere/zakop/internal/policy"
	"github.com/elllkere/zakop/internal/proxyroute"
	"github.com/elllkere/zakop/internal/ruleengine"
)

type Input struct {
	Config    config.Config
	RuleCIDRs map[int][]*net.IPNet
}

func Generate(in Input) (string, error) {
	cfg := in.Config
	directClients := collectClients(cfg, "direct")
	proxyClients := collectClients(cfg, "proxy")
	lanSubnets := policy.NormalizeIPv4CIDRs(policy.MustIPv4CIDRs(cfg.Main.LANSubnets...))
	reserved4 := reservedCIDRs(cfg)
	proxyTargets := proxyroute.Targets(cfg)
	var defaultProxyTarget proxyroute.Target
	if len(proxyTargets) > 0 {
		defaultProxyTarget = proxyTargets[0]
	}

	var b strings.Builder
	b.WriteString("table inet zakop {\n")
	writeSet(&b, "lan_subnets4", policy.CIDRStrings(lanSubnets))
	writeSet(&b, "reserved4", policy.CIDRStrings(reserved4))
	writeSet(&b, "direct_clients4", directClients)
	writeSet(&b, "proxy_clients4", proxyClients)
	writeRuleSets(&b, cfg, in.RuleCIDRs)
	if cfg.Main.ManageDNSMasq {
		b.WriteString("\tchain dns_prerouting {\n")
		b.WriteString("\t\ttype nat hook prerouting priority dstnat; policy accept;\n")
		b.WriteString(fmt.Sprintf("\t\t%s udp dport 53%s redirect to :53\n", lanSourceMatcher(cfg), counter(cfg)))
		b.WriteString(fmt.Sprintf("\t\t%s tcp dport 53%s redirect to :53\n", lanSourceMatcher(cfg), counter(cfg)))
		b.WriteString("\t}\n")
	}
	b.WriteString("\tchain prerouting {\n")
	b.WriteString("\t\ttype filter hook prerouting priority mangle; policy accept;\n")
	b.WriteString(lanGuardRule(cfg))
	b.WriteString("\t\treturn\n")
	b.WriteString("\t}\n")
	b.WriteString("\tchain from_lan {\n")
	if cfg.Main.ManageDNSMasq {
		// Plain DNS must reach the later nat redirect even in global mode or
		// for a force-proxy client. Otherwise TProxy would consume it first.
		b.WriteString(fmt.Sprintf("\t\tudp dport 53%s return\n", counter(cfg)))
		b.WriteString(fmt.Sprintf("\t\ttcp dport 53%s return\n", counter(cfg)))
	}
	b.WriteString(fmt.Sprintf("\t\tct status dnat%s return\n", counter(cfg)))
	b.WriteString(fmt.Sprintf("\t\tip saddr @direct_clients4%s return\n", counter(cfg)))
	b.WriteString(fmt.Sprintf("\t\tip daddr @reserved4%s return\n", counter(cfg)))
	if defaultProxyTarget.Tag != "" {
		b.WriteString(fmt.Sprintf("\t\tip saddr @proxy_clients4 meta l4proto { tcp, udp }%s jump %s\n", counter(cfg), defaultProxyTarget.Chain))
	}
	switch cfg.Main.RoutingMode {
	case "global":
		if defaultProxyTarget.Tag != "" {
			b.WriteString(fmt.Sprintf("\t\tmeta l4proto { tcp, udp }%s jump %s\n", counter(cfg), defaultProxyTarget.Chain))
		}
	default:
		if cfg.Main.FakeIPEnabled && defaultProxyTarget.Tag != "" && hasProxyDomainRule(cfg) {
			b.WriteString(fmt.Sprintf("\t\tip daddr %s meta l4proto { tcp, udp }%s jump %s\n", cfg.Main.FakeIPRange, counter(cfg), defaultProxyTarget.Chain))
		}
		if err := writeOrderedIPRules(&b, cfg, in.RuleCIDRs, proxyTargets); err != nil {
			return "", err
		}
	}
	b.WriteString("\t\treturn\n")
	b.WriteString("\t}\n")
	for _, target := range proxyTargets {
		b.WriteString(fmt.Sprintf("\tchain %s {\n", target.Chain))
		b.WriteString(fmt.Sprintf("\t\tmeta l4proto { tcp, udp }%s meta mark set %s tproxy ip to 127.0.0.1:%d accept\n", counter(cfg), cfg.Main.Mark, target.Port))
		b.WriteString("\t\treturn\n")
		b.WriteString("\t}\n")
	}
	b.WriteString("}\n")
	return b.String(), nil
}

func writeRuleSets(b *strings.Builder, cfg config.Config, ruleCIDRs map[int][]*net.IPNet) {
	for i, rule := range cfg.EffectiveRules() {
		if !ruleengine.HasIPMatch(rule) || len(ruleCIDRs[i]) == 0 {
			continue
		}
		writeSet(b, ruleSetName(i), policy.CIDRStrings(policy.NormalizeIPv4CIDRs(ruleCIDRs[i])))
	}
}

func writeOrderedIPRules(b *strings.Builder, cfg config.Config, ruleCIDRs map[int][]*net.IPNet, proxyTargets []proxyroute.Target) error {
	for i, rule := range cfg.EffectiveRules() {
		if !ruleengine.HasIPMatch(rule) {
			continue
		}
		if len(ruleCIDRs[i]) == 0 {
			continue
		}
		matches, err := rulePacketMatches(ruleSetName(i), rule)
		if err != nil {
			return fmt.Errorf("rule %q packet match: %w", rule.Name, err)
		}
		switch rule.Action {
		case "proxy":
			target, ok := proxyroute.Find(proxyTargets, rule.Outbound)
			if !ok {
				return fmt.Errorf("rule %q outbound %q has no TProxy target", rule.Name, rule.Outbound)
			}
			for _, match := range matches {
				b.WriteString(fmt.Sprintf("\t\t%s%s jump %s\n", match, counter(cfg), target.Chain))
			}
		case "direct", "block":
			for _, match := range matches {
				verdict := "return"
				if rule.Action == "block" {
					verdict = "drop"
				}
				b.WriteString(fmt.Sprintf("\t\t%s%s %s\n", match, counter(cfg), verdict))
			}
		}
	}
	return nil
}

func hasProxyDomainRule(cfg config.Config) bool {
	for _, rule := range cfg.EffectiveRules() {
		if rule.Enabled && rule.Action == "proxy" && ruleengine.HasDomainIncludes(rule) {
			return true
		}
	}
	return false
}

func rulePacketMatches(setName string, rule config.Rule) ([]string, error) {
	base := "ip daddr @" + setName
	protos := packetProtocols(rule)
	srcPorts, err := parseNFTPorts(rule.SrcPorts)
	if err != nil {
		return nil, err
	}
	dstPorts, err := parseNFTPorts(rule.DstPorts)
	if err != nil {
		return nil, err
	}
	hasPorts := len(srcPorts)+len(dstPorts) > 0

	if !hasPorts {
		if len(protos) == 0 {
			return []string{base + " meta l4proto { tcp, udp }"}, nil
		}
		out := make([]string, 0, len(protos))
		for _, proto := range protos {
			out = append(out, base+" meta l4proto "+proto)
		}
		return out, nil
	}

	if len(protos) == 0 {
		protos = []string{"tcp", "udp"}
	}
	if len(srcPorts) == 0 {
		srcPorts = []string{""}
	}
	if len(dstPorts) == 0 {
		dstPorts = []string{""}
	}

	var out []string
	for _, proto := range protos {
		for _, srcPort := range srcPorts {
			for _, dstPort := range dstPorts {
				parts := []string{base}
				if srcPort != "" {
					parts = append(parts, proto+" sport "+srcPort)
				}
				if dstPort != "" {
					parts = append(parts, proto+" dport "+dstPort)
				}
				out = append(out, strings.Join(parts, " "))
			}
		}
	}
	return out, nil
}

func packetProtocols(rule config.Rule) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(proto string) {
		proto = strings.ToLower(strings.TrimSpace(proto))
		if proto != "tcp" && proto != "udp" {
			return
		}
		if _, ok := seen[proto]; ok {
			return
		}
		seen[proto] = struct{}{}
		out = append(out, proto)
	}
	for _, proto := range rule.Proto {
		add(proto)
	}
	if _, ok := seen["tcp"]; ok {
		if _, ok := seen["udp"]; ok {
			return []string{"tcp", "udp"}
		}
	}
	return out
}

func parseNFTPorts(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	for _, value := range values {
		port, err := parseNFTPort(value)
		if err != nil {
			return nil, err
		}
		out = append(out, port)
	}
	return out, nil
}

func parseNFTPort(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("empty port")
	}
	parts := strings.Split(value, "-")
	if len(parts) > 2 {
		return "", fmt.Errorf("invalid port range %q", value)
	}
	start, err := parsePortNumber(parts[0])
	if err != nil {
		return "", err
	}
	end := start
	if len(parts) == 2 {
		end, err = parsePortNumber(parts[1])
		if err != nil {
			return "", err
		}
		if start > end {
			return "", fmt.Errorf("invalid port range %q", value)
		}
		return strconv.Itoa(start) + "-" + strconv.Itoa(end), nil
	}
	return strconv.Itoa(start), nil
}

func parsePortNumber(value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("port must be 1..65535")
	}
	return n, nil
}

func ruleSetName(index int) string {
	return fmt.Sprintf("rule4_%04d", index)
}

func lanGuardRule(cfg config.Config) string {
	return "\t\t" + lanSourceMatcher(cfg) + " jump from_lan\n"
}

func lanSourceMatcher(cfg config.Config) string {
	if len(cfg.Main.LANIfaces) == 0 {
		return "ip saddr @lan_subnets4"
	}
	return fmt.Sprintf("iifname %s ip saddr @lan_subnets4", ifaceMatcher(cfg.Main.LANIfaces))
}

func ifaceMatcher(ifaces []string) string {
	quoted := make([]string, 0, len(ifaces))
	for _, iface := range ifaces {
		quoted = append(quoted, quoteNFTString(iface))
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return "{ " + strings.Join(quoted, ", ") + " }"
}

func quoteNFTString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func counter(cfg config.Config) string {
	if cfg.Main.NFTCounters {
		return " counter"
	}
	return ""
}

func writeSet(b *strings.Builder, name string, elements []string) {
	sort.Strings(elements)
	b.WriteString(fmt.Sprintf("\tset %s {\n", name))
	b.WriteString("\t\ttype ipv4_addr\n")
	b.WriteString("\t\tflags interval\n")
	b.WriteString("\t\tauto-merge\n")
	if len(elements) > 0 {
		b.WriteString("\t\telements = {")
		for i, element := range elements {
			if i == 0 {
				b.WriteString(" ")
			} else {
				b.WriteString(", ")
			}
			b.WriteString(element)
		}
		b.WriteString(" }\n")
	}
	b.WriteString("\t}\n")
}

func collectClients(cfg config.Config, policy string) []string {
	var out []string
	for _, client := range cfg.Clients {
		if client.Policy == policy {
			out = append(out, client.IP)
		}
	}
	return out
}

func reservedCIDRs(cfg config.Config) []*net.IPNet {
	values := []string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"192.168.0.0/16",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",
	}
	// 198.18.0.0/15 is intentionally omitted when used as FakeIP space.
	if !cfg.Main.FakeIPEnabled || cfg.Main.FakeIPRange != "198.18.0.0/15" {
		values = append(values, "198.18.0.0/15")
	}
	return policy.MustIPv4CIDRs(values...)
}
