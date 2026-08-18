package importer

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/elllkere/zakop/internal/config"
)

type ApplyOptions struct {
	ConfigPath   string
	Source       string
	Subscription bool
	Replace      bool
}

func ApplyNodes(nodes []Node, opts ApplyOptions) (int, error) {
	if strings.TrimSpace(opts.ConfigPath) == "" {
		return 0, fmt.Errorf("config path is required")
	}
	if len(nodes) == 0 {
		return 0, fmt.Errorf("no nodes to apply")
	}

	data, err := os.ReadFile(opts.ConfigPath)
	if err != nil {
		return 0, err
	}
	chunks := parseChunks(string(data))
	used := collectOutboundTags(chunks)
	var matchedSubscriptionTags map[string][]string

	if opts.Replace && strings.TrimSpace(opts.Source) != "" {
		source := strings.TrimSpace(opts.Source)
		matchedSubscriptionTags = collectSubscriptionTags(chunks, source)
		filtered := chunks[:0]
		for _, chunk := range chunks {
			if chunk.Type == "outbound" && chunk.Options["subscription"] == source {
				continue
			}
			filtered = append(filtered, chunk)
		}
		chunks = filtered
		used = collectOutboundTags(chunks)
	}

	var appended []string
	for i, node := range nodes {
		outbound := node.Outbound
		if tag := takeSubscriptionTag(matchedSubscriptionTags, subscriptionNodeKey(outbound)); tag != "" {
			// A subscription refresh is allowed to change node parameters, but a
			// rule may already refer to this node's UCI tag. Keep the tag when the
			// protocol endpoint and credentials identify the same node.
			outbound.Tag = tag
		} else if strings.TrimSpace(outbound.Tag) == "" {
			outbound.Tag = uniqueTag(tagBase(node, opts, i), used)
		} else {
			outbound.Tag = uniqueTag(outbound.Tag, used)
		}
		if strings.TrimSpace(outbound.Label) == "" {
			outbound.Label = outbound.Tag
		}
		used[outbound.Tag] = struct{}{}
		appended = append(appended, formatOutbound(outbound, opts))
	}

	if opts.Subscription && opts.Source != "" {
		now := strconv.FormatInt(time.Now().Unix(), 10)
		for i := range chunks {
			if chunks[i].Type == "subscription" && chunks[i].Name == opts.Source {
				chunks[i].Text = setOption(chunks[i].Text, "last_update", now)
				chunks[i].Text = setOption(chunks[i].Text, "node_count", strconv.Itoa(len(appended)))
				chunks[i].Text = setOption(chunks[i].Text, "last_error", "")
			}
		}
	}

	var b strings.Builder
	for _, chunk := range chunks {
		if len(chunk.Text) == 0 {
			continue
		}
		for _, line := range chunk.Text {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
		b.WriteByte('\n')
	}
	for _, section := range appended {
		b.WriteString(section)
		if !strings.HasSuffix(section, "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}

	if _, err := config.Parse(b.String()); err != nil {
		return 0, fmt.Errorf("generated UCI is invalid: %w", err)
	}
	if err := os.WriteFile(opts.ConfigPath, []byte(b.String()), 0644); err != nil {
		return 0, err
	}
	return len(appended), nil
}

type chunk struct {
	Type    string
	Name    string
	Options map[string]string
	Text    []string
}

func parseChunks(data string) []chunk {
	var chunks []chunk
	var cur *chunk

	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "config ") || trimmed == "config" {
			chunks = append(chunks, chunk{Options: map[string]string{}, Text: []string{line}})
			cur = &chunks[len(chunks)-1]
			fields := splitFields(trimmed)
			if len(fields) >= 2 {
				cur.Type = fields[1]
			}
			if len(fields) >= 3 {
				cur.Name = fields[2]
			}
			continue
		}
		if cur == nil {
			if line == "" && len(chunks) == 0 {
				continue
			}
			chunks = append(chunks, chunk{Options: map[string]string{}, Text: []string{line}})
			cur = &chunks[len(chunks)-1]
			continue
		}
		cur.Text = append(cur.Text, line)
		fields := splitFields(strings.TrimSpace(line))
		if len(fields) == 3 && fields[0] == "option" {
			cur.Options[fields[1]] = fields[2]
		}
	}

	return chunks
}

func collectOutboundTags(chunks []chunk) map[string]struct{} {
	used := map[string]struct{}{
		config.BuiltinDirectOutbound:  {},
		config.BuiltinBlockedOutbound: {},
		"block":                       {},
		"proxy_default":               {},
	}
	for _, chunk := range chunks {
		if chunk.Type != "outbound" {
			continue
		}
		tag := strings.TrimSpace(firstNonEmpty(chunk.Options["tag"], chunk.Name))
		if tag != "" {
			used[tag] = struct{}{}
		}
	}
	return used
}

// collectSubscriptionTags returns existing tags grouped by the stable
// subscription-node identity. A slice is used because a subscription can
// legitimately contain duplicate nodes.
func collectSubscriptionTags(chunks []chunk, source string) map[string][]string {
	tags := make(map[string][]string)
	for _, chunk := range chunks {
		if chunk.Type != "outbound" || chunk.Options["subscription"] != source {
			continue
		}
		key := subscriptionNodeKey(config.Outbound{
			Type:     chunk.Options["type"],
			Server:   firstNonEmpty(chunk.Options["server"], chunk.Options["address"]),
			Port:     optionPort(chunk.Options["port"]),
			UUID:     chunk.Options["uuid"],
			Password: chunk.Options["password"],
			Method:   firstNonEmpty(chunk.Options["method"], chunk.Options["shadowsocks_encrypt_method"]),
		})
		tag := strings.TrimSpace(firstNonEmpty(chunk.Options["tag"], chunk.Name))
		if key != "" && tag != "" {
			tags[key] = append(tags[key], tag)
		}
	}
	return tags
}

func takeSubscriptionTag(tags map[string][]string, key string) string {
	if len(tags) == 0 || key == "" {
		return ""
	}
	matched := tags[key]
	if len(matched) == 0 {
		return ""
	}
	tag := matched[0]
	tags[key] = matched[1:]
	return tag
}

// subscriptionNodeKey intentionally excludes mutable transport and display
// fields. Providers commonly change those during a refresh, while the
// protocol endpoint plus its credential identifies the same logical node.
func subscriptionNodeKey(outbound config.Outbound) string {
	typ := strings.ToLower(strings.TrimSpace(outbound.Type))
	server := strings.ToLower(strings.TrimSpace(outbound.Server))
	if typ == "" || server == "" || outbound.Port <= 0 {
		return ""
	}
	credential := ""
	switch typ {
	case "vless":
		credential = strings.TrimSpace(outbound.UUID)
	case "hysteria2", "trojan":
		credential = strings.TrimSpace(outbound.Password)
	case "shadowsocks":
		credential = strings.TrimSpace(outbound.Method) + "\x00" + strings.TrimSpace(outbound.Password)
	default:
		return ""
	}
	if credential == "" {
		return ""
	}
	return typ + "\x00" + server + "\x00" + strconv.Itoa(outbound.Port) + "\x00" + credential
}

func optionPort(value string) int {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return port
}

func tagBase(node Node, opts ApplyOptions, index int) string {
	if opts.Subscription && opts.Source != "" {
		return opts.Source + "_" + shortHash(firstNonEmpty(node.Raw, node.Outbound.Type, node.Outbound.Server))
	}
	return firstNonEmpty(node.Outbound.Label, node.Outbound.Server, fmt.Sprintf("node_%d", index+1))
}

func uniqueTag(base string, used map[string]struct{}) string {
	tag := sanitizeTag(base)
	if tag == "" {
		tag = "node"
	}
	candidate := tag
	for i := 2; ; i++ {
		if _, ok := used[candidate]; !ok {
			return candidate
		}
		candidate = fmt.Sprintf("%s_%d", tag, i)
	}
}

var tagCharRE = regexp.MustCompile(`[^A-Za-z0-9_]+`)

func sanitizeTag(value string) string {
	value = strings.TrimSpace(value)
	value = tagCharRE.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return ""
	}
	if value[0] >= '0' && value[0] <= '9' {
		value = "node_" + value
	}
	if len(value) > 48 {
		value = value[:48]
		value = strings.TrimRight(value, "_")
	}
	return value
}

func shortHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:10]
}

func formatOutbound(out config.Outbound, opts ApplyOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "config outbound %s\n", quote(out.Tag))
	writeOption(&b, "tag", out.Tag)
	writeOption(&b, "label", out.Label)
	writeOption(&b, "type", out.Type)
	writeOption(&b, "imported", "1")
	if opts.Subscription {
		writeOption(&b, "subscription", opts.Source)
	} else if opts.Source != "" {
		writeOption(&b, "import_source", opts.Source)
	}
	writeOption(&b, "server", out.Server)
	if out.Port > 0 {
		writeOption(&b, "port", strconv.Itoa(out.Port))
	}
	writeOption(&b, "uuid", out.UUID)
	writeOption(&b, "flow", out.Flow)
	writeBool(&b, "tls", out.TLS)
	writeOption(&b, "server_name", out.ServerName)
	writeBool(&b, "reality", out.Reality)
	writeOption(&b, "reality_public_key", out.RealityPublicKey)
	writeOption(&b, "reality_short_id", out.RealityShortID)
	writeList(&b, "alpn", out.ALPN)
	writeOption(&b, "tls_min_version", out.TLSMinVersion)
	writeOption(&b, "tls_max_version", out.TLSMaxVersion)
	writeList(&b, "tls_cipher_suites", out.TLSCipherSuites)
	writeBool(&b, "ech", out.ECH)
	writeList(&b, "ech_config", out.ECHConfig)
	writeOption(&b, "ech_config_path", out.ECHConfigPath)
	writeOption(&b, "utls_fingerprint", out.UTLSFingerprint)
	writeOption(&b, "transport", out.Transport)
	writeOption(&b, "packet_encoding", out.PacketEncoding)
	writeOption(&b, "grpc_service_name", out.GRPCServiceName)
	writeList(&b, "http_host", out.HTTPHost)
	writeOption(&b, "httpupgrade_host", out.HTTPUpgradeHost)
	writeOption(&b, "http_path", out.HTTPPath)
	writeOption(&b, "http_method", out.HTTPMethod)
	writeOption(&b, "ws_host", out.WSHost)
	writeOption(&b, "ws_path", out.WSPath)
	if out.WSEarlyData > 0 {
		writeOption(&b, "websocket_early_data", strconv.Itoa(out.WSEarlyData))
	}
	writeOption(&b, "websocket_early_data_header", out.WSEarlyDataHeader)
	writeOption(&b, "password", out.Password)
	writeOption(&b, "method", out.Method)
	writeBool(&b, "insecure", out.Insecure)
	writeOption(&b, "hysteria_obfs_type", out.HysteriaObfsType)
	writeOption(&b, "hysteria_obfs_password", out.HysteriaObfsPassword)
	if out.HysteriaUpMbps > 0 {
		writeOption(&b, "hysteria_up_mbps", strconv.Itoa(out.HysteriaUpMbps))
	}
	if out.HysteriaDownMbps > 0 {
		writeOption(&b, "hysteria_down_mbps", strconv.Itoa(out.HysteriaDownMbps))
	}
	return b.String()
}

