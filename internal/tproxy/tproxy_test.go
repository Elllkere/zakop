package tproxy

import (
	"reflect"
	"testing"
)

func TestPlanEnsureIdempotent(t *testing.T) {
	cfg := Config{Mark: "0x101", Table: 101}
	rules := "32765: from all fwmark 0x101 lookup 101\n"
	routes := "local default dev lo scope host\n"

	if got := PlanEnsure(rules, routes, cfg); len(got) != 0 {
		t.Fatalf("expected no commands for existing state, got %+v", got)
	}

	got := PlanEnsure("", "", cfg)
	want := []Command{RuleAddCommand(cfg), RouteAddCommand(cfg)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestPlanCleanupOnlyZakopOwned(t *testing.T) {
	cfg := Config{Mark: "0x101", Table: 101}
	rules := "" +
		"100: from all fwmark 0x999 lookup 101\n" +
		"101: from all fwmark 0x101 lookup 200\n" +
		"102: from all fwmark 0x101 lookup 101\n"
	routes := "default via 192.0.2.1 dev eth0\nlocal default dev lo scope host\n"

	got := PlanCleanup(rules, routes, cfg)
	want := []Command{RuleDelCommand(cfg), RouteDelCommand(cfg)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestCommandsUseIPv4(t *testing.T) {
	cfg := Config{Mark: "0x101", Table: 101}
	for _, cmd := range []Command{
		RuleAddCommand(cfg),
		RuleDelCommand(cfg),
		RouteAddCommand(cfg),
		RouteDelCommand(cfg),
	} {
		if len(cmd.Args) == 0 || cmd.Args[0] != "-4" {
			t.Fatalf("expected IPv4 command, got %+v", cmd)
		}
	}
}

func TestRouteTableMissing(t *testing.T) {
	if !RouteTableMissing("Error: ipv4: FIB table does not exist.\nDump terminated\n", 2) {
		t.Fatal("expected missing FIB table to be treated as missing")
	}
	if RouteTableMissing("permission denied", 1) {
		t.Fatal("permission denied must not be treated as missing")
	}
}

func TestRulePresentExactMarkAndTable(t *testing.T) {
	cfg := Config{Mark: "0x101", Table: 101}
	for _, tt := range []struct {
		rule string
		want bool
	}{
		{"32765: from all fwmark 0x101 lookup 101", true},
		{"32765: from all fwmark 257 lookup 101", true},
		{"32765: from all fwmark 0x101/0xffffffff lookup 101", true},
		{"32765: from all fwmark 0x1010 lookup 101", false},
		{"32765: from all fwmark 0x101 lookup 1010", false},
		{"32765: from all fwmark 0x101/0xffff lookup 101", false},
		{"32765: from all fwmark 101 lookup 101", false},
		{"32765: not from all fwmark 0x101 lookup 101", false},
		{"32765: from 192.168.8.1 fwmark 0x101 lookup 101", false},
		{"32765: from all fwmark 0x101 lookup 101 suppress_prefixlength 0", false},
		{"32765: from all fwmark 0x101 lookup 101 proto static", true},
	} {
		if got := RulePresent(tt.rule, cfg); got != tt.want {
			t.Errorf("%s: got %v want %v", tt.rule, got, tt.want)
		}
	}
}

func TestRoutePresentExactLoopback(t *testing.T) {
	if RoutePresent("local default dev loopback-other scope host") {
		t.Fatal("matched device prefix")
	}
	if !RoutePresent("local 0.0.0.0/0 dev lo scope host") {
		t.Fatal("missed local default")
	}
}
