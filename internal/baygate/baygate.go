package baygate

import (
	"sync"

	"github.com/lacsar712/waferlot/internal/throttle"
	"github.com/lacsar712/waferlot/internal/trip"
	"github.com/lacsar712/waferlot/internal/wallclock"
)

type Gates struct {
	mu          sync.Mutex
	clk         wallclock.Clock
	isolatorCfg trip.Settings
	isolators   map[string]*trip.Isolator
	buckets     map[string]*throttle.Bucket
	rates       map[string]rateSpec
}

type rateSpec struct {
	rate  float64
	burst int
}

func NewGates(clk wallclock.Clock) *Gates {
	return NewGatesWithIsolator(clk, trip.DefaultSettings())
}

func NewGatesWithIsolator(clk wallclock.Clock, cfg trip.Settings) *Gates {
	return &Gates{
		clk:         clk,
		isolatorCfg: cfg,
		isolators:   make(map[string]*trip.Isolator),
		buckets:     make(map[string]*throttle.Bucket),
		rates:       make(map[string]rateSpec),
	}
}

func (g *Gates) Ensure(id string, rate float64, burst int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.isolators[id]; !ok {
		g.isolators[id] = trip.New(g.clk, g.isolatorCfg)
	}
	spec := rateSpec{rate: rate, burst: burst}
	old, exists := g.rates[id]
	if !exists || old != spec {
		g.buckets[id] = throttle.New(g.clk, rate, burst)
		g.rates[id] = spec
	}
}

func (g *Gates) Isolator(id string) *trip.Isolator {
	g.mu.Lock()
	defer g.mu.Unlock()
	b, ok := g.isolators[id]
	if !ok {
		b = trip.New(g.clk, g.isolatorCfg)
		g.isolators[id] = b
	}
	return b
}

func (g *Gates) Bucket(id string) *throttle.Bucket {
	g.mu.Lock()
	defer g.mu.Unlock()
	b, ok := g.buckets[id]
	if !ok {
		b = throttle.New(g.clk, 5, 5)
		g.buckets[id] = b
	}
	return b
}

type Snapshot struct {
	Isolators map[string]trip.Snapshot     `json:"isolators"`
	Buckets   map[string]throttle.Snapshot `json:"buckets"`
}

func (g *Gates) Snapshot() Snapshot {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := Snapshot{
		Isolators: make(map[string]trip.Snapshot, len(g.isolators)),
		Buckets:   make(map[string]throttle.Snapshot, len(g.buckets)),
	}
	for id, b := range g.isolators {
		s.Isolators[id] = b.Snapshot()
	}
	for id, b := range g.buckets {
		s.Buckets[id] = b.Snapshot()
	}
	return s
}

func (g *Gates) Restore(s Snapshot) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, snap := range s.Isolators {
		b := trip.New(g.clk, g.isolatorCfg)
		b.Restore(snap)
		g.isolators[id] = b
	}
	for id, snap := range s.Buckets {
		b := throttle.New(g.clk, snap.Rate, int(snap.Burst))
		b.Restore(snap)
		g.buckets[id] = b
	}
}

func (g *Gates) Public() map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string]any, len(g.isolators))
	for id, b := range g.isolators {
		out[id] = b.Snapshot()
	}
	return out
}
