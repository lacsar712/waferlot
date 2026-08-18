package holdbin

import (
	"sync"
	"time"

	"github.com/lacsar712/waferlot/internal/dispatch"
	"github.com/lacsar712/waferlot/internal/mask"
)

type Item struct {
	DeadAt       time.Time `json:"dead_at"`
	Reason       string    `json:"reason"`
	EventID      string    `json:"event_id"`
	ForwardID    string    `json:"forward_id"`
	CollectorID  string    `json:"collector_id"`
	Kind         string    `json:"kind"`
	Attempt      int       `json:"attempt"`
	Body         []byte    `json:"body"`
	BodyRedacted []byte    `json:"body_redacted,omitempty"`
	ReissueOf    string    `json:"reissue_of,omitempty"`
}

func FromWork(j dispatch.Work, reason string, now time.Time) Item {
	return Item{
		DeadAt:       now,
		Reason:       reason,
		EventID:      j.EventID,
		ForwardID:    j.ForwardID,
		CollectorID:  j.CollectorID,
		Kind:         j.Kind,
		Attempt:      j.Attempt,
		Body:         append([]byte(nil), j.Body...),
		BodyRedacted: mask.JSON(j.Body),
		ReissueOf:    j.ReissueOf,
	}
}

type Bin struct {
	mu    sync.Mutex
	max   int
	items []Item
}

func New(max int) *Bin {
	if max < 20 {
		max = 100
	}
	return &Bin{max: max}
}

func (q *Bin) Push(it Item) {
	if len(it.Body) > 0 && len(it.BodyRedacted) == 0 {
		it.BodyRedacted = mask.JSON(it.Body)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, it)
	if len(q.items) > q.max {
		q.items = append([]Item(nil), q.items[len(q.items)-q.max:]...)
	}
}

func (q *Bin) List() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Item, len(q.items))
	for i := range q.items {
		cp := q.items[len(q.items)-1-i]
		cp.Body = nil
		out[i] = cp
	}
	return out
}

func (q *Bin) Get(forwardID string) (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := len(q.items) - 1; i >= 0; i-- {
		if q.items[i].ForwardID == forwardID {
			return q.items[i], true
		}
	}
	return Item{}, false
}

func (q *Bin) Remove(forwardID string) (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, it := range q.items {
		if it.ForwardID == forwardID {
			q.items = append(q.items[:i], q.items[i+1:]...)
			return it, true
		}
	}
	return Item{}, false
}

func (q *Bin) Snapshot() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Item, len(q.items))
	copy(out, q.items)
	return out
}

func (q *Bin) Restore(items []Item) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append([]Item(nil), items...)
}

func (q *Bin) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
