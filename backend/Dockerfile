# --- Build stage ---
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install CGO deps for SQLite
RUN apk add --no-cache gcc musl-dev git

# Accept GIT_COMMIT from Portainer build args (portainer passes them in),
# with a fallback to reading from .git/HEAD in the build context.
ARG GIT_COMMIT=

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .

# Resolve GIT_COMMIT:
#   1. If passed as --build-arg from Portainer, use it.
#   2. Otherwise read .git/HEAD from the build context (works for Repository source).
#   3. Fall back to "dev" if neither is available.
RUN GIT_COMMIT_ARG="${GIT_COMMIT:-}"; \
    if [ -n "$GIT_COMMIT_ARG" ]; then \
      echo "Commit from build arg: $GIT_COMMIT_ARG"; \
      GIT_COMMIT_VAL="$GIT_COMMIT_ARG"; \
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
      echo "[DEBUG] .git/HEAD not found, using dev"; \
      GIT_COMMIT_VAL=dev; \
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