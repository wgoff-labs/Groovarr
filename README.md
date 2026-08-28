# Groovarr 🎵

A self-hosted Lidarr monitor that only adds **popular** new releases to your music library. Rewritten in **Go** (backend) + **Next.js** (frontend) from the original Python `lidarr-hits-bot`. Includes a built-in **Discord bot** so you can still talk to it from your server.

## Features

- 🎯 **Popularity filtering** — only albums/tracks above your threshold are added
- 📅 **Daily automated check** (cron: 9 AM America/Detroit)
- ✂️ **Auto-prune** — removes below-threshold tracks from disk after download
- 🔒 **Never-prune** — protect specific tracks from being deleted
- 🌐 **Web UI** — dashboard, artist manager, settings (LAN-accessible)
- 🤖 **Discord bot** — `?add`, `?list`, `?check`, `?prune`, `?help`
- 📊 **Last.fm primary, Deezer fallback** for popularity scoring
- 🗂️ **5 root folder support** with per-folder metadata profile auto-detection
- 💿 **Tracks or Album mode** (you currently use **tracks**)

## Stack

| Layer    | Tech                                   |
| -------- | -------------------------------------- |
| Backend  | Go 1.24, SQLite (mattn/go-sqlite3)     |
| Frontend | Next.js 14, TypeScript, Tailwind CSS   |
| Bot      | disgo (modern Discord Go library)      |
| Scheduler| robfig/cron                            |
| Sources  | Deezer, Last.fm, Lidarr REST API       |

## Deploy with Portainer

1. In Portainer, create a new **Stack** from this `docker-compose.yml`
2. Set environment variables in the **Environment** tab (copy from `.env.example`)
3. **Only these environment variables are required** (all other configuration is stored in the database and managed via the web UI):
   - `PORT` (default: 8080)
   - `AUTH_USERNAME` (optional - if set, enables basic auth)
   - `AUTH_PASSWORD` (optional - if set, enables basic auth)
   - `DB_PATH` (default: `/data/groovarr.db`)
   - `DB_SALT` (optional, for future encryption features)
4. After first login, go to the **Settings page** in the web UI to configure:
   - **Discord**: Bot token, allowed channels/users, home channel for reports
   - **Lidarr**: URL, API key, quality profile, root folders
   - **Last.fm**: API key
   - **Popularity**: Threshold (1-100), download mode (tracks/album)
   - **Scheduler**: Check time, timezone
5. Deploy. Web UI: `http://<your-server-ip>:3000`. Backend API: `:8080`.

## Local Development

### Backend
```bash
cd backend
go mod tidy
go run .                  # serves API on :8080
```

### Frontend
```bash
cd frontend
npm install
npm run dev               # serves Next.js on :3000, proxies /api → :8080
```

## Database

SQLite stored at `/data/groovarr.db` (a Docker volume in production). To reset:

```bash
docker volume rm groovarr_groovarr-data
```

## Commands (Discord bot)

```
?help, ?status, ?list, ?check, ?prune, ?add <artist>, ?remove <artist>
```

## Layout

```
Groovarr/
├── backend/             # Go service (REST API + bot + scheduler)
│   ├── cmd/             # (future: cmd/groovarr split)
│   ├── internal/
│   │   ├── api/         # HTTP handlers
│   │   ├── clients/     # Lidarr, Deezer, Last.fm
│   │   ├── config/      # Env loading
│   │   ├── core/        # Business logic (checker, pruner, popularity)
│   │   ├── discord/     # disgo bot
│   │   └── scheduler/   # Cron
│   └── store/           # SQLite
├── frontend/            # Next.js 14 + TypeScript
│   └── src/app/         # /, /artists, /settings
├── docker-compose.yml
└── .env.example
```

## License

MIT