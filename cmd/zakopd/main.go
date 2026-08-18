package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/elllkere/zakop/internal/config"
	"github.com/elllkere/zakop/internal/dnsproxy"
	"github.com/elllkere/zakop/internal/importer"
	"github.com/elllkere/zakop/internal/nft"
	"github.com/elllkere/zakop/internal/outboundpool"
	"github.com/elllkere/zakop/internal/provider"
	"github.com/elllkere/zakop/internal/singbox"
	"github.com/elllkere/zakop/internal/status"
	"github.com/elllkere/zakop/internal/tproxy"
)

const (
	defaultOutDir = "/tmp/zakop"
	nftFileName   = "zakop.nft"
	sbFileName    = "sing-box.json"
)

var version = "dev"
var singBoxLogPath = "/tmp/zakop/sing-box.log"

type options struct {
	configPath        string
	outDir            string
	skipRuntimeChecks bool
	skipSingBoxCheck  bool
}

type readyOptions struct {
	configPath string
	timeout    time.Duration
}

type importOptions struct {
	configPath string
	filePath   string
}

type subscriptionOptions struct {
	configPath string
	name       string
}

type providerOptions struct {
	configPath string
	name       string
}

type downloadOptions struct {
	configPath string
	rawURL     string
	outputPath string
	via        string
	outbound   string
}

type outboundLatencyOptions struct {
	configPath string
	tag        string
}

