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

// Run publishes immediately and then on every tick until ctx is cancelled.
func (p *Publisher) Run(ctx context.Context) {
	p.publish()
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			p.publish() // final retained update on shutdown
			return
		case <-t.C:
			p.publish()
		}
	}
}

func (p *Publisher) publish() {
	snapshot := p.store.Snapshot()
	if snapshot.UpdatedAt.IsZero() {
		return // nothing fetched yet; don't publish an empty document
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		log.Printf("[publisher] marshal error: %v", err)
		return
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
		return // debug mode: no broker running
	}
	if err := p.srv.Publish(p.topic, payload, true, 1); err != nil {
		log.Printf("[publisher] publish error: %v", err)
	}
}
