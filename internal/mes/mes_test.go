package mes_test

import (
	"testing"
	"time"

	"github.com/lacsar712/waferlot/internal/mes"
	"github.com/lacsar712/waferlot/internal/wallclock"
)

func TestMatchesSegmentBoundary(t *testing.T) {
	cases := []struct {
		prefix    string
		eventType string
		want      bool
	}{
		{"lot", "lot.track_in", true},
		{"lot.", "lot.track_in", true},
		{"lot.track_in", "lot.track_in", true},
		{"", "anything.else", true},
		{"lo", "lot.track_in", false},
		{"lot.t", "lot.track_in", false},
		{"etch", "lot.track_in", false},
		{"lot", "lots.track_in", false},
	}
	for _, tc := range cases {
		d := mes.Collector{
			Enabled:      true,
			KindPrefixes: []string{tc.prefix},
		}
		got := d.Matches(tc.eventType)
		if got != tc.want {
			t.Fatalf("prefix %q vs type %q: Matches=%v want %v", tc.prefix, tc.eventType, got, tc.want)
		}
	}
}

func TestMatchesDisabledNeverFires(t *testing.T) {
	d := mes.Collector{
		Enabled:      false,
		KindPrefixes: []string{""},
	}
	if d.Matches("lot.track_in") {
		t.Fatal("disabled collector must not match")
	}
}

func TestRegistryMatchingSkipsDisabled(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(0, 0))
	reg := mes.NewRegistry(clk)
	off := false
	d, err := reg.Create(mes.CreateInput{
		Name:         "off",
		URL:          "http://127.0.0.1:8080/api/v1/echo",
		Secret:       "abcdefgh",
		KindPrefixes: []string{"lot"},
		Enabled:      &off,
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.Enabled {
		t.Fatal("expected disabled")
	}
	got := reg.Matching("lot.track_in")
	if len(got) != 0 {
		t.Fatalf("disabled collector leaked into matching: %+v", got)
	}
}
