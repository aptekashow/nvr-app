FROM golang:1.22-alpine AS builder

WORKDIR /app
RUN apk add --no-cache git ffmpeg

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /nvr-backend ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates tzdata
WORKDIR /app
COPY --from=builder /nvr-backend .
EXPOSE 8080
CMD ["./nvr-backend"]
