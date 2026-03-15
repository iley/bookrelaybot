FROM --platform=linux/amd64 golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bookrelaybot ./cmd/bookrelaybot

FROM --platform=linux/amd64 debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates calibre && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /bookrelaybot /usr/local/bin/bookrelaybot

ENTRYPOINT ["bookrelaybot"]
