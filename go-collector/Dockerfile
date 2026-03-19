# Builder image
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /collector ./cmd/collector/

# Final minimal image
FROM alpine:latest

WORKDIR /app

COPY --from=builder /collector /app/collector

# Run the executable
CMD ["/app/collector"]
