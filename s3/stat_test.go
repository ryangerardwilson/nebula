package s3

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatReportsTimeAndSizeOnALocalRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("S3_ROOT", root)
	ctx := context.Background()
	if err := MakeBucket(ctx, "notes"); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "today.txt")
	if err := os.WriteFile(src, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Put(ctx, src, "notes", "day/today.txt"); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 11, 19, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(root, "notes", "day", "today.txt"), when, when); err != nil {
		t.Fatal(err)
	}
	mtime, size, err := Stat(ctx, "notes", "day/today.txt")
	if err != nil || size != 6 || !mtime.Equal(when) {
		t.Fatalf("%v %d %v", mtime, size, err)
	}
}

func TestStatMissingIsNotFound(t *testing.T) {
	t.Setenv("S3_ROOT", t.TempDir())
	ctx := context.Background()
	if err := MakeBucket(ctx, "notes"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Stat(ctx, "notes", "gone.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, _, err := Stat(ctx, "nobucket", "gone.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestParseStatReadsSecondsAndSize(t *testing.T) {
	mtime, size, err := parseStat("1763600000,4096\n")
	if err != nil || size != 4096 || mtime.Unix() != 1763600000 {
		t.Fatalf("%v %d %v", mtime, size, err)
	}
	if _, _, err := parseStat("garbage"); err == nil {
		t.Fatal("garbage parsed")
	}
}
