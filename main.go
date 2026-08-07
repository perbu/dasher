// Command dasher runs a data aggregator and embedded MQTT broker for an
// ESP32 dashboard display.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os/signal"
	"sync"
	"syscall"

	mqtt "github.com/mochi-mqtt/server/v2"

	"dasher/internal/broker"
	"dasher/internal/config"
	"dasher/internal/fetchers"
	"dasher/internal/publisher"
	"dasher/internal/state"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	configPath := flag.String("config", "config.yaml", "path to YAML config file")
	envPath := flag.String("env", ".env", "path to optional .env file with secrets")
	debug := flag.Bool("debug", false, "development mode: print payloads to stdout, no MQTT broker")
	flag.Parse()

	if err := config.LoadEnvFile(*envPath); err != nil {
		log.Fatalf("env file: %v", err)
	}

	cfg, err := config.Load(*configPath, !*debug)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// In debug mode the broker is not started at all; srv stays nil and the
	// publisher only prints payloads to stdout.
	var srv *mqtt.Server
	if !*debug {
		srv, err = broker.New(
			cfg.MQTT.ListenAddr,
			cfg.MQTT.Topic,
			cfg.MQTT.TLSCert,
			cfg.MQTT.TLSKey,
			cfg.MQTT.ClientCA,
		)
		if err != nil {
			log.Fatalf("broker: %v", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store := &state.Store{}

	// Must be set before any fetcher starts.
	fetchers.Verbose = *debug

	claude, err := fetchers.NewClaude(cfg.Claude, store)
	if err != nil {
		log.Fatalf("claude: %v", err)
	}

	var wg sync.WaitGroup
	for name, run := range map[string]func(context.Context){
		"github": fetchers.NewGitHub(cfg.GitHub, store).Run,
		"claude": claude.Run,
		"yr":     fetchers.NewYr(cfg.Yr, store).Run,
	} {
		wg.Add(1)
		go func(name string, run func(context.Context)) {
			defer wg.Done()
			log.Printf("[%s] fetcher started", name)
			run(ctx)
		}(name, run)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		publisher.New(srv, store, cfg.MQTT.Topic, cfg.Publish.Interval, *debug).Run(ctx)
	}()

	if srv != nil {
		go func() {
			log.Printf("MQTT broker listening (mTLS) on %s, topic %q", cfg.MQTT.ListenAddr, cfg.MQTT.Topic)
			if err := srv.Serve(); err != nil {
				stop() // broker died; trigger shutdown
				log.Printf("broker serve: %v", err)
			}
		}()
	} else {
		log.Println("debug mode: MQTT broker disabled, payloads go to stdout")
	}

	<-ctx.Done()
	log.Println("shutting down...")
	if srv != nil {
		if err := srv.Close(); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("broker close: %v", err)
		}
	}
	wg.Wait()
	log.Println("bye")
}
