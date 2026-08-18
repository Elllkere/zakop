package provider

import (
	"fmt"
	"net"
	"os"

	"github.com/elllkere/zakop/internal/config"
	"github.com/elllkere/zakop/internal/policy"
	"github.com/elllkere/zakop/internal/ruleengine"
)

func LoadRuleCIDRs(cfg config.Config) (map[int][]*net.IPNet, error) {
	out := map[int][]*net.IPNet{}
	for i, rule := range cfg.EffectiveRules() {
		if !ruleengine.HasIPMatch(rule) {
			continue
		}
		var all []*net.IPNet
		for _, value := range rule.IPCIDRs {
			cidr, err := policy.ParseIPv4CIDR(value)
			if err != nil {
				return nil, err
			}
			all = append(all, cidr)
		}
		for _, path := range rule.Files {
			cidrs, err := policy.LoadIPv4CIDRsFile(path)
			if err != nil {
				return nil, err
			}
			all = append(all, cidrs...)
		}
		for _, providerName := range append(rule.IPProviders, rule.Providers...) {
			provider, ok := cfg.ProviderByName(providerName)
			if !ok || provider.Type != "ip" {
				continue
			}
			cachePath := provider.CachePath()
			cidrs, err := policy.LoadIPv4CIDRsFile(cachePath)
			if err != nil {
				if os.IsNotExist(err) {
					restored, restoreErr := provider.RestoreDefaultCache()
					if restoreErr != nil {
						fmt.Fprintf(os.Stderr, "warning: provider %q cache restore failed: %v\n", provider.Name, restoreErr)
						continue
					}
					if restored {
						cidrs, err = policy.LoadIPv4CIDRsFile(cachePath)
						if err == nil {
							all = append(all, cidrs...)
							continue
						}
						if !os.IsNotExist(err) {
							return nil, err
						}
					}
					fmt.Fprintf(os.Stderr, "warning: provider %q cache %q is missing; skipping provider until zakopd providers update %s\n", provider.Name, cachePath, provider.Name)
					continue
				}
				return nil, err
			}
			_ = provider.MirrorDefaultCache()
			all = append(all, cidrs...)
		}
		out[i] = policy.NormalizeIPv4CIDRs(all)
	}
	return out, nil
}
