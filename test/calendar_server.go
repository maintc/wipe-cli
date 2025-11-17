package test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// CalendarEvent represents a calendar event
type CalendarEvent struct {
	ID        string
	Summary   string
	StartTime time.Time
}

// CalendarServer is a test HTTP server that serves ICS calendar files
// It supports multiple calendars (one per server) at paths like /server-name/basic.ics
type CalendarServer struct {
	server *httptest.Server
	// events is a map of server name -> event ID -> event
	events map[string]map[string]CalendarEvent
	mu     sync.RWMutex
	t      *testing.T
}

// NewCalendarServer creates a new test calendar server
func NewCalendarServer(t *testing.T) *CalendarServer {
	cs := &CalendarServer{
		events: make(map[string]map[string]CalendarEvent),
		t:      t,
	}

	mux := http.NewServeMux()

	// Endpoint to get calendar for a specific server: /server-name/basic.ics
	mux.HandleFunc("/", cs.handleCalendar)

	// Endpoint to add events (for test control)
	// POST /add-event?server=X&id=Y&summary=Z&start=W
	mux.HandleFunc("/add-event", cs.handleAddEvent)

	// Endpoint to remove events (for test control)
	// POST /remove-event?server=X&id=Y
	mux.HandleFunc("/remove-event", cs.handleRemoveEvent)

	// Endpoint to clear all events
	// POST /clear-events or /clear-events?server=X
	mux.HandleFunc("/clear-events", cs.handleClearEvents)

	// Endpoint to list events
	// GET /list-events or /list-events?server=X
	mux.HandleFunc("/list-events", cs.handleListEvents)

	// Create unstarted server so we can set a fixed port
	cs.server = httptest.NewUnstartedServer(mux)

	// Use fixed port 45975
	listener, err := net.Listen("tcp", "127.0.0.1:45975")
	if err != nil {
		t.Fatalf("Failed to listen on port 45975: %v", err)
	}
	cs.server.Listener = listener
	cs.server.Start()

	return cs
}

// NewRemoteCalendarServer creates a CalendarServer wrapper that connects to an existing calendar server
func NewRemoteCalendarServer(t *testing.T, baseURL string) *CalendarServer {
	// Remove any trailing /server-name/basic.ics to get base URL
	// Just use the protocol://host:port part
	if idx := strings.Index(baseURL, "//"); idx != -1 {
		rest := baseURL[idx+2:]
		if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
			baseURL = baseURL[:idx+2+slashIdx]
		}
	}

	// Create a mock server struct that uses HTTP endpoints instead of in-memory
	cs := &CalendarServer{
		events: nil, // Not used for remote server
		t:      t,
		server: &httptest.Server{
			URL: baseURL,
		},
	}

	return cs
}

// GetServerURL returns the calendar URL for a specific server
func (cs *CalendarServer) GetServerURL(serverName string) string {
	return fmt.Sprintf("%s/%s/basic.ics", cs.server.URL, serverName)
}

// BaseURL returns the base server URL
func (cs *CalendarServer) BaseURL() string {
	return cs.server.URL
}

// Close stops the calendar server (no-op for remote servers)
func (cs *CalendarServer) Close() {
	// Don't close remote servers
	if cs.events == nil {
		return
	}
	cs.server.Close()
}

