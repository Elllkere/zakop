package nft

import (
	"fmt"
	"strings"
	"testing"

	"github.com/elllkere/zakop/internal/config"
)

func scaleConfig(n int) config.Config {
	cfg := testConfig()
	base := cfg.Outbounds[0]
	cfg.Outbounds = nil
	for i := 0; i < n; i++ {
		o := base
		o.Tag = fmt.Sprintf("proxy_%04d", i)
		cfg.Outbounds = append(cfg.Outbounds, o)
	}
	return cfg
}

func Test126TargetsDoNotBecomeSequentialJumps(t *testing.T) {
	cfg := scaleConfig(126)
	cfg.Main.RoutingMode = "global"
	out, err := Generate(Input{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "\tchain to_proxy_") != 126 {
		t.Fatal("wrong target count")
	}
	if strings.Count(out, "jump to_proxy_") != 2 {
		t.Fatal("global packet scans target chains")
	}
}

func BenchmarkGenerateNetworkScale(b *testing.B) {
	for _, n := range []int{1, 126} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			cfg := scaleConfig(n)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				out, err := Generate(Input{Config: cfg})
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(len(out)), "nft-bytes")
			}
		})
	}
}
