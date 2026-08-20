package trip

import (
	"sync"
	"time"

	"github.com/lacsar712/waferlot/internal/wallclock"
)

type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

type Settings struct {
	FailThreshold int
	OpenFor       time.Duration
	Probes        int
}

func DefaultSettings() Settings {
	return Settings{
		FailThreshold: 5,
		OpenFor:       30 * time.Second,
		Probes:        1,
	}
}

func (s Settings) normalize() Settings {
	if s.FailThreshold < 1 {
		s.FailThreshold = 5
	}
	if s.OpenFor <= 0 {
		s.OpenFor = 30 * time.Second
	}
	if s.Probes < 1 {
		s.Probes = 1
	}
	return s
}

type Isolator struct {
	mu         sync.Mutex
	clk        wallclock.Clock
	cfg        Settings
	state      State
	failures   int
	openedAt   time.Time
	probesLeft int
}

func New(clk wallclock.Clock, cfg Settings) *Isolator {
	return &Isolator{clk: clk, cfg: cfg.normalize(), state: Closed}
}

type Decision struct {
	Allow bool
	State State
	Note  string
}

func (b *Isolator) Allow() Decision {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maybeHalfOpenLocked()
	switch b.state {
	case Closed:
		return Decision{Allow: true, State: Closed, Note: "closed"}
	case Open:
		return Decision{Allow: false, State: Open, Note: "open"}
	case HalfOpen:
		if b.probesLeft <= 0 {
			return Decision{Allow: false, State: HalfOpen, Note: "probe_budget_empty"}
		}
		b.probesLeft--
		return Decision{Allow: true, State: HalfOpen, Note: "probe"}
	default:
		return Decision{Allow: false, State: b.state, Note: "unknown"}
	}
}

func (b *Isolator) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case HalfOpen:
		// a probe succeeded: close the isolator and clear the backlog of failures
		b.state = Closed
		b.failures = 0
		b.probesLeft = 0
	case Closed:
		// a healthy call resets the rolling failure count so an isolated
		// failure cannot accumulate across interleaved successes to trip
		b.failures = 0
	case Open:
		// stay open until the timer elapses; successes are not observed
		// while open because Allow() rejects calls before they are made
	}
}

func (b *Isolator) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case HalfOpen:
		b.tripLocked()
		return
	case Closed:
		b.failures++
		if b.failures >= b.cfg.FailThreshold {
			b.tripLocked()
		}
	case Open:
		// stay open until timer
	}
}

func (b *Isolator) tripLocked() {
	b.state = Open
	b.openedAt = b.clk.Now()
	b.probesLeft = 0
}

func (b *Isolator) maybeHalfOpenLocked() {
	if b.state != Open {
		return
	}
	if b.clk.Now().Sub(b.openedAt) >= b.cfg.OpenFor {
		b.state = HalfOpen
		b.probesLeft = b.cfg.Probes
	}
}

func (b *Isolator) Snapshot() Snapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.maybeHalfOpenLocked()
	return Snapshot{
		State:      b.state.String(),
		Failures:   b.failures,
		OpenedAt:   b.openedAt,
		ProbesLeft: b.probesLeft,
	}
}

type Snapshot struct {
	State      string    `json:"state"`
	Failures   int       `json:"failures"`
	OpenedAt   time.Time `json:"opened_at,omitempty"`
	ProbesLeft int       `json:"probes_left"`
}

func (b *Isolator) Restore(s Snapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch s.State {
	case "open":
		b.state = Open
	case "half_open":
		b.state = HalfOpen
	default:
		b.state = Closed
	}
	b.failures = s.Failures
	b.openedAt = s.OpenedAt
	b.probesLeft = s.ProbesLeft
}
