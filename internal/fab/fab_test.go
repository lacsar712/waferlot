package fab_test

import (
	"testing"
	"time"

	"github.com/lacsar712/waferlot/internal/fab"
	"github.com/lacsar712/waferlot/internal/runlog"
	"github.com/lacsar712/waferlot/internal/settings"
)

func TestReplayEmptyJournalBodyFails(t *testing.T) {
	a, err := fab.New(settings.Config{
		Addr:         ":0",
		DataDir:      t.TempDir(),
		IngestSecret: "dev-tool-secret",
		Window:       5 * time.Minute,
		IdemTTL:      time.Hour,
		Workers:      1,
		PublicBase:   "http://127.0.0.1:8080",
		EchoPath:     "/api/v1/echo",
	})
	if err != nil {
		t.Fatal(err)
	}
	cols := a.Cols.List()
	if len(cols) == 0 {
		t.Fatal("expected seeded collector")
	}
	a.Log.Append(runlog.Entry{
		EventID:     "lot-empty",
		ForwardID:   "fwd-empty-body",
		CollectorID: cols[0].ID,
		Type:        "lot.track_in",
	})
	if _, err := a.Reissue("fwd-empty-body"); err == nil {
		t.Fatal("reissue of empty run log body must fail")
	}
	if a.Broker.Depth() != 0 {
		t.Fatalf("failed reissue must not enqueue, depth=%d", a.Broker.Depth())
	}
}
