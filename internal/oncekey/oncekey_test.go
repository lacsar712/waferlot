package oncekey_test

import (
	"errors"
	"testing"
	"time"

	"github.com/lacsar712/waferlot/internal/oncekey"
	"github.com/lacsar712/waferlot/internal/wallclock"
)

func TestRememberReplayAndConflict(t *testing.T) {
	clk := wallclock.NewFrozen(time.Unix(0, 0))
	s := oncekey.New(clk, time.Hour)
	id, replay, err := s.Remember("key-aaaa", "hash1", "lot1")
	if err != nil || replay || id != "lot1" {
		t.Fatalf("first: %s replay=%v err=%v", id, replay, err)
	}
	id, replay, err = s.Remember("key-aaaa", "hash1", "lot2")
	if err != nil || !replay || id != "lot1" {
		t.Fatalf("replay: %s replay=%v err=%v", id, replay, err)
	}
	_, _, err = s.Remember("key-aaaa", "hash2", "lot3")
	if !errors.Is(err, oncekey.ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	clk.Advance(2 * time.Hour)
	id, replay, err = s.Remember("key-aaaa", "hash2", "lot4")
	if err != nil || replay || id != "lot4" {
		t.Fatalf("after ttl: %s replay=%v err=%v", id, replay, err)
	}
}
