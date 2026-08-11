package fetchers

import (
	"context"
	"errors"
	"log"
	"os/exec"
	"runtime"
	"time"

	"dasher/internal/config"
	"dasher/internal/state"
)

// Presence pings a host on the LAN and records whether it answers. While the
// host is silent the owner is assumed away, and the device turns its screen
// off. It shells out to the system ping: raw ICMP would need root and a
// third-party package, and ping is present on every deploy target.
type Presence struct {
	cfg   config.PresenceConfig
	store *state.Store
	last  *bool // last reported state; the store is only updated on change
}

// NewPresence creates the presence fetcher.
func NewPresence(cfg config.PresenceConfig, store *state.Store) *Presence {
	return &Presence{cfg: cfg, store: store}
}

// Run starts the periodic check loop. It blocks until ctx is cancelled.
func (p *Presence) Run(ctx context.Context) {
	runLoop(ctx, "presence", p.cfg.Interval, p.check)
}

func (p *Presence) check(ctx context.Context) error {
	home, err := ping(ctx, p.cfg.Host)
	if err != nil {
		return err
	}
	if p.last != nil && *p.last == home {
		return nil // unchanged; don't bump UpdatedAt and trigger a republish
	}
	p.store.SetPresence(home)
	if p.last != nil {
		log.Printf("[presence] %s is now %s", p.cfg.Host, map[bool]string{true: "home", false: "away"}[home])
	}
	p.last = &home
	return nil
}

// ping sends a single ICMP echo and reports whether a reply came back. A
// non-zero exit from ping means "no reply", not an error; only a failure to
// run ping at all is one.
func ping(ctx context.Context, host string) (bool, error) {
	// The -W flag is not portable: Linux takes a timeout in seconds, macOS a
	// per-reply wait in milliseconds.
	wait := []string{"-W", "2"}
	if runtime.GOOS == "darwin" {
		wait = []string{"-W", "2000"}
	}
	args := append([]string{"-c", "1"}, append(wait, host)...)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := exec.CommandContext(ctx, "ping", args...).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
