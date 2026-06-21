# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26.4

FROM golang:${GO_VERSION}-bookworm AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gopher-email ./cmd/gopher-email

FROM alpine:latest
# Add su-exec
RUN apk add --no-cache su-exec

WORKDIR /app

# add user
RUN addgroup -g 1000 dougpark && \
    adduser -u 1000 -G dougpark -D dougpark

COPY --from=builder /out/gopher-email /usr/local/bin/gopher-email

# Set ownership of the app directory
RUN chown -R dougpark:dougpark /app

# Switch to the non-root user
USER dougpark

# ENTRYPOINT ["/usr/local/bin/gopher-email"]
CMD ["run", "--config", "/app/config.yaml", "--verbose"]
