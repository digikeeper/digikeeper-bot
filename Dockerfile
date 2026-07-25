# --- Build stage ---
FROM golang:1.26-bookworm AS builder

WORKDIR /usr/app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /usr/app/bin/digikeeper-bot ./cmd/bot

# --- Runtime stage ---
FROM debian:bookworm-slim AS main

RUN groupadd --gid 10001 app \
    && useradd --uid 10001 --gid app --no-create-home --shell /usr/sbin/nologin app \
    && install -d --owner=app --group=app --mode=0750 /data

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/app/bin/digikeeper-bot /usr/local/bin/digikeeper-bot
COPY --chmod=755 docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

# Matches the SQLITE_PATH volume mount configured in deployment/docker-compose.yml.
VOLUME ["/data"]

USER app:app

EXPOSE 8081
# metrics
EXPOSE 8091

ENTRYPOINT ["docker-entrypoint.sh"]
