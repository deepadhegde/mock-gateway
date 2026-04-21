FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o mock-gateway ./cmd/main.go

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/mock-gateway .
COPY config.yaml .
COPY specs/ specs/
COPY ui/ ui/
EXPOSE 9000
CMD ["./mock-gateway", "--config", "config.yaml"]
