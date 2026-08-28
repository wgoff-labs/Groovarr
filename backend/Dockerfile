# --- Build stage ---
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install CGO deps for SQLite
RUN apk add --no-cache gcc musl-dev

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o groovarr-backend .

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
