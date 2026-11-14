package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKeepLastTenDays(t *testing.T) {
	today := time.Date(2026, 10, 8, 15, 0, 0, 0, time.Local)
	old, ok := Expired("2026-09-27", today, 10)
	if !ok || !old {
		t.Fatalf("11-day-old snapshot should expire: expired=%v match=%v", old, ok)
	}
	edge, ok := Expired("2026-09-29", today, 10)
	if !ok || edge {
		t.Fatalf("snapshot on the 10th day should stay: expired=%v match=%v", edge, ok)
	}
	if _, ok := Expired("notes.txt", today, 10); ok {
		t.Fatal("unrelated file matched a snapshot day")
	}
}

func TestPruneRemovesOnlyOldSnapshots(t *testing.T) {
	dir := t.TempDir()
	today := time.Date(2026, 10, 8, 3, 15, 0, 0, time.Local)
	keep := filepath.Join(dir, "2026-10-08")
	drop := filepath.Join(dir, "2026-09-27")
	other := filepath.Join(dir, "readme.txt")
	for _, path := range []string{keep, drop} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	n, err := Prune(dir, today, 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pruned %d", n)
	}
	if _, err := os.Stat(drop); !os.IsNotExist(err) {
		t.Fatal("old snapshot still present")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal(err)
	}
}
