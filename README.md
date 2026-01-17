# crtmon - Certificate Monitoring & Enumeration

Automated certificate monitoring tool for continuous domain discovery and enumeration across multiple cloud providers' SNI ranges.

## Features

- 🔍 **Real-time Monitoring** - Certificate stream processing via Certstream
- 🎯 **SNI IP Range Discovery** - Monthly automatic downloads from Amazon, Google, DigitalOcean, Microsoft, and Oracle
- 📊 **Domain Enumeration** - Intelligent routing to feroxbuster and puredns
- 📢 **Discord & Telegram Alerts** - Instant notifications for new discoveries
- 🔒 **Admin Panel** - Web interface for target management and configuration
- 📈 **Risk Scoring** - Automated risk assessment for discovered domains
- 🚀 **Production Ready** - Systemd service with auto-restart and resource limits
- 🔄 **Easy Updates** - One-command update process

## Quick Start - Local Testing (30 seconds)

```bash
cd /path/to/crtmon
./quick-start.sh
```

This will:
- Generate config
- Build binary
- Run crtmon locally
- Open admin panel at http://localhost:8080

Press `Ctrl+C` to stop.

## VPS Deployment (One Command)

### Prerequisites

- Private GitHub repository with crtmon code
- SSH key configured for GitHub access
- VPS with Ubuntu/Debian and sudo access
- Go 1.18+ (install.sh handles this automatically)

### Step 1: Create Private GitHub Repository

```bash
# On GitHub:
# 1. New repository → Private → "crtmon"
# 2. No README, .gitignore, or license needed yet
```

### Step 2: Push Code to GitHub

```bash
cd /path/to/crtmon
git remote add origin git@github.com:your-org/crtmon.git
git branch -M main
git push -u origin main
```

### Step 3: Deploy to VPS

SSH into your VPS and run:

```bash
GITHUB_REPO=git@github.com:your-org/crtmon.git sudo ./install.sh
```

**That's it!** The script will:
- ✅ Check system requirements
- ✅ Create unprivileged `crtmon` service user
- ✅ Clone repository to `/opt/crtmon`
- ✅ Compile binary
- ✅ Generate config directory (`/home/crtmon/.config/crtmon`)
- ✅ Install and start systemd service
- ✅ Configure log rotation
- ✅ Output admin panel URL and credentials

### Step 4: Configure

Edit the configuration:

```bash
sudo nano /home/crtmon/.config/crtmon/provider.yaml
```

Add Discord webhook (get from Discord server settings):

```yaml
discord:
  webhook_url: https://discordapp.com/api/webhooks/YOUR/WEBHOOK
  enabled: true
```

Or Telegram (get from BotFather):

```yaml
telegram:
  bot_token: YOUR_BOT_TOKEN
  chat_id: YOUR_CHAT_ID
  enabled: true
```

Restart service:

```bash
sudo systemctl restart crtmon
```

### Step 5: Verify

Check service is running:

```bash
sudo systemctl status crtmon
```

View logs in real-time:

```bash
sudo journalctl -u crtmon -f
```

Access admin panel (credentials shown after install):

```
http://your-vps-ip:8080
```

## Updating

Update to latest version:

```bash
cd /opt/crtmon
make update
```

This will:
- Pull latest code from GitHub
- Rebuild binary
- Restart systemd service automatically

## Service Management

```bash
# Check status
sudo systemctl status crtmon

# Start service
sudo systemctl start crtmon

# Stop service
sudo systemctl stop crtmon

# Restart service
sudo systemctl restart crtmon

# View logs (real-time)
sudo journalctl -u crtmon -f

# View logs (last 50 lines)
sudo journalctl -u crtmon -n 50

# View logs (today only)
sudo journalctl -u crtmon --since today

# Enable auto-start on boot
sudo systemctl enable crtmon

# Disable auto-start on boot
sudo systemctl disable crtmon
```

## Building Locally

### Build

```bash
make build
```

### Install (Local)

```bash
sudo make install
```

Installs to `/usr/local/bin/crtmon`

### Run

```bash
./crtmon
```

Open http://localhost:8080 in browser.

## Configuration

Configuration file: `/home/crtmon/.config/crtmon/provider.yaml`

Edit with:

```bash
sudo nano /home/crtmon/.config/crtmon/provider.yaml
```

### Required Settings

```yaml
# Admin panel credentials
admin:
  username: admin
  password: your-secure-password
  port: 8080

# Certificate monitoring
certstream:
  enabled: true

# Discord notifications (optional)
discord:
  webhook_url: https://discordapp.com/api/webhooks/...
  enabled: true

# Telegram notifications (optional)
telegram:
  bot_token: YOUR_BOT_TOKEN
  chat_id: YOUR_CHAT_ID
  enabled: true

# Target domains to monitor
targets: 
  - example.com
  - target.org
```

### Advanced Settings

```yaml
# SNI IP ranges (monthly refresh)
sni:
  enabled: true
  check_interval_days: 30
  sources:
    - amazon
    - google
    - digitalocean
    - microsoft
    - oracle

# Enumeration tools
feroxbuster:
  enabled: true
  wordlist: /usr/share/wordlists/dirb/common.txt
  threads: 50
  timeout: 5

puredns:
  enabled: true
  wordlist: /usr/share/wordlists/dns/all.txt
  threads: 100
  timeout: 3

# Risk scoring
risk_scoring:
  enabled: true
  min_score: 5
```

After editing configuration, restart:

```bash
sudo systemctl restart crtmon
```

## Admin Panel

Access at `http://your-vps-ip:8080`

### Add Target Domain

1. Click "Add Target"
2. Enter domain name (e.g., `example.com`)
3. Click "Add"
4. Service will restart automatically
5. Monitor will begin certificate collection for this domain

