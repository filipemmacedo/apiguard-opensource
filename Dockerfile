FROM golang:1.25-bookworm AS build

WORKDIR /src

ARG TARGETOS=linux
ARG TARGETARCH=amd64

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/apiguard ./cmd/apiguard
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/migrate-sqlite ./cmd/migrate-sqlite

FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=build /out/apiguard /app/apiguard
COPY --from=build /out/migrate-sqlite /app/migrate-sqlite
COPY scripts/api-guard-entrypoint.sh /app/entrypoint.sh

EXPOSE 8080

RUN tr -d '\r' < /app/entrypoint.sh > /app/entrypoint.sh.tmp \
  && mv /app/entrypoint.sh.tmp /app/entrypoint.sh \
  && chmod +x /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]
