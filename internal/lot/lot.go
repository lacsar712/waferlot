package lot

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type Status string

const (
	Queued   Status = "queued"
	Active   Status = "active"
	Held     Status = "held"
	Complete Status = "complete"
	Scrapped Status = "scrapped"
)

func (s Status) Valid() bool {
	switch s {
	case Queued, Active, Held, Complete, Scrapped:
		return true
	default:
		return false
	}
}

// Record is a wafer lot moving through the FAB flow.
type Record struct {
	ID             string    `json:"id"`
	Product        string    `json:"product"`
	WaferCount     int       `json:"wafer_count"`
	CarrierID      string    `json:"carrier_id"`
	CurrentStep    string    `json:"current_step"`
	CurrentTool    string    `json:"current_tool,omitempty"`
	ProcessProgram string    `json:"process_program,omitempty"`
	Status         Status    `json:"status"`
	HoldCode       string    `json:"hold_code,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Yard struct {
	mu   sync.Mutex
	byID map[string]Record
}

func New() *Yard {
	return &Yard{byID: make(map[string]Record)}
}

func Seed() *Yard {
	y := New()
	now := time.Unix(1_700_000_000, 0).UTC()
	items := []Record{
		{ID: "LOT-1001", Product: "N7-LOGIC", WaferCount: 25, CarrierID: "FOUP-A12", CurrentStep: "litho.photo", Status: Queued, UpdatedAt: now},
		{ID: "LOT-1002", Product: "N7-LOGIC", WaferCount: 24, CarrierID: "FOUP-B07", CurrentStep: "etch.poly", Status: Active, CurrentTool: "ETCH-07", ProcessProgram: "PP-ETCH-POLY-V2", UpdatedAt: now},
		{ID: "LOT-1044", Product: "N5-SRAM", WaferCount: 25, CarrierID: "FOUP-C03", CurrentStep: "cmp.oxide", Status: Held, HoldCode: "PARTICLE", UpdatedAt: now},
	}
	for _, r := range items {
		y.byID[r.ID] = r
	}
	return y
}

func (y *Yard) Put(r Record) error {
	r.ID = strings.TrimSpace(r.ID)
	r.Product = strings.TrimSpace(r.Product)
	r.CarrierID = strings.TrimSpace(r.CarrierID)
	if r.ID == "" {
		return fmt.Errorf("lot id is required")
	}
	if r.Product == "" {
		return fmt.Errorf("product is required")
	}
	if r.WaferCount < 1 || r.WaferCount > 25 {
		return fmt.Errorf("wafer_count must be in [1,25]")
	}
	if r.CarrierID == "" {
		return fmt.Errorf("carrier_id is required")
	}
	if !r.Status.Valid() {
		return fmt.Errorf("unknown lot status %q", r.Status)
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = time.Now().UTC()
	}
	y.mu.Lock()
	defer y.mu.Unlock()
	y.byID[r.ID] = r
	return nil
}

func (y *Yard) Get(id string) (Record, bool) {
	y.mu.Lock()
	defer y.mu.Unlock()
	r, ok := y.byID[id]
	return r, ok
}

func (y *Yard) List() []Record {
	y.mu.Lock()
	defer y.mu.Unlock()
	out := make([]Record, 0, len(y.byID))
	for _, r := range y.byID {
		out = append(out, r)
	}
	return out
}

func (y *Yard) Hold(id, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return fmt.Errorf("hold_code is required")
	}
	y.mu.Lock()
	defer y.mu.Unlock()
	r, ok := y.byID[id]
	if !ok {
		return fmt.Errorf("unknown lot %s", id)
	}
	if r.Status == Complete || r.Status == Scrapped {
		return fmt.Errorf("lot %s is %s", id, r.Status)
	}
	r.Status = Held
	r.HoldCode = code
	r.UpdatedAt = time.Now().UTC()
	y.byID[id] = r
	return nil
}

func (y *Yard) Release(id string) error {
	y.mu.Lock()
	defer y.mu.Unlock()
	r, ok := y.byID[id]
	if !ok {
		return fmt.Errorf("unknown lot %s", id)
	}
	if r.Status != Held {
		return fmt.Errorf("lot %s is not held", id)
	}
	r.Status = Queued
	r.HoldCode = ""
	r.UpdatedAt = time.Now().UTC()
	y.byID[id] = r
	return nil
}

func (y *Yard) ApplyTrackIn(id, toolID, stepID, processProgram string) error {
	y.mu.Lock()
	defer y.mu.Unlock()
	r, ok := y.byID[id]
	if !ok {
		return fmt.Errorf("unknown lot %s", id)
	}
	if r.Status == Held {
		return fmt.Errorf("lot %s is held (%s)", id, r.HoldCode)
	}
	if r.Status == Complete || r.Status == Scrapped {
		return fmt.Errorf("lot %s is %s", id, r.Status)
	}
	r.Status = Active
	r.CurrentTool = toolID
	r.CurrentStep = stepID
	r.ProcessProgram = processProgram
	r.UpdatedAt = time.Now().UTC()
	y.byID[id] = r
	return nil
}

func (y *Yard) ApplyTrackOut(id string) error {
	y.mu.Lock()
	defer y.mu.Unlock()
	r, ok := y.byID[id]
	if !ok {
		return fmt.Errorf("unknown lot %s", id)
	}
	if r.Status != Active {
		return fmt.Errorf("lot %s is not active", id)
	}
	r.Status = Queued
	r.CurrentTool = ""
	r.UpdatedAt = time.Now().UTC()
	y.byID[id] = r
	return nil
}