### Remove Target Domain

1. Find domain in list
2. Click "Remove"
3. Service will restart automatically

### View Results

- **Certificates**: Latest certificates discovered
- **Domains**: Enumerated subdomains
- **Alerts**: All Discord/Telegram notifications sent
- **Logs**: System activity and errors

## Troubleshooting

### Service won't start

Check logs:

```bash
sudo journalctl -u crtmon -n 50
```

Common issues:

```bash
# Permission denied on config directory
sudo chown -R crtmon:crtmon /home/crtmon/.config/crtmon

# Port 8080 already in use
sudo lsof -i :8080
sudo kill -9 <PID>

# Missing Go installation
go version  # Should be 1.18+
```

### High CPU or Memory Usage

Systemd limits are set to:
- Memory: 1 GB
- CPU: 80%

Check current usage:

```bash
systemctl status crtmon
```

Increase limits in `/etc/systemd/system/crtmon.service`:

```ini
MemoryLimit=2G
CPUQuota=160%
```

Then reload:

```bash
sudo systemctl daemon-reload
sudo systemctl restart crtmon
```

### No notifications arriving

1. Check webhook URL is correct:
   ```bash
   sudo nano /home/crtmon/.config/crtmon/provider.yaml
   ```

2. Test webhook manually:
   ```bash
   curl -X POST -H 'Content-type: application/json' \
     --data '{"text":"Test message"}' \
     YOUR_WEBHOOK_URL
   ```

3. Check logs:
   ```bash
   sudo journalctl -u crtmon -f
   ```

### Certificates not being collected

1. Verify certstream is enabled:
   ```bash
   grep -A2 "certstream:" /home/crtmon/.config/crtmon/provider.yaml
   ```

2. Check target domains are in config:
   ```bash
   grep "targets:" -A10 /home/crtmon/.config/crtmon/provider.yaml
   ```

3. Verify admin panel shows targets:
   - Open http://your-vps-ip:8080
   - Check "Targets" section

4. Restart service:
   ```bash
   sudo systemctl restart crtmon
   ```

## Uninstall

Remove crtmon from VPS:

```bash
# Stop service
sudo systemctl stop crtmon

# Remove service file
sudo rm /etc/systemd/system/crtmon.service

# Remove binary
sudo rm /usr/local/bin/crtmon

# Reload systemd
sudo systemctl daemon-reload

# Remove installation directory (optional)
sudo rm -rf /opt/crtmon

# Remove config directory (optional, keeps data if you want backup)
sudo rm -rf /home/crtmon/.config/crtmon

# Remove service user (optional)
sudo userdel -r crtmon
```

## GitHub Actions CI/CD

On every push to GitHub:
- ✅ Tests on Go 1.18, 1.19, 1.20, 1.21
- ✅ Code linting (golangci-lint)
- ✅ Security scanning (Gosec)
- ✅ Builds multi-platform releases

View results: GitHub → Actions tab

## Architecture

```
crtmon/
├── main.go              # Entry point
├── certstream.go        # Certificate stream monitoring
├── sni.go              # SNI IP range discovery
├── enum.go             # Domain enumeration (feroxbuster/puredns)
├── admin.go            # Web admin panel
├── send.go             # Discord/Telegram notifications
├── config.go           # Configuration management
├── help.go             # Help text
├── update.go           # Update checking
├── version.go          # Version info
├── Makefile            # Build automation
├── install.sh          # VPS installer
├── quick-start.sh      # Local testing script
└── crtmon.service      # Systemd unit file
```

## Security Features

- 🔒 Service runs as unprivileged user (`crtmon`)
- 🛡️ Resource limits prevent runaway processes
- 🔐 Admin panel requires password authentication
- 📝 All actions logged to systemd journal
- 🚫 API requires JWT tokens
- 🔑 Secrets never committed to git (.gitignore protection)
- ↩️ Auto-restart on crash for high availability
- 🔍 No hardcoded credentials

## Monitoring & Logging

All logs go to systemd journal. View with:

```bash
# Real-time
sudo journalctl -u crtmon -f

# Last N lines
sudo journalctl -u crtmon -n 100

# Since specific time
sudo journalctl -u crtmon --since "2 hours ago"

# Today only
sudo journalctl -u crtmon --since today

# Export to file
sudo journalctl -u crtmon > /tmp/crtmon-logs.txt

# With system context
sudo journalctl -u crtmon -o verbose
```

Logs are automatically rotated daily (see `/etc/logrotate.d/crtmon`).

## Performance Tuning

### Increase Enumeration Speed

Edit `/home/crtmon/.config/crtmon/provider.yaml`:

```yaml
feroxbuster:
  threads: 100        # Increase from 50
  timeout: 5
  rate_limit: 0       # Remove rate limit if server allows

puredns:
  threads: 200        # Increase from 100
  timeout: 3
```

Restart:

```bash
sudo systemctl restart crtmon
```

### Optimize Memory Usage

Reduce SNI refresh frequency:

```yaml
sni:
  check_interval_days: 45  # Increase from 30
```

Or disable SNI if not needed:

```yaml
sni:
  enabled: false
```

### Monitor Resource Usage

```bash
# Check live usage
systemctl status crtmon

# Check limits
cat /etc/systemd/system/crtmon.service | grep -E "Memory|CPU"

# Monitor over time
watch -n 1 'systemctl status crtmon | grep -E "CPU|Memory"'
```

## License

See LICENSE file

## Version

Current version shown with:

```bash
/opt/crtmon/crtmon -version
# or
make version
```

---

**Questions?** Check logs with `sudo journalctl -u crtmon -f` or review configuration at `/home/crtmon/.config/crtmon/provider.yaml`
