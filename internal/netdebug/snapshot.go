//go:build networkdebug

// Package netdebug implements bounded, RAM-only observations in diagnostic builds.
package netdebug

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/elllkere/zakop/internal/config"
	"github.com/elllkere/zakop/internal/tproxy"
)

const Directory = "/tmp/zakop/network-debug"

// Limits apply before redaction, including commands which emit unbounded output.
type boundedBuffer struct {
	bytes.Buffer
	remaining int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) > b.remaining {
		p = p[:b.remaining]
	}
	b.remaining -= len(p)
	_, _ = b.Buffer.Write(p)
	return n, nil
}

func run(ctx context.Context, name string, args ...string) string {
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, name, args...)
	b := &boundedBuffer{remaining: 2 << 20}
	cmd.Stdout, cmd.Stderr = b, b
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	return fmt.Sprintf("%s\n[result=%v; output capped at 2 MiB]\n", b.String(), err)
}

var quoted = regexp.MustCompile(`"(?:\\.|[^"\\])*"`)
var url = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s"<>]+`)
var secret = regexp.MustCompile(`(?i)(password|passwd|token|secret|private.?key|authorization|subscription|credential|uuid)`)
var kernelNetwork = regexp.MustCompile(`(?i)(NETDEV WATCHDOG|transmit queue.*timed out|rk_gmac|stmmac|dwmac|r8152|Reset adapter|nf_conntrack.*(full|drop)|soft lockup|rcu.*stall)`)

func sanitize(s string) string {
	var out strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if secret.MatchString(line) {
			out.WriteString("[sensitive line omitted]\n")
			continue
		}
		out.WriteString(url.ReplaceAllString(line, "[URL omitted]"))
		out.WriteByte('\n')
	}
	return out.String()
}

func kernelLines(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if kernelNetwork.MatchString(line) && !secret.MatchString(line) {
			out = append(out, line)
		}
	}
	if len(out) > 100 {
		out = out[len(out)-100:]
	}
	return sanitize(strings.Join(out, "\n"))
}

func redactNFT(s string) string {
	return sanitize(quoted.ReplaceAllStringFunc(s, func(text string) string {
		// Preserve actual interface names for cross-table path analysis, without
		// retaining arbitrary comments or strings from unrelated applications.
		value, err := strconv.Unquote(text)
		if err == nil && value != "." && filepath.Base(value) == value && len(value) <= 15 {
			if _, err := os.Stat(filepath.Join("/sys/class/net", value)); err == nil {
				return text
			}
		}
		// Fixed diagnostic labels are also safe to preserve.
		switch text {
		case `"debug_already_marked"`, `"debug_dnat_marked"`, `"debug_dnat_marked_late"`, `"debug_unmatched_direct"`, `"debug_tcp_attempt"`, `"debug_udp_attempt"`, `"debug_tproxy_fallthrough"`:
			return text
		}
		return `"[text omitted]"`
	}))
}

func read(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Sprintln(err)
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, 512<<10))
	return string(b)
}

// No argv, environment, UCI dump or generated sing-box JSON is collected.
func processes() string {
	entries, _ := filepath.Glob("/proc/[0-9]*/comm")
	var b strings.Builder
	for _, p := range entries {
		name := strings.TrimSpace(read(p))
		pid := filepath.Base(filepath.Dir(p))
		fmt.Fprintf(&b, "pid=%s comm=%s\n", pid, name)
		if name == "sing-box" || name == "zakopd" {
			fds, _ := os.ReadDir(filepath.Join(filepath.Dir(p), "fd"))
			fmt.Fprintf(&b, "fds=%d\n%s\n", len(fds), read(filepath.Join(filepath.Dir(p), "status")))
		}
	}
	return sanitize(b.String())
}

// Collect writes at most 4 MiB per snapshot. Full snapshots rotate to 4 files;
// samples rotate to 12 files. The lock is released by the kernel after a crash.
func Collect(ctx context.Context, cfg config.Config, dir, reason string, light bool) (string, error) {
	return collect(ctx, cfg, dir, reason, light, run)
}

