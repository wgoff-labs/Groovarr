# --- Build stage for the Next.js frontend ---
FROM node:22-alpine AS frontend-builder
WORKDIR /build

# Cache npm deps
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci

# Build the standalone Next.js bundle
COPY frontend/ ./
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

# --- Build stage for the Go backend ---
FROM golang:1.24-alpine AS builder
WORKDIR /build

# Install CGO deps for SQLite
RUN apk add --no-cache gcc musl-dev git

# Accept GIT_COMMIT / VERSION from Portainer build args.
# Warren: update VERSION to the current commit hash before pushing.
# Example: VERSION=79cccaf docker compose build && docker compose up -d
ARG GIT_COMMIT=
ARG VERSION=
# CACHE_BUST changes every build to force Docker to re-fetch the build context
# and re-run all layers after it. Must be updated manually before each push.
ARG CACHE_BUST=1

# Cache deps
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Touch the version file using the CACHE_BUST arg so this layer (and all
# subsequent layers including COPY . .) invalidate when CACHE_BUST changes.
RUN echo "CACHE_BUST=$CACHE_BUST" >> /tmp/.build-marker

# Copy the backend source plus the freshly built frontend dist (so `//go:embed all:dist` picks it up).
# We copy the standalone output (server.js, node_modules, .next/server for SSR) and
# also the static directory (client chunks, CSS — standalone doesn't include these).
COPY backend/ ./
COPY --from=frontend-builder --chown=root:root /build/.next/standalone/. ./internal/frontend/dist/
COPY --from=frontend-builder --chown=root:root /build/.next/static ./internal/frontend/dist/.next/static

# Resolve GIT_COMMIT (in priority order):
#   1. GIT_COMMIT build arg (passed by user locally)
#   2. VERSION build arg (Warren sets this to the commit hash before pushing)
#   3. "dev" as last resort
RUN set -e; \
    echo "=== Groovarr Build ==="; \
    GIT_COMMIT_VAL=dev; \
    if [ -n "${GIT_COMMIT}" ]; then \
      GIT_COMMIT_VAL="${GIT_COMMIT}"; \
      echo "Using GIT_COMMIT build arg: $GIT_COMMIT_VAL"; \
    elif [ -n "${VERSION}" ]; then \
      GIT_COMMIT_VAL="${VERSION}"; \
      echo "Using VERSION build arg: $GIT_COMMIT_VAL"; \
    else \
      echo "version.txt not found, using dev"; \
    fi; \
    BUILD_NUM=$(cat /build/build.txt 2>/dev/null | tr -d ' \n' || echo "0"); \
    echo "Building with GIT_COMMIT=$GIT_COMMIT_VAL BUILD_NUM=$BUILD_NUM"; \
    set -x; \
    CGO_ENABLED=1 GOOS=linux go build \
        -ldflags="-s -w -X main.BuildNumber=$BUILD_NUM -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        -o groovarr-backend . || { \
        echo "BUILD FAILED - go build output above"; \
        exit 1; \
    }

# --- Runtime stage ---
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata sqlite nodejs npm

WORKDIR /app
COPY --from=builder /build/groovarr-backend /app/groovarr-backend

ENV DB_PATH=/data/watchlist.db
ENV PORT=8080

EXPOSE 8080

VOLUME ["/data"]

ENTRYPOINT ["/app/groovarr-backend"]
