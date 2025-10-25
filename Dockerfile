# Build stage
FROM golang:1.24.9-alpine3.22 AS builder

WORKDIR /app

# Install git (required for go-git)
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o barnacle ./cmd/barnacle

# Runtime stage
FROM docker:28.5.1-cli

# Install Docker Compose plugin
RUN apk add --no-cache \
    docker-compose \
    git \
    openssh-client \
    ca-certificates

# Copy the binary from builder
COPY --from=builder /app/barnacle /usr/local/bin/barnacle

# Create .ssh directory and add GitHub's host keys
RUN mkdir -p /root/.ssh && \
    ssh-keyscan github.com >> /root/.ssh/known_hosts && \
    chmod 600 /root/.ssh/known_hosts

# Set working directory
WORKDIR /app

# Default environment variables
ENV BRANCH=main

ENTRYPOINT ["/usr/local/bin/barnacle"]
