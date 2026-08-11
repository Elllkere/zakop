package outboundpool

import (
	"bufio"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elllkere/neto/internal/config"
)

type fakeAPI struct {
	healthy  map[string]bool
	selected string
	tested   []string
	changes  []string
}

func (f *fakeAPI) Delay(_ context.Context, outbound string, _ string, _ time.Duration) error {
	f.tested = append(f.tested, outbound)
	if !f.healthy[outbound] {
		return errors.New("failed")
	}
	return nil
}

func (f *fakeAPI) Selected(context.Context, string) (string, error) { return f.selected, nil }

func (f *fakeAPI) Select(_ context.Context, _ string, outbound string) error {
	f.selected = outbound
	f.changes = append(f.changes, outbound)
	return nil
}

func TestCheckUsesFirstHealthyOutboundAndRecoversPriority(t *testing.T) {
	api := &fakeAPI{healthy: map[string]bool{"second": true}, selected: "first"}
	manager := &Manager{API: api}
	pool := config.OutboundPool{Tag: "pool", Outbounds: []string{"first", "second", "third"}, CheckURL: "https://example.com"}

	selected, err := manager.Check(context.Background(), pool)
	if err != nil || selected != "second" {
		t.Fatalf("selected=%q err=%v", selected, err)
	}
	if !reflect.DeepEqual(api.tested, []string{"first", "second"}) || !reflect.DeepEqual(api.changes, []string{"second"}) {
		t.Fatalf("unexpected failover: tested=%v changes=%v", api.tested, api.changes)
	}

	api.healthy["first"] = true
	api.tested = nil
	selected, err = manager.Check(context.Background(), pool)
	if err != nil || selected != "first" {
		t.Fatalf("recovery selected=%q err=%v", selected, err)
	}
	if !reflect.DeepEqual(api.tested, []string{"first"}) || !reflect.DeepEqual(api.changes, []string{"second", "first"}) {
		t.Fatalf("unexpected priority recovery: tested=%v changes=%v", api.tested, api.changes)
	}
}

func TestCheckKeepsSelectionWhenAllOutboundsFail(t *testing.T) {
	api := &fakeAPI{healthy: map[string]bool{}, selected: "second"}
	manager := &Manager{API: api}
	pool := config.OutboundPool{Tag: "pool", Outbounds: []string{"first", "second"}, CheckURL: "https://example.com"}

	if _, err := manager.Check(context.Background(), pool); err == nil {
		t.Fatal("expected all-outbounds failure")
	}
	if api.selected != "second" || len(api.changes) != 0 {
		t.Fatalf("selection changed on total failure: selected=%q changes=%v", api.selected, api.changes)
	}
}

func TestReadChunked(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("5\r\nhello\r\n6\r\n world\r\n0\r\nX-Test: yes\r\n\r\n"))
	got, err := readChunked(reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world" {
		t.Fatalf("unexpected chunked body %q", got)
	}
}
