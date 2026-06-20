# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26.4

FROM golang:${GO_VERSION}-bookworm AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gopher-email ./cmd/gopher-email

FROM alpine:latest
WORKDIR /app

COPY --from=builder /out/gopher-email /usr/local/bin/gopher-email

# ENTRYPOINT ["/usr/local/bin/gopher-email"]
CMD ["run", "--config", "/app/config.yaml", "--verbose"]
