package tool_test

import (
	"testing"

	"github.com/lacsar712/waferlot/internal/tool"
)

func TestSeededLithoAcceptsQualifiedProgram(t *testing.T) {
	f := tool.Seed()
	if err := f.CanTrackIn("LITHO-01", "LOT-1001", "PP-LITHO-193NM-V3", 25); err != nil {
		t.Fatal(err)
	}
	if err := f.CanTrackIn("LITHO-01", "LOT-1001", "PP-ETCH-POLY-V2", 25); err == nil {
		t.Fatal("unqualified process program must be rejected")
	}
	if err := f.CanTrackIn("CMP-02", "LOT-1001", "PP-CMP-OXIDE-V2", 25); err == nil {
		t.Fatal("tool in PM must not accept track-in")
	}
}

func TestTrackInOutCycle(t *testing.T) {
	f := tool.Seed()
	if err := f.TrackIn("ETCH-07", "LOT-1002"); err != nil {
		t.Fatal(err)
	}
	got, ok := f.Get("ETCH-07")
	if !ok || got.State != tool.Running || got.CurrentLot != "LOT-1002" {
		t.Fatalf("after track-in: %+v", got)
	}
	if err := f.TrackOut("ETCH-07", "LOT-1002"); err != nil {
		t.Fatal(err)
	}
	got, _ = f.Get("ETCH-07")
	if got.State != tool.Idle || got.CurrentLot != "" {
		t.Fatalf("after track-out: %+v", got)
	}
}
