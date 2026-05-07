package cache

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func openTestCache(t *testing.T) *Cache {
	t.Helper()
	dir := t.TempDir()
	c, err := Open(filepath.Join(dir, "cache.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestGetSyncState_MissingReturnsNoRows(t *testing.T) {
	c := openTestCache(t)
	_, err := c.GetSyncState("designs")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want sql.ErrNoRows, got %v", err)
	}
}

func TestUpsertAndGetSyncState_RoundTrip(t *testing.T) {
	c := openTestCache(t)
	want := SyncState{
		ResourceType: "designs",
		Cursor:       "abc-cursor",
		WatermarkAt:  1234567,
		LastSyncedAt: 7654321,
	}
	if err := c.UpsertSyncState(want); err != nil {
		t.Fatalf("UpsertSyncState: %v", err)
	}
	got, err := c.GetSyncState("designs")
	if err != nil {
		t.Fatalf("GetSyncState: %v", err)
	}
	if *got != want {
		t.Fatalf("round trip mismatch:\n got %#v\nwant %#v", *got, want)
	}
}

func TestUpsertSyncState_FillsLastSyncedAtWhenZero(t *testing.T) {
	c := openTestCache(t)
	if err := c.UpsertSyncState(SyncState{ResourceType: "templates"}); err != nil {
		t.Fatalf("UpsertSyncState: %v", err)
	}
	got, err := c.GetSyncState("templates")
	if err != nil {
		t.Fatalf("GetSyncState: %v", err)
	}
	if got.LastSyncedAt == 0 {
		t.Fatalf("LastSyncedAt should be auto-filled, got 0")
	}
}

func TestUpsertSyncState_OnConflictUpdates(t *testing.T) {
	c := openTestCache(t)
	if err := c.UpsertSyncState(SyncState{ResourceType: "designs", Cursor: "v1", LastSyncedAt: 100}); err != nil {
		t.Fatalf("UpsertSyncState first: %v", err)
	}
	if err := c.UpsertSyncState(SyncState{ResourceType: "designs", Cursor: "v2", LastSyncedAt: 200}); err != nil {
		t.Fatalf("UpsertSyncState second: %v", err)
	}
	got, err := c.GetSyncState("designs")
	if err != nil {
		t.Fatalf("GetSyncState: %v", err)
	}
	if got.Cursor != "v2" || got.LastSyncedAt != 200 {
		t.Fatalf("on-conflict update failed: %#v", *got)
	}
}

func TestResetSyncState_DropsAllRows(t *testing.T) {
	c := openTestCache(t)
	for _, rt := range []string{"designs", "templates", "folders"} {
		if err := c.UpsertSyncState(SyncState{ResourceType: rt, LastSyncedAt: 1}); err != nil {
			t.Fatalf("UpsertSyncState(%s): %v", rt, err)
		}
	}
	if err := c.ResetSyncState(); err != nil {
		t.Fatalf("ResetSyncState: %v", err)
	}
	for _, rt := range []string{"designs", "templates", "folders"} {
		_, err := c.GetSyncState(rt)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("after reset, want ErrNoRows for %s, got %v", rt, err)
		}
	}
}
