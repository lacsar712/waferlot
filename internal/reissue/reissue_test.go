package reissue_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/lacsar712/waferlot/internal/mask"
	"github.com/lacsar712/waferlot/internal/reissue"
	"github.com/lacsar712/waferlot/internal/runlog"
)

func TestFromRunLogUsesOriginalBody(t *testing.T) {
	body := []byte(`{"type":"lot.track_in","payload":{"password":"hunter2","lot_id":"LOT-1001","process_program":"PP-LITHO-193NM-V3"}}`)
	e := runlog.Entry{
		EventID:      "lot1",
		ForwardID:    "fwd1",
		CollectorID:  "col1",
		Type:         "lot.track_in",
		Body:         body,
		BodyRedacted: mask.JSON(body),
	}
	j, err := reissue.FromRunLog(e, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(j.Body, body) {
		t.Fatalf("reissue body %s want original, not redacted %s", j.Body, e.BodyRedacted)
	}
	if bytes.Contains(j.Body, []byte("***")) {
		t.Fatal("reissue used masked payload")
	}
}

func TestFromJournalRejectsEmptyBody(t *testing.T) {
	_, err := reissue.FromRunLog(runlog.Entry{
		EventID:     "lot1",
		ForwardID:   "fwd-empty",
		CollectorID: "col1",
		Type:        "lot.track_in",
	}, time.Unix(1, 0))
	if err == nil {
		t.Fatal("empty run log body must not become a reissue work item")
	}
}