// AddEventForServer adds an event to a specific server's calendar
func (cs *CalendarServer) AddEventForServer(serverName, id, summary string, startTime time.Time) {
	// If this is a remote server, use HTTP endpoint
	if cs.events == nil {
		reqURL := fmt.Sprintf("%s/add-event?server=%s&id=%s&summary=%s&start=%s",
			cs.server.URL,
			url.QueryEscape(serverName),
			url.QueryEscape(id),
			url.QueryEscape(summary),
			url.QueryEscape(startTime.Format(time.RFC3339)))
		resp, err := http.Post(reqURL, "", nil)
		if err != nil {
			cs.t.Fatalf("Failed to add event to remote server: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			cs.t.Fatalf("Failed to add event to remote server, status: %d", resp.StatusCode)
		}
		cs.t.Logf("Event added to remote server %s: %s - %s at %s", serverName, id, summary, startTime.Format(time.RFC3339))
		return
	}

	// Local server - direct manipulation
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.events[serverName] == nil {
		cs.events[serverName] = make(map[string]CalendarEvent)
	}

	cs.events[serverName][id] = CalendarEvent{
		ID:        id,
		Summary:   summary,
		StartTime: startTime,
	}

	cs.t.Logf("Event added for %s: %s - %s at %s", serverName, id, summary, startTime.Format(time.RFC3339))
}

// RemoveEventForServer removes an event from a specific server's calendar
func (cs *CalendarServer) RemoveEventForServer(serverName, id string) {
	// If this is a remote server, use HTTP endpoint
	if cs.events == nil {
		reqURL := fmt.Sprintf("%s/remove-event?server=%s&id=%s",
			cs.server.URL,
			url.QueryEscape(serverName),
			url.QueryEscape(id))
		resp, err := http.Post(reqURL, "", nil)
		if err != nil {
			cs.t.Fatalf("Failed to remove event from remote server: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			cs.t.Fatalf("Failed to remove event from remote server, status: %d", resp.StatusCode)
		}
		cs.t.Logf("Event removed from remote server %s: %s", serverName, id)
		return
	}

	// Local server - direct manipulation
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.events[serverName] != nil {
		delete(cs.events[serverName], id)
	}
	cs.t.Logf("Event removed from %s: %s", serverName, id)
}

// ClearEventsForServer removes all events from a specific server's calendar
func (cs *CalendarServer) ClearEventsForServer(serverName string) {
	// If this is a remote server, use HTTP endpoint
	if cs.events == nil {
		reqURL := fmt.Sprintf("%s/clear-events?server=%s", cs.server.URL, url.QueryEscape(serverName))
		resp, err := http.Post(reqURL, "", nil)
		if err != nil {
			cs.t.Fatalf("Failed to clear events on remote server: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			cs.t.Fatalf("Failed to clear events on remote server, status: %d", resp.StatusCode)
		}
		cs.t.Logf("All events cleared on remote server for %s", serverName)
		return
	}

	// Local server - direct manipulation
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.events[serverName] != nil {
		cs.events[serverName] = make(map[string]CalendarEvent)
	}
	cs.t.Logf("All events cleared for %s", serverName)
}

// ClearAllEvents removes all events from all servers
func (cs *CalendarServer) ClearAllEvents() {
	// If this is a remote server, use HTTP endpoint
	if cs.events == nil {
		reqURL := fmt.Sprintf("%s/clear-events", cs.server.URL)
		resp, err := http.Post(reqURL, "", nil)
		if err != nil {
			cs.t.Fatalf("Failed to clear events on remote server: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			cs.t.Fatalf("Failed to clear events on remote server, status: %d", resp.StatusCode)
		}
		cs.t.Log("All events cleared on remote server")
		return
	}

	// Local server - direct manipulation
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.events = make(map[string]map[string]CalendarEvent)
	cs.t.Log("All events cleared")
}

// handleCalendar serves the ICS calendar file for a specific server
func (cs *CalendarServer) handleCalendar(w http.ResponseWriter, r *http.Request) {
	// Extract server name from path: /server-name/basic.ics
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	if len(parts) != 2 || parts[1] != "basic.ics" {
		http.Error(w, "Not found - expected /{server-name}/basic.ics", http.StatusNotFound)
		return
	}

	serverName := parts[0]

	cs.mu.RLock()
	serverEvents := cs.events[serverName]
	eventCount := len(serverEvents)
	cs.mu.RUnlock()

	cs.t.Logf("Calendar requested for %s (%d event(s))", serverName, eventCount)

	ics := cs.generateICS(serverName, serverEvents)
	w.Header().Set("Content-Type", "text/calendar")
	w.Write([]byte(ics))
}

// handleAddEvent handles adding events via HTTP POST
func (cs *CalendarServer) handleAddEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serverName := r.URL.Query().Get("server")
	eventID := r.URL.Query().Get("id")
	summary := r.URL.Query().Get("summary")
	startTime := r.URL.Query().Get("start")

	if serverName == "" || eventID == "" || summary == "" || startTime == "" {
		http.Error(w, "Missing parameters (server, id, summary, start required)", http.StatusBadRequest)
		return
	}

	// Parse start time (RFC3339 or iCal format)
	var parsedTime time.Time
	var err error

	// Try RFC3339 first
	parsedTime, err = time.Parse(time.RFC3339, startTime)
	if err != nil {
		// Try iCal format (20060102T150405Z)
		parsedTime, err = time.Parse("20060102T150405Z", startTime)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid time format: %v", err), http.StatusBadRequest)
			return
		}
	}

	cs.mu.Lock()
	if cs.events[serverName] == nil {
		cs.events[serverName] = make(map[string]CalendarEvent)
	}
	cs.events[serverName][eventID] = CalendarEvent{
		ID:        eventID,
		Summary:   summary,
		StartTime: parsedTime,
	}
	cs.mu.Unlock()

	cs.t.Logf("Event added for %s: %s - %s at %s", serverName, eventID, summary, parsedTime.Format(time.RFC3339))
	fmt.Fprintf(w, "Event added for %s: %s\n", serverName, eventID)
}

// handleRemoveEvent handles removing events via HTTP POST
func (cs *CalendarServer) handleRemoveEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serverName := r.URL.Query().Get("server")
	eventID := r.URL.Query().Get("id")

	if serverName == "" || eventID == "" {
		http.Error(w, "Missing parameters (server, id required)", http.StatusBadRequest)
		return
	}

	cs.mu.Lock()
	if cs.events[serverName] != nil {
		delete(cs.events[serverName], eventID)
	}
	cs.mu.Unlock()

	cs.t.Logf("Event removed from %s: %s", serverName, eventID)
	fmt.Fprintf(w, "Event removed from %s: %s\n", serverName, eventID)
}

// handleClearEvents handles clearing all events via HTTP POST
func (cs *CalendarServer) handleClearEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serverName := r.URL.Query().Get("server")

	cs.mu.Lock()
	if serverName != "" {
		// Clear events for specific server
		if cs.events[serverName] != nil {
			cs.events[serverName] = make(map[string]CalendarEvent)
		}
		cs.mu.Unlock()
		cs.t.Logf("All events cleared for %s", serverName)
		fmt.Fprintf(w, "All events cleared for %s\n", serverName)
	} else {
		// Clear all events
		cs.events = make(map[string]map[string]CalendarEvent)
		cs.mu.Unlock()
		cs.t.Log("All events cleared")
		fmt.Fprintln(w, "All events cleared")
	}
}

