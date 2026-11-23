package s3

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTripOnALocalRoot(t *testing.T) {
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
	if err := Put(ctx, src, "notes", "today.txt"); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "out.txt")
	if err := Get(ctx, "notes", "today.txt", dst); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(dst)
	if err != nil || string(body) != "hello\n" {
		t.Fatalf("%q %v", body, err)
	}
	objects, err := List(ctx, "notes", "")
	if err != nil || len(objects) != 1 || objects[0].Key != "today.txt" || objects[0].Size != 6 {
		t.Fatal(objects, err)
	}
	if err := Copy(ctx, "notes", "today.txt", "notes", "archive/today.txt"); err != nil {
		t.Fatal(err)
	}
	if err := Move(ctx, "notes", "today.txt", "notes", "moved.txt"); err != nil {
		t.Fatal(err)
	}
	ok, err := Exists(ctx, "notes", "today.txt")
	if err != nil || ok {
		t.Fatal(ok, err)
	}
	if err := Delete(ctx, "notes", "moved.txt"); err != nil {
		t.Fatal(err)
	}
	if err := Delete(ctx, "notes", "archive/today.txt"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveBucket(ctx, "notes"); err != nil {
		t.Fatal(err)
	}
}

func TestPutRefusesAMissingBucketAndABadName(t *testing.T) {
	t.Setenv("S3_ROOT", t.TempDir())
	src := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(src, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Put(context.Background(), src, "notes", "a.txt")
	if err == nil || !os.IsNotExist(err) && !strings.Contains(err.Error(), "not found") {
		t.Fatal(err)
	}
	var errw bytes.Buffer
	if code := run([]string{"make", "bucket", "Bad"}, ioDiscard(), &errw); code != 2 {
		t.Fatalf("code %d %s", code, errw.String())
	}
}

func TestCLIPutGetAndList(t *testing.T) {
	t.Setenv("S3_ROOT", t.TempDir())
	var out, errw bytes.Buffer
	if code := run([]string{"make", "bucket", "notes"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	src := filepath.Join(t.TempDir(), "today.txt")
	if err := os.WriteFile(src, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := run([]string{"put", src, "s3://notes/today.txt"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	if !strings.Contains(out.String(), "put s3://notes/today.txt") {
		t.Fatal(out.String())
	}
	dst := filepath.Join(t.TempDir(), "out.txt")
	out.Reset()
	if code := run([]string{"get", "s3://notes/today.txt", dst}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	body, err := os.ReadFile(dst)
	if err != nil || string(body) != "hello\n" {
		t.Fatal(string(body), err)
	}
	out.Reset()
	if code := run([]string{"list", "s3://notes"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	if !strings.Contains(out.String(), "today.txt") {
		t.Fatal(out.String())
	}
	out.Reset()
	if code := run([]string{"delete", "s3://notes/today.txt"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	out.Reset()
	if code := run([]string{"remove", "bucket", "notes"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	help := bytes.Buffer{}
	if code := run(nil, &help, &errw); code != 0 || strings.Contains(help.String(), "\ncommands:\n") {
		t.Fatal(help.String())
	}
	if !strings.Contains(help.String(), "nebula s3 put ./today.txt s3://notes/today.txt") {
		t.Fatal(help.String())
	}
}

func ioDiscard() *bytes.Buffer { return &bytes.Buffer{} }

func TestPutMakingBucketMakesAMissingBucket(t *testing.T) {
	root := t.TempDir()
	t.Setenv("S3_ROOT", root)
	src := filepath.Join(t.TempDir(), "a.sqlite")
	if err := os.WriteFile(src, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := PutMakingBucket(ctx, src, "backups", "a.sqlite"); err != nil {
		t.Fatal(err)
	}
	if ok, err := Exists(ctx, "backups", "a.sqlite"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := PutMakingBucket(ctx, src, "backups", "a.sqlite"); err != nil {
		t.Fatal(err)
	}
}
