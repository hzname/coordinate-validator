FROM golang:1.21-alpine AS builder

RUN apk add --no-cache protobuf-dev git

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Generate protobuf
RUN protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto/coordinate.proto

# Build
RUN CGO_ENABLED=1 go build -o /coordinate-validator ./cmd/server

# Production image
FROM alpine:3.18

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /coordinate-validator .

EXPOSE 50051

CMD ["./coordinate-validator"]
