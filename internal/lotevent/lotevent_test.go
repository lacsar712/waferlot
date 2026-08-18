package lotevent_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/lacsar712/waferlot/internal/lotevent"
)

func sampleBody() []byte {
	return []byte(`{"type":"lot.track_in","payload":{"lot_id":"LOT-1001","tool_id":"LITHO-01","step_id":"litho.photo","process_program":"PP-LITHO-193NM-V3","wafer_count":25}}`)
}

func TestParseAndPrefix(t *testing.T) {
	env, err := lotevent.Parse(sampleBody())
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != "lot.track_in" {
		t.Fatalf("type %s", env.Type)
	}
	if !lotevent.MatchPrefix("lot.track_in", "lot") {
		t.Fatal("lot should match lot.track_in")
	}
	if lotevent.MatchPrefix("lot.track_in", "etch") {
		t.Fatal("etch should not match")
	}
	if _, err := lotevent.Parse([]byte(`{"type":"Lot.Track_In","payload":{}}`)); err == nil {
		t.Fatal("uppercase type should fail")
	}
}

func TestMatchPrefixSegmentBoundary(t *testing.T) {
	cases := []struct {
		eventType string
		prefix    string
		want      bool
	}{
		{"lot.track_in", "lot", true},
		{"lot.track_in", "lot.", true},
		{"lot.track_in", "lot.track_in", true},
		{"lot.track_in", "", true},
		{"lot.track_in", "lo", false},
		{"lot.track_in", "lot.t", false},
		{"lot.track_in", "etch", false},
		{"lots.track_in", "lot", false},
	}
	for _, tc := range cases {
		got := lotevent.MatchPrefix(tc.eventType, tc.prefix)
		if got != tc.want {
			t.Fatalf("MatchPrefix(%q, %q)=%v want %v", tc.eventType, tc.prefix, got, tc.want)
		}
	}
}

func TestParseWrapsSyntaxError(t *testing.T) {
	_, err := lotevent.Parse([]byte(`{`))
	if err == nil {
		t.Fatal("expected syntax error")
	}
	var syn *json.SyntaxError
	if !errors.As(err, &syn) {
		t.Fatalf("want json.SyntaxError via errors.As, got %v", err)
	}
}

func TestParseRequiresProcessProgram(t *testing.T) {
	_, err := lotevent.Parse([]byte(`{"type":"lot.track_in","payload":{"lot_id":"LOT-1001","tool_id":"LITHO-01","step_id":"litho.photo","wafer_count":25}}`))
	if err == nil {
		t.Fatal("process_program must be required on track-in")
	}
}
