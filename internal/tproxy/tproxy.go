package tproxy

import (
	"strconv"
	"strings"
)

type Config struct {
	Mark  string
	Table int
}

type Command struct {
	Name string
	Args []string
}

type State struct {
	RulePresent  bool
	RoutePresent bool
}

func RuleAddCommand(cfg Config) Command {
	return Command{Name: "ip", Args: []string{"-4", "rule", "add", "fwmark", cfg.Mark, "table", tableString(cfg)}}
}

func RuleDelCommand(cfg Config) Command {
	return Command{Name: "ip", Args: []string{"-4", "rule", "del", "fwmark", cfg.Mark, "table", tableString(cfg)}}
}

func RouteAddCommand(cfg Config) Command {
	return Command{Name: "ip", Args: []string{"-4", "route", "add", "local", "default", "dev", "lo", "table", tableString(cfg)}}
}

func RouteDelCommand(cfg Config) Command {
	return Command{Name: "ip", Args: []string{"-4", "route", "del", "local", "default", "dev", "lo", "table", tableString(cfg)}}
}

func Inspect(ruleShow, routeShow string, cfg Config) State {
	return State{
		RulePresent:  RulePresent(ruleShow, cfg),
		RoutePresent: RoutePresent(routeShow),
	}
}

func PlanEnsure(ruleShow, routeShow string, cfg Config) []Command {
	state := Inspect(ruleShow, routeShow, cfg)
	var commands []Command
	if !state.RulePresent {
		commands = append(commands, RuleAddCommand(cfg))
	}
	if !state.RoutePresent {
		commands = append(commands, RouteAddCommand(cfg))
	}
	return commands
}

func PlanCleanup(ruleShow, routeShow string, cfg Config) []Command {
	state := Inspect(ruleShow, routeShow, cfg)
	var commands []Command
	if state.RulePresent {
		commands = append(commands, RuleDelCommand(cfg))
	}
	if state.RoutePresent {
		commands = append(commands, RouteDelCommand(cfg))
	}
	return commands
}

func RulePresent(ruleShow string, cfg Config) bool {
	wantMark, err := strconv.ParseUint(cfg.Mark, 0, 32)
	if err != nil {
		return false
	}
	table := tableString(cfg)
	for _, raw := range strings.Split(ruleShow, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if i := strings.Index(line, ":"); i >= 0 {
			line = strings.TrimSpace(line[i+1:])
		}
		fields := strings.Fields(line)
		// A restricted or inverted rule is not our unconditional source rule.
		if len(fields) < 6 || fields[0] != "from" || fields[1] != "all" || fields[2] != "fwmark" || (fields[4] != "lookup" && fields[4] != "table") {
			continue
		}
		if len(fields) != 6 && !(len(fields) == 8 && (fields[6] == "proto" || fields[6] == "protocol")) {
			continue
		}
		markOK, tableOK := false, false
		for i := 0; i+1 < len(fields); i++ {
			switch fields[i] {
			case "fwmark":
				parts := strings.Split(fields[i+1], "/")
				value, e := strconv.ParseUint(parts[0], 0, 32)
				mask := uint64(0xffffffff)
				if len(parts) == 2 {
					mask, err = strconv.ParseUint(parts[1], 0, 32)
				} else {
					err = nil
				}
				markOK = len(parts) <= 2 && e == nil && err == nil && value == wantMark && mask == 0xffffffff
			case "lookup", "table":
				tableOK = fields[i+1] == table
			}
		}
		if markOK && tableOK {
			return true
		}
	}
	return false
}

func RoutePresent(routeShow string) bool {
	for _, raw := range strings.Split(routeShow, "\n") {
		line := strings.TrimSpace(raw)
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "local" || (fields[1] != "default" && fields[1] != "0.0.0.0/0") {
			continue
		}
		for i := 2; i+1 < len(fields); i++ {
			if fields[i] == "dev" && fields[i+1] == "lo" {
				return true
			}
		}
	}
	return false
}

func RouteTableMissing(output string, exitCode int) bool {
	if exitCode == 2 {
		return true
	}
	output = strings.ToLower(output)
	return strings.Contains(output, "fib table does not exist") ||
		strings.Contains(output, "cannot find device") ||
		strings.Contains(output, "no such file or directory")
}

func tableString(cfg Config) string {
	return strconv.Itoa(cfg.Table)
}
