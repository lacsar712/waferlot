package intake_test

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/lacsar712/waferlot/internal/canon"
	"github.com/lacsar712/waferlot/internal/httphead"
	"github.com/lacsar712/waferlot/internal/intake"
	"github.com/lacsar712/waferlot/internal/mes"
	"github.com/lacsar712/waferlot/internal/once"
	"github.com/lacsar712/waferlot/internal/oncekey"
	"github.com/lacsar712/waferlot/internal/seal"
	"github.com/lacsar712/waferlot/internal/toolkey"
	"github.com/lacsar712/waferlot/internal/waitline"
	"github.com/lacsar712/waferlot/internal/wallclock"
)

func lotBody() []byte {
	return []byte(`{"type":"lot.track_in","payload":{"lot_id":"LOT-1001","tool_id":"LITHO-01","step_id":"litho.photo","process_program":"PP-LITHO-193NM-V3","wafer_count":25}}`)
}

func TestPipelineAcceptsSignedEvent(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	cols := mes.NewRegistry(clk)
	enabled := true
	_, err := cols.Create(mes.CreateInput{
		Name:         "echo",
		URL:          "http://127.0.0.1:8080/api/v1/echo",
		Secret:       "abcdefgh",
		KindPrefixes: []string{"lot"},
		Enabled:      &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := &intake.Pipeline{
		Clk:    clk,
		Window: 5 * time.Minute,
		Keys:   toolkey.New("tool", "supersecret"),
		Nonces: once.New(clk, 5*time.Minute),
		Idem:   oncekey.New(clk, time.Hour),
		Cols:   cols,
		Broker: waitline.NewBroker(clk),
	}
	body := lotBody()
	n := "abcdefghijklmnop"
	ts := clk.Now().Unix()
	sig, err := seal.Sign("supersecret", ts, n, body)
	if err != nil {
		t.Fatal(err)
	}
	h := make(http.Header)
	h.Set(httphead.Timestamp, "1700000000")
	h.Set(httphead.Nonce, n)
	h.Set(httphead.Signature, sig)
	h.Set(httphead.Idempotency, "idemkey1")
	h.Set(httphead.ToolKey, "tool")
	res, code, err := p.Handle(h, body)
	if err != nil {
		t.Fatal(err)
	}
	if code != http.StatusAccepted {
		t.Fatalf("code %d", code)
	}
	if res.Matched != 1 {
		t.Fatalf("matched %d hash %s", res.Matched, canon.SHA256Hex(body))
	}
}

func TestPipelineRejectsPartialTypePrefix(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	cols := mes.NewRegistry(clk)
	enabled := true
	_, err := cols.Create(mes.CreateInput{
		Name:         "too-short",
		URL:          "http://127.0.0.1:8080/api/v1/echo",
		Secret:       "abcdefgh",
		KindPrefixes: []string{"lo"},
		Enabled:      &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := &intake.Pipeline{
		Clk:    clk,
		Window: 5 * time.Minute,
		Keys:   toolkey.New("tool", "supersecret"),
		Nonces: once.New(clk, 5*time.Minute),
		Idem:   oncekey.New(clk, time.Hour),
		Cols:   cols,
		Broker: waitline.NewBroker(clk),
	}
	body := lotBody()
	n := "qrstuvwxyzabcdef"
	ts := clk.Now().Unix()
	sig, err := seal.Sign("supersecret", ts, n, body)
	if err != nil {
		t.Fatal(err)
	}
	h := make(http.Header)
	h.Set(httphead.Timestamp, "1700000000")
	h.Set(httphead.Nonce, n)
	h.Set(httphead.Signature, sig)
	h.Set(httphead.Idempotency, "idemkey-partial")
	h.Set(httphead.ToolKey, "tool")
	res, code, err := p.Handle(h, body)
	if err != nil {
		t.Fatal(err)
	}
	if code != http.StatusAccepted {
		t.Fatalf("code %d", code)
	}
	if res.Matched != 0 {
		t.Fatalf("prefix %q must not match %q, matched=%d ids=%v", "lo", "lot.track_in", res.Matched, res.ForwardIDs)
	}
}

func newPipeline(t *testing.T, clk *wallclock.Frozen) *intake.Pipeline {
	t.Helper()
	cols := mes.NewRegistry(clk)
	enabled := true
	if _, err := cols.Create(mes.CreateInput{
		Name:         "echo",
		URL:          "http://127.0.0.1:8080/api/v1/echo",
		Secret:       "abcdefgh",
		KindPrefixes: []string{"lot"},
		Enabled:      &enabled,
	}); err != nil {
		t.Fatal(err)
	}
	return &intake.Pipeline{
		Clk:    clk,
		Window: 5 * time.Minute,
		Keys:   toolkey.New("tool", "supersecret"),
		Nonces: once.New(clk, 5*time.Minute),
		Idem:   oncekey.New(clk, time.Hour),
		Cols:   cols,
		Broker: waitline.NewBroker(clk),
	}
}

func signedHeaders(t *testing.T, body []byte, nonce, idem string, ts int64) http.Header {
	t.Helper()
	sig, err := seal.Sign("supersecret", ts, nonce, body)
	if err != nil {
		t.Fatal(err)
	}
	h := make(http.Header)
	h.Set(httphead.Timestamp, strconv.FormatInt(ts, 10))
	h.Set(httphead.Nonce, nonce)
	h.Set(httphead.Signature, sig)
	h.Set(httphead.Idempotency, idem)
	h.Set(httphead.ToolKey, "tool")
	return h
}

func TestPipelineSkewIsBadRequest(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	p := newPipeline(t, clk)
	body := lotBody()
	ts := clk.Now().Add(-20 * time.Minute).Unix()
	_, code, err := p.Handle(signedHeaders(t, body, "skewnonce16chars", "idem-skew-01", ts), body)
	if err == nil {
		t.Fatal("expected skew error")
	}
	if code != http.StatusBadRequest {
		t.Fatalf("code %d want 400", code)
	}
}

func TestPipelineIdempotencyConflict(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	p := newPipeline(t, clk)
	body1 := []byte(`{"type":"lot.track_in","payload":{"lot_id":"LOT-1001","tool_id":"LITHO-01","step_id":"litho.photo","process_program":"PP-LITHO-193NM-V3","wafer_count":25}}`)
	body2 := []byte(`{"type":"lot.track_in","payload":{"lot_id":"LOT-1002","tool_id":"LITHO-01","step_id":"litho.photo","process_program":"PP-LITHO-193NM-V3","wafer_count":24}}`)
	ts := clk.Now().Unix()
	_, code1, err := p.Handle(signedHeaders(t, body1, "idemnonce16charA", "same-idem-key", ts), body1)
	if err != nil || code1 != http.StatusAccepted {
		t.Fatalf("first: code=%d err=%v", code1, err)
	}
	_, code2, err := p.Handle(signedHeaders(t, body2, "idemnonce16charB", "same-idem-key", ts), body2)
	if err == nil {
		t.Fatal("expected conflict")
	}
	if code2 != http.StatusConflict {
		t.Fatalf("code %d want 409", code2)
	}
}

func TestPipelineUnknownSourceKeyUnauthorized(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	p := newPipeline(t, clk)
	body := lotBody()
	h := signedHeaders(t, body, "unknownsrc16char", "idem-unknown-01", clk.Now().Unix())
	h.Set(httphead.ToolKey, "no-such-key")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unknown tool key panicked: %v", r)
		}
	}()
	_, code, err := p.Handle(h, body)
	if err == nil {
		t.Fatal("expected unauthorized")
	}
	if code != http.StatusUnauthorized {
		t.Fatalf("code %d want 401", code)
	}
}

func TestPipelineInvalidJSONIsBadRequest(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	p := newPipeline(t, clk)
	body := []byte(`{`)
	_, code, err := p.Handle(signedHeaders(t, body, "jsonnonce16chars", "idem-json-01", clk.Now().Unix()), body)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if code != http.StatusBadRequest {
		t.Fatalf("broken json want 400, got %d", code)
	}
}

func TestPipelineMissingPayloadUnprocessable(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	p := newPipeline(t, clk)
	body := []byte(`{"type":"lot.track_in"}`)
	_, code, err := p.Handle(signedHeaders(t, body, "paylnonce16chars", "idem-payload-01", clk.Now().Unix()), body)
	if err == nil {
		t.Fatal("expected payload error")
	}
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("missing payload want 422, got %d", code)
	}
}

func TestPipelineDuplicateNonceConflict(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	p := newPipeline(t, clk)
	body := lotBody()
	ts := clk.Now().Unix()
	n := "dupnonce16charsx"
	_, code1, err := p.Handle(signedHeaders(t, body, n, "idem-nonce-a", ts), body)
	if err != nil || code1 != http.StatusAccepted {
		t.Fatalf("first: code=%d err=%v", code1, err)
	}
	_, code2, err := p.Handle(signedHeaders(t, body, n, "idem-nonce-b", ts), body)
	if err == nil {
		t.Fatal("expected nonce conflict")
	}
	if code2 != http.StatusConflict {
		t.Fatalf("duplicate nonce want 409, got %d", code2)
	}
}
