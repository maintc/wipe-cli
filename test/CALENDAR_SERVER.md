# Test Calendar Server

The calendar server is a test utility that provides **per-server** ICS calendar endpoints for E2E testing.

Each server has its own calendar URL: `http://localhost:PORT/server-name/basic.ics`

## Usage

### Running the Calendar Server

To start the calendar server in standalone mode:

```bash
RUN_CALENDAR_SERVER=1 go test -v -run TestCalendarServer_Standalone ./test -timeout 1h
```

The server will:
- Start an HTTP server on a random available port
- Print the calendar URLs for each server
- Print example curl commands
- Run indefinitely until you press Ctrl+C

Example output:
```
╔═══════════════════════════════════════════════════════════════╗
║          Test Calendar Server Running                         ║
╚═══════════════════════════════════════════════════════════════╝

Base URL: http://127.0.0.1:44921

Calendar URLs (per-server):
  us-weekly: http://127.0.0.1:44921/us-weekly/basic.ics
  us-long:   http://127.0.0.1:44921/us-long/basic.ics
  us-build:  http://127.0.0.1:44921/us-build/basic.ics
  train:     http://127.0.0.1:44921/train/basic.ics
```

### API Endpoints

Once running, you can interact with per-server calendars:

#### View Calendar for a Server (ICS)
```bash
# Get calendar for us-build
curl http://localhost:PORT/us-build/basic.ics

# Get calendar for us-weekly
curl http://localhost:PORT/us-weekly/basic.ics
```

#### List Events (JSON)
```bash
# List events for a specific server
curl http://localhost:PORT/list-events?server=us-build

# List all events from all servers
curl http://localhost:PORT/list-events
```

Output (specific server):
```json
{
  "server": "us-build",
  "count": 1,
  "events": [
    {
      "id": "wipe1",
      "summary": "wipe",
      "start_time": "2025-11-16T21:00:00Z"
    }
  ]
}
```

Output (all servers):
```json
{
  "total_count": 2,
  "servers": [
    {
      "server": "us-weekly",
      "count": 1,
      "events": [{"id": "restart1", "summary": "restart", ...}]
    },
    {
      "server": "us-build",
      "count": 1,
      "events": [{"id": "wipe1", "summary": "wipe", ...}]
    }
  ]
}
```

#### Add Event to a Server
```bash
# Add restart event for us-weekly (using RFC3339 format)
curl -X POST "http://localhost:PORT/add-event?server=us-weekly&id=restart1&summary=restart&start=2025-11-16T20:00:00Z"

# Add wipe event for us-build (using iCal format)
curl -X POST "http://localhost:PORT/add-event?server=us-build&id=wipe1&summary=wipe&start=20251116T210000Z"
```

Parameters:
- `server` - Server name (us-weekly, us-long, us-build, train)
- `id` - Unique event identifier (within that server)
- `summary` - Event summary (just "restart" or "wipe")
- `start` - Start time (RFC3339 or iCal format)

#### Remove Event from a Server
```bash
curl -X POST "http://localhost:PORT/remove-event?server=us-build&id=wipe1"
```

#### Clear Events
```bash
# Clear all events for a specific server
curl -X POST "http://localhost:PORT/clear-events?server=us-build"

# Clear all events from all servers
curl -X POST http://localhost:PORT/clear-events
```

## Integration with E2E Tests

The E2E tests can connect to a running calendar server using the `E2E_CALENDAR_URL` environment variable:

```bash
# Terminal 1: Start the calendar server
RUN_CALENDAR_SERVER=1 go test -v -run TestCalendarServer_Standalone ./test -timeout 1h

# Terminal 2: Run E2E tests using the calendar server
E2E_CALENDAR_URL="http://127.0.0.1:PORT" \
E2E_SERVER_PATH=/var/www/servers \
E2E_TEST=1 go test -v -run TestE2E_FullIntegration ./test -timeout 15m
```

(Replace `PORT` with the actual port from Terminal 1's output)

The E2E tests will:
- Connect to the existing calendar server
- Clear any existing events
- Add test events for us-weekly, us-long, us-build, and train
- Verify the events execute correctly

## Using with Real Daemon

You can also point the `wiped` daemon to the test calendar server for manual testing:

1. Start the calendar server (Terminal 1)
2. Update your servers in the config to use the test calendar URLs:
   ```bash
   wipe update us-build --calendar http://127.0.0.1:PORT/us-build/basic.ics
   ```
3. Start/restart the daemon:
   ```bash
   sudo systemctl restart wiped@$USER
   ```
4. Add test events via curl (Terminal 2):
   ```bash
   # Schedule a wipe in 2 minutes
   curl -X POST "http://127.0.0.1:PORT/add-event?server=us-build&id=wipe1&summary=wipe&start=$(date -u -d '+2 minutes' +%Y-%m-%dT%H:%M:%SZ)"
   ```
5. Watch the daemon logs:
   ```bash
   journalctl -u wiped@$USER -f
   ```

## Calendar Format

The calendar server generates ICS files matching the real Google Calendar format used in production:

```ics
BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//wipe-cli//E2E Test//EN
CALSCALE:GREGORIAN
METHOD:PUBLISH
X-WR-CALNAME:us-build
X-WR-TIMEZONE:UTC
BEGIN:VEVENT
UID:wipe1
SUMMARY:wipe
DTSTART:20251116T210000Z
DTEND:20251116T210000Z
END:VEVENT
END:VCALENDAR
```

Key differences from the old implementation:
- **Per-server calendars**: Each server has its own URL (`/server-name/basic.ics`)
- **Simple summaries**: Just "restart" or "wipe" (not "[RESTART] server-name")
- **Server-scoped operations**: Events are managed per-server, not globally
