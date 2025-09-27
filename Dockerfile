FROM golang:1.21-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -o /go-bin ./cmd

FROM alpine:latest

RUN apk add --no-cache sqlite

COPY --from=builder /go-bin /app/go-bin

WORKDIR /app

CMD ["/app/go-bin"]