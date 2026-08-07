// Package fetchers contains the periodic background data collectors.
package fetchers

import (
	"context"
	"log"
	"time"
)

// runLoop executes fetch immediately and then on every tick of interval
// until ctx is cancelled. Fetch errors are logged, never fatal.
func runLoop(ctx context.Context, name string, interval time.Duration, fetch func(context.Context) error) {
	do := func() {
		if err := fetch(ctx); err != nil {
			if ctx.Err() != nil {
				return // shutting down
			}
			log.Printf("[%s] fetch error: %v", name, err)
		}
	}
	do()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			do()
		}
	}
}
