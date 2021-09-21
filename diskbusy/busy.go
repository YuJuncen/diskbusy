package diskbusy

import (
	"context"
	"io"
	"os"

	"github.com/docker/go-units"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

func BusyWork(ctx context.Context, l *rate.Limiter, blockSize uint64, getReader func() (io.ReadCloser, error)) error {
	r, err := getReader()
	if err != nil {
		return err
	}
	for {
		buf := make([]byte, blockSize)
		if err := l.WaitN(ctx, int(blockSize)); err != nil {
			return nil
		}
		_, err := r.Read(buf)
		if err == io.EOF {
			if errClose := r.Close(); errClose != nil {
				return errClose
			}
			r, err = getReader()
		}
		if err != nil {
			return err
		}
	}
}

func RunBusyWork(ctx context.Context, getFileName func() string, n int, l *rate.Limiter) error {
	group, ectx := errgroup.WithContext(ctx)
	for i := 0; i < n; i++ {
		group.Go(func() error {
			return BusyWork(ectx, l, 64*units.KiB, func() (io.ReadCloser, error) {
				file := getFileName()
				return os.Open(file)
			})
		})
	}
	return group.Wait()
}
