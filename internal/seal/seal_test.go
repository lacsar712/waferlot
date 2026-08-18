package seal_test

import (
	"errors"
	"testing"
	"time"

	"github.com/lacsar712/waferlot/internal/seal"
	"github.com/lacsar712/waferlot/internal/wallclock"
)

func TestSignAndVerify(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	body := []byte(`{"type":"lot.track_in","payload":{}}`)
	nonce := "abcdefghijklmnop"
	sig, err := seal.Sign("supersecret", clk.Now().Unix(), nonce, body)
	if err != nil {
		t.Fatal(err)
	}
	err = seal.Verify(clk, 5*time.Minute, []string{"supersecret"}, seal.Headers{
		Timestamp: clk.Now().Unix(),
		Nonce:     nonce,
		Signature: sig,
	}, body)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestVerifyRejectsSkewAndWrongSecret(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	body := []byte(`{}`)
	nonce := "abcdefghijklmnop"
	sig, err := seal.Sign("supersecret", clk.Now().Unix(), nonce, body)
	if err != nil {
		t.Fatal(err)
	}
	clk.Advance(10 * time.Minute)
	err = seal.Verify(clk, 5*time.Minute, []string{"supersecret"}, seal.Headers{
		Timestamp: 1_700_000_000,
		Nonce:     nonce,
		Signature: sig,
	}, body)
	if err == nil {
		t.Fatal("expected skew error")
	}
	clk.Set(time.Unix(1_700_000_000, 0))
	err = seal.Verify(clk, 5*time.Minute, []string{"other"}, seal.Headers{
		Timestamp: 1_700_000_000,
		Nonce:     nonce,
		Signature: sig,
	}, body)
	if err == nil {
		t.Fatal("expected mismatch")
	}
}

func TestVerifySkewUnwraps(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	body := []byte(`{}`)
	nonce := "abcdefghijklmnop"
	sig, err := seal.Sign("supersecret", clk.Now().Unix(), nonce, body)
	if err != nil {
		t.Fatal(err)
	}
	clk.Advance(10 * time.Minute)
	err = seal.Verify(clk, 5*time.Minute, []string{"supersecret"}, seal.Headers{
		Timestamp: 1_700_000_000,
		Nonce:     nonce,
		Signature: sig,
	}, body)
	if !errors.Is(err, seal.ErrSkew) {
		t.Fatalf("want ErrSkew, got %v", err)
	}
}

func TestVerifyEmptySecretsNoPanic(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(1_700_000_000, 0))
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("empty secrets panicked: %v", r)
		}
	}()
	err := seal.Verify(clk, 5*time.Minute, nil, seal.Headers{
		Timestamp: clk.Now().Unix(),
		Nonce:     "abcdefghijklmnop",
		Signature: "v1=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for empty secrets")
	}
}
