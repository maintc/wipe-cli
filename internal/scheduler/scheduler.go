package scheduler

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/maintc/wipe-cli/internal/calendar"
	"github.com/maintc/wipe-cli/internal/config"
	"github.com/maintc/wipe-cli/internal/discord"
	"github.com/maintc/wipe-cli/internal/executor"
)

// ScheduledEvent represents an event with server context
type ScheduledEvent struct {
	Server    config.Server
	Event     calendar.Event
	Scheduled time.Time
}

// Scheduler manages scheduled events
type Scheduler struct {
	events         []ScheduledEvent
	lookaheadHours int
	webhookURL     string
	eventDelay     int
	executedEvents map[string]bool // Track executed events to prevent duplicates
	mutex          sync.Mutex
}

// New creates a new Scheduler
func New(lookaheadHours int, webhookURL string, eventDelay int) *Scheduler {
	return &Scheduler{
		events:         make([]ScheduledEvent, 0),
		lookaheadHours: lookaheadHours,
		webhookURL:     webhookURL,
		eventDelay:     eventDelay,
		executedEvents: make(map[string]bool),
	}
}

// GetEvents returns a copy of the current events (thread-safe)
func (s *Scheduler) GetEvents() []ScheduledEvent {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Return a copy to prevent external modification
	eventsCopy := make([]ScheduledEvent, len(s.events))
	copy(eventsCopy, s.events)
	return eventsCopy
}

// UpdateEvents fetches calendars and updates the schedule
func (s *Scheduler) UpdateEvents(servers []config.Server) error {
	log.Println("Updating calendar events...")

	var allEvents []ScheduledEvent

	for _, server := range servers {
		log.Printf("Fetching calendar for %s...", server.Name)

		cal, err := calendar.FetchCalendar(server.CalendarURL)
		if err != nil {
			log.Printf("Error fetching calendar for %s: %v", server.Name, err)
			continue
		}

		events, err := calendar.GetUpcomingEvents(cal, s.lookaheadHours)
		if err != nil {
			log.Printf("Error parsing events for %s: %v", server.Name, err)
			continue
		}

		log.Printf("Found %d upcoming event(s) for %s", len(events), server.Name)

		for _, event := range events {
			allEvents = append(allEvents, ScheduledEvent{
				Server:    server,
				Event:     event,
				Scheduled: event.StartTime,
			})
		}
	}

	// Resolve conflicts (same server, same time, wipe takes precedence)
	allEvents = s.resolveConflicts(allEvents)

	// Sort by time
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].Scheduled.Before(allEvents[j].Scheduled)
	})

	// Detect changes before updating
	s.detectEventChanges(allEvents)

	s.events = allEvents

	log.Printf("Total scheduled events: %d", len(s.events))
	s.logUpcomingEvents()

	return nil
}

// resolveConflicts removes restart events if a wipe event exists at the same time
func (s *Scheduler) resolveConflicts(events []ScheduledEvent) []ScheduledEvent {
	// Group by server path and time
	type key struct {
		serverPath string
		time       string // Use string representation for grouping
	}

	eventMap := make(map[key][]ScheduledEvent)

	for _, event := range events {
		k := key{
			serverPath: event.Server.Path,
			time:       event.Scheduled.Format(time.RFC3339),
		}
		eventMap[k] = append(eventMap[k], event)
	}

	var resolved []ScheduledEvent

	for _, group := range eventMap {
		if len(group) == 1 {
			resolved = append(resolved, group[0])
			continue
		}

		// If multiple events at same time, prefer wipe over restart
		hasWipe := false
		var wipeEvent ScheduledEvent

		for _, event := range group {
			if event.Event.Type == calendar.EventTypeWipe {
				hasWipe = true
				wipeEvent = event
				break
			}
		}

		if hasWipe {
			resolved = append(resolved, wipeEvent)
			log.Printf("Conflict resolved: Wipe takes precedence for %s at %s",
				wipeEvent.Server.Name, wipeEvent.Scheduled.Format(time.RFC3339))
		} else {
			// All restarts, just take the first one
			resolved = append(resolved, group[0])
		}
	}

	return resolved
}

// detectEventChanges compares old and new events and sends Discord notifications for changes
func (s *Scheduler) detectEventChanges(newEvents []ScheduledEvent) {
	// Build maps for comparison using a unique key for each event
	oldEventMap := make(map[string]ScheduledEvent)
	newEventMap := make(map[string]ScheduledEvent)

	for _, event := range s.events {
		key := fmt.Sprintf("%s|%s|%s", event.Server.Path, event.Event.Type, event.Scheduled.Format(time.RFC3339))
		oldEventMap[key] = event
	}

	for _, event := range newEvents {
		key := fmt.Sprintf("%s|%s|%s", event.Server.Path, event.Event.Type, event.Scheduled.Format(time.RFC3339))
		newEventMap[key] = event
	}

	// Find added events
	var added []ScheduledEvent
	for key, event := range newEventMap {
		if _, exists := oldEventMap[key]; !exists {
			added = append(added, event)
		}
	}

	// Find removed events
	var removed []ScheduledEvent
	for key, event := range oldEventMap {
		if _, exists := newEventMap[key]; !exists {
			removed = append(removed, event)
		}
	}

	// Send notifications for added events
	if len(added) > 0 {
		s.notifyEventsAdded(added)
	}

	// Send notifications for removed events
	if len(removed) > 0 {
		s.notifyEventsRemoved(removed)
	}
}

