package intake

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/lacsar712/waferlot/internal/canon"
	"github.com/lacsar712/waferlot/internal/dispatch"
	"github.com/lacsar712/waferlot/internal/httphead"
	"github.com/lacsar712/waferlot/internal/lotevent"
	"github.com/lacsar712/waferlot/internal/lotid"
	"github.com/lacsar712/waferlot/internal/mes"
	"github.com/lacsar712/waferlot/internal/once"
	"github.com/lacsar712/waferlot/internal/oncekey"
	"github.com/lacsar712/waferlot/internal/seal"
	"github.com/lacsar712/waferlot/internal/stepper"
	"github.com/lacsar712/waferlot/internal/toolkey"
	"github.com/lacsar712/waferlot/internal/waitline"
	"github.com/lacsar712/waferlot/internal/wallclock"
)

type Pipeline struct {
	Clk    wallclock.Clock
	Window time.Duration
	Keys   *toolkey.Keys
	Nonces *once.Book
	Idem   *oncekey.Store
	Cols   *mes.Registry
	Broker *waitline.Broker
}

type Result struct {
	EventID    string   `json:"event_id"`
	Replay     bool     `json:"replay"`
	Matched    int      `json:"matched"`
	ForwardIDs []string `json:"forward_ids"`
}

func (p *Pipeline) Handle(h http.Header, body []byte) (Result, int, error) {
	in, err := httphead.ParseInbound(h)
	if err != nil {
		return Result{}, http.StatusBadRequest, err
	}
	if err := canon.ValidIdempotencyKey(in.IdemKey); err != nil {
		return Result{}, http.StatusBadRequest, err
	}
	secrets := p.Keys.Secrets(in.ToolKey)
	if len(secrets) == 0 {
		return Result{}, http.StatusUnauthorized, fmt.Errorf("unknown tool key %q", in.ToolKey)
	}
	if err := seal.Verify(p.Clk, p.Window, secrets, seal.Headers{
		Timestamp: in.Timestamp,
		Nonce:     in.Nonce,
		Signature: in.Signature,
	}, body); err != nil {
		if err == seal.ErrSkew {
			return Result{}, http.StatusBadRequest, err
		}
		return Result{}, http.StatusUnauthorized, err
	}
	env, err := lotevent.Parse(body)
	if err != nil {
		var syn *json.SyntaxError
		if errors.As(err, &syn) {
			return Result{}, http.StatusBadRequest, err
		}
		return Result{}, http.StatusUnprocessableEntity, err
	}
	if err := p.Nonces.CheckAndRemember(in.Nonce); err != nil {
		return Result{}, http.StatusConflict, err
	}
	now := p.Clk.Now()
	eventID := lotid.New("lot", now)
	bodyHash := canon.SHA256Hex(body)
	existing, replay, err := p.Idem.Remember(in.IdemKey, bodyHash, eventID)
	if err != nil {
		if errors.Is(err, oncekey.ErrConflict) {
			return Result{}, http.StatusConflict, err
		}
		return Result{}, http.StatusBadRequest, err
	}
	if replay {
		return Result{EventID: existing, Replay: true}, http.StatusOK, nil
	}
	matched := p.Cols.Matching(env.Type)
	plan := stepper.Fanout(eventID, env.Type, body, matched, now)
	ids := make([]string, 0, len(plan.Items))
	for _, item := range plan.Items {
		d, ok := p.Cols.Get(item.CollectorID)
		if !ok {
			continue
		}
		p.Broker.Ensure(d.ID, d.Ordered, d.MaxInFlight)
		p.Broker.Enqueue(dispatch.Work{
			EventID:     eventID,
			ForwardID:   item.ForwardID,
			CollectorID: item.CollectorID,
			Kind:        env.Type,
			Body:        append([]byte(nil), body...),
			Attempt:     0,
			NotBefore:   now,
			CreatedAt:   now,
		}, d.Ordered, d.MaxInFlight)
		ids = append(ids, item.ForwardID)
	}
	return Result{
		EventID:    eventID,
		Matched:    len(ids),
		ForwardIDs: ids,
	}, http.StatusAccepted, nil
}
