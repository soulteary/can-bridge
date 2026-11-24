# ---- Build Stage ----
FROM golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder

WORKDIR /app

COPY --link go.mod go.sum ./
RUN go mod download

COPY --link *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o can-bridge .

# ---- Runtime Stage ----
ARG ALPINE_VERSION=3.24.4
FROM alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

WORKDIR /app

COPY --link --from=builder /app/can-bridge /usr/local/bin/can-bridge

EXPOSE 5260

ENV CAN_PORTS="can0"
ENV SERVER_PORT="5260"

CMD ["can-bridge"]
