package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chillcheck-gateway/internal/ble"
	"chillcheck-gateway/internal/client"
	"chillcheck-gateway/internal/config"
	"chillcheck-gateway/internal/reading"
	"chillcheck-gateway/internal/sampler"
	"chillcheck-gateway/internal/spool"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to the config file")
	simulate := flag.Bool("simulate", false, "force simulation mode (no Bluetooth hardware)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	smp := sampler.New(cfg.Interval)
	cl := client.New(cfg.APIURL, cfg.GatewayKey)
	sp := spool.New(cfg.SpoolPath, cfg.SpoolMax)

	var src ble.Source
	if cfg.Simulate || *simulate {
		macs := cfg.SimulateMACs
		if len(macs) == 0 {
			macs = []string{"A4:C1:38:00:00:01", "A4:C1:38:00:00:02", "A4:C1:38:00:00:03"}
		}
		src = &ble.Simulator{MACs: macs, Interval: 10 * time.Second}
		log.Printf("simulation mode: %d virtual sensors -> %s", len(macs), cfg.APIURL)
	} else {
		src = ble.NewScanner()
		log.Printf("scanning for BLE sensors -> %s (sampling every %s)", cfg.APIURL, cfg.Interval)
	}

	// Feed every decoded broadcast into the sampler.
	go func() {
		if err := src.Run(ctx, smp.Observe); err != nil && ctx.Err() == nil {
			log.Printf("sensor source stopped: %v", err)
			stop()
		}
	}()
	go smp.Run(ctx)

	// Deliver each sampled batch; the loop ends when the sampler closes its channel.
	for batch := range smp.Batches() {
		deliver(cl, sp, batch)
	}
	log.Println("gateway stopped")
}

// deliver sends the spool backlog plus the new batch. On any failure it buffers
// the new batch to disk and keeps the backlog, so an outage never drops a reading.
func deliver(cl *client.Client, sp *spool.Spool, batch []reading.Reading) {
	spooled, _ := sp.LoadAll()
	all := append(spooled, batch...)
	if len(all) == 0 {
		return
	}

	// Fresh context so the final flush still runs during shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	accepted, ignored, err := cl.Send(ctx, all)
	if err != nil {
		if aerr := sp.Append(batch); aerr != nil {
			log.Printf("spool write failed: %v", aerr)
		}
		log.Printf("offline: buffered %d readings (%d queued) - %v", len(batch), len(all), err)
		return
	}
	if len(spooled) > 0 {
		_ = sp.Clear()
	}
	log.Printf("sent %d readings (accepted %d, ignored %d)", len(all), accepted, ignored)
}
