# Groovarr

Lidarr Hits Monitor & Auto-Pruner - a Discord-integrated music library management tool.

## Features

- **Dashboard** - View library status, artist count, and last check time
- **Artist Management** - Add/remove artists from your watchlist
- **Auto-Pruning** - Automatically remove tracks below popularity threshold
- **Daily Checks** - Scheduled scanning with Discord reports
- **Settings** - Configure Lidarr, Last.fm, popularity thresholds, and more via the web UI

## Architecture

Single container deployment: the Go backend embeds and serves the Next.js frontend. All configuration is stored in SQLite via the settings page.

```
┌─────────────────────────────────┐
│     groovarr-backend:8080        │
│  ┌─────────────┬─────────────┐  │
│  │  Go API     │ Next.js UI  │  │
│  │  /api/*     │ /*          │  │
│  └─────────────┴─────────────┘  │
│  ┌─────────────────────────┐    │
│  │      SQLite DB           │    │
│  │  (artists, settings)     │    │
│  └─────────────────────────┘    │
└─────────────────────────────────┘
```

## Deployment (Portainer)

### 1. Clone/Update Repository

On your Proxmox host:
```bash
cd /opt/groovarr
git pull origin main
```

### 2. Configure Environment

Copy `.env.example` to `.env` and set your values:
```bash
cp .env.example .env
nano .env
```

Required environment variables:
```env
PORT=8080                    # Internal port (default: 8080)
AUTH_USERNAME=admin         # Web UI login username
AUTH_PASSWORD=changeme       # Web UI login password
DB_PATH=/data/groovarr.db   # Database location (leave as-is)
DB_SALT=                    # Optional: database encryption salt
```

### 3. Deploy Stack

In Portainer:
1. Create a new Stack
2. Paste the contents of `docker-compose.yml`
3. Add environment variables from your `.env` file
4. Deploy

Or via CLI on Proxmox:
```bash
cd /opt/groovarr
docker compose up -d
```

### 4. Access

- **Web UI**: http://your-proxmox-host:8080
- **API**: http://your-proxmox-host:8080/api/status

## Configuration (Settings Page)

After first login, configure via the Settings page:
- **Lidarr URL** (e.g., `http://10.0.0.244:8686`)
- **Lidarr API Key**
- **Lidarr Root Folder**
- **Last.fm API Key** (for popularity data)
- **Popularity Threshold** (0-100)
- **Daily Check Schedule** (cron format, e.g., `0 9 * * *`)
- **Discord Token** (optional)
- **Report Channel ID** (optional)

All settings persist in the SQLite database.

## Discord Commands (if bot enabled)

- `?help` - Show command list
- `?status` - Library status
- `?list` - Your watchlist
- `?check` - Run daily check
- `?add <artist>` - Add artist
- `?remove <artist>` - Remove artist
- `?prune` - Run pruning

## Tech Stack

- **Backend**: Go 1.24 + SQLite
- **Frontend**: Next.js 16 + TypeScript
- **Discord**: DisGo v0.19.6
- **Deployment**: Docker on Proxmox

## License

MIT
