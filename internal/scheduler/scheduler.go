package scheduler

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
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

// Scheduler manages scheduled events using gocron
type Scheduler struct {
	gocron         gocron.Scheduler
	events         []ScheduledEvent
	lookaheadHours int
	webhookURL     string
	eventDelay     int
	scheduledJobs  map[string]uuid.UUID        // Track gocron job IDs by time key
	jobEvents      map[string][]ScheduledEvent // Mutable event list per job (updated on calendar refresh)
	executingJobs  map[string]bool             // Track which jobs are currently executing (by timeKey)
	mutex          sync.Mutex
}

// New creates a new Scheduler
func New(lookaheadHours int, webhookURL string, eventDelay int) (*Scheduler, error) {
	gocronScheduler, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("failed to create gocron scheduler: %w", err)
	}

	s := &Scheduler{
		gocron:         gocronScheduler,
		events:         make([]ScheduledEvent, 0),
		lookaheadHours: lookaheadHours,
		webhookURL:     webhookURL,
		eventDelay:     eventDelay,
		scheduledJobs:  make(map[string]uuid.UUID),
		jobEvents:      make(map[string][]ScheduledEvent),
		executingJobs:  make(map[string]bool),
	}

	// Start the gocron scheduler
	s.gocron.Start()

	return s, nil
}

// Shutdown gracefully shuts down the scheduler
func (s *Scheduler) Shutdown() error {
	return s.gocron.Shutdown()
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
	s.mutex.Lock()
	defer s.mutex.Unlock()

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

	// Detect changes
	oldEvents := s.events
	s.detectEventChanges(oldEvents, allEvents)

	s.events = allEvents

	// Group events by time (truncated to minute) and schedule gocron jobs
	if err := s.scheduleJobs(); err != nil {
		return fmt.Errorf("failed to schedule jobs: %w", err)
	}

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
func (s *Scheduler) detectEventChanges(oldEvents, newEvents []ScheduledEvent) {
	// Build maps for comparison using a unique key for each event
	oldEventMap := make(map[string]ScheduledEvent)
	newEventMap := make(map[string]ScheduledEvent)

	for _, event := range oldEvents {
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

// scheduleJobs groups events by time and creates gocron jobs for each time-group
func (s *Scheduler) scheduleJobs() error {
	// Group events by time (truncated to minute)
	eventGroups := make(map[string][]ScheduledEvent)
	timeKeys := make(map[string]time.Time)

	for _, event := range s.events {
		timeKey := event.Scheduled.Truncate(time.Minute).Format(time.RFC3339)
		eventGroups[timeKey] = append(eventGroups[timeKey], event)
		if _, exists := timeKeys[timeKey]; !exists {
			timeKeys[timeKey] = event.Scheduled.Truncate(time.Minute)
		}
	}

	// Build set of current time keys
	currentTimeKeys := make(map[string]bool)
	for timeKey := range eventGroups {
		currentTimeKeys[timeKey] = true
	}

	// Update event lists for existing jobs AND schedule new jobs
	for timeKey, events := range eventGroups {
		scheduleTime := timeKeys[timeKey]

		// Skip events in the past
		if scheduleTime.Before(time.Now()) {
			log.Printf("Skipping past event at %s", timeKey)
			continue
		}

		// Make a copy of events for this time group
		eventsCopy := make([]ScheduledEvent, len(events))
		copy(eventsCopy, events)

		// Check if job already scheduled
		if _, exists := s.scheduledJobs[timeKey]; exists {
			// Job exists - UPDATE the event list (allows add/remove of individual servers)
			s.jobEvents[timeKey] = eventsCopy
			log.Printf("Updated event list for %s (%d server(s))",
				scheduleTime.Format("Mon Jan 02 15:04 MST"), len(events))
			continue
		}

		// Job doesn't exist - CREATE new job
		// Store the event list
		s.jobEvents[timeKey] = eventsCopy

		// Schedule one job for this time-group
		// Pass timeKey so we can look up current events at execution time
		tk := timeKey // Capture for closure
		job, err := s.gocron.NewJob(
			gocron.OneTimeJob(
				gocron.OneTimeJobStartDateTime(scheduleTime),
			),
			gocron.NewTask(
				func() {
					// Mark as executing IMMEDIATELY to prevent cancellation during UpdateEvents
					s.mutex.Lock()
					s.executingJobs[tk] = true
					currentEvents, exists := s.jobEvents[tk]
					s.mutex.Unlock()

					// Ensure we remove the executing mark when done
					defer func() {
						s.mutex.Lock()
						delete(s.executingJobs, tk)
						s.mutex.Unlock()
					}()

					if !exists || len(currentEvents) == 0 {
						log.Printf("No events found for %s at execution time, skipping", tk)
						return
					}

					// Execute without re-marking (already marked above)
					s.executeEventGroupInternal(currentEvents)
				},
			),
			gocron.WithSingletonMode(gocron.LimitModeReschedule),
		)

		if err != nil {
			return fmt.Errorf("failed to schedule job for %s: %w", timeKey, err)
		}

		s.scheduledJobs[timeKey] = job.ID()
		log.Printf("Scheduled job for %s (%d server(s))",
			scheduleTime.Format("Mon Jan 02 15:04 MST"), len(events))
	}

	// Cancel jobs that are no longer needed (timeKey completely gone)
	for timeKey, jobID := range s.scheduledJobs {
		if !currentTimeKeys[timeKey] {
			// Check if this job is currently executing
			// Never cancel a job that's in progress
			if s.executingJobs[timeKey] {
				log.Printf("Keeping job for %s (currently executing)", timeKey)
				continue
			}

			if err := s.gocron.RemoveJob(jobID); err != nil {
				log.Printf("Warning: failed to remove job for %s: %v", timeKey, err)
			}
			delete(s.scheduledJobs, timeKey)
			delete(s.jobEvents, timeKey)
			log.Printf("Cancelled job for time: %s", timeKey)
		}
	}

	return nil
}

// executeEventGroupInternal performs the actual event execution
// Note: The gocron job closure handles marking executingJobs before calling this
func (s *Scheduler) executeEventGroupInternal(events []ScheduledEvent) {
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

	// Execute all servers together, passing which ones need wipes
	if err := executor.ExecuteEventBatch(servers, wipeServers, s.webhookURL, s.eventDelay); err != nil {
		log.Printf("Error executing event group: %v", err)
	}
}
