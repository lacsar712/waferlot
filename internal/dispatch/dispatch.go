package dispatch

import "time"

type Work struct {
	EventID     string    `json:"event_id"`
	ForwardID   string    `json:"forward_id"`
	CollectorID string    `json:"collector_id"`
	Kind        string    `json:"kind"`
	Body        []byte    `json:"body"`
	Attempt     int       `json:"attempt"`
	NotBefore   time.Time `json:"not_before"`
	CreatedAt   time.Time `json:"created_at"`
	ReissueOf   string    `json:"reissue_of,omitempty"`
}

func (w Work) Clone() Work {
	c := w
	if w.Body != nil {
		c.Body = append([]byte(nil), w.Body...)
	}
	return c
}
