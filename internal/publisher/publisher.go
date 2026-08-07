// Package publisher periodically publishes the aggregated state to MQTT.
package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	mqtt "github.com/mochi-mqtt/server/v2"

	"dasher/internal/state"
)

// Publisher publishes the aggregated payload to a topic with retain=true.
// A nil server means debug mode: payloads are only printed to stdout.
type Publisher struct {
	srv      *mqtt.Server
	store    *state.Store
	topic    string
	interval time.Duration
	debug    bool
}

// New creates a Publisher. If debug is true, every payload is printed to
// stdout instead of being published (srv may be nil).
func New(srv *mqtt.Server, store *state.Store, topic string, interval time.Duration, debug bool) *Publisher {
	return &Publisher{srv: srv, store: store, topic: topic, interval: interval, debug: debug}
}

// warmupInterval is how often Run retries before the first fetch lands.
const warmupInterval = 250 * time.Millisecond

// Run publishes as soon as the first data is available and then on every
// tick until ctx is cancelled.
func (p *Publisher) Run(ctx context.Context) {
	// No fetcher has finished when Run starts, so the first publish finds an
	// empty snapshot and skips. Poll on a short interval until one lands:
	// ticking straight at p.interval would leave the retained topic (and, in
	// debug mode, stdout) empty for a full cycle after every restart.
	interval := warmupInterval
	if p.publish() {
		interval = p.interval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			p.publish() // final retained update on shutdown
			return
		case <-t.C:
			if p.publish() && interval == warmupInterval {
				interval = p.interval
				t.Reset(interval)
			}
		}
	}
}

// publish sends the current snapshot to the topic, and to stdout in debug
// mode. It reports whether any data has been fetched yet; false means the
// snapshot was still empty and nothing was sent.
func (p *Publisher) publish() bool {
	snapshot := p.store.Snapshot()
	if snapshot.UpdatedAt.IsZero() {
		return false // nothing fetched yet; don't publish an empty document
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		// Data exists, so this is a real failure rather than warming up.
		log.Printf("[publisher] marshal error: %v", err)
		return true
	}
	if p.debug {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, payload, "", "  "); err != nil {
			log.Printf("[publisher] indent error: %v", err)
		} else {
			fmt.Println(pretty.String())
		}
	}
	if p.srv == nil {
		return true // debug mode: no broker running
	}
	if err := p.srv.Publish(p.topic, payload, true, 1); err != nil {
		log.Printf("[publisher] publish error: %v", err)
	}
	return true
}
