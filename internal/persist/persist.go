package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lacsar712/waferlot/internal/baygate"
	"github.com/lacsar712/waferlot/internal/dispatch"
	"github.com/lacsar712/waferlot/internal/holdbin"
	"github.com/lacsar712/waferlot/internal/mes"
	"github.com/lacsar712/waferlot/internal/once"
	"github.com/lacsar712/waferlot/internal/oncekey"
	"github.com/lacsar712/waferlot/internal/runlog"
	"github.com/lacsar712/waferlot/internal/toolkey"
	"github.com/lacsar712/waferlot/internal/waitline"
)

type Snapshot struct {
	SavedAt     time.Time        `json:"saved_at"`
	Collectors  []mes.Collector  `json:"collectors"`
	Idempotency []oncekey.Record `json:"idempotency"`
	Nonces      []once.Record    `json:"nonces"`
	RunLog      []runlog.Entry   `json:"runlog"`
	HoldBin     []holdbin.Item   `json:"holdbin"`
	Work        []dispatch.Work  `json:"work"`
	ToolKeys    []toolkey.Key    `json:"tool_keys"`
	Gates       baygate.Snapshot `json:"gates"`
}

type File struct {
	mu   sync.Mutex
	path string
}

func New(dir string) (*File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &File{path: filepath.Join(dir, "snapshot.json")}, nil
}

func (f *File) Save(s Snapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s.SavedAt = time.Now().UTC()
	tmp := f.path + ".tmp"
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

func (f *File) Load() (Snapshot, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, err := os.ReadFile(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Snapshot{}, false, nil
		}
		return Snapshot{}, false, err
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return Snapshot{}, false, err
	}
	return s, true, nil
}

type Deps struct {
	Cols   *mes.Registry
	Idem   *oncekey.Store
	Nonces *once.Book
	Log    *runlog.Log
	Dead   *holdbin.Bin
	Broker *waitline.Broker
	Keys   *toolkey.Keys
	Gates  *baygate.Gates
}

func Capture(d Deps) Snapshot {
	return Snapshot{
		Collectors:  d.Cols.List(),
		Idempotency: d.Idem.Snapshot(),
		Nonces:      d.Nonces.Snapshot(),
		RunLog:      d.Log.Snapshot(),
		HoldBin:     d.Dead.Snapshot(),
		Work:        d.Broker.Snapshot(),
		ToolKeys:    d.Keys.Snapshot(),
		Gates:       d.Gates.Snapshot(),
	}
}

func Apply(d Deps, s Snapshot) {
	if len(s.Collectors) > 0 {
		d.Cols.Restore(s.Collectors)
	}
	d.Idem.Restore(s.Idempotency)
	d.Nonces.Restore(s.Nonces)
	d.Log.Restore(s.RunLog)
	d.Dead.Restore(s.HoldBin)
	meta := map[string]waitline.LineMeta{}
	for _, dest := range s.Collectors {
		meta[dest.ID] = waitline.LineMeta{Ordered: dest.Ordered, MaxInFlight: dest.MaxInFlight}
	}
	d.Broker.Restore(s.Work, meta)
	if len(s.ToolKeys) > 0 {
		d.Keys.Restore(s.ToolKeys)
	}
	d.Gates.Restore(s.Gates)
}
