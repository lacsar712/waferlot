package lot_test

import (
	"testing"

	"github.com/lacsar712/waferlot/internal/lot"
)

func TestHoldBlocksTrackIn(t *testing.T) {
	y := lot.Seed()
	if err := y.ApplyTrackIn("LOT-1044", "CMP-02", "cmp.oxide", "PP-CMP-OXIDE-V2"); err == nil {
		t.Fatal("held lot must not track in")
	}
	if err := y.Release("LOT-1044"); err != nil {
		t.Fatal(err)
	}
	if err := y.ApplyTrackIn("LOT-1044", "CMP-02", "cmp.oxide", "PP-CMP-OXIDE-V2"); err != nil {
		t.Fatal(err)
	}
}
