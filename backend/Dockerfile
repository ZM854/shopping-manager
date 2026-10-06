FROM golang:1.26.4-bookworm AS dependencies
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download && go mod verify

FROM dependencies AS development
RUN GOBIN=/usr/local/bin go install github.com/air-verse/air@v1.67.4 \
    && air -v \
    && apt-get update && apt-get install -y --no-install-recommends wget \
    && rm -rf /var/lib/apt/lists/*
COPY . .
EXPOSE 8080
CMD ["air", "-c", ".air.toml"]

FROM dependencies AS testing
COPY . .
CMD ["go", "test", "-count=1", "-tags=integration", "./..."]

FROM dependencies AS migration-builder
RUN CGO_ENABLED=0 GOBIN=/out go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@v4.20.1 \
    && test -s /out/migrate

FROM alpine:3.22 AS migrations
RUN apk add --no-cache ca-certificates
COPY --from=migration-builder /out/migrate /usr/local/bin/migrate
ENTRYPOINT ["migrate"]

FROM dependencies AS builder
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api && test -s /out/api

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates \
    && addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /out/api ./api
USER app
EXPOSE 8080
CMD ["./api"]
