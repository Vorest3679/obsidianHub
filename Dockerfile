FROM golang:1.26 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/obsidianhub ./cmd/subhub

FROM debian:trixie-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/obsidianhub /usr/local/bin/obsidianhub

WORKDIR /app
ENTRYPOINT ["/usr/local/bin/obsidianhub"]
CMD ["-config", "/app/config.json"]
