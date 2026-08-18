package mes

import (
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/lacsar712/waferlot/internal/lotevent"
	"github.com/lacsar712/waferlot/internal/lotid"
	"github.com/lacsar712/waferlot/internal/wallclock"
)

// Collector is a manufacturing execution HTTP endpoint that receives lot-step events.
type Collector struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	Secret       string    `json:"secret"`
	PrevSecret   string    `json:"prev_secret,omitempty"`
	PrevUntil    time.Time `json:"prev_until,omitempty"`
	KindPrefixes []string  `json:"kind_prefixes"`
	Ordered      bool      `json:"ordered"`
	Rate         float64   `json:"rate"`
	Burst        int       `json:"burst"`
	MaxInFlight  int       `json:"max_in_flight"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CreateInput struct {
	Name         string   `json:"name"`
	URL          string   `json:"url"`
	Secret       string   `json:"secret"`
	KindPrefixes []string `json:"kind_prefixes"`
	Ordered      bool     `json:"ordered"`
	Rate         float64  `json:"rate"`
	Burst        int      `json:"burst"`
	MaxInFlight  int      `json:"max_in_flight"`
	Enabled      *bool    `json:"enabled"`
}

func (in CreateInput) normalize() (CreateInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return in, fmt.Errorf("name is required")
	}
	if len(in.Name) > 80 {
		return in, fmt.Errorf("name too long")
	}
	if err := ValidateURL(in.URL); err != nil {
		return in, err
	}
	if strings.TrimSpace(in.Secret) == "" {
		return in, fmt.Errorf("secret is required")
	}
	if len(in.Secret) < 8 {
		return in, fmt.Errorf("secret must be at least 8 characters")
	}
	cleaned := make([]string, 0, len(in.KindPrefixes))
	for _, p := range in.KindPrefixes {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if err := lotevent.ValidateKind(strings.TrimSuffix(p, ".")); err != nil && !strings.HasSuffix(p, ".") {
			stem := strings.TrimSuffix(p, ".")
			if err2 := lotevent.ValidateKind(stem); err2 != nil {
				return in, fmt.Errorf("kind prefix %q: %w", p, err2)
			}
		}
		cleaned = append(cleaned, p)
	}
	if len(cleaned) == 0 {
		cleaned = []string{""}
	}
	in.KindPrefixes = cleaned
	if in.Rate <= 0 {
		in.Rate = 5
	}
	if in.Burst < 1 {
		in.Burst = 5
	}
	if in.MaxInFlight < 1 {
		in.MaxInFlight = 2
	}
	if in.MaxInFlight > 16 {
		in.MaxInFlight = 16
	}
	return in, nil
}

func ValidateURL(raw string) error {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url scheme must be http or https")
	}
	if u.Host == "" {
		return fmt.Errorf("url host is required")
	}
	if u.User != nil {
		return fmt.Errorf("url must not embed credentials")
	}
	return nil
}

func (d Collector) Matches(eventType string) bool {
	if !d.Enabled {
		return false
	}
	for _, p := range d.KindPrefixes {
		if lotevent.MatchPrefix(eventType, p) {
			return true
		}
	}
	return false
}

func (d Collector) OutboundSecrets(now time.Time) []string {
	secs := []string{d.Secret}
	if d.PrevSecret != "" && (d.PrevUntil.IsZero() || now.Before(d.PrevUntil)) {
		secs = append(secs, d.PrevSecret)
	}
	return secs
}

type Registry struct {
	mu    sync.Mutex
	clk   wallclock.Clock
	items map[string]Collector
}

func NewRegistry(clk wallclock.Clock) *Registry {
	return &Registry{clk: clk, items: make(map[string]Collector)}
}

func (r *Registry) Create(in CreateInput) (Collector, error) {
	in, err := in.normalize()
	if err != nil {
		return Collector{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.clk.Now()
	d := Collector{
		ID:           lotid.New("col", now),
		Name:         in.Name,
		URL:          strings.TrimSpace(in.URL),
		Secret:       in.Secret,
		KindPrefixes: in.KindPrefixes,
		Ordered:      in.Ordered,
		Rate:         in.Rate,
		Burst:        in.Burst,
		MaxInFlight:  in.MaxInFlight,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if in.Enabled != nil {
		d.Enabled = *in.Enabled
	}
	r.items[d.ID] = d
	return d, nil
}

func (r *Registry) Put(d Collector) error {
	if d.ID == "" {
		return fmt.Errorf("collector id required")
	}
	if err := ValidateURL(d.URL); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	d.UpdatedAt = r.clk.Now()
	r.items[d.ID] = d
	return nil
}

func (r *Registry) Get(id string) (Collector, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.items[id]
	return d, ok
}

func (r *Registry) SetEnabled(id string, enabled bool) (Collector, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.items[id]
	if !ok {
		return Collector{}, fmt.Errorf("collector %s not found", id)
	}
	d.Enabled = enabled
	d.UpdatedAt = r.clk.Now()
	r.items[id] = d
	return d, nil
}

func (r *Registry) RotateSecret(id, next string, overlap time.Duration) (Collector, error) {
	if len(next) < 8 {
		return Collector{}, fmt.Errorf("secret must be at least 8 characters")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.items[id]
	if !ok {
		return Collector{}, fmt.Errorf("collector %s not found", id)
	}
	now := r.clk.Now()
	d.PrevSecret = d.Secret
	d.PrevUntil = now.Add(overlap)
	d.Secret = next
	d.UpdatedAt = now
	r.items[id] = d
	return d, nil
}

func (r *Registry) List() []Collector {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Collector, 0, len(r.items))
	for _, d := range r.items {
		out = append(out, d)
	}
	return out
}

func (r *Registry) Matching(eventType string) []Collector {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Collector, 0)
	for _, d := range r.items {
		if d.Matches(eventType) {
			out = append(out, d)
		}
	}
	return out
}

func (r *Registry) Restore(items []Collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = make(map[string]Collector, len(items))
	for _, d := range items {
		r.items[d.ID] = d
	}
}

func (r *Registry) Public(d Collector) map[string]any {
	return map[string]any{
		"id":              d.ID,
		"name":            d.Name,
		"url":             d.URL,
		"kind_prefixes":   d.KindPrefixes,
		"ordered":         d.Ordered,
		"rate":            d.Rate,
		"burst":           d.Burst,
		"max_in_flight":   d.MaxInFlight,
		"enabled":         d.Enabled,
		"created_at":      d.CreatedAt,
		"updated_at":      d.UpdatedAt,
		"has_prev_secret": d.PrevSecret != "" && (d.PrevUntil.IsZero() || r.clk.Now().Before(d.PrevUntil)),
	}
}
