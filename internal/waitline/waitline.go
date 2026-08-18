package waitline

import (
	"container/heap"
	"sync"
	"time"

	"github.com/lacsar712/waferlot/internal/dispatch"
	"github.com/lacsar712/waferlot/internal/wallclock"
)

type item struct {
	j     dispatch.Work
	index int
}

type dueHeap []*item

func (h dueHeap) Len() int { return len(h) }

func (h dueHeap) Less(i, j int) bool {
	if h[i].j.NotBefore.Equal(h[j].j.NotBefore) {
		return h[i].j.CreatedAt.Before(h[j].j.CreatedAt)
	}
	return h[i].j.NotBefore.Before(h[j].j.NotBefore)
}

func (h dueHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *dueHeap) Push(x any) {
	it := x.(*item)
	it.index = len(*h)
	*h = append(*h, it)
}

func (h *dueHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	it.index = -1
	*h = old[:n-1]
	return it
}

type CollectorLine struct {
	ID        string
	Ordered   bool
	inFlight  int
	maxFlight int
	ready     dueHeap
}

func newCollectorLine(id string, ordered bool, maxFlight int) *CollectorLine {
	if maxFlight < 1 {
		maxFlight = 1
	}
	if ordered {
		maxFlight = 1
	}
	dq := &CollectorLine{ID: id, Ordered: ordered, maxFlight: maxFlight}
	heap.Init(&dq.ready)
	return dq
}

type Broker struct {
	mu    sync.Mutex
	clk   wallclock.Clock
	dests map[string]*CollectorLine
}

func NewBroker(clk wallclock.Clock) *Broker {
	return &Broker{clk: clk, dests: make(map[string]*CollectorLine)}
}

func (b *Broker) Ensure(id string, ordered bool, maxFlight int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.dests[id]; ok {
		return
	}
	b.dests[id] = newCollectorLine(id, ordered, maxFlight)
}

func (b *Broker) Enqueue(j dispatch.Work, ordered bool, maxFlight int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	dq, ok := b.dests[j.CollectorID]
	if !ok {
		dq = newCollectorLine(j.CollectorID, ordered, maxFlight)
		b.dests[j.CollectorID] = dq
	}
	heap.Push(&dq.ready, &item{j: j.Clone()})
}

func (b *Broker) Lease() (dispatch.Work, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.clk.Now()
	var best *CollectorLine
	var bestDue time.Time
	for _, dq := range b.dests {
		if dq.inFlight >= dq.maxFlight {
			continue
		}
		if dq.ready.Len() == 0 {
			continue
		}
		head := dq.ready[0].j
		if head.NotBefore.After(now) {
			continue
		}
		if best == nil || head.NotBefore.Before(bestDue) {
			best = dq
			bestDue = head.NotBefore
		}
	}
	if best == nil {
		return dispatch.Work{}, false
	}
	it := heap.Pop(&best.ready).(*item)
	best.inFlight++
	return it.j, true
}

func (b *Broker) Release(collectorID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if dq, ok := b.dests[collectorID]; ok && dq.inFlight > 0 {
		dq.inFlight--
	}
}

func (b *Broker) Depth() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, dq := range b.dests {
		n += dq.ready.Len() + dq.inFlight
	}
	return n
}

func (b *Broker) DepthByCollector() map[string]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[string]int, len(b.dests))
	for id, dq := range b.dests {
		out[id] = dq.ready.Len() + dq.inFlight
	}
	return out
}

func (b *Broker) Snapshot() []dispatch.Work {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []dispatch.Work
	for _, dq := range b.dests {
		for _, it := range dq.ready {
			out = append(out, it.j.Clone())
		}
	}
	return out
}

func (b *Broker) Restore(jobs []dispatch.Work, meta map[string]LineMeta) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dests = make(map[string]*CollectorLine)
	for id, m := range meta {
		b.dests[id] = newCollectorLine(id, m.Ordered, m.MaxInFlight)
	}
	for _, j := range jobs {
		dq, ok := b.dests[j.CollectorID]
		if !ok {
			dq = newCollectorLine(j.CollectorID, false, 2)
			b.dests[j.CollectorID] = dq
		}
		heap.Push(&dq.ready, &item{j: j.Clone()})
	}
}

type LineMeta struct {
	Ordered     bool
	MaxInFlight int
}
