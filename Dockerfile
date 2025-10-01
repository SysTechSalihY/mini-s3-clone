FROM golang:1.25.1 AS builder

# Set working directory inside the container
WORKDIR /app

# Enable Go modules
ENV GO111MODULE=on
ENV CGO_ENABLED=0

# Copy go.mod and go.sum first for caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the project
COPY . .
#Expose the port
EXPOSE ${PORT}
# Build the binary
RUN go build -o s3clone ./main.go

# =========================
# Stage 2: Create minimal runtime image
# =========================
FROM debian:bookworm-slim

# Set working directory
WORKDIR /app

# Copy the compiled binary from builder
COPY --from=builder /app/s3clone .

# Copy data directory for
