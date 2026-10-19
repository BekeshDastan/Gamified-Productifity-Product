FROM golang:1.26.9-bookworm AS build
WORKDIR /src
COPY services/api/go.mod services/api/go.sum ./
RUN go mod download
COPY services/api/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/ ./cmd/api ./cmd/worker ./cmd/migrate ./cmd/healthcheck

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 ascend && useradd --uid 10001 --gid ascend --no-create-home ascend \
    && mkdir -p /data/blobs && chown -R ascend:ascend /data
COPY --from=build /out/ /usr/local/bin/
USER 10001:10001
WORKDIR /data
ENTRYPOINT ["/usr/local/bin/api"]
