package trip_test

import (
	"testing"
	"time"

	"github.com/lacsar712/waferlot/internal/trip"
	"github.com/lacsar712/waferlot/internal/wallclock"
)

func TestOpensAfterThresholdAndRecovers(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(0, 0))
	b := trip.New(clk, trip.Settings{FailThreshold: 2, OpenFor: time.Second, Probes: 1})
	b.Failure()
	if !b.Allow().Allow {
		t.Fatal("still closed after one failure")
	}
	b.Failure()
	if b.Allow().Allow {
		t.Fatal("should be open")
	}
	clk.Advance(time.Second)
	d := b.Allow()
	if !d.Allow || d.State != trip.HalfOpen {
		t.Fatalf("expected half-open probe, got %+v", d)
	}
	b.Success()
	if b.Allow().State != trip.Closed {
		t.Fatal("expected closed after probe success")
	}
}

func TestSuccessClearsFailureCount(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(0, 0))
	b := trip.New(clk, trip.Settings{FailThreshold: 2, OpenFor: time.Hour, Probes: 1})
	b.Failure()
	b.Success()
	b.Failure()
	if !b.Allow().Allow || b.Allow().State != trip.Closed {
		t.Fatal("a failure after success must not open the isolator")
	}
}

func TestHalfOpenFailureReopens(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(0, 0))
	b := trip.New(clk, trip.Settings{FailThreshold: 1, OpenFor: time.Second, Probes: 1})
	b.Failure()
	if b.Allow().Allow {
		t.Fatal("should be open")
	}
	clk.Advance(time.Second)
	d := b.Allow()
	if !d.Allow || d.State != trip.HalfOpen {
		t.Fatalf("expected half-open probe, got %+v", d)
	}
	b.Failure()
	if b.Allow().Allow {
		t.Fatal("failed probe must open the isolator again")
	}
}