func writeOption(b *strings.Builder, name string, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(b, "\toption %s %s\n", name, quote(value))
}

func writeBool(b *strings.Builder, name string, value bool) {
	if value {
		writeOption(b, name, "1")
	}
}

func writeList(b *strings.Builder, name string, values []string) {
	for _, value := range values {
		writeOption(b, name, value)
	}
}

func quote(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return "\"" + value + "\""
}

func setOption(lines []string, name string, value string) []string {
	replaced := false
	prefix := "option " + name + " "
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines[i] = "\toption " + name + " " + quote(value)
			replaced = true
		}
	}
	if !replaced {
		lines = append(lines, "\toption "+name+" "+quote(value))
	}
	return lines
}

func splitFields(s string) []string {
	var fields []string
	var b strings.Builder
	var quote rune
	escaped := false
	inField := false

	flush := func() {
		if inField {
			fields = append(fields, b.String())
			b.Reset()
			inField = false
		}
	}

	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			inField = true
			escaped = false
			continue
		}
		switch {
		case quote != 0:
			if r == '\\' {
				escaped = true
				inField = true
				continue
			}
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
			inField = true
		case r == '\'' || r == '"':
			quote = r
			inField = true
		case r == ' ' || r == '\t':
			flush()
		default:
			b.WriteRune(r)
			inField = true
		}
	}
	flush()
	return fields
}
