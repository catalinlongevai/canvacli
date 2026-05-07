package cache

import (
	"path/filepath"
	"strings"
	"testing"
)

// Documents observed allowlist behavior across known bypass / false-positive
// vectors. Each "wantReject=true" row is a confirmed defense; "wantReject=false"
// rows are either legitimately accepted or known false-positives that the
// allowlist incorrectly blocks (flagged in comments).
func TestExecReadOnly_BypassMatrix(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_ = c.UpsertDesign(Design{ID: "abc", Title: "T", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})

	cases := []struct {
		name       string
		q          string
		wantReject bool
	}{
		{"line-comment-hides-mutation", "SELECT 1 -- ; DROP TABLE designs", true},
		{"block-comment-only", "SELECT 1 /* ; */", true},
		{"block-comment-hides-insert", "SELECT 1 /* */; INSERT INTO meta VALUES('x','y')", true},
		{"multistmt-after-union", "SELECT 1 UNION ALL SELECT 1; DROP TABLE designs", true},
		{"with-then-delete", "WITH foo AS (SELECT 1) DELETE FROM designs", true},
		{"injection-trailing-comment", "SELECT * FROM designs WHERE id = 'abc'; DROP TABLE designs --'", true},
		{"mixed-case-allowed", "seleCT * FrOm designs", false},
		{"multistmt-pragma-second", `SELECT * FROM "designs"; PRAGMA case_sensitive_like = 1`, true},
		{"trailing-whitespace-semi-allowed", "SELECT 1   ;   ", false},
		{"drop-in-subquery-rejected", "SELECT 1 INTERSECT SELECT * FROM (DROP TABLE designs)", true},
		{"newline-multistmt", "SELECT\n1\n;\nDELETE FROM designs", true},
		// FALSE POSITIVE: a quoted identifier "insert" is legitimate SQL but the
		// regex blocks it. Documented here to surface in CI if the bug is fixed.
		{"FP-quoted-identifier-insert", `SELECT "insert" FROM designs`, true},
		{"explain-rejected", "EXPLAIN SELECT 1", true},
		// FALSE POSITIVE: a string literal containing "INSERT" is harmless but
		// the regex (no string-literal awareness) blocks it.
		{"FP-string-literal-insert", "SELECT 'INSERT' FROM designs", true},
		// FALSE POSITIVE: LIKE-filtering on a keyword substring.
		{"FP-like-keyword", "SELECT id FROM designs WHERE title LIKE '%INSERT%'", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.ExecReadOnly(tc.q, 10)
			rejected := err != nil && (strings.Contains(err.Error(), "read-only") || strings.Contains(err.Error(), "multiple statements"))
			if tc.wantReject && !rejected {
				t.Errorf("expected reject; got err=%v", err)
			}
			if !tc.wantReject && rejected {
				t.Errorf("expected accept; got err=%v", err)
			}
		})
	}
}

// Confirms the cache table survives a comment-style bypass attempt.
func TestExecReadOnly_DesignsTableSurvivesBypass(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_ = c.UpsertDesign(Design{ID: "abc", Title: "T", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	_, _ = c.ExecReadOnly("SELECT 1 -- ; DROP TABLE designs", 10)
	rows, err := c.ExecReadOnly("SELECT id FROM designs", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("designs table state unexpected: rows=%d err=%v", len(rows), err)
	}
}

// Confirms the outer LIMIT wraps and dominates a giant inner LIMIT.
func TestExecReadOnly_OuterLimitDominates(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	for i := 0; i < 50; i++ {
		_ = c.UpsertDesign(Design{ID: string(rune('a' + i)), Title: "x", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	}
	rows, err := c.ExecReadOnly("SELECT id FROM designs LIMIT 1000000", 5)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rows) != 5 {
		t.Errorf("expected 5 rows, got %d", len(rows))
	}
}

// Confirms ORDER BY in user query survives the SELECT * FROM (...) wrapping.
func TestExecReadOnly_OrderByThroughWrap(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_ = c.UpsertDesign(Design{ID: "b", Title: "B", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	_ = c.UpsertDesign(Design{ID: "a", Title: "A", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	rows, err := c.ExecReadOnly("SELECT id FROM designs ORDER BY title", 10)
	if err != nil {
		t.Fatalf("ORDER BY through wrap failed: %v", err)
	}
	if len(rows) != 2 || rows[0]["id"] != "a" {
		t.Fatalf("unexpected ordering: %+v", rows)
	}
}
