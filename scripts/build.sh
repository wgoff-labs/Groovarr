#!/bin/bash
# Run this before `docker compose build` or `docker compose up --build`
# to bake the current git commit hash into the image tag.
set -e

cd "$(dirname "$0")"

COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "dev")

# Update .env with the current commit
if grep -q "^GIT_COMMIT=" .env 2>/dev/null; then
    sed -i "s/^GIT_COMMIT=.*/GIT_COMMIT=$COMMIT/" .env
else
    echo "GIT_COMMIT=$COMMIT" >> .env
fi

echo "GIT_COMMIT=$COMMIT" 
