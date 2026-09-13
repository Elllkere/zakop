//go:build networkdebug

package netdebug

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elllkere/zakop/internal/config"
)

func TestSnapshotRotationPermissionsAndCooldown(t *testing.T) {
	cfg := config.Defaults()
	dir := t.TempDir()
	runner := func(context.Context, string, ...string) string {
		return "counter packets 7 bytes 10\npassword=must-not-be-saved"
	}
	for i := 0; i < 6; i++ {
		path, err := collect(context.Background(), cfg, dir, "manual", false, runner)
		if err != nil {
			t.Fatal(err)
		}
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0600 {
			t.Fatal(st.Mode())
		}
		data, _ := os.ReadFile(path)
		if strings.Contains(string(data), "must-not-be-saved") {
			t.Fatal("secret leaked")
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "event-*.txt"))
	if len(files) != 4 {
		t.Fatal("rotation", len(files))
	}
	path, err := collect(context.Background(), cfg, dir, "ifdown", false, runner)
	if err != nil || path == "" {
		t.Fatal(path, err)
	}
	path, err = collect(context.Background(), cfg, dir, "ifdown", false, runner)
	if err != nil || path != "" {
		t.Fatal("cooldown failed", path, err)
	}
}

func TestRedaction(t *testing.T) {
	input := "password=p123\ntoken=t123\nprivate_key=k123\nsubscription=s123\nhttps://u:p@host/path?secret=xxx\nNETDEV WATCHDOG: eth0 transmit queue timed out\n"
	output := sanitize(input)
	for _, sensitive := range []string{"p123", "t123", "k123", "s123", "u:p", "xxx"} {
		if strings.Contains(output, sensitive) {
			t.Fatal(output)
		}
	}
	if !strings.Contains(kernelLines(input), "NETDEV WATCHDOG") {
		t.Fatal("network event missing")
	}
	if strings.Contains(kernelLines(input), "password") {
		t.Fatal("application log leaked")
	}
	nft := `iifname "br-lan" counter comment "arbitrary-credential"`
	if strings.Contains(quoted.ReplaceAllString(nft, `"[text omitted]"`), "arbitrary-credential") {
		t.Fatal("nft text leaked")
	}
}

func TestBoundedOutput(t *testing.T) {
	b := &boundedBuffer{remaining: 4}
	n, err := b.Write([]byte("abcdefgh"))
	if n != 8 || err != nil || b.String() != "abcd" {
		t.Fatal(b.String(), n, err)
	}
	_, _ = b.Write([]byte("more"))
	if b.Len() != 4 {
		t.Fatal(b.Len())
	}
}
