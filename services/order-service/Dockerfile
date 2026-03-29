# Build stage
FROM golang:1.25 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o order-service .

# Run stage
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/order-service .

EXPOSE 8081

CMD ["./order-service"]