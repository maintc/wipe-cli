---
name: Bug report
about: Create a report to help us improve
title: ''
labels: bug
assignees: jalbert

---

**Describe the bug**
A clear and concise description of what the bug is.

**To Reproduce**
Steps to reproduce the behavior:
1. Run command '...'
2. With configuration '...'
3. See error

**Expected behavior**
A clear and concise description of what you expected to happen.

**Environment (please complete the following information):**
 - OS: [e.g. Ubuntu 24.04, Windows 11]
 - Go version: [e.g. 1.23.4]
 - wipe-cli version: [e.g. v1.0.0]

**Configuration**
Please include relevant parts of your `~/.config/wiped/config.yaml` (remove sensitive information like API keys and Discord webhooks):

```yaml
# Paste your sanitized config here
```

**Logs**
If applicable, add logs from the wiped service:
```
sudo journalctl -u wiped@$USER.service -n 100
```

**Additional context**
Add any other context about the problem here.

