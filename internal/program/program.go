package program

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type Status string

const (
	Draft     Status = "draft"
	Qualified Status = "qualified"
	Obsolete  Status = "obsolete"
)

func (s Status) Valid() bool {
	switch s {
	case Draft, Qualified, Obsolete:
		return true
	default:
		return false
	}
}

// Program is a process program executed on a tool at a step.
type Program struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Revision   int       `json:"revision"`
	Family     string    `json:"family"`
	Status     Status    `json:"status"`
	Tools      []string  `json:"qualified_tools"`
	StepFamily string    `json:"step_family"`
	UpdatedAt  time.Time `json:"updated_at"`
	Notes      string    `json:"notes,omitempty"`
}

type Catalog struct {
	mu   sync.Mutex
	byID map[string]Program
}

func New() *Catalog {
	return &Catalog{byID: make(map[string]Program)}
}

func Seed() *Catalog {
	c := New()
	now := time.Unix(1_700_000_000, 0).UTC()
	items := []Program{
		{ID: "PP-LITHO-193NM-V3", Name: "193nm photo", Revision: 3, Family: "litho", Status: Qualified, Tools: []string{"LITHO-01"}, StepFamily: "litho", UpdatedAt: now, Notes: "production photo"},
		{ID: "PP-LITHO-OVERLAY-V1", Name: "overlay send-ahead", Revision: 1, Family: "litho", Status: Qualified, Tools: []string{"LITHO-01"}, StepFamily: "litho", UpdatedAt: now},
		{ID: "PP-ETCH-POLY-V2", Name: "poly etch", Revision: 2, Family: "etch", Status: Qualified, Tools: []string{"ETCH-07"}, StepFamily: "etch", UpdatedAt: now},
		{ID: "PP-ETCH-OXIDE-V4", Name: "oxide etch", Revision: 4, Family: "etch", Status: Qualified, Tools: []string{"ETCH-07"}, StepFamily: "etch", UpdatedAt: now},
		{ID: "PP-IMP-WELL-V1", Name: "well implant", Revision: 1, Family: "implant", Status: Qualified, Tools: []string{"IMP-03"}, StepFamily: "implant", UpdatedAt: now},
		{ID: "PP-CMP-OXIDE-V2", Name: "oxide polish", Revision: 2, Family: "cmp", Status: Qualified, Tools: []string{"CMP-02"}, StepFamily: "cmp", UpdatedAt: now},
		{ID: "PP-MET-CDSEM-V1", Name: "CD-SEM", Revision: 1, Family: "metrology", Status: Qualified, Tools: []string{"MET-11"}, StepFamily: "metrology", UpdatedAt: now},
		{ID: "PP-LITHO-DRAFT-V9", Name: "experimental photo", Revision: 9, Family: "litho", Status: Draft, Tools: []string{"LITHO-01"}, StepFamily: "litho", UpdatedAt: now, Notes: "not released"},
	}
	for _, p := range items {
		c.byID[p.ID] = p
	}
	return c
}

func (c *Catalog) Put(p Program) error {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.Family = strings.ToLower(strings.TrimSpace(p.Family))
	p.StepFamily = strings.ToLower(strings.TrimSpace(p.StepFamily))
	if p.ID == "" {
		return fmt.Errorf("process program id is required")
	}
	if !strings.HasPrefix(p.ID, "PP-") {
		return fmt.Errorf("process program id must start with PP-")
	}
	if p.Name == "" {
		return fmt.Errorf("process program name is required")
	}
	if p.Revision < 1 {
		return fmt.Errorf("revision must be >= 1")
	}
	if !p.Status.Valid() {
		return fmt.Errorf("unknown process program status %q", p.Status)
	}
	if p.Family == "" || p.StepFamily == "" {
		return fmt.Errorf("family and step_family are required")
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now().UTC()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byID[p.ID] = p
	return nil
}

func (c *Catalog) Get(id string) (Program, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.byID[id]
	return p, ok
}

func (c *Catalog) List() []Program {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Program, 0, len(c.byID))
	for _, p := range c.byID {
		out = append(out, p)
	}
	return out
}

func (c *Catalog) Usable(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.byID[id]
	if !ok {
		return fmt.Errorf("unknown process program %s", id)
	}
	if p.Status != Qualified {
		return fmt.Errorf("process program %s is %s", id, p.Status)
	}
	return nil
}

func (c *Catalog) QualifiedOn(id, toolID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.byID[id]
	if !ok || p.Status != Qualified {
		return false
	}
	for _, t := range p.Tools {
		if t == toolID {
			return true
		}
	}
	return false
}

func (c *Catalog) ForFamily(family string) []Program {
	c.mu.Lock()
	defer c.mu.Unlock()
	family = strings.ToLower(strings.TrimSpace(family))
	out := make([]Program, 0)
	for _, p := range c.byID {
		if p.Family == family && p.Status == Qualified {
			out = append(out, p)
		}
	}
	return out
}