// notifyEventsAdded sends Discord notification for newly added events
func (s *Scheduler) notifyEventsAdded(events []ScheduledEvent) {
	if s.webhookURL == "" {
		return
	}

	// Group by event type
	restarts := []string{}
	wipes := []string{}

	for _, event := range events {
		timeStr := event.Scheduled.Format("Mon Jan 02 15:04 MST")
		eventStr := fmt.Sprintf("%s at %s", event.Server.Name, timeStr)

		if event.Event.Type == calendar.EventTypeWipe {
			wipes = append(wipes, eventStr)
		} else {
			restarts = append(restarts, eventStr)
		}
	}

	var description strings.Builder
	description.WriteString(fmt.Sprintf("**%d** new event(s) scheduled:\n\n", len(events)))

	if len(restarts) > 0 {
		description.WriteString("**Restarts:**\n")
		for _, r := range restarts {
			description.WriteString(fmt.Sprintf("• %s\n", r))
		}
		if len(wipes) > 0 {
			description.WriteString("\n")
		}
	}

	if len(wipes) > 0 {
		description.WriteString("**Wipes:**\n")
		for _, w := range wipes {
			description.WriteString(fmt.Sprintf("• %s\n", w))
		}
	}

	log.Printf("Calendar events added: %d", len(events))
	discord.SendSuccess(s.webhookURL, "Calendar Events Added", description.String())
}

// notifyEventsRemoved sends Discord notification for removed events
func (s *Scheduler) notifyEventsRemoved(events []ScheduledEvent) {
	if s.webhookURL == "" {
		return
	}

	// Group by event type
	restarts := []string{}
	wipes := []string{}

	for _, event := range events {
		timeStr := event.Scheduled.Format("Mon Jan 02 15:04 MST")
		eventStr := fmt.Sprintf("%s at %s", event.Server.Name, timeStr)

		if event.Event.Type == calendar.EventTypeWipe {
			wipes = append(wipes, eventStr)
		} else {
			restarts = append(restarts, eventStr)
		}
	}

	var description strings.Builder
	description.WriteString(fmt.Sprintf("**%d** event(s) removed:\n\n", len(events)))

	if len(restarts) > 0 {
		description.WriteString("**Restarts:**\n")
		for _, r := range restarts {
			description.WriteString(fmt.Sprintf("• %s\n", r))
		}
		if len(wipes) > 0 {
			description.WriteString("\n")
		}
	}

	if len(wipes) > 0 {
		description.WriteString("**Wipes:**\n")
		for _, w := range wipes {
			description.WriteString(fmt.Sprintf("• %s\n", w))
		}
	}

	log.Printf("Calendar events removed: %d", len(events))
	discord.SendWarning(s.webhookURL, "Calendar Events Removed", description.String())
}

// logUpcomingEvents prints a summary of upcoming events
func (s *Scheduler) logUpcomingEvents() {
	if len(s.events) == 0 {
		log.Println("No upcoming events in the next", s.lookaheadHours, "hours")
		return
	}

	log.Println("Upcoming events:")
	for _, event := range s.events {
		timeUntil := time.Until(event.Scheduled).Round(time.Minute)
		log.Printf("  %s - %s [%s] (in %s)",
			event.Scheduled.Format("Mon Jan 02 15:04 MST"),
			event.Server.Name,
			event.Event.Type,
			timeUntil)
	}
}

// ProcessDueEvents checks for events that should execute now
func (s *Scheduler) ProcessDueEvents() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	now := time.Now()

	// Group events by time (within 1 minute)
	eventGroups := make(map[string][]ScheduledEvent)

	for _, event := range s.events {
		// Check if event is due (scheduled time has passed)
		if event.Scheduled.Before(now) || event.Scheduled.Equal(now) {
			timeKey := event.Scheduled.Truncate(time.Minute).Format(time.RFC3339)

			// Check if already executed
			eventKey := fmt.Sprintf("%s-%s-%s", timeKey, event.Server.Path, event.Event.Type)
			if s.executedEvents[eventKey] {
				continue
			}

			eventGroups[timeKey] = append(eventGroups[timeKey], event)
		}
	}

	// Execute each group
	for timeKey, group := range eventGroups {
		// Group is a collection of events happening at the same time
		// Further group by event type since we want to execute all events of same type together
		s.executeEventGroup(timeKey, group)
	}
}

// executeEventGroup executes a group of events that occur at the same time
func (s *Scheduler) executeEventGroup(timeKey string, events []ScheduledEvent) {
	if len(events) == 0 {
		return
	}

	// Process all events together (restarts and wipes in single batch)
	// Extract all servers
	servers := make([]config.Server, len(events))
	wipeServers := make(map[string]bool) // Track which servers need wipe

	for i, event := range events {
		servers[i] = event.Server
		if event.Event.Type == calendar.EventTypeWipe {
			wipeServers[event.Server.Path] = true
		}
	}

	// Mark as executed before running (to prevent duplicates)
	for _, event := range events {
		eventKey := fmt.Sprintf("%s-%s-%s", timeKey, event.Server.Path, event.Event.Type)
		s.executedEvents[eventKey] = true
	}

	// Execute all servers together, passing which ones need wipes
	if err := executor.ExecuteEventBatch(servers, wipeServers, s.webhookURL, s.eventDelay); err != nil {
		log.Printf("Error executing event group: %v", err)
	}
}
