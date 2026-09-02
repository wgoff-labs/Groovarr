#!/bin/bash
# Run this BEFORE `docker compose build` to:
#   1. Build the Next.js frontend
#   2. Copy the standalone output into the Go embed path
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

FRONTEND_DIST="frontend/.next/standalone"
BACKEND_EMBED="backend/internal/frontend/dist"

echo "=== Copying frontend to Go embed path ==="
rm -rf "$BACKEND_EMBED"
cp -r "$FRONTEND_DIST" "$BACKEND_EMBED"

echo "=== Updating .env ==="
if grep -q "^GIT_COMMIT=" .env 2>/dev/null; then
    sed -i "s/^GIT_COMMIT=.*/GIT_COMMIT=$COMMIT/" .env
else
    echo "GIT_COMMIT=$COMMIT" >> .env
fi

echo ""
echo "Ready to run: docker compose build && docker compose up -d"
echo "GIT_COMMIT=$COMMIT"
