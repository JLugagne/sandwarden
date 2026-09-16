# syntax=docker/dockerfile:1

# The desktop binary links GTK3/WebKitGTK, so the Go build needs cgo and the
# development headers. The frontend is built first and embedded from web/dist
# (see the go:embed directive in main.go).

# --- frontend ----------------------------------------------------------------
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# --- build -------------------------------------------------------------------
FROM golang:1.26-bookworm AS build
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev libx11-dev \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=1 go build -tags "gtk3,production" -trimpath -ldflags "-s -w" -o /sandwarden .

# Extract just the binary:
#   docker buildx build --target binary --output type=local,dest=bin .

# --- binary ------------------------------------------------------------------
# Export-only stage: produces a directory containing only the binary.
FROM scratch AS binary
COPY --from=build /sandwarden /sandwarden

# --- runtime -----------------------------------------------------------------
FROM debian:bookworm-slim AS runtime
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates libgtk-3-0 libwebkit2gtk-4.1-0 \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /sandwarden /usr/local/bin/sandwarden

# A desktop app needs the host display, the sandboxd socket and the sbx CLI.
# Example:
#   docker run --rm \
#     -e DISPLAY -v /tmp/.X11-unix:/tmp/.X11-unix \
#     -v "$HOME/.local/state/sandboxes:$HOME/.local/state/sandboxes" \
#     -v /usr/bin/sbx:/usr/local/bin/sbx:ro \
#     sandwarden
ENTRYPOINT ["sandwarden"]