// handleListEvents lists all events as JSON
func (cs *CalendarServer) handleListEvents(w http.ResponseWriter, r *http.Request) {
	serverName := r.URL.Query().Get("server")

	cs.mu.RLock()
	defer cs.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if serverName != "" {
		// List events for specific server
		serverEvents := cs.events[serverName]
		fmt.Fprintf(w, "{\n  \"server\": %q,\n  \"count\": %d,\n  \"events\": [\n", serverName, len(serverEvents))

		first := true
		for _, event := range serverEvents {
			if !first {
				fmt.Fprint(w, ",\n")
			}
			first = false
			fmt.Fprintf(w, "    {\n      \"id\": %q,\n      \"summary\": %q,\n      \"start_time\": %q\n    }",
				event.ID, event.Summary, event.StartTime.Format(time.RFC3339))
		}

		fmt.Fprint(w, "\n  ]\n}\n")
	} else {
		// List all events from all servers
		totalCount := 0
		for _, serverEvents := range cs.events {
			totalCount += len(serverEvents)
		}

		fmt.Fprintf(w, "{\n  \"total_count\": %d,\n  \"servers\": [\n", totalCount)

		firstServer := true
		for server, serverEvents := range cs.events {
			if !firstServer {
				fmt.Fprint(w, ",\n")
			}
			firstServer = false

			fmt.Fprintf(w, "    {\n      \"server\": %q,\n      \"count\": %d,\n      \"events\": [\n", server, len(serverEvents))

			firstEvent := true
			for _, event := range serverEvents {
				if !firstEvent {
					fmt.Fprint(w, ",\n")
				}
				firstEvent = false
				fmt.Fprintf(w, "        {\n          \"id\": %q,\n          \"summary\": %q,\n          \"start_time\": %q\n        }",
					event.ID, event.Summary, event.StartTime.Format(time.RFC3339))
			}

			fmt.Fprint(w, "\n      ]\n    }")
		}

		fmt.Fprint(w, "\n  ]\n}\n")
	}
}

// generateICS creates an ICS calendar file from events for a specific server
func (cs *CalendarServer) generateICS(serverName string, events map[string]CalendarEvent) string {
	ics := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//wipe-cli//E2E Test//EN
CALSCALE:GREGORIAN
METHOD:PUBLISH
X-WR-CALNAME:` + serverName + `
X-WR-TIMEZONE:UTC
`

	for _, event := range events {
		startTime := event.StartTime.UTC().Format("20060102T150405Z")
		ics += fmt.Sprintf(`BEGIN:VEVENT
UID:%s
SUMMARY:%s
DTSTART:%s
DTEND:%s
END:VEVENT
`, event.ID, event.Summary, startTime, startTime)
	}

	ics += "END:VCALENDAR\n"
	return ics
}
