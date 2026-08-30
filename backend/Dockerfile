# --- Build stage ---
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
COPY go.mod go.sum ./
RUN go mod download

# Touch the version file using the CACHE_BUST arg so this layer (and all
# subsequent layers including COPY . .) invalidate when CACHE_BUST changes.
RUN echo "CACHE_BUST=$CACHE_BUST" >> /tmp/.build-marker

# Build
COPY . .

# Resolve GIT_COMMIT (in priority order):
#   1. GIT_COMMIT build arg (passed by user locally)
#   2. VERSION build arg (Warren sets this to the commit hash before pushing)
#   3. version.txt in the build context (git commit hash)
#   4. .git/HEAD fallback
#   5. "dev" as last resort
RUN echo "[DEBUG] .dockerignore contents:" && cat /.dockerignore 2>/dev/null || echo "(empty or not found)" && \
    echo "[DEBUG] Build context root ls:" && ls -la /build/ 2>/dev/null | head -5; \
    GIT_COMMIT_VAL=dev; \
    if [ -n "${GIT_COMMIT}" ]; then \
      GIT_COMMIT_VAL="${GIT_COMMIT}"; \
      echo "[DEBUG] Using GIT_COMMIT build arg: $GIT_COMMIT_VAL"; \
    elif [ -n "${VERSION}" ]; then \
      GIT_COMMIT_VAL="${VERSION}"; \
      echo "[DEBUG] Using VERSION build arg: $GIT_COMMIT_VAL"; \
    elif [ -f /build/version.txt ]; then \
      GIT_COMMIT_VAL=$(cat /build/version.txt | tr -d ' \n'); \
      echo "[DEBUG] Read GIT_COMMIT=$GIT_COMMIT_VAL from /build/version.txt"; \
    elif [ -f .git/HEAD ]; then \
      echo "[DEBUG] .git/HEAD found, contents: $(cat .git/HEAD)"; \
      ref=$(cat .git/HEAD | tr -d '\n'); \
      case "$ref" in \
        ref:*) \
          branch=$(echo "$ref" | sed 's/^ref: *//'); \
          echo "[DEBUG] Detected branch ref: $branch"; \
          if [ -f ".git/$branch" ]; then \
            GIT_COMMIT_VAL=$(cat ".git/$branch" | cut -c1-7); \
            echo "[DEBUG] Read commit from .git/$branch: $GIT_COMMIT_VAL"; \
          elif [ -f .git/HEAD ] && [ $(cat .git/HEAD | wc -l) -eq 1 ]; then \
            GIT_COMMIT_VAL=$(cat .git/HEAD | head -c 7); \
            echo "[DEBUG] Falling back to .git/HEAD content: $GIT_COMMIT_VAL"; \
          else \
            GIT_COMMIT_VAL=$(git rev-parse --short HEAD 2>/dev/null || echo dev); \
            echo "[DEBUG] git rev-parse fallback: $GIT_COMMIT_VAL"; \
          fi \
          ;; \
        *) \
          GIT_COMMIT_VAL=$(echo "$ref" | head -c 7); \
          echo "[DEBUG] Detached HEAD: $GIT_COMMIT_VAL"; \
          ;; \
      esac; \
    else \
      echo "[DEBUG] version.txt and .git/HEAD not found, using dev"; \
    fi; \
    echo "Building with GIT_COMMIT=$GIT_COMMIT_VAL" && \
    CGO_ENABLED=1 GOOS=linux go build \
        -ldflags="-s -w -X main.GitCommit=$GIT_COMMIT_VAL -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        -o groovarr-backend .

# --- Runtime stage ---
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata sqlite

WORKDIR /app
COPY --from=builder /build/groovarr-backend /app/groovarr-backend

ENV DB_PATH=/data/watchlist.db
ENV PORT=8080

EXPOSE 8080

VOLUME ["/data"]

ENTRYPOINT ["/app/groovarr-backend"]