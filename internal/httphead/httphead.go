package httphead

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	Timestamp   = "X-Wafer-Timestamp"
	Nonce       = "X-Wafer-Nonce"
	Signature   = "X-Wafer-Signature"
	Idempotency = "Idempotency-Key"
	ToolKey     = "X-Wafer-Tool-Key"
	LotID       = "X-Wafer-Lot-Id"
	ForwardID   = "X-Wafer-Forward-Id"
	Attempt     = "X-Wafer-Attempt"
	CollectorID = "X-Wafer-Collector"
	BodySHA     = "X-Wafer-Body-Sha256"
)

type Inbound struct {
	Timestamp int64
	Nonce     string
	Signature string
	IdemKey   string
	ToolKey   string
}

func ParseInbound(h http.Header) (Inbound, error) {
	var in Inbound
	ts := strings.TrimSpace(h.Get(Timestamp))
	if ts == "" {
		return in, fmt.Errorf("missing %s", Timestamp)
	}
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || n <= 0 {
		return in, fmt.Errorf("invalid %s", Timestamp)
	}
	in.Timestamp = n
	in.Nonce = strings.TrimSpace(h.Get(Nonce))
	if in.Nonce == "" {
		return in, fmt.Errorf("missing %s", Nonce)
	}
	in.Signature = strings.TrimSpace(h.Get(Signature))
	if in.Signature == "" {
		return in, fmt.Errorf("missing %s", Signature)
	}
	in.IdemKey = strings.TrimSpace(h.Get(Idempotency))
	if in.IdemKey == "" {
		return in, fmt.Errorf("missing %s", Idempotency)
	}
	in.ToolKey = strings.TrimSpace(h.Get(ToolKey))
	if in.ToolKey == "" {
		in.ToolKey = "tool"
	}
	return in, nil
}
