#!/bin/bash
# Run this BEFORE `docker compose build` to:
#   1. Build the Next.js frontend
#   2. Copy the standalone + static assets into the Go embed path
#   3. Update .env with the current commit hash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$ROOT_DIR"

COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "dev")

echo "=== Building frontend ==="
cd frontend
npm ci
npm run build
cd ..

STANDALONE="frontend/.next/standalone"
STATIC="frontend/.next/static"
SERVER="frontend/.next/server"
EMBED="backend/internal/frontend/dist"

echo "=== Copying frontend to Go embed path ==="
rm -rf "$EMBED"
mkdir -p "$EMBED"

# Copy standalone (server.js, node_modules, .next/server for SSR)
cp -r "$STANDALONE"/* "$EMBED/"

# Copy .next/static (client chunks, CSS) — standalone doesn't include these
mkdir -p "$EMBED/.next/static"
cp -r "$STATIC"/* "$EMBED/.next/static/"

echo "=== Updating .env ==="
if grep -q "^GIT_COMMIT=" .env 2>/dev/null; then
    sed -i "s/^GIT_COMMIT=.*/GIT_COMMIT=$COMMIT/" .env
else
    echo "GIT_COMMIT=$COMMIT" >> .env
fi

echo ""
echo "Ready to run: docker compose build && docker compose up -d"
echo "GIT_COMMIT=$COMMIT"