type outboundLatencyResult struct {
	Tag       string `json:"tag"`
	Label     string `json:"label"`
	Server    string `json:"server"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

type outboundLatencyReport struct {
	Target  string                  `json:"target"`
	Results []outboundLatencyResult `json:"results"`
}

const latencyTestURL = "https://www.gstatic.com/generate_204"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "zakopd:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("missing command")
	}

	switch args[0] {
	case "version":
		fmt.Printf("zakopd %s\n", version)
		return nil
	case "check":
		opts, err := parseOptions(args[0], args[1:], true)
		if err != nil {
			return err
		}
		return commandCheck(opts)
	case "compile":
		opts, err := parseOptions(args[0], args[1:], false)
		if err != nil {
			return err
		}
		_, _, _, err = compile(opts)
		return err
	case "apply":
		opts, err := parseOptions(args[0], args[1:], false)
		if err != nil {
			return err
		}
		return commandApply(opts)
	case "status":
		opts, err := parseOptions(args[0], args[1:], false)
		if err != nil {
			return err
		}
		return commandStatus(opts)
	case "debug":
		opts, err := parseOptions(args[0], args[1:], false)
		if err != nil {
			return err
		}
		return commandDebug(opts)
	case "run":
		opts, err := parseOptions(args[0], args[1:], false)
		if err != nil {
			return err
		}
		return commandRun(opts)
	case "ready":
		opts, err := parseReadyOptions(args[1:])
		if err != nil {
			return err
		}
		return commandReady(opts)
	case "import-uri":
		opts, err := parseImportOptions(args[1:])
		if err != nil {
			return err
		}
		return commandImportURI(opts)
	case "subscriptions":
		opts, err := parseSubscriptionOptions(args[1:])
		if err != nil {
			return err
		}
		return commandSubscriptionsUpdate(opts)
	case "providers":
		opts, err := parseProviderOptions(args[1:])
		if err != nil {
			return err
		}
		return commandProvidersUpdate(opts)
	case "download":
		opts, err := parseDownloadOptions(args[1:])
		if err != nil {
			return err
		}
		return commandDownload(opts)
	case "outbounds":
		opts, err := parseOutboundLatencyOptions(args[1:])
		if err != nil {
			return err
		}
		return commandOutboundsLatency(opts)
	case "logs":
		return commandLogs(args[1:])
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func parseOutboundLatencyOptions(args []string) (outboundLatencyOptions, error) {
	if len(args) == 0 || args[0] != "latency" {
		return outboundLatencyOptions{}, fmt.Errorf("usage: zakopd outbounds latency [tag] [options]")
	}
	fs := flag.NewFlagSet("zakopd outbounds latency", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := outboundLatencyOptions{configPath: config.DefaultPath}
	fs.StringVar(&opts.configPath, "config", opts.configPath, "path to UCI config")
	if err := fs.Parse(args[1:]); err != nil {
		return outboundLatencyOptions{}, err
	}
	if fs.NArg() > 1 {
		return outboundLatencyOptions{}, fmt.Errorf("unexpected argument %q", fs.Arg(1))
	}
	if fs.NArg() == 1 {
		opts.tag = strings.TrimSpace(fs.Arg(0))
	}
	return opts, nil
}

func parseReadyOptions(args []string) (readyOptions, error) {
	fs := flag.NewFlagSet("zakopd ready", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := readyOptions{
		configPath: config.DefaultPath,
		timeout:    30 * time.Second,
	}
	fs.StringVar(&opts.configPath, "config", opts.configPath, "path to UCI config")
	fs.DurationVar(&opts.timeout, "timeout", opts.timeout, "maximum time to wait for end-to-end DNS readiness")
	if err := fs.Parse(args); err != nil {
		return readyOptions{}, err
	}
	if fs.NArg() != 0 {
		return readyOptions{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if opts.timeout <= 0 {
		return readyOptions{}, fmt.Errorf("ready timeout must be positive")
	}
	return opts, nil
}

func parseDownloadOptions(args []string) (downloadOptions, error) {
	fs := flag.NewFlagSet("zakopd download", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := downloadOptions{configPath: config.DefaultPath}
	fs.StringVar(&opts.configPath, "config", opts.configPath, "path to UCI config")
	fs.StringVar(&opts.rawURL, "url", opts.rawURL, "HTTP(S) URL to download")
	fs.StringVar(&opts.outputPath, "output", opts.outputPath, "absolute output path")
	fs.StringVar(&opts.via, "via", opts.via, "download via direct or proxy")
	fs.StringVar(&opts.outbound, "outbound", opts.outbound, "custom outbound tag for proxy mode")
	if err := fs.Parse(args); err != nil {
		return downloadOptions{}, err
	}
	if fs.NArg() != 0 {
		return downloadOptions{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	parsedURL, err := url.Parse(opts.rawURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return downloadOptions{}, fmt.Errorf("download requires a valid HTTP(S) -url")
	}
	if !filepath.IsAbs(opts.outputPath) {
		return downloadOptions{}, fmt.Errorf("download requires an absolute -output path")
	}
	return opts, nil
}

func parseOptions(command string, args []string, checkFlags bool) (options, error) {
	fs := flag.NewFlagSet("zakopd "+command, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := options{
		configPath: config.DefaultPath,
		outDir:     defaultOutDir,
	}
	fs.StringVar(&opts.configPath, "config", opts.configPath, "path to UCI config")
	fs.StringVar(&opts.outDir, "out-dir", opts.outDir, "runtime output directory")
	if checkFlags {
		fs.BoolVar(&opts.skipRuntimeChecks, "skip-runtime-checks", false, "skip nft command validation")
		fs.BoolVar(&opts.skipSingBoxCheck, "skip-singbox-check", false, "skip sing-box binary validation")
	}
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return opts, nil
}

func parseImportOptions(args []string) (importOptions, error) {
	fs := flag.NewFlagSet("zakopd import-uri", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := importOptions{configPath: config.DefaultPath}
	fs.StringVar(&opts.configPath, "config", opts.configPath, "path to UCI config")
	fs.StringVar(&opts.filePath, "file", opts.filePath, "file containing share links")
	if err := fs.Parse(args); err != nil {
		return importOptions{}, err
	}
	if opts.filePath == "" {
		return importOptions{}, fmt.Errorf("import-uri requires -file")
	}
	if fs.NArg() != 0 {
		return importOptions{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return opts, nil
}

func parseSubscriptionOptions(args []string) (subscriptionOptions, error) {
	if len(args) == 0 || args[0] != "update" {
		return subscriptionOptions{}, fmt.Errorf("usage: zakopd subscriptions update [name] [options]")
	}
	fs := flag.NewFlagSet("zakopd subscriptions update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := subscriptionOptions{configPath: config.DefaultPath}
	fs.StringVar(&opts.configPath, "config", opts.configPath, "path to UCI config")
	if err := fs.Parse(args[1:]); err != nil {
		return subscriptionOptions{}, err
	}
	if fs.NArg() > 1 {
		return subscriptionOptions{}, fmt.Errorf("unexpected argument %q", fs.Arg(1))
	}
	if fs.NArg() == 1 {
		opts.name = fs.Arg(0)
	}
	return opts, nil
}

func parseProviderOptions(args []string) (providerOptions, error) {
	if len(args) == 0 || args[0] != "update" {
		return providerOptions{}, fmt.Errorf("usage: zakopd providers update [name] [options]")
	}
	fs := flag.NewFlagSet("zakopd providers update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := providerOptions{configPath: config.DefaultPath}
	fs.StringVar(&opts.configPath, "config", opts.configPath, "path to UCI config")
	if err := fs.Parse(args[1:]); err != nil {
		return providerOptions{}, err
	}
	if fs.NArg() > 1 {
		return providerOptions{}, fmt.Errorf("unexpected argument %q", fs.Arg(1))
	}
	if fs.NArg() == 1 {
		opts.name = fs.Arg(0)
	}
	return opts, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: zakopd <version|check|compile|apply|status|debug|run|ready|import-uri|subscriptions|providers|download|outbounds|logs> [options]")
}

func commandCheck(opts options) error {
	cfg, err := config.LoadFile(opts.configPath)
	if err != nil {
		return err
	}
	printWarnings(cfg)
	if !cfg.Main.Enabled {
		fmt.Println("config ok: zakop is disabled")
		return nil
	}

	_, _, sbPath, err := compile(opts)
	if err != nil {
		return err
	}
	if !opts.skipRuntimeChecks {
		if err := requireCommand("nft"); err != nil {
			return err
		}
		if err := command("nft", "-c", "-f", nftPath(opts)).Run(); err != nil {
			return fmt.Errorf("nft validation failed: %w", err)
		}
	}
	if !opts.skipSingBoxCheck {
		if !singbox.BinaryExists(cfg.Main.SingBoxBin) {
			return fmt.Errorf("sing-box binary is missing or not executable: %s", cfg.Main.SingBoxBin)
		}
		if err := singbox.CheckBinary(cfg.Main.SingBoxBin, sbPath); err != nil {
			return err
		}
	}
	fmt.Println("config ok")
	return nil
}

func commandApply(opts options) error {
	cfg, err := config.LoadFile(opts.configPath)
	if err != nil {
		return err
	}
	if !cfg.Main.Enabled {
		_ = deleteNftTable()
		_ = cleanupRouting(cfg.Main.Mark, cfg.Main.Table)
		fmt.Println("zakop is disabled; nft table and routing removed")
		return nil
	}
	cfg, nftPath, _, err := compile(opts)
	if err != nil {
		return err
	}
	if err := requireCommand("nft"); err != nil {
		return err
	}
	if err := requireCommand("ip"); err != nil {
		return err
	}
	if out, err := command("nft", "-c", "-f", nftPath).CombinedOutput(); err != nil {
		return fmt.Errorf("nft validation failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	_ = deleteNftTable()
	if out, err := command("nft", "-f", nftPath).CombinedOutput(); err != nil {
		return fmt.Errorf("nft apply failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if err := ensureRouting(cfg.Main.Mark, cfg.Main.Table); err != nil {
		_ = deleteNftTable()
		_ = cleanupRouting(cfg.Main.Mark, cfg.Main.Table)
		return err
	}
	fmt.Println("nft and routing applied")
	return nil
}

func commandStatus(opts options) error {
	cfg, err := config.LoadFile(opts.configPath)
	if err != nil {
		return err
	}
	fmt.Println(status.Summary(cfg))
	return nil
}

func commandLogs(args []string) error {
	if len(args) == 0 || args[0] != "sing-box" {
		return fmt.Errorf("usage: zakopd logs sing-box [clear]")
	}
	if len(args) > 2 {
		return fmt.Errorf("unexpected argument %q", args[2])
	}
	if len(args) == 2 {
		if args[1] != "clear" {
			return fmt.Errorf("unexpected argument %q", args[1])
		}
		if err := os.MkdirAll(filepath.Dir(singBoxLogPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(singBoxLogPath, nil, 0644); err != nil {
			return err
		}
		fmt.Println("sing-box log cleared")
		return nil
	}

	data, err := readFileTail(singBoxLogPath, 128<<10)
	if err != nil {
		return err
	}
	fmt.Print(string(data))
	return nil
}

func readFileTail(path string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = 128 << 10
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}

	var offset int64
	if st.Size() > maxBytes {
		offset = st.Size() - maxBytes
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, err
		}
	}

	data, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		return append([]byte("[older log lines omitted]\n"), data...), nil
	}
	return data, nil
}

func commandRun(opts options) error {
	cfg, err := config.LoadFile(opts.configPath)
	if err != nil {
		return err
	}
	if !cfg.Main.Enabled {
		fmt.Println("zakop is disabled")
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(cfg.OutboundPools) == 0 {
		return dnsproxy.New(cfg).Run(ctx)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 2)
	go func() {
		errCh <- dnsproxy.New(cfg).Run(runCtx)
	}()
	go func() {
		manager := outboundpool.New(cfg, func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		})
		errCh <- manager.Run(runCtx)
	}()

	firstErr := <-errCh
	cancel()
	secondErr := <-errCh
	if firstErr != nil {
		return firstErr
	}
	return secondErr
}

func commandReady(opts readyOptions) error {
	cfg, err := config.LoadFile(opts.configPath)
	if err != nil {
		return err
	}
	if !cfg.Main.Enabled {
		return fmt.Errorf("zakop is disabled")
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	if err := dnsproxy.WaitReady(ctx, cfg.Main.DNSListen); err != nil {
		return err
	}
	fmt.Println("DNS ready")
	return nil
}

func commandImportURI(opts importOptions) error {
	data, err := os.ReadFile(opts.filePath)
	if err != nil {
		return err
	}
	nodes, err := importer.ParseLinks(string(data))
	if err != nil {
		return err
	}
	count, err := importer.ApplyNodes(nodes, importer.ApplyOptions{
		ConfigPath: opts.configPath,
		Source:     "manual",
	})
	if err != nil {
		return err
	}
	fmt.Printf("imported nodes: %d\n", count)
	return nil
}

func commandSubscriptionsUpdate(opts subscriptionOptions) error {
	cfg, err := config.LoadFile(opts.configPath)
	if err != nil {
		return err
	}
	var matched int
	var updated int
	for _, sub := range cfg.Subscriptions {
		if opts.name != "" && sub.Name != opts.name {
			continue
		}
		matched++
		if !sub.Enabled {
			fmt.Printf("subscription %s is disabled; skipped\n", sub.Name)
			continue
		}
		body, err := fetchSubscription(cfg, sub)
		if err != nil {
			return fmt.Errorf("subscription %s: %w", sub.Name, err)
		}
		nodes, err := importer.ParseLinks(string(body))
		if err != nil {
			return fmt.Errorf("subscription %s: %w", sub.Name, err)
		}
		count, err := importer.ApplyNodes(nodes, importer.ApplyOptions{
			ConfigPath:   opts.configPath,
			Source:       sub.Name,
			Subscription: true,
			Replace:      true,
		})
		if err != nil {
			return fmt.Errorf("subscription %s: %w", sub.Name, err)
		}
		fmt.Printf("subscription %s: imported nodes: %d\n", sub.Name, count)
		updated += count
	}
	if matched == 0 {
		if opts.name != "" {
			return fmt.Errorf("subscription %q not found", opts.name)
		}
		return fmt.Errorf("no subscriptions configured")
	}
	fmt.Printf("subscriptions updated nodes: %d\n", updated)
	return nil
}

func commandProvidersUpdate(opts providerOptions) error {
	cfg, err := loadConfigForManagement(opts.configPath)
	if err != nil {
		return err
	}
	var matched int
	var updated int
	for _, p := range cfg.Providers {
		if opts.name != "" && p.Name != opts.name {
			continue
		}
		matched++
		if p.Source != "script" && strings.TrimSpace(p.URL) == "" {
			return fmt.Errorf("provider %s: url is required", p.Name)
		}
		if p.Source == "script" && strings.TrimSpace(p.ScriptPath) == "" {
			return fmt.Errorf("provider %s: script_path is required", p.Name)
		}
		body, err := fetchProvider(cfg, p)
		if err != nil {
			return fmt.Errorf("provider %s: %w", p.Name, err)
		}
		items, err := provider.NormalizeDownloadedList(p, body)
		if err != nil {
			return fmt.Errorf("provider %s: %w", p.Name, err)
		}
		path, err := provider.WriteCache(p, items)
		if err != nil {
			return fmt.Errorf("provider %s: %w", p.Name, err)
		}
		if err := provider.UpdateMetadata(opts.configPath, p.Name, path, len(items)); err != nil {
			return fmt.Errorf("provider %s: %w", p.Name, err)
		}
		fmt.Printf("provider %s: updated items: %d\n", p.Name, len(items))
		updated += len(items)
	}
	if matched == 0 {
		if opts.name != "" {
			return fmt.Errorf("provider %q not found", opts.name)
		}
		return fmt.Errorf("no providers configured")
	}
	fmt.Printf("providers updated items: %d\n", updated)
	return nil
}

func commandDebug(opts options) error {
	cfg, err := config.LoadFile(opts.configPath)
	if err != nil {
		return err
	}
	fmt.Println("=== zakop config ===")
	fmt.Printf("config: %s\n", opts.configPath)
	fmt.Printf("outbounds: %s\n", status.OutboundsSummary(cfg))
	printWarnings(cfg)
	fmt.Println("=== DNS summary ===")
	printDNSSummary(cfg)
	fmt.Println("=== update transport ===")
	fmt.Printf("update_via: %s\n", cfg.Main.UpdateVia)
	if cfg.Main.UpdateOutbound != "" {
		fmt.Printf("update_outbound: %s\n", cfg.Main.UpdateOutbound)
	}
	fmt.Println("=== generated files ===")
	printPath("nft", nftPath(opts))
	printPath("sing-box", singBoxPath(opts))
	fmt.Println("=== lan scope ===")
	fmt.Printf("lan_subnets4: %s\n", debugList(cfg.Main.LANSubnets))
	fmt.Printf("lan_ifaces: %s\n", debugList(cfg.Main.LANIfaces))
	fmt.Println("=== zakopd status ===")
	fmt.Println(status.Summary(cfg))
	fmt.Println("=== nft table ===")
	printCommand("nft", "list", "table", "inet", "zakop")
	fmt.Println("=== ip rules ===")
	printCommand("ip", "-4", "rule", "show")
	fmt.Println("=== ip route table ===")
	printCommand("ip", "-4", "route", "show", "table", strconv.Itoa(cfg.Main.Table))
	fmt.Println("=== processes ===")
	printCommand("sh", "-c", "ps w | grep -E 'zakopd|sing-box' | grep -v grep")
	fmt.Println("=== listeners ===")
	if _, err := exec.LookPath("ss"); err == nil {
		printCommand("sh", "-c", "ss -lnp | grep -E '5353|15353|15354|15355|16001' || true")
	} else {
		printCommand("sh", "-c", "netstat -lnp 2>/dev/null | grep -E '5353|15353|15354|15355|16001' || netstat -ln 2>/dev/null | grep -E '5353|15353|15354|15355|16001' || true")
	}
	return nil
}

func printDNSSummary(cfg config.Config) {
	upstream := cfg.Main.DNSUpstream()
	fmt.Printf("dns_listen: %s\n", cfg.Main.DNSListen)
	fmt.Printf("real_dns_mode: %s\n", cfg.Main.RealDNSMode)
	if strings.TrimSpace(cfg.Main.RealDNSOutbound) != "" {
		fmt.Printf("real_dns_outbound: %s\n", cfg.Main.RealDNSOutbound)
	}
	fmt.Printf("real_dns_transport: %s\n", upstream.Protocol)
	fmt.Printf("real_dns_server: %s\n", upstream.Address())
	if upstream.TLSName != "" {
		fmt.Printf("real_dns_server_name: %s\n", upstream.TLSName)
	}
	if upstream.Protocol == "https" {
		fmt.Printf("real_dns_path: %s\n", upstream.Path)
	}
	fmt.Printf("dns_server fakeip: listener=%s tag=fakeip\n", cfg.Main.SingBoxDNSFakeIPAddr())
	fmt.Printf("dns_server real-direct: listener=%s tag=real-direct dial=direct\n", cfg.Main.SingBoxDNSRealDirectAddr())
	realProxyDial := singbox.DNSProxyOutbound(cfg)
	if realProxyDial == config.BuiltinDirectOutbound {
		realProxyDial = "direct"
	}
	fmt.Printf("dns_server real-proxy: listener=%s tag=real-proxy dial=%s\n", cfg.Main.SingBoxDNSRealProxyAddr(), realProxyDial)
}

func debugList(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ",")
}

func fetchSubscription(cfg config.Config, sub config.Subscription) ([]byte, error) {
	switch sub.UpdateVia {
	case "", "direct":
		return fetchURLWithCurl(sub.URL, "")
	case "proxy":
		return fetchURLViaProxy(cfg, sub.URL, sub.UpdateOutbound)
	default:
		return nil, fmt.Errorf("unsupported update_via %q", sub.UpdateVia)
	}
}

func fetchProvider(cfg config.Config, p config.Provider) ([]byte, error) {
	switch p.Source {
	case "", "url":
	case "script":
		return fetchProviderWithScript(cfg, p)
	default:
		return nil, fmt.Errorf("unsupported provider source %q", p.Source)
	}
	switch p.UpdateVia {
	case "", "direct":
		return fetchURLWithCurl(p.URL, "")
	case "proxy":
		return fetchURLViaProxy(cfg, p.URL, p.UpdateOutbound)
	default:
		return nil, fmt.Errorf("unsupported update_via %q", p.UpdateVia)
	}
}

func fetchURLViaProxy(cfg config.Config, rawURL string, updateOutbound string) ([]byte, error) {
	return withTemporaryProxy(cfg, updateOutbound, func(proxy string) ([]byte, error) {
		return fetchURLWithCurl(rawURL, proxy)
	})
}

func commandDownload(opts downloadOptions) error {
	cfg, err := loadConfigForManagement(opts.configPath)
	if err != nil {
		return err
	}
	via := strings.TrimSpace(opts.via)
	if via == "" {
		via = cfg.Main.UpdateVia
	}
	outbound := strings.TrimSpace(opts.outbound)
	if outbound == "" {
		outbound = cfg.Main.UpdateOutbound
	}
	switch via {
	case "", "direct":
		return downloadURLToFile(opts.rawURL, opts.outputPath, "")
	case "proxy":
		return withTemporaryProxyDo(cfg, outbound, func(proxy string) error {
			return downloadURLToFile(opts.rawURL, opts.outputPath, proxy)
		})
	default:
		return fmt.Errorf("unsupported update_via %q", via)
	}
}

type outboundLatencyItem struct {
	outbound config.Outbound
}

type outboundLatencyMeasurement struct {
	delayMS int64
	err     error
}

func commandOutboundsLatency(opts outboundLatencyOptions) error {
	cfg, err := loadConfigForManagement(opts.configPath)
	if err != nil {
		return err
	}
	if err := requireCommand("curl"); err != nil {
		return err
	}
	if !singbox.BinaryExists(cfg.Main.SingBoxBin) {
		return fmt.Errorf("sing-box binary is missing or not executable: %s", cfg.Main.SingBoxBin)
	}

	items := make([]outboundLatencyItem, 0, len(cfg.EnabledCustomOutbounds()))
	for _, outbound := range cfg.EnabledCustomOutbounds() {
		if opts.tag != "" && outbound.Tag != opts.tag {
			continue
		}
		items = append(items, outboundLatencyItem{outbound: outbound})
	}
	if len(items) == 0 {
		if opts.tag != "" {
			return fmt.Errorf("outbound %q not found", opts.tag)
		}
		return fmt.Errorf("no custom outbounds configured")
	}

	results := make([]outboundLatencyResult, 0, len(items))
	const batchSize = 32
	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}
		batchResults, err := testOutboundLatencyBatch(cfg, items[start:end])
		if err != nil {
			return err
		}
		results = append(results, batchResults...)
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].OK != results[j].OK {
			return results[i].OK
		}
		if results[i].OK && results[i].LatencyMS != results[j].LatencyMS {
			return results[i].LatencyMS < results[j].LatencyMS
		}
		return strings.ToLower(results[i].Label) < strings.ToLower(results[j].Label)
	})

	return json.NewEncoder(os.Stdout).Encode(outboundLatencyReport{
		Target:  latencyTestURL,
		Results: results,
	})
}

func testOutboundLatencyBatch(cfg config.Config, items []outboundLatencyItem) ([]outboundLatencyResult, error) {
	controllerPort, err := freeLocalPort()
	if err != nil {
		return nil, err
	}
	outboundTags := make([]string, 0, len(items))
	for _, item := range items {
		outboundTags = append(outboundTags, item.outbound.Tag)
	}

	proxyJSON, err := singbox.GenerateLatencyClient(cfg, outboundTags, controllerPort)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "zakop-latency-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "sing-box.json")
	if err := os.WriteFile(path, append(proxyJSON, '\n'), 0600); err != nil {
		return nil, err
	}
	if err := singbox.CheckBinary(cfg.Main.SingBoxBin, path); err != nil {
		return nil, err
	}

	var stderr bytes.Buffer
	cmd := exec.Command(cfg.Main.SingBoxBin, "run", "-c", path)
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	if err := waitForPort(controllerPort, 5*time.Second); err != nil {
		return nil, fmt.Errorf("temporary sing-box Clash API did not start: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	results := make([]outboundLatencyResult, len(items))
	for i, item := range items {
		results[i] = outboundLatencyResult{
			Tag:    item.outbound.Tag,
			Label:  firstNonEmptyString(item.outbound.Label, item.outbound.Tag),
			Server: fmt.Sprintf("%s:%d", item.outbound.Server, item.outbound.Port),
		}
	}

	// The first successful URLTest warms DNS and protocol state. Its value is
	// intentionally discarded so cold setup does not inflate the report.
	warmup := measureOutboundLatencyPass(controllerPort, items, nil)
	measure := make([]bool, len(items))
	for i := range warmup {
		if warmup[i].err != nil {
			results[i].Error = outboundLatencyError(warmup[i].err)
			continue
		}
		measure[i] = true
	}

	measured := measureOutboundLatencyPass(controllerPort, items, measure)
	for i := range measured {
		if !measure[i] {
			continue
		}
		if measured[i].err != nil {
			results[i].Error = outboundLatencyError(measured[i].err)
			continue
		}
		results[i].OK = true
		results[i].LatencyMS = measured[i].delayMS
		results[i].Error = ""
	}
	return results, nil
}

func measureOutboundLatencyPass(controllerPort int, items []outboundLatencyItem, enabled []bool) []outboundLatencyMeasurement {
	measurements := make([]outboundLatencyMeasurement, len(items))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, item := range items {
		if enabled != nil && !enabled[i] {
			continue
		}
		wg.Add(1)
		go func(index int, item outboundLatencyItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			delayMS, err := queryOutboundDelay(controllerPort, item.outbound.Tag)
			measurements[index] = outboundLatencyMeasurement{delayMS: delayMS, err: err}
		}(i, item)
	}
	wg.Wait()
	return measurements
}

func queryOutboundDelay(controllerPort int, outboundTag string) (int64, error) {
	endpoint := fmt.Sprintf(
		"http://127.0.0.1:%d/proxies/%s/delay?url=%s&timeout=10000",
		controllerPort,
		url.PathEscape(outboundTag),
		url.QueryEscape(latencyTestURL),
	)
	out, err := command("curl",
		"-fsS",
		"--noproxy", "*",
		"--connect-timeout", "2",
		"--max-time", "12",
		endpoint,
	).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = err.Error()
		}
		return 0, errors.New(message)
	}
	return parseOutboundDelay(out)
}

func parseOutboundDelay(raw []byte) (int64, error) {
	var response struct {
		Delay int64 `json:"delay"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return 0, fmt.Errorf("invalid sing-box URLTest response: %w", err)
	}
	if response.Delay < 1 {
		return 0, fmt.Errorf("invalid sing-box URLTest delay %d", response.Delay)
	}
	return response.Delay, nil
}

