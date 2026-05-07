package cache

import (
	"path/filepath"
	"testing"
	"time"
)

// Confirms modernc/sqlite respects context cancellation: a runaway recursive
// CTE wrapped in count(*) (so LIMIT can't short-circuit) gets killed at the
// 5s timeout boundary rather than running forever.
func TestExecReadOnly_TimeoutCancelsLongQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 5s timeout test in -short mode")
	}
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()

	q := `WITH RECURSIVE t(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM t WHERE n < 1000000000) SELECT count(*) FROM t`
	start := time.Now()
	_, err := c.ExecReadOnly(q, 100)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected timeout error, got nil after %v", elapsed)
	}
	if elapsed > 8*time.Second {
		t.Errorf("query did not honor 5s timeout (elapsed=%v)", elapsed)
	}
}
