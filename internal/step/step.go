package step

import (
	"fmt"
	"strings"
	"sync"
)

// Step is a process step in the FAB flow. A lot tracks in/out of a tool
// while executing a process program at this step.
type Step struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Seq              int      `json:"seq"`
	Family           string   `json:"family"`
	AllowedPrograms  []string `json:"allowed_process_programs"`
	TrackInRequired  bool     `json:"track_in_required"`
	MetrologyFollows bool     `json:"metrology_follows"`
	HoldOnFail       bool     `json:"hold_on_fail"`
}

type Flow struct {
	mu   sync.Mutex
	byID map[string]Step
	seq  []string
}

func New() *Flow {
	return &Flow{byID: make(map[string]Step)}
}

func Seed() *Flow {
	f := New()
	items := []Step{
		{ID: "litho.photo", Name: "photo", Seq: 10, Family: "litho", AllowedPrograms: []string{"PP-LITHO-193NM-V3", "PP-LITHO-OVERLAY-V1"}, TrackInRequired: true, MetrologyFollows: true, HoldOnFail: true},
		{ID: "etch.poly", Name: "poly etch", Seq: 20, Family: "etch", AllowedPrograms: []string{"PP-ETCH-POLY-V2"}, TrackInRequired: true, MetrologyFollows: true, HoldOnFail: true},
		{ID: "implant.well", Name: "well implant", Seq: 30, Family: "implant", AllowedPrograms: []string{"PP-IMP-WELL-V1"}, TrackInRequired: true, MetrologyFollows: false, HoldOnFail: true},
		{ID: "cmp.oxide", Name: "oxide CMP", Seq: 40, Family: "cmp", AllowedPrograms: []string{"PP-CMP-OXIDE-V2"}, TrackInRequired: true, MetrologyFollows: true, HoldOnFail: true},
		{ID: "met.cd", Name: "CD metrology", Seq: 50, Family: "metrology", AllowedPrograms: []string{"PP-MET-CDSEM-V1"}, TrackInRequired: false, MetrologyFollows: false, HoldOnFail: false},
	}
	for _, s := range items {
		f.byID[s.ID] = s
		f.seq = append(f.seq, s.ID)
	}
	return f
}

func (f *Flow) Put(s Step) error {
	s.ID = strings.ToLower(strings.TrimSpace(s.ID))
	s.Name = strings.TrimSpace(s.Name)
	s.Family = strings.ToLower(strings.TrimSpace(s.Family))
	if s.ID == "" {
		return fmt.Errorf("step id is required")
	}
	if !strings.Contains(s.ID, ".") {
		return fmt.Errorf("step id must be family.name")
	}
	if s.Name == "" {
		return fmt.Errorf("step name is required")
	}
	if s.Seq < 1 {
		return fmt.Errorf("step seq must be >= 1")
	}
	if len(s.AllowedPrograms) == 0 {
		return fmt.Errorf("step must list allowed process programs")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byID[s.ID]; !ok {
		f.seq = append(f.seq, s.ID)
	}
	f.byID[s.ID] = s
	return nil
}

func (f *Flow) Get(id string) (Step, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.byID[id]
	return s, ok
}

func (f *Flow) List() []Step {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Step, 0, len(f.seq))
	for _, id := range f.seq {
		if s, ok := f.byID[id]; ok {
			out = append(out, s)
		}
	}
	return out
}

func (f *Flow) Allows(stepID, processProgram string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.byID[stepID]
	if !ok {
		return false
	}
	for _, p := range s.AllowedPrograms {
		if p == processProgram {
			return true
		}
	}
	return false
}

func (f *Flow) Next(afterID string) (Step, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, id := range f.seq {
		if id == afterID && i+1 < len(f.seq) {
			return f.byID[f.seq[i+1]], true
		}
	}
	return Step{}, false
}

func (f *Flow) Previous(id string) (Step, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, cur := range f.seq {
		if cur == id && i > 0 {
			return f.byID[f.seq[i-1]], true
		}
	}
	return Step{}, false
}

func (f *Flow) ValidateMove(fromID, toID string) error {
	if fromID == "" {
		_, ok := f.Get(toID)
		if !ok {
			return fmt.Errorf("unknown step %s", toID)
		}
		return nil
	}
	next, ok := f.Next(fromID)
	if !ok {
		return fmt.Errorf("step %s has no successor", fromID)
	}
	if next.ID != toID {
		return fmt.Errorf("expected next step %s, got %s", next.ID, toID)
	}
	return nil
}
