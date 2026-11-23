package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const Version = "0.1.0"

// Main runs the s3 command.
func Main(args []string) int {
	return run(args, os.Stdout, os.Stderr)
}

func run(args []string, out, errw io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && args[0] == "help") {
		fmt.Fprint(out, paint(helpText, out))
		return 0
	}
	switch args[0] {
	case "version":
		if len(args) != 1 {
			return usage(errw, "usage: nebula s3 version")
		}
		fmt.Fprintln(out, Version)
		return 0
	case "make":
		return makeBucket(args[1:], out, errw)
	case "remove":
		return removeBucket(args[1:], out, errw)
	case "list":
		return list(args[1:], out, errw)
	case "put":
		return put(args[1:], out, errw)
	case "get":
		return get(args[1:], out, errw)
	case "delete":
		return deleteObject(args[1:], out, errw)
	case "copy":
		return transfer(false, args[1:], out, errw)
	case "move":
		return transfer(true, args[1:], out, errw)
	default:
		return usage(errw, "usage: nebula s3 help")
	}
}

func makeBucket(args []string, out, errw io.Writer) int {
	if len(args) != 2 || args[0] != "bucket" {
		return usage(errw, "usage: nebula s3 make bucket <name>")
	}
	if err := MakeBucket(context.Background(), args[1]); err != nil {
		return fail(errw, err)
	}
	fmt.Fprintf(out, "bucket %s\n", args[1])
	return 0
}

func removeBucket(args []string, out, errw io.Writer) int {
	if len(args) != 2 || args[0] != "bucket" {
		return usage(errw, "usage: nebula s3 remove bucket <name>")
	}
	if err := RemoveBucket(context.Background(), args[1]); err != nil {
		return fail(errw, err)
	}
	fmt.Fprintf(out, "removed %s\n", args[1])
	return 0
}

func list(args []string, out, errw io.Writer) int {
	if len(args) > 1 {
		return usage(errw, "usage: nebula s3 list [s3://<bucket>[/<prefix>]]")
	}
	ctx := context.Background()
	if len(args) == 0 {
		names, err := ListBuckets(ctx)
		if err != nil {
			return fail(errw, err)
		}
		for _, name := range names {
			fmt.Fprintln(out, name)
		}
		return 0
	}
	bucket, prefix, err := parseURI(args[0], false)
	if err != nil {
		return fail(errw, err)
	}
	objects, err := List(ctx, bucket, prefix)
	if err != nil {
		return fail(errw, err)
	}
	for _, object := range objects {
		fmt.Fprintf(out, "%d %s\n", object.Size, object.Key)
	}
	return 0
}

func put(args []string, out, errw io.Writer) int {
	if len(args) != 2 {
		return usage(errw, "usage: nebula s3 put <path> s3://<bucket>/<key>")
	}
	bucket, key, err := parseURI(args[1], true)
	if err != nil {
		return fail(errw, err)
	}
	if err := Put(context.Background(), args[0], bucket, key); err != nil {
		return fail(errw, err)
	}
	fmt.Fprintf(out, "put s3://%s/%s\n", bucket, key)
	return 0
}

func get(args []string, out, errw io.Writer) int {
	if len(args) != 2 {
		return usage(errw, "usage: nebula s3 get s3://<bucket>/<key> <path>")
	}
	bucket, key, err := parseURI(args[0], true)
	if err != nil {
		return fail(errw, err)
	}
	if err := Get(context.Background(), bucket, key, args[1]); err != nil {
		return fail(errw, err)
	}
	fmt.Fprintf(out, "get s3://%s/%s\n", bucket, key)
	_ = out
	return 0
}

func deleteObject(args []string, out, errw io.Writer) int {
	if len(args) != 1 {
		return usage(errw, "usage: nebula s3 delete s3://<bucket>/<key>")
	}
	bucket, key, err := parseURI(args[0], true)
	if err != nil {
		return fail(errw, err)
	}
	if err := Delete(context.Background(), bucket, key); err != nil {
		return fail(errw, err)
	}
	fmt.Fprintf(out, "delete s3://%s/%s\n", bucket, key)
	return 0
}

func transfer(move bool, args []string, out, errw io.Writer) int {
	word := "copy"
	if move {
		word = "move"
	}
	if len(args) != 2 {
		return usage(errw, "usage: nebula s3 "+word+" s3://<bucket>/<key> s3://<bucket>/<key>")
	}
	srcBucket, srcKey, err := parseURI(args[0], true)
	if err != nil {
		return fail(errw, err)
	}
	dstBucket, dstKey, err := parseURI(args[1], true)
	if err != nil {
		return fail(errw, err)
	}
	ctx := context.Background()
	if move {
		err = Move(ctx, srcBucket, srcKey, dstBucket, dstKey)
	} else {
		err = Copy(ctx, srcBucket, srcKey, dstBucket, dstKey)
	}
	if err != nil {
		return fail(errw, err)
	}
	fmt.Fprintf(out, "%s s3://%s/%s s3://%s/%s\n", word, srcBucket, srcKey, dstBucket, dstKey)
	return 0
}

func parseURI(text string, keyRequired bool) (string, string, error) {
	rest, ok := strings.CutPrefix(text, "s3://")
	if !ok || rest == "" {
		return "", "", fmt.Errorf("%w: object must look like s3://bucket/key", ErrUsage)
	}
	bucket, key, _ := strings.Cut(rest, "/")
	key = strings.Trim(key, "/")
	if err := checkBucket(bucket); err != nil {
		return "", "", err
	}
	if keyRequired && key == "" {
		return "", "", fmt.Errorf("%w: object must look like s3://bucket/key", ErrUsage)
	}
	if key != "" {
		if err := checkKey(key); err != nil {
			return "", "", err
		}
	}
	return bucket, key, nil
}

func fail(errw io.Writer, err error) int {
	fmt.Fprintln(errw, err.Error())
	if errors.Is(err, ErrUsage) {
		return 2
	}
	return 1
}

func usage(errw io.Writer, msg string) int {
	fmt.Fprintln(errw, msg)
	return 2
}

func paint(text string, w io.Writer) string {
	if os.Getenv("NO_COLOR") != "" {
		return text
	}
	file, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return text
	}
	return "\033[38;5;245m" + text + "\033[0m"
}

const helpText = `s3
store and retrieve objects on del-vc2

global actions:
  nebula s3 help
    show this help
  nebula s3 version
    print the installed version

features:
  make and remove a bucket
  # make bucket <name> | remove bucket <name>
  nebula s3 make bucket notes
  nebula s3 remove bucket notes

  put, get, list, and delete an object
  # put <path> s3://<bucket>/<key> | get s3://<bucket>/<key> <path> | list [s3://<bucket>[/<prefix>]] | delete s3://<bucket>/<key>
  nebula s3 put ./today.txt s3://notes/today.txt
  nebula s3 get s3://notes/today.txt ./today.txt
  nebula s3 list
  nebula s3 list s3://notes
  nebula s3 delete s3://notes/today.txt

  copy and move an object
  # copy s3://<bucket>/<key> s3://<bucket>/<key> | move s3://<bucket>/<key> s3://<bucket>/<key>
  nebula s3 copy s3://notes/today.txt s3://notes/yesterday.txt
  nebula s3 move s3://notes/today.txt s3://notes/archive.txt
`
