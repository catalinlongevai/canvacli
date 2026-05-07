package auth

import (
	"strings"
	"testing"
)

func TestNewState_LengthAndCharset(t *testing.T) {
	s := NewState()
	if len(s) < 32 {
		t.Fatalf("state too short: %d", len(s))
	}
	if strings.ContainsAny(s, "+/=") {
		t.Fatalf("state contains non-url-safe chars: %q", s)
	}
}

func TestNewState_Unique(t *testing.T) {
	if NewState() == NewState() {
		t.Fatal("two states collided")
	}
}
