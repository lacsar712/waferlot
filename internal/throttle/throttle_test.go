package throttle_test

import (
	"testing"
	"time"

	"github.com/lacsar712/waferlot/internal/throttle"
	"github.com/lacsar712/waferlot/internal/wallclock"
)

func TestTakeBlocksAfterBurst(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(0, 0))
	b := throttle.New(clk, 0.001, 1)
	if wait := b.Take(); wait != 0 {
		t.Fatalf("first take must proceed, wait=%s", wait)
	}
	if wait := b.Take(); wait == 0 {
		t.Fatal("burst exhausted; second take must return a wait")
	}
}
