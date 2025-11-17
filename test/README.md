# End-to-End Testing

## Overview

The E2E test provides a full integration test of `wipe-cli` with:

- ✅ Real Rust & Carbon installations
- ✅ Real production server paths
- ✅ Real daemon execution with scheduler  
- ✅ Real Discord notifications
- ✅ Real hook script execution
- ✅ Local calendar HTTP server for controlled events

---

## Quick Start

```bash
# 1. Create .env file from template
cd test/
cp .env.example .env
# Edit .env and add your Discord webhook URL (optional)

# 2. Ensure hook scripts exist
ls -la /opt/wiped/  # stop-servers.sh, start-servers.sh, pre-start-hook.sh

# 3. Run E2E test
E2E_TEST=1 go test -v ./test/... -timeout 15m

# Alternative: Set environment variables directly (without .env file)
E2E_TEST=1 E2E_SERVER_PATH=/var/www/servers go test -v ./test/... -timeout 15m

# With custom Discord webhook
E2E_DISCORD_WEBHOOK="https://discord.com/api/webhooks/YOUR_WEBHOOK" \
E2E_TEST=1 E2E_SERVER_PATH=/var/www/servers go test -v ./test/... -timeout 15m
```

---

## Environment Variables

The test reads configuration from a `.env` file in the `test/` directory or from environment variables:

- `E2E_TEST=1` - **Required** to enable the test
- `E2E_SERVER_PATH` - Server base path (default: `/var/www/servers`)
- `E2E_DISCORD_WEBHOOK` - Discord webhook URL (optional, notifications disabled if not set)
- `E2E_CALENDAR_URL` - Use existing calendar server (optional, see [Calendar Server](#calendar-server))

**Setup:**
```bash
cd test/
cp .env.example .env
# Edit .env with your values
```

The `.env` file is gitignored to prevent committing sensitive information.

---

## What It Does

### Timeline

```
T+0s:    Test starts, creates temporary config
         Checks Rust/Carbon installations (auto-installs if needed)
         Creates server directories: us-weekly, us-long, us-build, train
         Starts daemon with test config

T+10s:   Daemon fetches calendar
         Test adds events (all at T+60s):
         - RESTART: us-weekly, us-long
         - WIPE: us-build, train
         - Future events (T+61m) outside lookahead, appear at execution time

T+60s:   BATCH EVENT FIRES (2 restart + 2 wipe together)
         ✓ Stops all 4 servers
         ✓ Syncs Rust & Carbon (parallel)
         ✓ Wipes files for us-build & train
         ✓ Runs pre-start-hook with all 4 servers
         ✓ Starts all 4 servers

T+90s:   Test verifies:
         ✓ All servers updated (RustDedicated binary present)
         ✓ Wipe files deleted (*.map, *.sav*, *.db*)
         ✓ Future events still scheduled (prevents Nov 16 scenario)
         Stops daemon gracefully
         PASS ✅
```

### Expected Duration

- **First run**: ~10-15 minutes (Rust download ~8GB)
- **Subsequent runs**: ~90 seconds (cached installations)

---

## Requirements

### System
- Linux with systemd
- ~15GB disk space (Rust installation)
- ~2GB RAM
- Internet connection

### Software
- Go 1.23+
- SteamCMD (auto-installed)
- rsync

### Hook Scripts
The test uses your **real hook scripts** at `/opt/wiped/`:
- `stop-servers.sh`
- `start-servers.sh`
- `pre-start-hook.sh`

**Safety Check:**
```bash
# Ensure scripts only operate on passed server paths (not global operations)
grep -E "systemctl (stop|start) ['\"]?rs-\*" /opt/wiped/*.sh \
  && echo "⚠️  WARNING: Global operations found!" \
  || echo "✅ Scripts look safe"
```

**✅ SAFE** scripts operate only on `$@` arguments  
**❌ UNSAFE** scripts use wildcards like `rs-*` or global systemctl operations

---

## Server Configuration

The test creates 4 servers in `E2E_SERVER_PATH`:

| Server | Event Type | Wipe Blueprints | Generate Map |
|--------|-----------|----------------|--------------|
| `us-weekly` | RESTART | false | true (on wipe only) |
| `us-long` | RESTART | false | true (on wipe only) |
| `us-build` | WIPE | false | false |
| `train` | WIPE | false | false |

**Note**: Map generation does NOT occur for restart events.

The test expects:
- **Real servers** to be running with actual files
- Hook scripts to provision the servers via Ansible
- Wipe files (`*.map`, `*.sav*`, `*.db*`) to exist for wipe servers

---

## Calendar Server

For faster test iterations, run the calendar server separately:

**Terminal 1:**
```bash
RUN_CALENDAR_SERVER=1 go test -v -run TestCalendarServer_Standalone ./test -timeout 1h
```

**Terminal 2:**
```bash
E2E_CALENDAR_URL="http://127.0.0.1:45975" \
E2E_SERVER_PATH=/var/www/servers \
E2E_TEST=1 go test -v ./test/... -timeout 15m
```

See [CALENDAR_SERVER.md](CALENDAR_SERVER.md) for API documentation.

---

## Troubleshooting

### Test Skipped
```bash
# Ensure E2E_TEST=1 is set
E2E_TEST=1 go test -v ./test/...
```

### Permission Denied on /opt/
```bash
# Give your user ownership
sudo chown -R $USER /opt
```

### Timeout
```bash
# Increase timeout for first run
E2E_TEST=1 go test -v ./test/... -timeout 20m
```

### SteamCMD Failed
```bash
sudo add-apt-repository multiverse
sudo apt install steamcmd
```

### Hook Scripts Not Found
```bash
sudo make install
ls -la /opt/wiped/
```

---

## Test Files

- `e2e_test.go` - Full integration test
- `calendar_server.go` - HTTP calendar server (ICS format)
- `calendar_server_example_test.go` - Standalone calendar server
- `E2E_TEST_CONFIG_EXAMPLE.yaml` - Example generated config
- `CALENDAR_SERVER.md` - Calendar server API documentation

---

## What This Proves

✅ **Config override works** - Custom config path support  
✅ **Calendar parsing works** - ICS events detected  
✅ **Event scheduling works** - gocron schedules jobs correctly  
✅ **Rust installation works** - SteamCMD downloads successfully  
✅ **Carbon installation works** - Downloads and extracts  
✅ **Restart flow works** - Stop → Sync → Hook → Start  
✅ **Wipe flow works** - Stop → Sync → Wipe → Hook → Start  
✅ **Batch execution works** - Mixed restart/wipe in single batch  
✅ **File patterns work** - Correct files deleted during wipe  
✅ **Race condition fixed** - Jobs don't cancel during execution  
✅ **Future events preserved** - Lookahead doesn't interfere  
✅ **Discord integration works** - Webhooks sent successfully  
✅ **Daemon lifecycle works** - Starts and stops gracefully

**This proves the entire system works end-to-end!** 🎉
