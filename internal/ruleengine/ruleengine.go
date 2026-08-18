package ruleengine

import (
	"strings"

	"github.com/elllkere/zakop/internal/config"
)

type Decision struct {
	Rule     *config.Rule
	Action   string
	DNSMode  string
	Outbound string
}

func MatchDomain(r config.Rule, name string) bool {
	if !r.Enabled {
		return false
	}
	name = normalizeDomain(name)
	if name == "" {
		return false
	}
	if domainExcluded(r, name) {
		return false
	}
	if hasDomainIncludes(r) {
		return matchAnyDomain(r.DomainEquals, name, matchEquals) ||
			matchAnyDomain(r.DomainContains, name, matchContains) ||
			matchAnyDomain(r.DomainStartsWith, name, matchStartsWith) ||
			matchAnyDomain(r.DomainEndsWith, name, matchEndsWith)
	}
	return false
}

func DomainDecision(rules []config.Rule, name string) Decision {
	for i := range rules {
		if !MatchDomain(rules[i], name) {
			continue
		}
		r := &rules[i]
		return Decision{
			Rule:     r,
			Action:   r.Action,
			DNSMode:  effectiveDNSMode(*r),
			Outbound: r.Outbound,
		}
	}
	return Decision{Action: "direct", DNSMode: "real_ip"}
}

func HasIPMatch(r config.Rule) bool {
	return r.Enabled && len(r.IPCIDRs)+len(r.Files)+len(r.IPProviders)+len(r.Providers) > 0
}

func HasDomainIncludes(r config.Rule) bool {
	return hasDomainIncludes(r)
}

func effectiveDNSMode(r config.Rule) string {
	if r.Action == "direct" {
		return "real_ip"
	}
	if r.Action == "block" {
		return "block"
	}
	if r.DNSMode != "auto" {
		return r.DNSMode
	}
	if r.Action == "proxy" && hasDomainIncludes(r) {
		return "fakeip"
	}
	return "real_ip"
}

func hasDomainIncludes(r config.Rule) bool {
	return len(r.DomainEquals)+len(r.DomainContains)+len(r.DomainStartsWith)+len(r.DomainEndsWith) > 0
}

func domainExcluded(r config.Rule, name string) bool {
	return matchAnyDomain(r.ExcludeDomainEquals, name, matchEquals) ||
		matchAnyDomain(r.ExcludeDomainContains, name, matchContains) ||
		matchAnyDomain(r.ExcludeDomainStartsWith, name, matchStartsWith) ||
		matchAnyDomain(r.ExcludeDomainEndsWith, name, matchEndsWith)
}

func matchAnyDomain(patterns []string, name string, fn func(string, string) bool) bool {
	for _, pattern := range patterns {
		if fn(name, normalizeDomain(pattern)) {
			return true
		}
	}
	return false
}

func normalizeDomain(name string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(name)), ".")
}

func matchEquals(name string, pattern string) bool {
	return name == pattern
}

func matchContains(name string, pattern string) bool {
	return strings.Contains(name, pattern)
}

func matchStartsWith(name string, pattern string) bool {
	return strings.HasPrefix(name, pattern)
}

func matchEndsWith(name string, pattern string) bool {
	return strings.HasSuffix(name, pattern)
}
