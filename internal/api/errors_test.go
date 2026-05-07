package api

import (
	"errors"
	"testing"
)

func TestErrAuthRevoked_HasCorrectExitCode(t *testing.T) {
	e := &APIError{Code: "auth_revoked"}
	if e.ExitCode() != 2 {
		t.Fatalf("expected exit 2, got %d", e.ExitCode())
	}
}

func TestErrNotFound_HasCorrectExitCode(t *testing.T) {
	e := &APIError{Code: "not_found"}
	if e.ExitCode() != 3 {
		t.Fatalf("expected exit 3, got %d", e.ExitCode())
	}
}

func TestErrIs_MatchesByCode(t *testing.T) {
	e := error(&APIError{Code: "rate_limited"})
	if !errors.Is(e, ErrRateLimited) {
		t.Fatal("expected errors.Is(..., ErrRateLimited)")
	}
}
