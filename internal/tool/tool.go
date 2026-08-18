package tool

import (
	"fmt"
	"strings"
	"sync"
)

type State string

const (
	Idle        State = "idle"
	Running     State = "running"
	Down        State = "down"
	PM          State = "pm"
	Engineering State = "engineering"
)

func (s State) Valid() bool {
	switch s {
	case Idle, Running, Down, PM, Engineering:
		return true
	default:
		return false
	}
}

func (s State) AcceptsTrackIn() bool {
	return s == Idle || s == Running
}

// Tool is a piece of FAB equipment that posts lot-step events.
type Tool struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Bay               string   `json:"bay"`
	Family            string   `json:"family"`
	State             State    `json:"state"`
	QualifiedPrograms []string `json:"qualified_process_programs"`
	MaxWafers         int      `json:"max_wafers"`
	CurrentLot        string   `json:"current_lot,omitempty"`
	LastAlarm         string   `json:"last_alarm,omitempty"`
}

type Floor struct {
	mu   sync.Mutex
	byID map[string]Tool
}

func New() *Floor {
	return &Floor{byID: make(map[string]Tool)}
}

func Seed() *Floor {
	f := New()
	items := []Tool{
		{ID: "LITHO-01", Name: "193nm scanner", Bay: "BAY-L1", Family: "litho", State: Idle, QualifiedPrograms: []string{"PP-LITHO-193NM-V3", "PP-LITHO-OVERLAY-V1"}, MaxWafers: 25},
		{ID: "ETCH-07", Name: "poly etcher", Bay: "BAY-E2", Family: "etch", State: Idle, QualifiedPrograms: []string{"PP-ETCH-POLY-V2", "PP-ETCH-OXIDE-V4"}, MaxWafers: 25},
		{ID: "IMP-03", Name: "well implanter", Bay: "BAY-I1", Family: "implant", State: Idle, QualifiedPrograms: []string{"PP-IMP-WELL-V1"}, MaxWafers: 25},
		{ID: "CMP-02", Name: "oxide CMP", Bay: "BAY-C1", Family: "cmp", State: PM, QualifiedPrograms: []string{"PP-CMP-OXIDE-V2"}, MaxWafers: 25},
		{ID: "MET-11", Name: "CD-SEM", Bay: "BAY-M3", Family: "metrology", State: Idle, QualifiedPrograms: []string{"PP-MET-CDSEM-V1"}, MaxWafers: 25},
	}
	for _, t := range items {
		f.byID[t.ID] = t
	}
	return f
}

func (f *Floor) Put(t Tool) error {
	t.ID = strings.TrimSpace(t.ID)
	t.Name = strings.TrimSpace(t.Name)
	t.Bay = strings.TrimSpace(t.Bay)
	t.Family = strings.ToLower(strings.TrimSpace(t.Family))
	if t.ID == "" {
		return fmt.Errorf("tool id is required")
	}
	if t.Name == "" {
		return fmt.Errorf("tool name is required")
	}
	if t.Bay == "" {
		return fmt.Errorf("bay is required")
	}
	if t.Family == "" {
		return fmt.Errorf("family is required")
	}
	if !t.State.Valid() {
		return fmt.Errorf("unknown tool state %q", t.State)
	}
	if t.MaxWafers < 1 || t.MaxWafers > 25 {
		return fmt.Errorf("max wafers must be in [1,25]")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[t.ID] = t
	return nil
}

func (f *Floor) Get(id string) (Tool, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	return t, ok
}

func (f *Floor) List() []Tool {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Tool, 0, len(f.byID))
	for _, t := range f.byID {
		out = append(out, t)
	}
	return out
}

func (f *Floor) SetState(id string, st State) error {
	if !st.Valid() {
		return fmt.Errorf("unknown tool state %q", st)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok {
		return fmt.Errorf("tool %s not found", id)
	}
	if t.State == Running && st == PM && t.CurrentLot != "" {
		return fmt.Errorf("tool %s still has lot %s", id, t.CurrentLot)
	}
	t.State = st
	if st != Running {
		t.CurrentLot = ""
	}
	f.byID[id] = t
	return nil
}

func (f *Floor) Qualified(id, processProgram string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok {
		return false
	}
	for _, p := range t.QualifiedPrograms {
		if p == processProgram {
			return true
		}
	}
	return false
}

func (f *Floor) CanTrackIn(id, lotID, processProgram string, wafers int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok {
		return fmt.Errorf("unknown tool %s", id)
	}
	if !t.State.AcceptsTrackIn() {
		return fmt.Errorf("tool %s state %s does not accept track-in", id, t.State)
	}
	if t.CurrentLot != "" && t.CurrentLot != lotID {
		return fmt.Errorf("tool %s is occupied by %s", id, t.CurrentLot)
	}
	if wafers < 0 || wafers > t.MaxWafers {
		return fmt.Errorf("wafer count %d exceeds tool max %d", wafers, t.MaxWafers)
	}
	okProg := false
	for _, p := range t.QualifiedPrograms {
		if p == processProgram {
			okProg = true
			break
		}
	}
	if !okProg {
		return fmt.Errorf("process program %s is not qualified on %s", processProgram, id)
	}
	return nil
}

func (f *Floor) TrackIn(id, lotID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok {
		return fmt.Errorf("unknown tool %s", id)
	}
	t.State = Running
	t.CurrentLot = lotID
	f.byID[id] = t
	return nil
}

func (f *Floor) TrackOut(id, lotID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok {
		return fmt.Errorf("unknown tool %s", id)
	}
	if t.CurrentLot != "" && t.CurrentLot != lotID {
		return fmt.Errorf("tool %s is running %s not %s", id, t.CurrentLot, lotID)
	}
	t.State = Idle
	t.CurrentLot = ""
	f.byID[id] = t
	return nil
}

func (f *Floor) Alarm(id, code string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok {
		return fmt.Errorf("unknown tool %s", id)
	}
	t.LastAlarm = strings.TrimSpace(code)
	if t.LastAlarm != "" {
		t.State = Down
	}
	f.byID[id] = t
	return nil
}
