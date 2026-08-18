package forwarder

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lacsar712/waferlot/internal/baygate"
	"github.com/lacsar712/waferlot/internal/collector"
	"github.com/lacsar712/waferlot/internal/dispatch"
	"github.com/lacsar712/waferlot/internal/holdbin"
	"github.com/lacsar712/waferlot/internal/mes"
	"github.com/lacsar712/waferlot/internal/retrywait"
	"github.com/lacsar712/waferlot/internal/runlog"
	"github.com/lacsar712/waferlot/internal/trip"
	"github.com/lacsar712/waferlot/internal/waitline"
	"github.com/lacsar712/waferlot/internal/wallclock"
)

type engineOpts struct {
	rate    float64
	burst   int
	timeout time.Duration
	trip    trip.Settings
}

func newTestEngine(t *testing.T, url string, clk wallclock.Clock) (*Engine, *mes.Collector) {
	t.Helper()
	return newTestEngineOpts(t, url, clk, engineOpts{})
}

func newTestEngineOpts(t *testing.T, url string, clk wallclock.Clock, opts engineOpts) (*Engine, *mes.Collector) {
	t.Helper()
	if opts.rate <= 0 {
		opts.rate = 100
	}
	if opts.burst < 1 {
		opts.burst = 100
	}
	if opts.timeout <= 0 {
		opts.timeout = 2 * time.Second
	}
	if opts.trip.FailThreshold < 1 {
		opts.trip = trip.DefaultSettings()
	}
	cols := mes.NewRegistry(clk)
	enabled := true
	d, err := cols.Create(mes.CreateInput{
		Name:         "t",
		URL:          url,
		Secret:       "abcdefgh",
		KindPrefixes: []string{""},
		Rate:         opts.rate,
		Burst:        opts.burst,
		MaxInFlight:  4,
		Enabled:      &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	broker := waitline.NewBroker(clk)
	broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
	gates := baygate.NewGatesWithIsolator(clk, opts.trip)
	e := New(clk, broker, cols, gates, collector.New(opts.timeout), runlog.New(50), holdbin.New(50), retrywait.Policy{
		Base:        time.Millisecond,
		Cap:         time.Millisecond,
		MaxAttempts: 8,
	})
	return e, &d
}

func sampleWork(collectorID string, clk wallclock.Clock, forwardID string) dispatch.Work {
	return dispatch.Work{
		EventID:     "lot_test",
		ForwardID:   forwardID,
		CollectorID: collectorID,
		Kind:        "lot.track_in",
		Body:        []byte(`{"type":"lot.track_in","payload":{"lot_id":"LOT-1001","tool_id":"LITHO-01","step_id":"litho.photo","process_program":"PP-LITHO-193NM-V3","wafer_count":25}}`),
		Attempt:     0,
		NotBefore:   clk.Now(),
		CreatedAt:   clk.Now(),
	}
}

func TestHandleRetriesStatus429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngine(t, srv.URL, clk)
	j := sampleWork(dest.ID, clk, "fwd_test429")
	e.handle(context.Background(), j)
	if e.dead.Len() != 0 {
		t.Fatalf("429 should not go to hold bin, got %d", e.dead.Len())
	}
	if e.broker.Depth() != 1 {
		t.Fatalf("429 should requeue, depth=%d", e.broker.Depth())
	}
}

func TestHandleTreats202AsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngine(t, srv.URL, clk)
	e.handle(context.Background(), sampleWork(dest.ID, clk, "fwd_test202"))
	if e.dead.Len() != 0 {
		t.Fatalf("202 should not go to hold bin, got %d", e.dead.Len())
	}
	if e.broker.Depth() != 0 {
		t.Fatalf("202 should not requeue, depth=%d", e.broker.Depth())
	}
	entries := e.log.List("", 10)
	if len(entries) == 0 || entries[0].Kind != "success" {
		t.Fatalf("want success run log, got %+v", entries)
	}
}

func TestHandleHonorsCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- c
		_, _ = io.Copy(io.Discard, c)
		_ = c.Close()
	}()
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngineOpts(t, "http://"+ln.Addr().String(), clk, engineOpts{timeout: 15 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.handle(ctx, sampleWork(dest.ID, clk, "fwd_cancel"))
	}()
	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("outbound request never started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handle ignored cancel and kept the outbound request alive")
	}
}

func TestHandleTimeoutIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngineOpts(t, srv.URL, clk, engineOpts{timeout: 50 * time.Millisecond})
	e.handle(context.Background(), sampleWork(dest.ID, clk, "fwd_timeout"))
	if e.dead.Len() != 0 {
		t.Fatalf("timeout must not go to hold bin, got %d", e.dead.Len())
	}
	if e.broker.Depth() != 1 {
		t.Fatalf("timeout must requeue, depth=%d", e.broker.Depth())
	}
}

func TestHandleSuccessClearsBreaker(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 2 {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngineOpts(t, srv.URL, clk, engineOpts{
		trip: trip.Settings{FailThreshold: 2, OpenFor: time.Hour, Probes: 1},
	})
	e.handle(context.Background(), sampleWork(dest.ID, clk, "fwd_brk_1"))
	e.handle(context.Background(), sampleWork(dest.ID, clk, "fwd_brk_2"))
	e.handle(context.Background(), sampleWork(dest.ID, clk, "fwd_brk_3"))
	snap := e.gates.Isolator(dest.ID).Snapshot()
	if snap.State != "closed" {
		t.Fatalf("success must clear failures; isolator state=%s failures=%d", snap.State, snap.Failures)
	}
	if e.dead.Len() != 0 {
		t.Fatalf("third 500 after a success must retry, not hold bin; holdbin=%d", e.dead.Len())
	}
}

func TestHandleRespectsRateLimit(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	e, dest := newTestEngineOpts(t, srv.URL, clk, engineOpts{rate: 0.0001, burst: 1})
	e.handle(context.Background(), sampleWork(dest.ID, clk, "fwd_rl_1"))
	e.handle(context.Background(), sampleWork(dest.ID, clk, "fwd_rl_2"))
	if hits.Load() != 1 {
		t.Fatalf("burst=1 must not POST twice, hits=%d", hits.Load())
	}
	if e.broker.Depth() != 1 {
		t.Fatalf("rate-limited work must requeue, depth=%d", e.broker.Depth())
	}
}