func outboundLatencyError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 240 {
		message = message[:240]
	}
	return message
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func fetchProviderWithScript(cfg config.Config, p config.Provider) ([]byte, error) {
	switch p.UpdateVia {
	case "", "direct":
		return runProviderScript(p, "")
	case "proxy":
		return withTemporaryProxy(cfg, p.UpdateOutbound, func(proxy string) ([]byte, error) {
			return runProviderScript(p, proxy)
		})
	default:
		return nil, fmt.Errorf("unsupported update_via %q", p.UpdateVia)
	}
}

func withTemporaryProxy(cfg config.Config, updateOutbound string, fn func(proxy string) ([]byte, error)) ([]byte, error) {
	var result []byte
	err := withTemporaryProxyDo(cfg, updateOutbound, func(proxy string) error {
		var err error
		result, err = fn(proxy)
		return err
	})
	return result, err
}

func withTemporaryProxyDo(cfg config.Config, updateOutbound string, fn func(proxy string) error) error {
	candidates := cfg.OutboundCandidateTags(updateOutbound)
	if len(candidates) == 0 {
		return fmt.Errorf("update_via proxy requires update_outbound or at least one custom outbound")
	}
	var failures []string
	for _, candidate := range candidates {
		if err := withTemporaryProxyCandidate(cfg, candidate, fn); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", candidate, err))
			continue
		}
		return nil
	}
	return fmt.Errorf("all outbound candidates failed: %s", strings.Join(failures, "; "))
}

