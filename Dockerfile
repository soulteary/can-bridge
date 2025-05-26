# ---- Build Stage ----
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY --link go.mod go.sum ./
RUN go mod download

COPY --link *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o can-bridge .

# ---- Runtime Stage ----
ARG ALPINE_VERSION=3.21
FROM alpine:3.21

WORKDIR /app

COPY --link --from=builder /app/can-bridge /usr/local/bin/can-bridge

EXPOSE 5260

ENV CAN_PORTS="can0"
ENV SERVER_PORT="5260"

CMD ["can-bridge"]