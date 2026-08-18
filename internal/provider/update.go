package provider

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/elllkere/zakop/internal/config"
	"github.com/elllkere/zakop/internal/policy"
)

func NormalizeDownloadedList(provider config.Provider, data []byte) ([]string, error) {
	switch provider.Type {
	case "domain":
		return normalizeDomainList(data), nil
	case "ip":
		return normalizeIPList(data)
	default:
		return nil, fmt.Errorf("provider %q has unsupported type %q", provider.Name, provider.Type)
	}
}

func WriteCache(provider config.Provider, items []string) (string, error) {
	path := provider.CachePath()
	var b strings.Builder
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		b.WriteString(item)
		b.WriteByte('\n')
	}
	data := []byte(b.String())
	if err := writeCacheFile(path, data); err != nil {
		return "", err
	}
	if err := provider.WritePersistentCache(data); err != nil {
		return "", err
	}
	return path, nil
}

func writeCacheFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func UpdateMetadata(configPath string, providerName string, localPath string, itemCount int) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	chunks := parseChunks(string(data))
	now := strconv.FormatInt(time.Now().Unix(), 10)
	var found bool
	for i := range chunks {
		if chunks[i].Type != "provider" || (chunks[i].Name != providerName && chunks[i].Options["name"] != providerName) {
			continue
		}
		chunks[i].Text = setOption(chunks[i].Text, "local_path", localPath)
		chunks[i].Text = setOption(chunks[i].Text, "last_update", now)
		chunks[i].Text = setOption(chunks[i].Text, "item_count", strconv.Itoa(itemCount))
		chunks[i].Text = setOption(chunks[i].Text, "last_error", "")
		found = true
		break
	}
	if !found {
		return fmt.Errorf("provider %q not found", providerName)
	}

	var b strings.Builder
	for _, chunk := range chunks {
		for _, line := range chunk.Text {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	if _, err := config.Parse(b.String()); err != nil {
		return fmt.Errorf("generated UCI is invalid: %w", err)
	}
	return os.WriteFile(configPath, []byte(b.String()), 0644)
}

func normalizeDomainList(data []byte) []string {
	values := cleanLines(data)
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = normalizeDomainValue(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeIPList(data []byte) ([]string, error) {
	values := cleanLines(data)
	cidrs := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		cidr, err := policy.ParseIPv4CIDR(value)
		if err != nil {
			if strings.Contains(value, ":") {
				continue
			}
			return nil, err
		}
		cidrs = append(cidrs, cidr)
	}
	return policy.CIDRStrings(policy.NormalizeIPv4CIDRs(cidrs)), nil
}

func cleanLines(data []byte) []string {
	var out []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(stripLineComment(scanner.Text()))
		for _, value := range strings.Fields(line) {
			if value != "" {
				out = append(out, value)
			}
		}
	}
	return out
}

func normalizeDomainValue(value string) string {
	value = strings.TrimRight(strings.ToLower(strings.TrimSpace(value)), ".")
	value = strings.TrimPrefix(value, "*.")
	value = strings.TrimPrefix(value, ".")
	return value
}

func stripLineComment(line string) string {
	if i := strings.IndexByte(line, '#'); i >= 0 {
		line = line[:i]
	}
	return line
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
			chunks = append(chunks, chunk{Options: map[string]string{}, Text: []string{line}})
			cur = &chunks[len(chunks)-1]
			continue
		}
		cur.Text = append(cur.Text, line)
		fields := splitFields(trimmed)
		if len(fields) == 3 && fields[0] == "option" {
			cur.Options[fields[1]] = fields[2]
		}
	}
	return chunks
}

func setOption(lines []string, name string, value string) []string {
	var out []string
	written := false
	prefix := "option " + name + " "
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			if strings.TrimSpace(value) != "" {
				out = append(out, "\toption "+name+" "+quote(value))
			}
			written = true
			continue
		}
		out = append(out, line)
	}
	if !written && strings.TrimSpace(value) != "" {
		out = append(out, "\toption "+name+" "+quote(value))
	}
	return out
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
		if quote != 0 {
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
				inField = true
				continue
			}
			b.WriteRune(r)
			inField = true
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			inField = true
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		b.WriteRune(r)
		inField = true
	}
	flush()
	return fields
}

func quote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return "\"" + s + "\""
}
