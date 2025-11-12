package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/maintc/wipe-cli/internal/config"
	"github.com/maintc/wipe-cli/internal/daemon"
)

func main() {
	log.Println("Starting wipe daemon...")

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
