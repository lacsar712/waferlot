package fab

import (
	"context"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/lacsar712/waferlot/internal/baygate"
	"github.com/lacsar712/waferlot/internal/collector"
	"github.com/lacsar712/waferlot/internal/echo"
	"github.com/lacsar712/waferlot/internal/forwarder"
	"github.com/lacsar712/waferlot/internal/holdbin"
	"github.com/lacsar712/waferlot/internal/intake"
	"github.com/lacsar712/waferlot/internal/lot"
	"github.com/lacsar712/waferlot/internal/mes"
	"github.com/lacsar712/waferlot/internal/once"
	"github.com/lacsar712/waferlot/internal/oncekey"
	"github.com/lacsar712/waferlot/internal/persist"
	"github.com/lacsar712/waferlot/internal/program"
	"github.com/lacsar712/waferlot/internal/reissue"
	"github.com/lacsar712/waferlot/internal/retrywait"
	"github.com/lacsar712/waferlot/internal/runlog"
	"github.com/lacsar712/waferlot/internal/settings"
	"github.com/lacsar712/waferlot/internal/step"
	"github.com/lacsar712/waferlot/internal/tool"
	"github.com/lacsar712/waferlot/internal/toolkey"
	"github.com/lacsar712/waferlot/internal/waitline"
	"github.com/lacsar712/waferlot/internal/wallclock"
)

const Version = "0.1.0"

type Plant struct {
	Cfg      settings.Config
	Clk      wallclock.Clock
	Cols     *mes.Registry
	Idem     *oncekey.Store
	Nonces   *once.Book
	Broker   *waitline.Broker
	Keys     *toolkey.Keys
	Log      *runlog.Log
	Dead     *holdbin.Bin
	Gates    *baygate.Gates
	Loop     *echo.Sink
	Pipe     *intake.Pipeline
	Engine   *forwarder.Engine
	Snap     *persist.File
	Tools    *tool.Floor
	Steps    *step.Flow
	Programs *program.Catalog
	Lots     *lot.Yard
	Started  time.Time
}

func New(cfg settings.Config) (*Plant, error) {
	clk := wallclock.Real{}
	a := &Plant{
		Cfg:      cfg,
		Clk:      clk,
		Cols:     mes.NewRegistry(clk),
		Idem:     oncekey.New(clk, cfg.IdemTTL),
		Nonces:   once.New(clk, cfg.Window),
		Broker:   waitline.NewBroker(clk),
		Keys:     toolkey.New("tool", cfg.IngestSecret),
		Log:      runlog.New(500),
		Dead:     holdbin.New(200),
		Gates:    baygate.NewGates(clk),
		Loop:     echo.New(50),
		Tools:    tool.Seed(),
		Steps:    step.Seed(),
		Programs: program.Seed(),
		Lots:     lot.Seed(),
		Started:  time.Now(),
	}
	a.Pipe = &intake.Pipeline{
		Clk:    clk,
		Window: cfg.Window,
		Keys:   a.Keys,
		Nonces: a.Nonces,
		Idem:   a.Idem,
		Cols:   a.Cols,
		Broker: a.Broker,
	}
	a.Engine = forwarder.New(clk, a.Broker, a.Cols, a.Gates, collector.New(10*time.Second), a.Log, a.Dead, retrywait.Default())
	snap, err := persist.New(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	a.Snap = snap
	if s, ok, err := snap.Load(); err != nil {
		return nil, err
	} else if ok {
		persist.Apply(a.deps(), s)
	}
	if len(a.Cols.List()) == 0 {
		if err := a.seedEcho(); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func (a *Plant) deps() persist.Deps {
	return persist.Deps{
		Cols:   a.Cols,
		Idem:   a.Idem,
		Nonces: a.Nonces,
		Log:    a.Log,
		Dead:   a.Dead,
		Broker: a.Broker,
		Keys:   a.Keys,
		Gates:  a.Gates,
	}
}

func (a *Plant) seedEcho() error {
	raw, err := url.JoinPath(a.Cfg.PublicBase, a.Cfg.EchoPath)
	if err != nil {
		return err
	}
	enabled := true
	d, err := a.Cols.Create(mes.CreateInput{
		Name:         "echo-mes",
		URL:          raw,
		Secret:       "dev-echo-secret",
		KindPrefixes: []string{""},
		Ordered:      false,
		Rate:         20,
		Burst:        20,
		MaxInFlight:  4,
		Enabled:      &enabled,
	})
	if err != nil {
		return err
	}
	a.Broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
	return nil
}

func (a *Plant) StartForwarders(ctx context.Context) {
	a.Engine.Run(ctx, a.Cfg.Workers)
	go a.snapshotLoop(ctx)
}

func (a *Plant) snapshotLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			a.save()
			return
		case <-t.C:
			a.save()
		}
	}
}

func (a *Plant) save() {
	if err := a.Snap.Save(persist.Capture(a.deps())); err != nil {
		log.Printf("snapshot: %v", err)
	}
}

func (a *Plant) Reissue(forwardID string) (string, error) {
	now := a.Clk.Now()
	if it, ok := a.Dead.Get(forwardID); ok {
		j, err := reissue.FromHoldBin(it, now)
		if err != nil {
			return "", err
		}
		d, ok := a.Cols.Get(j.CollectorID)
		if !ok {
			return "", os.ErrNotExist
		}
		a.Broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
		a.Broker.Enqueue(j, d.Ordered, d.MaxInFlight)
		_, _ = a.Dead.Remove(forwardID)
		return j.ForwardID, nil
	}
	e, ok := a.Log.Get(forwardID)
	if !ok {
		return "", os.ErrNotExist
	}
	j, err := reissue.FromRunLog(e, now)
	_ = err
	d, ok := a.Cols.Get(j.CollectorID)
	if !ok {
		return "", os.ErrNotExist
	}
	a.Broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
	a.Broker.Enqueue(j, d.Ordered, d.MaxInFlight)
	return j.ForwardID, nil
}
