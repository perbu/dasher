// Package broker sets up the embedded MQTT broker with mutual TLS.
package broker

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
)

// New creates a configured MQTT broker with a single mutual-TLS listener.
// External clients must present a certificate signed by clientCAFile, and
// are restricted to subscribing to topic (read-only) once connected.
// Publishing happens only from within this process via the inline client,
// which talks to the broker directly in-memory and never goes through the
// network listener or these auth checks.
func New(listenAddr, topic, certFile, keyFile, clientCAFile string) (*mqtt.Server, error) {
	srv := mqtt.New(&mqtt.Options{
		InlineClient: true, // allow the process itself to publish/subscribe
	})

	err := srv.AddHook(new(auth.Hook), &auth.Options{
		Ledger: &auth.Ledger{
			Auth: auth.AuthRules{
				{Allow: true}, // admission is already gated by the client cert check below
			},
			ACL: auth.ACLRules{
				{
					Remote: "*",
					Filters: auth.Filters{
						auth.RString(topic): auth.ReadOnly,
						"#":                 auth.Deny, // everything else, explicitly closed
					},
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("adding auth hook: %w", err)
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("loading TLS keypair: %w", err)
	}

	caPEM, err := os.ReadFile(clientCAFile)
	if err != nil {
		return nil, fmt.Errorf("reading client CA: %w", err)
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no certificates found in %s", clientCAFile)
	}

	// The TCP listener upgrades to TLS when a TLSConfig is supplied.
	lc := listeners.Config{
		ID:      "mqtt1",
		Address: listenAddr,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    clientCAs,
		},
	}

	l := listeners.NewTCP(lc)
	if err := srv.AddListener(l); err != nil {
		return nil, fmt.Errorf("adding TLS listener: %w", err)
	}

	return srv, nil
}
