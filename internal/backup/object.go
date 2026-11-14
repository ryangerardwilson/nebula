package backup

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ryangerardwilson/nebula/internal/agent"
	"github.com/ryangerardwilson/nebula/internal/cache"
	"github.com/ryangerardwilson/nebula/internal/layout"
	"github.com/ryangerardwilson/nebula/s3x"
)

func runObject(ctx context.Context, out io.Writer) error {
	if err := s3x.MakeBucket(ctx, s3x.DatabaseBucket); err != nil {
		return err
	}
	if err := s3x.MakeBucket(ctx, s3x.BackupBucket); err != nil {
		return err
	}
	names, err := agent.Names()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	day := DayName(now)
	for _, name := range names {
		if err := ensureObject(ctx, name); err != nil {
			return err
		}
		src := name + ".sqlite"
		if err := s3x.Copy(ctx, s3x.DatabaseBucket, src, s3x.BackupBucket, day+"/"+src); err != nil {
			return err
		}
	}
	pruned, err := pruneObjectDays(ctx, now)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "backup s3://%s/%s\npruned %d\n", s3x.BackupBucket, day, pruned)
	return nil
}

func ensureObject(ctx context.Context, name string) error {
	if cache.Has(name) {
		return cache.Publish(ctx, name)
	}
	ok, err := s3x.Exists(ctx, s3x.DatabaseBucket, name+".sqlite")
	if err != nil || ok {
		return err
	}
	_, err = cache.Open(ctx, name)
	return err
}

func pruneObjectDays(ctx context.Context, today time.Time) (int, error) {
	objects, err := s3x.List(ctx, s3x.BackupBucket, "")
	if err != nil {
		return 0, err
	}
	pruned := 0
	keep := layout.KeepDays()
	for _, object := range objects {
		day, _, ok := strings.Cut(object.Key, "/")
		if !ok {
			continue
		}
		expired, match := Expired(day, today, keep)
		if !match || !expired {
			continue
		}
		if err := s3x.Delete(ctx, s3x.BackupBucket, object.Key); err != nil {
			return pruned, err
		}
		pruned++
	}
	return pruned, nil
}
