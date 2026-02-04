package discoverycache

import "testing"

func TestAddLimiter(t *testing.T) {
	l := NewAddLimiter(2, 10)
	if !l.Allow(0) {
		t.Fatal("expected allow")
	}
	if !l.Allow(0) {
		t.Fatal("expected allow")
	}
	if l.Allow(0) {
		t.Fatal("expected deny")
	}
	// window rolls
	if !l.Allow(11) {
		t.Fatal("expected allow after window")
	}
}
