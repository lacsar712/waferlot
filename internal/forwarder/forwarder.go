package forwarder

import (
	"context"
	"log"
	"time"

	"github.com/lacsar712/waferlot/internal/baygate"
	"github.com/lacsar712/waferlot/internal/collector"
	"github.com/lacsar712/waferlot/internal/dispatch"
	"github.com/lacsar712/waferlot/internal/holdbin"
	"github.com/lacsar712/waferlot/internal/mes"
	"github.com/lacsar712/waferlot/internal/outcome"
	"github.com/lacsar712/waferlot/internal/retrywait"
	"github.com/lacsar712/waferlot/internal/runlog"
	"github.com/lacsar712/waferlot/internal/waitline"
	"github.com/lacsar712/waferlot/internal/wallclock"
)

type Engine struct {
	clk    wallclock.Clock
	broker *waitline.Broker
	cols   *mes.Registry
	gates  *baygate.Gates
	client *collector.Client
	log    *runlog.Log
	dead   *holdbin.Bin
	policy retrywait.Policy
}

func New(clk wallclock.Clock, broker *waitline.Broker, cols *mes.Registry, gates *baygate.Gates, client *collector.Client, jlog *runlog.Log, dead *holdbin.Bin, policy retrywait.Policy) *Engine {
	return &Engine{
		clk:    clk,
		broker: broker,
		cols:   cols,
		gates:  gates,
		client: client,
		log:    jlog,
		dead:   dead,
		policy: policy.Validate(),
	}
}

func (e *Engine) Run(ctx context.Context, n int) {
	if n < 1 {
		n = 1
	}
	for i := 0; i < n; i++ {
		go e.loop(ctx)
	}
}

func (e *Engine) loop(ctx context.Context) {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.tick(ctx)
		}
	}
}

func (e *Engine) tick(ctx context.Context) {
	j, ok := e.broker.Lease()
	if !ok {
		return
	}
	defer e.broker.Release(j.CollectorID)
	e.handle(ctx, j)
}

func (e *Engine) handle(ctx context.Context, j dispatch.Work) {
	dest, ok := e.cols.Get(j.CollectorID)
	if !ok || !dest.Enabled {
		e.dead.Push(holdbin.FromWork(j, "collector missing or disabled", e.clk.Now()))
		e.note(j, "terminal", 0, "", "collector_unavailable")
		return
	}
	e.gates.Ensure(dest.ID, dest.Rate, dest.Burst)
	br := e.gates.Isolator(dest.ID)
	dec := br.Allow()
	if !dec.Allow {
		j.NotBefore = e.clk.Now().Add(200 * time.Millisecond)
		e.broker.Enqueue(j, dest.Ordered, dest.MaxInFlight)
		e.note(j, "skipped_open", 0, "", dec.Note)
		return
	}
	_ = e.gates.Bucket(dest.ID).Take()

	j.Attempt++
	res := e.client.Post(ctx, collector.Request{
		URL:         dest.URL,
		Secret:      dest.Secret,
		EventID:     j.EventID,
		ForwardID:   j.ForwardID,
		CollectorID: j.CollectorID,
		Attempt:     j.Attempt,
		Body:        j.Body,
		Now:         e.clk.Now(),
	})
	kind := outcome.Combine(res.Status, res.Error)
	errMsg := ""
	if res.Error != nil {
		errMsg = res.Error.Error()
	}
	e.noteWithStatus(j, kind.String(), res.Status, errMsg, res.Body)

	switch kind {
	case outcome.Success:
		br.Success()
		return
	case outcome.Retryable:
		br.Failure()
		if e.policy.Exhausted(j.Attempt) {
			e.dead.Push(holdbin.FromWork(j, "attempts exhausted", e.clk.Now()))
			e.note(j, "terminal", res.Status, errMsg, "exhausted")
			return
		}
		delay := e.policy.Delay(j.Attempt)
		j.NotBefore = e.clk.Now().Add(delay)
		e.broker.Enqueue(j, dest.Ordered, dest.MaxInFlight)
	case outcome.Terminal:
		br.Failure()
		e.dead.Push(holdbin.FromWork(j, "terminal http status", e.clk.Now()))
	default:
		log.Printf("unknown outcome kind for %s", j.ForwardID)
	}
}

func (e *Engine) note(j dispatch.Work, kind string, status int, errMsg, note string) {
	e.noteWithStatus(j, kind, status, errMsg, note)
}

func (e *Engine) noteWithStatus(j dispatch.Work, kind string, status int, errMsg, note string) {
	if len(note) > 240 {
		note = note[:240]
	}
	e.log.Append(runlog.Entry{
		At:          e.clk.Now(),
		EventID:     j.EventID,
		ForwardID:   j.ForwardID,
		CollectorID: j.CollectorID,
		Attempt:     j.Attempt,
		Kind:        kind,
		Status:      status,
		Error:       errMsg,
		Note:        note,
		Type:        j.Kind,
		Body:        j.Body,
		ReissueOf:   j.ReissueOf,
	})
}