func collect(ctx context.Context, cfg config.Config, dir, reason string, light bool, runner func(context.Context, string, ...string) string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	lock, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return "", fmt.Errorf("snapshot already running: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	prefix, keep := "event", 4
	if light {
		prefix, keep = "sample", 12
	}
	// Automatic event storms must not hammer the router; manual remains available.
	stamp := filepath.Join(dir, ".last-event")
	if !light && reason != "manual" {
		if st, e := os.Stat(stamp); e == nil && time.Since(st.ModTime()) < time.Minute {
			return "", nil
		}
	}
	limit := 4 << 20
	if light {
		limit = 256 << 10
	}
	b := &boundedBuffer{remaining: limit}
	fmt.Fprintf(b, "utc=%s reason=%s light=%v\n", time.Now().UTC().Format(time.RFC3339), sanitize(reason), light)
	fmt.Fprintf(b, "routing_mode=%s mark=%s table=%d tproxy_port=%d targets=%d manage_dnsmasq=%v\n", cfg.Main.RoutingMode, cfg.Main.Mark, cfg.Main.Table, cfg.Main.TProxyPort, len(cfg.ProxyTargetTags()), cfg.Main.ManageDNSMasq)
	fmt.Fprintf(b, "lan_subnets=%v lan_ifaces=%v real_dns_mode=%s real_dns_transport=%s fakeip=%v\n", cfg.Main.LANSubnets, cfg.Main.LANIfaces, cfg.Main.RealDNSMode, cfg.Main.RealDNSTransport, cfg.Main.FakeIPEnabled)
	// Counter summaries precede large route/set dumps so truncation cannot hide them.
	for _, args := range [][]string{{"-a", "list", "chain", "inet", "zakop", "from_lan"}, {"-a", "list", "chain", "inet", "zakop", "debug_late_prerouting"}, {"list", "counters", "table", "inet", "zakop"}} {
		fmt.Fprintf(b, "\n=== nft %v ===\n%s", args, redactNFT(runner(ctx, "nft", args...)))
	}
	for _, p := range []string{"/proc/uptime", "/proc/meminfo", "/proc/loadavg", "/proc/stat", "/proc/interrupts", "/proc/softirqs", "/proc/net/dev", "/proc/net/sockstat", "/proc/sys/net/netfilter/nf_conntrack_count", "/proc/sys/net/netfilter/nf_conntrack_max"} {
		fmt.Fprintf(b, "\n=== %s ===\n%s", p, read(p))
	}
	for _, name := range []string{"tcp_fwmark_accept", "fwmark_reflect", "ip_forward"} {
		fmt.Fprintf(b, "\nnet.ipv4.%s=%s", name, read("/proc/sys/net/ipv4/"+name))
	}
	ifaces, _ := os.ReadDir("/proc/sys/net/ipv4/conf")
	for _, iface := range ifaces {
		for _, key := range []string{"rp_filter", "src_valid_mark", "route_localnet", "accept_local"} {
			fmt.Fprintf(b, "\nnet.ipv4.conf.%s.%s=%s", iface.Name(), key, read(filepath.Join("/proc/sys/net/ipv4/conf", iface.Name(), key)))
		}
	}
	fmt.Fprintf(b, "\n=== processes (no arguments) ===\n%s", processes())
	commands := [][]string{{"ip", "-s", "link"}, {"ip", "-4", "rule", "show"}, {"ip", "-4", "route", "show", "table", "all"}, {"conntrack", "-S"}, {"ss", "-s"}}
	if !light {
		commands = append(commands, []string{"ip", "addr"}, []string{"free"}, []string{"ss", "-lnptu"}, []string{"ss", "-ntup"})
	}
	for _, args := range commands {
		fmt.Fprintf(b, "\n=== %s ===\n%s", strings.Join(args, " "), sanitize(runner(ctx, args[0], args[1:]...)))
	}
	// Preserve all rules, but never retain arbitrary quoted nft comments/prefixes.
	if !light {
		fmt.Fprintf(b, "\n=== nft ruleset (quoted strings redacted) ===\n%s", redactNFT(runner(ctx, "nft", "-a", "list", "ruleset")))
		// ethtool runs only on kernel-enumerated interfaces, never UCI shell text.
		ifaces, _ := os.ReadDir("/sys/class/net")
		for _, iface := range ifaces {
			if iface.Name() == "lo" {
				continue
			}
			for _, args := range [][]string{{iface.Name()}, {"-S", iface.Name()}} {
				fmt.Fprintf(b, "\n=== ethtool %v ===\n%s", args, sanitize(runner(ctx, "ethtool", args...)))
			}
		}
		for _, cmd := range []string{"dmesg", "logread"} {
			fmt.Fprintf(b, "\n=== %s network events only ===\n%s", cmd, kernelLines(runner(ctx, cmd)))
		}
	}
	fmt.Fprintln(b, "\nRaw application logs, argv, URLs, keys and full UCI/JSON intentionally excluded. Missing commands and truncation are recorded; this is not a complete raw system dump.")
	if b.remaining == 0 {
		marker := []byte("\n[SNAPSHOT TRUNCATED AT SIZE LIMIT]\n")
		copy(b.Bytes()[b.Len()-len(marker):], marker)
	}
	tmp, err := os.CreateTemp(dir, ".snapshot-")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(b.Bytes())
	closeErr := tmp.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	path := filepath.Join(dir, prefix+"-"+time.Now().UTC().Format("20060102T150405.000000000")+".txt")
	if err = os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	if !light && reason != "manual" {
		_ = os.WriteFile(stamp, nil, 0600)
	}
	files, _ := filepath.Glob(filepath.Join(dir, prefix+"-*.txt"))
	sort.Strings(files)
	for len(files) > keep {
		_ = os.Remove(files[0])
		files = files[1:]
	}
	return path, nil
}

// Watch samples once per minute. Missing processes or a changed network timeout
// count cause an event snapshot, without restarting anything or changing policy.
func Watch(ctx context.Context, configPath string) error {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	previous := ""
	for {
		cfg, err := config.LoadFile(configPath)
		if err != nil {
			return err
		}
		if !cfg.Main.Enabled {
			return nil
		}
		_, _ = Collect(ctx, cfg, Directory, "periodic", true)
		state := suspicious(ctx, cfg)
		if state != "" && state != previous {
			_, _ = Collect(ctx, cfg, Directory, state, false)
		}
		previous = state
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func suspicious(ctx context.Context, cfg config.Config) string {
	var result []string
	// Inspect comm, never argv or environment.
	found := false
	entries, _ := filepath.Glob("/proc/[0-9]*/comm")
	for _, p := range entries {
		if strings.TrimSpace(read(p)) == "sing-box" {
			found = true
			break
		}
	}
	if !found {
		result = append(result, "singbox-missing")
	}
	routes := run(ctx, "ip", "-4", "route", "show", "table", strconv.Itoa(cfg.Main.Table))
	if !tproxy.RoutePresent(routes) {
		result = append(result, "local-route-missing")
	}
	if !tproxy.RulePresent(run(ctx, "ip", "-4", "rule", "show"), tproxy.Config{Mark: cfg.Main.Mark, Table: cfg.Main.Table}) {
		result = append(result, "mark-rule-missing")
	}
	if !strings.Contains(run(ctx, "nft", "list", "table", "inet", "zakop"), "chain prerouting") {
		result = append(result, "nft-missing")
	}
	// Tail matching emits no messages in reason, only a count/signature.
	k := kernelLines(run(ctx, "dmesg"))
	if strings.TrimSpace(k) != "" {
		result = append(result, fmt.Sprintf("kernel-network-events-%x", hash(k)))
	}
	return strings.Join(result, ",")
}

func hash(s string) uint64 {
	var h uint64
	for _, c := range s {
		h = h*31 + uint64(c)
	}
	return h
}
