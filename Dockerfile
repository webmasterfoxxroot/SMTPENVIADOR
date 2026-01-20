# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache gcc musl-dev git

# Copy source code first
COPY . .

# Download dependencies and generate go.sum
RUN go mod tidy

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o smtpenviador ./cmd/main.go

# Final stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates, timezone data, and Chromium for browser automation
RUN apk --no-cache add ca-certificates tzdata chromium chromium-chromedriver

# Set Chrome path for chromedp
ENV CHROME_PATH=/usr/bin/chromium-browser

# Copy binary from builder
COPY --from=builder /app/smtpenviador .

# Create non-root user
RUN adduser -D -g '' appuser
USER appuser

EXPOSE 8080

CMD ["./smtpenviador"]
