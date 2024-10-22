package control

import (
	"context"
	"fmt"
	"time"
)

const (
	defaultWaitInterval = 1 * time.Second
	defaultNumRetries   = 0 // 0 means infinite retries
)

type WaitOptions struct {
	NumRetries   *int
	WaitInterval *time.Duration
}

// WaitUntilReady waits until the specified condition becomes true.
// checkFunc can return an error to return early.
func WaitUntilReady(ctx context.Context, checkFunc func() (bool, error), opts ...WaitOptions) error {
	var opt WaitOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	waitInterval := defaultWaitInterval
	if opt.WaitInterval != nil {
		waitInterval = *opt.WaitInterval
	}

	numRetries := defaultNumRetries
	if opt.NumRetries != nil {
		numRetries = *opt.NumRetries
	}

	for i := 0; i < numRetries || numRetries == 0; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitInterval):
			if ok, err := checkFunc(); err != nil {
				return fmt.Errorf("wait check failed: %w", err)
			} else if ok {
				return nil
			}
		}
	}
	return fmt.Errorf("check was not successful after %d attempts", numRetries)
}
