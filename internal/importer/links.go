package importer

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/elllkere/zakop/internal/config"
)

type Node struct {
	Outbound config.Outbound
	Raw      string
}

func ParseLinks(data string) ([]Node, error) {
	text := decodeSubscriptionText(strings.TrimSpace(data))
	var nodes []Node
	var errs []string

	for _, raw := range strings.Split(text, "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		if !strings.Contains(raw, "://") {
			continue
		}
		node, err := ParseLink(raw)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		nodes = append(nodes, node)
	}

	if len(nodes) == 0 {
		if len(errs) > 0 {
			return nil, fmt.Errorf("no supported nodes parsed: %s", strings.Join(errs, "; "))
		}
		return nil, fmt.Errorf("no supported nodes found")
	}
	return nodes, nil
}

func ParseLink(raw string) (Node, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, err
	}

	var out config.Outbound
	switch strings.ToLower(u.Scheme) {
	case "vless":
		out, err = parseVLESS(u)
	case "hysteria2", "hy2":
		out, err = parseHysteria2(u)
	case "ss":
		out, err = parseShadowsocks(raw)
	case "trojan":
		out, err = parseTrojan(u)
	default:
		err = fmt.Errorf("unsupported import scheme %q", u.Scheme)
	}
	if err != nil {
		return Node{}, fmt.Errorf("%s: %w", schemePrefix(raw), err)
	}
	return Node{Outbound: out, Raw: raw}, nil
}

func parseVLESS(u *url.URL) (config.Outbound, error) {
	server, port, err := serverPort(u)
	if err != nil {
		return config.Outbound{}, err
	}
	uuid := strings.TrimSpace(u.User.Username())
	if uuid == "" {
		return config.Outbound{}, fmt.Errorf("uuid is required")
	}

	q := u.Query()
	out := config.Outbound{
		Enabled:        true,
		Type:           "vless",
		Label:          linkLabel(u, server),
		Server:         server,
		Port:           port,
		UUID:           uuid,
		Flow:           firstQuery(q, "flow"),
		ServerName:     firstQuery(q, "sni", "serverName", "servername", "peer"),
		PacketEncoding: firstQuery(q, "packetEncoding", "packet_encoding"),
	}
	security := strings.ToLower(firstQuery(q, "security"))
	out.TLS = parseBool(firstQuery(q, "tls")) || security == "tls" || security == "reality"
	out.Reality = parseBool(firstQuery(q, "reality")) || security == "reality"
	out.RealityPublicKey = firstQuery(q, "pbk", "publicKey", "public_key", "reality_public_key")
	out.RealityShortID = firstQuery(q, "sid", "shortId", "short_id", "reality_short_id")
	out.UTLSFingerprint = firstQuery(q, "fp", "fingerprint", "utls", "utls_fingerprint")
	out.ALPN = splitCSV(firstQuery(q, "alpn"))
	out.Insecure = parseBool(firstQuery(q, "allowInsecure", "allow_insecure", "insecure"))
	applyTransport(&out, q)
	return out, nil
}

func parseHysteria2(u *url.URL) (config.Outbound, error) {
	server, port, err := serverPort(u)
	if err != nil {
		return config.Outbound{}, err
	}
	password := strings.TrimSpace(u.User.Username())
	if password == "" {
		password = firstQuery(u.Query(), "password", "auth")
	}
	if password == "" {
		return config.Outbound{}, fmt.Errorf("password is required")
	}

	q := u.Query()
	out := config.Outbound{
		Enabled:              true,
		Type:                 "hysteria2",
		Label:                linkLabel(u, server),
		Server:               server,
		Port:                 port,
		Password:             password,
		ServerName:           firstQuery(q, "sni", "serverName", "servername", "peer"),
		Insecure:             parseBool(firstQuery(q, "insecure", "allowInsecure", "allow_insecure")),
		HysteriaObfsType:     firstQuery(q, "obfs", "obfs_type", "obfs-type"),
		HysteriaObfsPassword: firstQuery(q, "obfs-password", "obfs_password"),
	}
	if n, err := atoi(firstQuery(q, "upmbps", "up_mbps")); err == nil {
		out.HysteriaUpMbps = n
	}
	if n, err := atoi(firstQuery(q, "downmbps", "down_mbps")); err == nil {
		out.HysteriaDownMbps = n
	}
	return out, nil
}

func parseTrojan(u *url.URL) (config.Outbound, error) {
	server, port, err := serverPort(u)
	if err != nil {
		return config.Outbound{}, err
	}
	password := strings.TrimSpace(u.User.Username())
	if password == "" {
		return config.Outbound{}, fmt.Errorf("password is required")
	}

	q := u.Query()
	security := strings.ToLower(firstQuery(q, "security"))
	out := config.Outbound{
		Enabled:    true,
		Type:       "trojan",
		Label:      linkLabel(u, server),
		Server:     server,
		Port:       port,
		Password:   password,
		TLS:        security != "none",
		ServerName: firstQuery(q, "sni", "serverName", "servername", "peer"),
		Insecure:   parseBool(firstQuery(q, "allowInsecure", "allow_insecure", "insecure")),
		ALPN:       splitCSV(firstQuery(q, "alpn")),
	}
	applyTransport(&out, q)
	return out, nil
}

