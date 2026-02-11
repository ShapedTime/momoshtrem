#===============
# Stage 1: Build
#===============

FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /momoshtrem ./cmd/momoshtrem

#===============
# Stage 2: Run
#===============

FROM alpine:3

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /momoshtrem /bin/momoshtrem
RUN chmod +x /bin/momoshtrem

# Create data directory
RUN mkdir -p /data /config

WORKDIR /data

EXPOSE 4444 36911 9090

ENTRYPOINT ["/bin/momoshtrem", "-config", "/config/config.yaml"]