func withTemporaryProxyCandidate(cfg config.Config, updateOutbound string, fn func(proxy string) error) error {
	if !singbox.BinaryExists(cfg.Main.SingBoxBin) {
		return fmt.Errorf("sing-box binary is missing or not executable: %s", cfg.Main.SingBoxBin)
	}
	port, err := freeLocalPort()
	if err != nil {
		return err
	}
	proxyJSON, err := singbox.GenerateProxyClient(cfg, updateOutbound, port)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "zakop-subscription-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "sing-box.json")
	if err := os.WriteFile(path, append(proxyJSON, '\n'), 0600); err != nil {
		return err
	}
	if err := singbox.CheckBinary(cfg.Main.SingBoxBin, path); err != nil {
		return err
	}

	var stderr bytes.Buffer
	cmd := exec.Command(cfg.Main.SingBoxBin, "run", "-c", path)
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	if err := waitForPort(port, 5*time.Second); err != nil {
		return fmt.Errorf("temporary sing-box did not start: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return fn(fmt.Sprintf("http://127.0.0.1:%d", port))
}

func runProviderScript(p config.Provider, proxy string) ([]byte, error) {
	const (
		maxStdout = 16 << 20
		maxStderr = 64 << 10
	)

	dir, err := os.MkdirTemp("", "zakop-provider-script-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	outputPath := filepath.Join(dir, "output.txt")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.ScriptPath)
	cmd.Env = providerScriptEnv(p, proxy, outputPath)

	var stdout cappedBuffer
	var stderr cappedBuffer
	stdout.limit = maxStdout
	stderr.limit = maxStderr
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("script timed out")
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("script failed: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("script failed: %w", err)
	}
	if data, ok, err := readProviderScriptOutput(outputPath, maxStdout); err != nil {
		return nil, err
	} else if ok {
		return data, nil
	}
	if stdout.truncated {
		return nil, fmt.Errorf("script output is too large")
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

func readProviderScriptOutput(path string, maxSize int64) ([]byte, bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if st.Size() == 0 {
		return nil, false, nil
	}
	if st.Size() > maxSize {
		return nil, false, fmt.Errorf("script output file is too large")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func providerScriptEnv(p config.Provider, proxy string, outputPath string) []string {
	env := append([]string{}, os.Environ()...)
	env = append(env,
		"ZAKOP_PROVIDER_NAME="+p.Name,
		"ZAKOP_PROVIDER_LABEL="+p.Label,
		"ZAKOP_PROVIDER_TYPE="+p.Type,
		"ZAKOP_PROVIDER_URL="+p.URL,
		"ZAKOP_PROVIDER_CACHE="+p.CachePath(),
		"ZAKOP_PROVIDER_OUTPUT="+outputPath,
		"ZAKOP_PROVIDER_UPDATE_VIA="+p.UpdateVia,
	)
	if proxy != "" {
		env = append(env,
			"ZAKOP_PROVIDER_PROXY="+proxy,
			"HTTP_PROXY="+proxy,
			"HTTPS_PROXY="+proxy,
			"ALL_PROXY="+proxy,
			"http_proxy="+proxy,
			"https_proxy="+proxy,
			"all_proxy="+proxy,
			"NO_PROXY=127.0.0.1,localhost,::1",
			"no_proxy=127.0.0.1,localhost,::1",
		)
	}
	return env
}

type cappedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		if len(p) > 0 {
			b.truncated = true
		}
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.Buffer.Write(p)
	return len(p), nil
}

func fetchURLWithCurl(rawURL string, proxy string) ([]byte, error) {
	if err := requireCommand("curl"); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "zakop-subscription-curl-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	const maxBody = 16 << 20
	outPath := filepath.Join(dir, "subscription.txt")
	args := []string{
		"-fsSL",
		"--connect-timeout", "15",
		"--max-time", "60",
		"--max-filesize", strconv.Itoa(maxBody),
		"--user-agent", "zakop/1",
		"--output", outPath,
	}
	if proxy != "" {
		args = append(args, "--proxy", proxy)
	} else {
		args = append(args, "--noproxy", "*")
	}
	args = append(args, rawURL)

	out, err := command("curl", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("curl failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	st, err := os.Stat(outPath)
	if err != nil {
		return nil, err
	}
	if st.Size() > maxBody {
		return nil, fmt.Errorf("subscription response is too large")
	}
	return os.ReadFile(outPath)
}

func downloadURLToFile(rawURL string, outputPath string, proxy string) error {
	if err := requireCommand("curl"); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}
	tmp := outputPath + ".tmp"
	_ = os.Remove(tmp)
	defer os.Remove(tmp)

	const maxBody = 64 << 20
	args := []string{
		"-fsSL",
		"--connect-timeout", "15",
		"--max-time", "300",
		"--max-filesize", strconv.Itoa(maxBody),
		"--user-agent", "zakop/1",
		"--output", tmp,
	}
	if proxy != "" {
		args = append(args, "--proxy", proxy)
	} else {
		args = append(args, "--noproxy", "*")
	}
	args = append(args, rawURL)

	out, err := command("curl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("curl failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	st, err := os.Stat(tmp)
	if err != nil {
		return err
	}
	if st.Size() == 0 {
		return fmt.Errorf("downloaded file is empty")
	}
	if st.Size() > maxBody {
		return fmt.Errorf("downloaded file is too large")
	}
	if err := os.Chmod(tmp, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, outputPath)
}

func freeLocalPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func waitForPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}

func loadConfigForManagement(path string) (config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config.Config{}, err
	}
	return config.Parse(string(data))
}

func compile(opts options) (config.Config, string, string, error) {
	cfg, err := config.LoadFile(opts.configPath)
	if err != nil {
		return config.Config{}, "", "", err
	}
	cidrs, err := provider.LoadRuleCIDRs(cfg)
	if err != nil {
		return config.Config{}, "", "", err
	}
	nftText, err := nft.Generate(nft.Input{Config: cfg, RuleCIDRs: cidrs})
	if err != nil {
		return config.Config{}, "", "", err
	}
	sbJSON, err := singbox.Generate(cfg)
	if err != nil {
		return config.Config{}, "", "", err
	}
	if err := validateGeneratedSingBox(sbJSON); err != nil {
		return config.Config{}, "", "", err
	}
	if err := os.MkdirAll(opts.outDir, 0755); err != nil {
		return config.Config{}, "", "", err
	}
	nftPath := nftPath(opts)
	sbPath := singBoxPath(opts)
	if err := os.WriteFile(nftPath, []byte(nftText), 0644); err != nil {
		return config.Config{}, "", "", err
	}
	if err := os.WriteFile(sbPath, append(sbJSON, '\n'), 0644); err != nil {
		return config.Config{}, "", "", err
	}
	return cfg, nftPath, sbPath, nil
}

func validateGeneratedSingBox(data []byte) error {
	raw := string(data)
	for _, forbidden := range []string{
		`"detour": "direct"`,
		`"domain_strategy"`,
		`"rule_set"`,
		`"rule-set"`,
		`"/tmp/sing-box/rulesets`,
	} {
		if strings.Contains(raw, forbidden) {
			return fmt.Errorf("generated sing-box config contains unsupported %s; zakop must not generate sing-box rule-set files", forbidden)
		}
	}
	return nil
}

func nftPath(opts options) string {
	return filepath.Join(opts.outDir, nftFileName)
}

func singBoxPath(opts options) string {
	return filepath.Join(opts.outDir, sbFileName)
}

func requireCommand(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("required command %q not found", name)
	}
	return nil
}

func command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	return cmd
}

func ensureRouting(mark string, table int) error {
	cfg := tproxy.Config{Mark: mark, Table: table}
	rules, routes, err := readRoutingState(cfg)
	if err != nil {
		return err
	}
	for _, c := range tproxy.PlanEnsure(rules, routes, cfg) {
		if out, err := command(c.Name, c.Args...).CombinedOutput(); err != nil {
			if strings.Contains(string(out), "File exists") {
				continue
			}
			return fmt.Errorf("%s %s failed: %w: %s", c.Name, strings.Join(c.Args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func readRoutingState(cfg tproxy.Config) (string, string, error) {
	rules, err := command("ip", "-4", "rule", "show").Output()
	if err != nil {
		return "", "", fmt.Errorf("ip rule show failed: %w", err)
	}
	routes, err := command("ip", "-4", "route", "show", "table", fmt.Sprintf("%d", cfg.Table)).CombinedOutput()
	if err != nil {
		if tproxy.RouteTableMissing(string(routes), exitCode(err)) {
			return string(rules), "", nil
		}
		return "", "", fmt.Errorf("ip route show table %d failed: %w", cfg.Table, err)
	}
	return string(rules), string(routes), nil
}

func deleteNftTable() error {
	_ = requireCommand("nft")
	_, err := command("nft", "delete", "table", "inet", "zakop").CombinedOutput()
	return err
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

func printPath(label string, path string) {
	if st, err := os.Stat(path); err == nil {
		fmt.Printf("%s: %s exists size=%d\n", label, path, st.Size())
		return
	}
	fmt.Printf("%s: %s missing\n", label, path)
}

func printCommand(name string, args ...string) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	if err != nil {
		fmt.Printf("%s %s: %v\n", name, strings.Join(args, " "), err)
	}
}

func printWarnings(cfg config.Config) {
	for _, warning := range cfg.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", warning)
	}
}

func cleanupRouting(mark string, table int) error {
	if err := requireCommand("ip"); err != nil {
		return err
	}
	cfg := tproxy.Config{Mark: mark, Table: table}
	for {
		rules, routes, err := readRoutingState(cfg)
		if err != nil {
			return err
		}
		commands := tproxy.PlanCleanup(rules, routes, cfg)
		if len(commands) == 0 {
			return nil
		}
		for _, c := range commands {
			if out, err := command(c.Name, c.Args...).CombinedOutput(); err != nil {
				return fmt.Errorf("%s %s failed: %w: %s", c.Name, strings.Join(c.Args, " "), err, strings.TrimSpace(string(out)))
			}
		}
	}
}
