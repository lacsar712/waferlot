package outcome_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/lacsar712/waferlot/internal/outcome"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "deadline exceeded" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestHTTPStatus(t *testing.T) {
	cases := []struct {
		code int
		kind outcome.Kind
	}{
		{200, outcome.Success},
		{204, outcome.Success},
		{408, outcome.Retryable},
		{429, outcome.Retryable},
		{500, outcome.Retryable},
		{503, outcome.Retryable},
		{400, outcome.Terminal},
		{404, outcome.Terminal},
		{422, outcome.Terminal},
	}
	for _, tc := range cases {
		if got := outcome.HTTPStatus(tc.code); got != tc.kind {
			t.Fatalf("status %d: got %s want %s", tc.code, got, tc.kind)
		}
	}
}

func TestHTTPStatus429IsRetryable(t *testing.T) {
	if outcome.HTTPStatus(429) != outcome.Retryable {
		t.Fatalf("429: got %s want retryable", outcome.HTTPStatus(429))
	}
}

func TestHTTPStatus202IsSuccess(t *testing.T) {
	if outcome.HTTPStatus(202) != outcome.Success {
		t.Fatalf("202: got %s want success", outcome.HTTPStatus(202))
	}
}

func TestNetErrorTimeoutUnwraps(t *testing.T) {
	err := fmt.Errorf("outbound post: %w", timeoutErr{})
	if got := outcome.NetError(err); got != outcome.Retryable {
		t.Fatalf("wrapped timeout: got %s want retryable", got)
	}
}

func TestNetErrorHTTPClientTimeoutChain(t *testing.T) {
	// Reproduces what the collector surfaces on a collector timeout: the
	// http.Client.Timeout deadline becomes context.DeadlineExceeded (which is
	// os.ErrDeadlineExceeded), wrapped with %w so the chain stays intact. This
	// must stay retryable and never reach the terminal hold bin.
	err := fmt.Errorf("outbound post: %w", context.DeadlineExceeded)
	if got := outcome.NetError(err); got != outcome.Retryable {
		t.Fatalf("collector timeout: got %s want retryable", got)
	}
}
