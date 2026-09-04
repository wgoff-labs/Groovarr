# Groovarr Project To-Do List (Updated 2026-09-04)

## P0 - Critical
- [ ] Unify configuration: Consolidate Env vs DB sources into a single truth (env vars are defaults, DB is runtime source for dynamic settings). Currently dual-path with secrets in DB.
- [ ] API Security: Wire `authMiddleware` into `main.go` — middleware code exists in `backend/internal/api/auth.go` but not applied to handler chain.
- [ ] Apply `config.SanitizeError()` to all API handlers — helpers exist in `sanitize.go` but not used.

## P1 - Important
- [ ] Remove `backend/internal/frontend/dist` from repo; move to build-time dependency (add to `.gitignore`).
- [ ] Log Sanitization: Ensure no sensitive data is logged in the API layer.
- [ ] Add input validation to API handlers to prevent data injection.
- [ ] Remove secrets (Lidarr API key, Last.fm API key, Discord token) from DB settings table; read from env only.

## P2 - Operational & Testing
- [ ] Expand test coverage: Core logic (`checker.go`, `popularity.go`, `config/`, `store/`) still lacks unit tests.
- [ ] Verify Discord report path for production scheduled checks.
- [ ] Verify login & settings persistence across service restarts.
- [ ] Refactor `scripts/build.sh` for cleaner build/embed separation.
- [ ] Ensure `.dockerignore` excludes `backend/internal/frontend/dist/`.
- [ ] Replace magic string settings keys with typed constants/struct.