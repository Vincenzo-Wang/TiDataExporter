# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Tidy dependencies and build the application
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/server

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata dumb-init

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/main .
COPY --from=builder /app/config ./config

# Create non-root user
RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -D appuser && \
    chown -R appuser:appgroup /app

USER appuser

EXPOSE 8080

ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["./main"]
