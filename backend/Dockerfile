# --- Build stage ---
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install CGO deps for SQLite
RUN apk add --no-cache gcc musl-dev git

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .

# Extract git commit from the build context. The build context is either:
#   - the full git repo (Portainer "Repository" source, or local git pull)
#   - a snapshot (regular "Web editor" source)
# In both cases, .git/HEAD and .git/refs/heads/<branch> are available.
# We fall back to "dev" if .git/ isn't present.
RUN GIT_COMMIT=$( \
      if [ -f .git/HEAD ]; then \
        ref=$(cat .git/HEAD | tr -d '\n'); \
        case "$ref" in \
          ref:*) \
            branch=$(echo "$ref" | sed 's@^ref: *@@'); \
            if [ -f ".git/$branch" ]; then cat ".git/$branch" | tr -d '\n'; \
            else git rev-parse --short HEAD 2>/dev/null || echo dev; fi \
            ;; \
          *) echo "$ref" | head -c 7 ;; \
        esac; \
      else echo dev; \
    fi) && \
    echo "Building with GIT_COMMIT=$GIT_COMMIT" && \
    CGO_ENABLED=1 GOOS=linux go build \
        -ldflags="-s -w -X main.GitCommit=$GIT_COMMIT -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
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