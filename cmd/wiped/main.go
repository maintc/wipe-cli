package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/maintc/wipe-cli/internal/config"
	"github.com/maintc/wipe-cli/internal/daemon"
	"github.com/maintc/wipe-cli/internal/version"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "", "Path to config file (default: ~/.config/wiped/config.yaml)")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	// Show version if requested
	if *showVersion {
		fmt.Println(version.GetFullVersion())
		os.Exit(0)
	}

	log.Printf("Starting wipe daemon (%s)...", version.GetVersion())

	// Set custom config path if provided
	if *configPath != "" {
		config.CustomConfigPath = *configPath
		log.Printf("Using custom config: %s", *configPath)
	}

	// Initialize config
	config.InitConfig()

	// Create daemon instance
	d := daemon.New()

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal")
		cancel()
	}()

	// Run the daemon
	if err := d.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Daemon error: %v\n", err)
		os.Exit(1)
	}

	log.Println("Wipe daemon stopped")
}
