package ruleengine

import (
	"testing"

	"github.com/elllkere/zakop/internal/config"
)

func TestDomainMatchTypes(t *testing.T) {
	tests := []struct {
		name string
		rule config.Rule
		host string
	}{
		{
			name: "equals",
			rule: baseRule(config.Rule{DomainEquals: []string{"youtube.com"}}),
			host: "youtube.com",
		},
		{
			name: "contains",
			rule: baseRule(config.Rule{DomainContains: []string{"youtube"}}),
			host: "notyoutube.com",
		},
		{
			name: "starts_with",
			rule: baseRule(config.Rule{DomainStartsWith: []string{"you"}}),
			host: "youtube.com",
		},
		{
			name: "ends_with",
			rule: baseRule(config.Rule{DomainEndsWith: []string{"youtube.com"}}),
			host: "notyoutube.com",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !MatchDomain(tc.rule, tc.host) {
				t.Fatalf("expected %s to match %s", tc.name, tc.host)
			}
		})
	}
}

func TestRootAndSubdomainsPattern(t *testing.T) {
	r := baseRule(config.Rule{
		DomainEquals:   []string{"youtube.com"},
		DomainEndsWith: []string{".youtube.com"},
	})
	for _, host := range []string{"youtube.com", "www.youtube.com"} {
		if !MatchDomain(r, host) {
			t.Fatalf("expected %s to match", host)
		}
	}
	for _, host := range []string{"notyoutube.com", "youtube.kz"} {
		if MatchDomain(r, host) {
			t.Fatalf("did not expect %s to match", host)
		}
	}
}

func TestContainsIncludeEqualsAndEndsWithExclude(t *testing.T) {
	r := baseRule(config.Rule{
		DomainContains:        []string{"youtube"},
		ExcludeDomainEquals:   []string{"youtube.kz"},
		ExcludeDomainEndsWith: []string{".youtube.kz"},
	})
	if !MatchDomain(r, "youtube.com") {
		t.Fatal("expected youtube.com to match")
	}
	if MatchDomain(r, "youtube.kz") {
		t.Fatal("youtube.kz must be excluded by equals")
	}
	if MatchDomain(r, "www.youtube.kz") {
		t.Fatal("www.youtube.kz must be excluded by ends_with")
	}
}

func TestRuleWithoutIncludeDoesNotMatch(t *testing.T) {
	r := baseRule(config.Rule{})
	if MatchDomain(r, "example.org") {
		t.Fatal("rule without includes must not match")
	}
}

func TestFirstMatchPriorityDirectWins(t *testing.T) {
	rules := []config.Rule{
		baseRule(config.Rule{Name: "direct", Priority: 10, Action: "direct", DNSMode: "real_ip", DomainEndsWith: []string{"youtube.com"}}),
		baseRule(config.Rule{Name: "proxy", Priority: 20, Action: "proxy", DNSMode: "fakeip", DomainContains: []string{"youtube"}}),
	}
	decision := DomainDecision(rules, "www.youtube.com")
	if decision.Action != "direct" {
		t.Fatalf("got %s, want direct", decision.Action)
	}
}

func TestFirstMatchPriorityProxyWins(t *testing.T) {
	rules := []config.Rule{
		baseRule(config.Rule{Name: "proxy", Priority: 10, Action: "proxy", DNSMode: "fakeip", DomainContains: []string{"youtube"}}),
		baseRule(config.Rule{Name: "direct", Priority: 20, Action: "direct", DNSMode: "real_ip", DomainEndsWith: []string{"youtube.com"}}),
	}
	decision := DomainDecision(rules, "www.youtube.com")
	if decision.Action != "proxy" || decision.DNSMode != "fakeip" {
		t.Fatalf("got %+v, want proxy fakeip", decision)
	}
}

func baseRule(r config.Rule) config.Rule {
	if r.Name == "" {
		r.Name = "rule"
	}
	if r.Action == "" {
		r.Action = "proxy"
	}
	if r.DNSMode == "" {
		r.DNSMode = "fakeip"
	}
	if r.Outbound == "" {
		r.Outbound = "proxy_default"
	}
	r.Enabled = true
	return r
}
