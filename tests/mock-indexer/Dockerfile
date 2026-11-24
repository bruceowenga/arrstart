# Build stage: compile a static Go binary for the mock Torznab indexer.
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod .
COPY tests/mock-indexer/main.go ./main.go
RUN go build -o /bin/mock-indexer main.go

# Run stage: minimal image, non-root user, no shell, no package manager.
FROM alpine:3.20
RUN addgroup -S mock-indexer && adduser -S mock-indexer -G mock-indexer
WORKDIR /app
COPY --from=builder /bin/mock-indexer /bin/mock-indexer
USER mock-indexer
EXPOSE 9999
ENTRYPOINT ["/bin/mock-indexer"]
