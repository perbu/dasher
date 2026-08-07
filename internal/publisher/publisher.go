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

// pollInterval is how often Run looks for new data. Fetches complete at
// unpredictable times — the three sources have different intervals and
// latencies — so the publisher polls the store rather than waiting out a
// full publish interval to notice a change.
const pollInterval = 500 * time.Millisecond

// Run publishes whenever the aggregated data changes, and at least once per
// configured interval, until ctx is cancelled.
func (p *Publisher) Run(ctx context.Context) {
	var (
		lastData time.Time // UpdatedAt of the snapshot last published
		lastSent time.Time // when that publish happened
	)
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			// Final retained update so the topic reflects the last state.
			if snapshot := p.store.Snapshot(); !snapshot.UpdatedAt.IsZero() {
				p.publish(snapshot, "shutdown")
			}
			return
		case now := <-t.C:
			snapshot := p.store.Snapshot()
			if snapshot.UpdatedAt.IsZero() {
				continue // nothing fetched yet; don't publish an empty document
			}
			var reason string
			switch {
			case !snapshot.UpdatedAt.Equal(lastData):
				// Each source appears as it lands rather than at the next
				// tick, so a restart fills the dashboard in within a second.
				reason = "changed"
			case now.Sub(lastSent) >= p.interval:
				// Refresh the retained message even when nothing moved, so
				// publish.interval still bounds the age of what subscribers see.
				reason = "heartbeat"
			default:
				continue
			}
			p.publish(snapshot, reason)
			lastData, lastSent = snapshot.UpdatedAt, now
		}
	}
}

// publish sends a snapshot to the topic, and to stdout in debug mode.
// reason records what triggered it, for the log.
func (p *Publisher) publish(snapshot state.Payload, reason string) {
	payload, err := json.Marshal(snapshot)
	if err != nil {
		log.Printf("[publisher] marshal error: %v", err)
		return
	}
	if p.debug {
		// Payload size matters — the ESP32 parses this — so report it
		// alongside the document itself.
		log.Printf("[publisher] %s: %d bytes", reason, len(payload))
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, payload, "", "  "); err != nil {
			log.Printf("[publisher] indent error: %v", err)
		} else {
			fmt.Println(pretty.String())
		}
	}
	if p.srv == nil {
		return // debug mode: no broker running
	}
	if err := p.srv.Publish(p.topic, payload, true, 1); err != nil {
		log.Printf("[publisher] publish error: %v", err)
	}
}
