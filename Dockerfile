# ── Stage 1: Development (hot-reload with Air) ────────────────
FROM golang:1.24-alpine AS dev

RUN apk add --no-cache git curl make

WORKDIR /app

# Install Air for hot-reload
RUN go install github.com/air-verse/air@latest

COPY go.mod go.sum ./
RUN go mod download

CMD ["air"]

# ── Stage 2: Builder ──────────────────────────────────────────
FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o taskflow .

# ── Stage 3: Production (minimal image) ───────────────────────
FROM alpine:3.20 AS production

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /build/taskflow .
COPY --from=builder /build/static ./static
COPY --from=builder /build/*.html ./

EXPOSE 8080

CMD ["./taskflow"]
