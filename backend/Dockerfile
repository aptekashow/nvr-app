FROM golang:1.22-alpine AS builder

WORKDIR /app

# Avoid sumdb/proxy interception issues on some networks
ENV GOPROXY=https://proxy.golang.org,https://goproxy.io,direct
ENV GOSUMDB=off
ENV GOTOOLCHAIN=local

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /nvr-backend ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates tzdata
WORKDIR /app
COPY --from=builder /nvr-backend .
EXPOSE 8080
CMD ["./nvr-backend"]
