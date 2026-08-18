package runlog_test

import (
	"testing"
	"time"

	"github.com/lacsar712/waferlot/internal/runlog"
)

func TestGetPreservesBodyForReplay(t *testing.T) {
	log := runlog.New(50)
	body := []byte(`{"type":"lot.track_in","payload":{"password":"hunter2","process_program":"PP-LITHO-193NM-V3"}}`)
	log.Append(runlog.Entry{
		At:          time.Unix(1, 0),
		EventID:     "lot1",
		ForwardID:   "fwd1",
		CollectorID: "col1",
		Attempt:     1,
		Kind:        "terminal",
		Type:        "lot.track_in",
		Body:        body,
	})
	got, ok := log.Get("fwd1")
	if !ok {
		t.Fatal("missing entry")
	}
	if string(got.Body) != string(body) {
		t.Fatalf("Get stripped body: %q", got.Body)
	}
	listed := log.List("", 10)
	if len(listed) != 1 {
		t.Fatalf("list n=%d", len(listed))
	}
	if listed[0].Body != nil {
		t.Fatalf("List must hide raw body, got %q", listed[0].Body)
	}
}
