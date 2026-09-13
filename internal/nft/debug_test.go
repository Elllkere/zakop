//go:build networkdebug

package nft

import (
	"regexp"
	"strings"
	"testing"
)

func TestDebugObservesWithoutChangingVerdicts(t *testing.T) {
	cfg := testConfig()
	cfg.Main.NFTCounters = false
	plain, err := generate(Input{Config: cfg}, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "debug_") {
		t.Fatal("debug enabled by default")
	}
	debug, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"debug_already_marked", "debug_dnat_marked", "debug_unmatched_direct", "debug_tcp_attempt", "debug_udp_attempt", "debug_tproxy_fallthrough"} {
		if !strings.Contains(debug, label) {
			t.Errorf("missing %s", label)
		}
	}
	var filtered []string
	inDebugChain := false
	for _, line := range strings.Split(debug, "\n") {
		if strings.Contains(line, "chain debug_late_prerouting") {
			inDebugChain = true
			continue
		}
		if inDebugChain {
			if strings.TrimSpace(line) == "}" {
				inDebugChain = false
			}
			continue
		}
		if strings.Contains(line, "counter debug_") {
			continue
		}
		if strings.Contains(line, "comment \"debug_") {
			if strings.Contains(line, "return") || strings.Contains(line, " set ") || strings.Contains(line, "accept") {
				t.Fatalf("observation changes verdict: %s", line)
			}
			continue
		}
		line = regexp.MustCompile(` counter name debug_[a-zA-Z0-9_]+`).ReplaceAllString(line, "")
		filtered = append(filtered, strings.ReplaceAll(line, " counter", ""))
	}
	if strings.Join(filtered, "\n") != plain {
		t.Fatal("debug changed non-counter routing rules")
	}
}
