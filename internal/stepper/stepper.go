package stepper

import (
	"time"

	"github.com/lacsar712/waferlot/internal/lotid"
	"github.com/lacsar712/waferlot/internal/mes"
)

type PlanItem struct {
	CollectorID string
	ForwardID   string
	URL         string
	Ordered     bool
}

type Plan struct {
	EventID    string
	Kind       string
	Items      []PlanItem
	DroppedOff int
}

func Fanout(eventID, eventType string, body []byte, dests []mes.Collector, now time.Time) Plan {
	_ = body
	p := Plan{EventID: eventID, Kind: eventType, Items: make([]PlanItem, 0, len(dests))}
	for _, d := range dests {
		if !d.Matches(eventType) {
			p.DroppedOff++
			continue
		}
		p.Items = append(p.Items, PlanItem{
			CollectorID: d.ID,
			ForwardID:   lotid.New("fwd", now),
			URL:         d.URL,
			Ordered:     d.Ordered,
		})
	}
	return p
}
