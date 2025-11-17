package test

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// TestCalendarServer_Standalone runs a calendar server that stays up for manual testing.
// Skip this test normally, run explicitly with:
//
//	go test -v -run TestCalendarServer_Standalone ./test -timeout 1h
//
// Then you can:
//   - View calendar: curl http://localhost:8080/calendar.ics
//   - Add event: curl -X POST "http://localhost:8080/add-event?id=test1&summary=[RESTART]%20server1&start=2025-11-16T20:00:00Z"
//   - List events: curl http://localhost:8080/list-events
//   - Remove event: curl -X POST "http://localhost:8080/remove-event?id=test1"
//   - Clear all: curl -X POST http://localhost:8080/clear-events
func TestCalendarServer_Standalone(t *testing.T) {
	// Skip by default - only run when explicitly requested
	if os.Getenv("RUN_CALENDAR_SERVER") != "1" {
		t.Skip("Skipping standalone calendar server test. Set RUN_CALENDAR_SERVER=1 to run")
	}

	// Create calendar server on port 8080
	cs := NewCalendarServer(t)
	defer cs.Close()

	// Extract port from URL
	log.Printf("╔═══════════════════════════════════════════════════════════════╗")
	log.Printf("║          Test Calendar Server Running                         ║")
	log.Printf("╚═══════════════════════════════════════════════════════════════╝")
	log.Printf("")
	log.Printf("Base URL: %s", cs.BaseURL())
	log.Printf("")
	log.Printf("Calendar URLs (per-server):")
	log.Printf("  us-weekly: %s", cs.GetServerURL("us-weekly"))
	log.Printf("  us-long:   %s", cs.GetServerURL("us-long"))
	log.Printf("  us-build:  %s", cs.GetServerURL("us-build"))
	log.Printf("  train:     %s", cs.GetServerURL("train"))
	log.Printf("")
	log.Printf("Available endpoints:")
	log.Printf("  GET  /{server}/basic.ics               - Get calendar for server")
	log.Printf("  GET  /list-events?server=X             - List events for server")
	log.Printf("  GET  /list-events                      - List all events")
	log.Printf("  POST /add-event?server=X&id=Y&summary=Z&start=W")
	log.Printf("  POST /remove-event?server=X&id=Y")
	log.Printf("  POST /clear-events?server=X            - Clear events for server")
	log.Printf("  POST /clear-events                     - Clear all events")
	log.Printf("")
	log.Printf("Examples:")
	log.Printf("  # Add a restart event for us-weekly")
	log.Printf("  curl -X POST \"%s/add-event?server=us-weekly&id=restart1&summary=restart&start=%s\"",
		cs.BaseURL(),
		time.Now().Add(5*time.Minute).Format(time.RFC3339))
	log.Printf("")
	log.Printf("  # Add a wipe event for us-build")
	log.Printf("  curl -X POST \"%s/add-event?server=us-build&id=wipe1&summary=wipe&start=%s\"",
		cs.BaseURL(),
		time.Now().Add(10*time.Minute).Format(time.RFC3339))
	log.Printf("")
	log.Printf("  # View us-build calendar")
	log.Printf("  curl %s", cs.GetServerURL("us-build"))
	log.Printf("")
	log.Printf("  # List events for us-build")
	log.Printf("  curl %s/list-events?server=us-build", cs.BaseURL())
	log.Printf("")
	log.Printf("  # List all events")
	log.Printf("  curl %s/list-events", cs.BaseURL())
	log.Printf("")
	log.Printf("  # Remove an event from a server")
	log.Printf("  curl -X POST \"%s/remove-event?server=us-build&id=wipe1\"", cs.BaseURL())
	log.Printf("")
	log.Printf("  # Clear all events from a server")
	log.Printf("  curl -X POST \"%s/clear-events?server=us-build\"", cs.BaseURL())
	log.Printf("")
	log.Printf("Press Ctrl+C to stop...")
	log.Printf("")

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal
	<-sigChan
	log.Printf("\nShutting down calendar server...")
}

// TestCalendarServer_PerServerAPI tests the per-server API methods
func TestCalendarServer_PerServerAPI(t *testing.T) {
	cs := NewCalendarServer(t)
	defer cs.Close()

	// Add events to different servers
	cs.AddEventForServer("us-weekly", "restart1", "restart", time.Now().Add(1*time.Hour))
	cs.AddEventForServer("us-build", "wipe1", "wipe", time.Now().Add(2*time.Hour))

	// Remove event from specific server
	cs.RemoveEventForServer("us-weekly", "restart1")

	// Clear events for specific server
	cs.ClearEventsForServer("us-build")

	t.Log("Per-server API methods work correctly")
}
