package proxyroute

import (
	"fmt"
	"strings"

	"github.com/elllkere/zakop/internal/config"
)

// Target describes the stable nftables -> TProxy inbound mapping for one
// selectable outbound or pool. Concrete outbounds keep their existing order
// and pools are appended in UCI order so adding a pool does not renumber
// existing TProxy listeners.
type Target struct {
	Tag     string
	Chain   string
	Inbound string
	Port    int
}

func Targets(cfg config.Config) []Target {
	tags := cfg.ProxyTargetTags()
	targets := make([]Target, 0, len(tags))
	for i, tag := range tags {
		targets = append(targets, Target{
			Tag:     tag,
			Chain:   fmt.Sprintf("to_proxy_%04d", i),
			Inbound: fmt.Sprintf("tproxy-%04d-in", i),
			Port:    cfg.Main.TProxyPort + i,
		})
	}
	return targets
}

func Find(targets []Target, tag string) (Target, bool) {
	tag = strings.TrimSpace(tag)
	for _, target := range targets {
		if target.Tag == tag {
			return target, true
		}
	}
	return Target{}, false
}
