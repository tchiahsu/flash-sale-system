FROM golang:1.25 AS builder

WORKDIR /app/

COPY go.mod go.sum ./

RUN go mod download

COPY main.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o inventory-service ./

FROM alpine:latest

WORKDIR /app/

COPY --from=builder /app/inventory-service ./

EXPOSE 8080

CMD ["./inventory-service"]