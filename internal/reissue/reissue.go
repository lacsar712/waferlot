package reissue

import (
	"fmt"
	"time"

	"github.com/lacsar712/waferlot/internal/dispatch"
	"github.com/lacsar712/waferlot/internal/holdbin"
	"github.com/lacsar712/waferlot/internal/lotid"
	"github.com/lacsar712/waferlot/internal/runlog"
)

func FromRunLog(e runlog.Entry, now time.Time) (dispatch.Work, error) {
	if len(e.Body) == 0 {
		return dispatch.Work{}, fmt.Errorf("run log entry %s has no stored body", e.ForwardID)
	}
	return dispatch.Work{
		EventID:     e.EventID,
		ForwardID:   lotid.New("fwd", now),
		CollectorID: e.CollectorID,
		Kind:        e.Type,
		Body:        append([]byte(nil), e.Body...),
		Attempt:     0,
		NotBefore:   now,
		CreatedAt:   now,
		ReissueOf:   e.ForwardID,
	}, nil
}

func FromHoldBin(it holdbin.Item, now time.Time) (dispatch.Work, error) {
	if len(it.Body) == 0 {
		return dispatch.Work{}, fmt.Errorf("hold bin item %s has no stored body", it.ForwardID)
	}
	return dispatch.Work{
		EventID:     it.EventID,
		ForwardID:   lotid.New("fwd", now),
		CollectorID: it.CollectorID,
		Kind:        it.Kind,
		Body:        append([]byte(nil), it.Body...),
		Attempt:     0,
		NotBefore:   now,
		CreatedAt:   now,
		ReissueOf:   it.ForwardID,
	}, nil
}
