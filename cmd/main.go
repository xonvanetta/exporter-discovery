package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xonvanetta/exporter-discovery/internal/k8s"
	"github.com/xonvanetta/exporter-discovery/internal/scanner"
)

func main() {
	flag.Parse()

	configPath := "./config.yaml"
	if flag.NArg() > 0 {
		configPath = flag.Arg(0)
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	interval, err := cfg.getScanInterval()
	if err != nil {
		log.Fatalf("Invalid scan interval: %v", err)
	}

	k8sClient, err := k8s.NewClient(cfg.Namespace, cfg.Modules)
	if err != nil {
		log.Fatalf("Failed to create kubernetes client: %v", err)
	}

	scan := scanner.New(cfg.Workers)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	log.Printf("Starting exporter-discovery with interval %s", interval)

	runDiscovery(ctx, cfg, scan, k8sClient)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			runDiscovery(ctx, cfg, scan, k8sClient)
		case <-ctx.Done():
			return
		}
	}
}

func runDiscovery(ctx context.Context, cfg *Config, scan *scanner.Scanner, k8sClient *k8s.Client) {
	log.Println("Starting network discovery...")

	targets := scan.ScanNetworks(ctx, cfg.Networks)

	totalTargets := 0
	for module, moduleTargets := range targets {
		totalTargets += len(moduleTargets)
		log.Printf("Found %d targets for module %s", len(moduleTargets), module)
	}

	if totalTargets == 0 {
		log.Println("No targets found")
		return
	}

	if err := k8sClient.UpdateScrapeConfigs(ctx, targets); err != nil {
		log.Printf("Failed to update ScrapeConfigs: %v", err)
		return
	}

	log.Printf("Successfully updated %d ScrapeConfigs with %d total targets", len(targets), totalTargets)
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [config-file]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Arguments:\n")
		fmt.Fprintf(os.Stderr, "  config-file    Path to configuration file (default: config.yaml)\n")
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s config.yaml\n", os.Args[0])
	}
}
