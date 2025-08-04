# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ipmi-exporter .

# Runtime stage
FROM alpine:latest

# Install ipmitool and ca-certificates
RUN apk --no-cache add ipmitool ca-certificates

# Create non-root user
RUN adduser -D -s /bin/sh ipmi

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/ipmi-exporter .

# Change ownership
RUN chown ipmi:ipmi /app/ipmi-exporter

# Switch to non-root user
USER ipmi

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/metrics || exit 1

# Run the application
CMD ["./ipmi-exporter"]