func parseShadowsocks(raw string) (config.Outbound, error) {
	withoutScheme := strings.TrimPrefix(raw, "ss://")
	fragment := ""
	if i := strings.IndexByte(withoutScheme, '#'); i >= 0 {
		fragment = withoutScheme[i+1:]
		withoutScheme = withoutScheme[:i]
	}
	if i := strings.IndexByte(withoutScheme, '?'); i >= 0 {
		withoutScheme = withoutScheme[:i]
	}

	if strings.Contains(withoutScheme, "@") {
		u, err := url.Parse(raw)
		if err != nil {
			return config.Outbound{}, err
		}
		server, port, err := serverPort(u)
		if err != nil {
			return config.Outbound{}, err
		}
		userInfo := u.User.String()
		if decoded, ok := decodeBase64Loose(userInfo); ok && strings.Contains(decoded, ":") {
			userInfo = decoded
		}
		method, password, ok := strings.Cut(userInfo, ":")
		if !ok {
			return config.Outbound{}, fmt.Errorf("method and password are required")
		}
		return config.Outbound{
			Enabled:  true,
			Type:     "shadowsocks",
			Label:    linkLabel(u, server),
			Server:   server,
			Port:     port,
			Method:   method,
			Password: password,
		}, nil
	}

	decoded, ok := decodeBase64Loose(withoutScheme)
	if !ok {
		return config.Outbound{}, fmt.Errorf("invalid SIP002 payload")
	}
	methodPassword, hostPort, ok := strings.Cut(decoded, "@")
	if !ok {
		return config.Outbound{}, fmt.Errorf("server address is required")
	}
	method, password, ok := strings.Cut(methodPassword, ":")
	if !ok {
		return config.Outbound{}, fmt.Errorf("method and password are required")
	}
	host, portText, err := net.SplitHostPort(hostPort)
	if err != nil {
		return config.Outbound{}, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return config.Outbound{}, fmt.Errorf("invalid port %q", portText)
	}
	if decodedFragment, err := url.QueryUnescape(fragment); err == nil {
		fragment = decodedFragment
	}
	return config.Outbound{
		Enabled:  true,
		Type:     "shadowsocks",
		Label:    firstNonEmpty(strings.TrimSpace(fragment), host),
		Server:   strings.Trim(host, "[]"),
		Port:     port,
		Method:   method,
		Password: password,
	}, nil
}

func applyTransport(out *config.Outbound, q url.Values) {
	network := strings.ToLower(firstQuery(q, "type", "network", "transport"))
	switch network {
	case "", "tcp":
		if network == "tcp" {
			out.Transport = "tcp"
		}
	case "ws", "websocket":
		out.Transport = "ws"
		out.WSHost = firstQuery(q, "host")
		out.WSPath = firstQuery(q, "path")
	case "grpc":
		out.Transport = "grpc"
		out.GRPCServiceName = firstQuery(q, "serviceName", "service_name", "grpc_service_name")
	case "http":
		out.Transport = "http"
		out.HTTPHost = splitCSV(firstQuery(q, "host"))
		out.HTTPPath = firstQuery(q, "path")
	case "httpupgrade":
		out.Transport = "httpupgrade"
		out.HTTPUpgradeHost = firstQuery(q, "host")
		out.HTTPPath = firstQuery(q, "path")
	case "quic":
		out.Transport = "quic"
	}
}

func decodeSubscriptionText(text string) string {
	if strings.Contains(text, "://") {
		return text
	}
	compact := strings.Join(strings.Fields(text), "")
	if decoded, ok := decodeBase64Loose(compact); ok && strings.Contains(decoded, "://") {
		return decoded
	}
	return text
}

func decodeBase64Loose(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	encodings := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	}
	for _, enc := range encodings {
		if out, err := enc.DecodeString(s); err == nil {
			return string(out), true
		}
	}
	if m := len(s) % 4; m != 0 {
		padded := s + strings.Repeat("=", 4-m)
		for _, enc := range []*base64.Encoding{base64.URLEncoding, base64.StdEncoding} {
			if out, err := enc.DecodeString(padded); err == nil {
				return string(out), true
			}
		}
	}
	return "", false
}

func serverPort(u *url.URL) (string, int, error) {
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return "", 0, fmt.Errorf("server is required")
	}
	portText := u.Port()
	if portText == "" {
		return "", 0, fmt.Errorf("port is required")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("invalid port %q", portText)
	}
	return host, port, nil
}

func linkLabel(u *url.URL, fallback string) string {
	label := strings.TrimSpace(u.Fragment)
	if label == "" && u.RawFragment != "" {
		label = strings.TrimSpace(u.RawFragment)
	}
	if decoded, err := url.QueryUnescape(label); err == nil {
		label = decoded
	}
	return firstNonEmpty(label, fallback)
}

func firstQuery(q url.Values, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(q.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func splitCSV(value string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func atoi(value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, fmt.Errorf("empty integer")
	}
	return strconv.Atoi(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func schemePrefix(raw string) string {
	if i := strings.Index(raw, "://"); i >= 0 {
		return raw[:i] + "://"
	}
	return "import"
}
