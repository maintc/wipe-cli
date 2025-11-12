package calendar

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/teambition/rrule-go"
)

// EventType represents the type of server event
type EventType string

const (
	EventTypeRestart EventType = "restart"
	EventTypeWipe    EventType = "wipe"
)

// Event represents a parsed calendar event
type Event struct {
	Type      EventType
	StartTime time.Time
	EndTime   time.Time
	Summary   string
}

// ScheduledEvent represents an event ready for execution
type ScheduledEvent struct {
	Type      EventType
	StartTime time.Time
}

// FetchCalendar downloads an .ics file from a URL
func FetchCalendar(url string) (*ics.Calendar, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch calendar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	cal, err := ics.ParseCalendar(strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse calendar: %w", err)
	}

	return cal, nil
}

// GetUpcomingEvents extracts restart and wipe events within the lookahead window
func GetUpcomingEvents(cal *ics.Calendar, lookaheadHours int) ([]Event, error) {
	now := time.Now()
	windowEnd := now.Add(time.Duration(lookaheadHours) * time.Hour)

	var events []Event

	for _, component := range cal.Components {
		if event, ok := component.(*ics.VEvent); ok {
			summaryProp := event.GetProperty(ics.ComponentPropertySummary)
			if summaryProp == nil {
				continue
			}
			summary := strings.ToLower(strings.TrimSpace(summaryProp.Value))

			// Only process "restart" or "wipe" events
			var eventType EventType
			if summary == "restart" {
				eventType = EventTypeRestart
			} else if summary == "wipe" {
				eventType = EventTypeWipe
			} else {
				continue
			}

			// Get start time
			dtstart := event.GetProperty(ics.ComponentPropertyDtStart)
			if dtstart == nil {
				continue
			}

			startTime, err := parseTimeWithTimezone(dtstart, cal)
			if err != nil {
				continue
			}

			// Get end time
			var endTime time.Time
			dtend := event.GetProperty(ics.ComponentPropertyDtEnd)
			if dtend != nil {
				endTime, _ = parseTimeWithTimezone(dtend, cal)
			} else {
				endTime = startTime.Add(1 * time.Hour) // Default 1 hour duration
			}

			// Check for recurring rule (use string literal since constant may not exist)
			rruleProp := event.GetProperty("RRULE")
			if rruleProp != nil {
				// Handle recurring events
				recurringEvents, err := expandRecurringEvent(startTime, endTime, rruleProp.Value, now, windowEnd, eventType, summary)
				if err == nil {
					events = append(events, recurringEvents...)
				}
			} else {
				// Single event
				if startTime.After(now) && startTime.Before(windowEnd) {
					events = append(events, Event{
						Type:      eventType,
						StartTime: startTime,
						EndTime:   endTime,
						Summary:   summary,
					})
				}
			}
		}
	}

	return events, nil
}

// expandRecurringEvent expands a recurring event within the time window
func expandRecurringEvent(startTime, endTime time.Time, rruleStr string, windowStart, windowEnd time.Time, eventType EventType, summary string) ([]Event, error) {
	// Parse RRULE
	r, err := rrule.StrToRRule(rruleStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RRULE: %w", err)
	}

	// Set the DTSTART for the rule
	r.DTStart(startTime)

	// Get occurrences within the window (extended slightly for safety)
	occurrences := r.Between(windowStart.Add(-24*time.Hour), windowEnd.Add(24*time.Hour), true)

	var events []Event
	duration := endTime.Sub(startTime)

	for _, occurrence := range occurrences {
		// Only include events within our actual window
		if occurrence.After(windowStart) && occurrence.Before(windowEnd) {
			events = append(events, Event{
				Type:      eventType,
				StartTime: occurrence,
				EndTime:   occurrence.Add(duration),
				Summary:   summary,
			})
		}
	}

	return events, nil
}

// parseTimeWithTimezone parses time from iCalendar property, respecting TZID parameter
func parseTimeWithTimezone(prop *ics.IANAProperty, cal *ics.Calendar) (time.Time, error) {
	if prop == nil {
		return time.Time{}, fmt.Errorf("nil property")
	}

	timeStr := prop.Value

	// Check if there's a TZID parameter
	tzid := ""
	if prop.ICalParameters != nil {
		if tzidParam, ok := prop.ICalParameters["TZID"]; ok && len(tzidParam) > 0 {
			tzid = tzidParam[0]
		}
	}

	// If we have a TZID, try to load that timezone
	var loc *time.Location
	if tzid != "" {
		// Try to load the timezone by IANA name
		if l, err := time.LoadLocation(tzid); err == nil {
			loc = l
		} else {
			// Fallback to UTC if we can't load the timezone
			loc = time.UTC
		}
	}

	// Common iCalendar time formats
	formats := []string{
		"20060102T150405Z",     // UTC format (Z suffix means UTC)
		"20060102T150405",      // Local/TZID format
		"2006-01-02T15:04:05Z", // ISO 8601 UTC
		"2006-01-02T15:04:05",  // ISO 8601 local
	}

	for _, format := range formats {
		var t time.Time
		var err error

		// If time string ends with Z, it's UTC regardless of TZID
		if strings.HasSuffix(timeStr, "Z") {
			t, err = time.Parse(format, timeStr)
		} else if loc != nil {
			// Parse in the specified timezone
			t, err = time.ParseInLocation(format, timeStr, loc)
		} else {
			// Parse as UTC if no timezone specified
			t, err = time.Parse(format, timeStr)
		}

		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse time: %s (tzid: %s)", timeStr, tzid)
}
