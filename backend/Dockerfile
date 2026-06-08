# ── builder ───────────────────────────────────────────────────────────────────
FROM golang:1.25.6-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Cache dependency downloads separately from source so a code-only change
# does not re-download modules.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /bin/playarena ./cmd/api

# ── runtime ───────────────────────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -g 65532 nonroot && \
    adduser -u 65532 -G nonroot -H -D nonroot

COPY --from=builder /bin/playarena /bin/playarena

USER nonroot:nonroot

EXPOSE 8080 9090

# Liveness probe via the internal observability server (never the public port).
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:9090/live || exit 1

ENTRYPOINT ["/bin/playarena"]
