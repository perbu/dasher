// Package fetchers contains the periodic background data collectors.
package fetchers

import (
	"context"
	"log"
	"net/http"
	"time"
)

// Verbose enables per-fetch and per-request timing logs. Set once at startup,
// before any fetcher is started; it is not safe to change afterwards.
var Verbose bool

// runLoop executes fetch immediately and then on every tick of interval
// until ctx is cancelled. Fetch errors are logged, never fatal.
func runLoop(ctx context.Context, name string, interval time.Duration, fetch func(context.Context) error) {
	do := func() {
		start := time.Now()
		err := fetch(ctx)
		elapsed := time.Since(start).Round(time.Millisecond)
		switch {
		case err != nil && ctx.Err() != nil:
			return // shutting down
		case err != nil:
			// Timing is included on the error path too: a fetch that fails
			// at ~15s hit the client timeout rather than being rejected.
			log.Printf("[%s] fetch failed after %s: %v", name, elapsed, err)
		case Verbose:
			log.Printf("[%s] fetch ok in %s", name, elapsed)
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

// do performs an HTTP round trip, logging the request and its outcome when
// Verbose is set. Only host and path are logged — query strings can carry
// parameters not worth putting in a log.
func do(name string, client *http.Client, req *http.Request) (*http.Response, error) {
	target := req.URL.Host + req.URL.Path
	if Verbose {
		log.Printf("[%s] -> %s %s", name, req.Method, target)
	}
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		if Verbose {
			log.Printf("[%s] <- %s failed after %s: %v", name, target, elapsed, err)
		}
		return nil, err
	}
	if Verbose {
		log.Printf("[%s] <- %s %s in %s", name, target, resp.Status, elapsed)
	}
	return resp, nil
}
