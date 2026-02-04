FROM golang:1.25 AS build

ENV CGO_ENABLED=0

WORKDIR /src
COPY . .

RUN go build -o /out/peer ./cmd/peer && \
    go build -o /out/replay ./cmd/replay && \
    go build -o /out/sim ./cmd/sim && \
    go build -o /out/webclient ./cmd/webclient && \
    go build -o /out/integration-ipc ./cmd/integration-ipc

FROM alpine:3.20

RUN apk add --no-cache ca-certificates busybox-extras && \
    addgroup -S ardents && \
    adduser -S -G ardents -h /var/lib/ardents -s /sbin/nologin ardents && \
    mkdir -p /var/lib/ardents && \
    chown -R ardents:ardents /var/lib/ardents

COPY --from=build /out/peer /usr/local/bin/peer
COPY --from=build /out/replay /usr/local/bin/replay
COPY --from=build /out/sim /usr/local/bin/sim
COPY --from=build /out/webclient /usr/local/bin/webclient
COPY --from=build /out/integration-ipc /usr/local/bin/integration-ipc

COPY docker/entrypoint-peer.sh /usr/local/bin/entrypoint-peer.sh
COPY docker/entrypoint-integration.sh /usr/local/bin/entrypoint-integration.sh

RUN chmod +x /usr/local/bin/entrypoint-peer.sh /usr/local/bin/entrypoint-integration.sh

USER ardents
WORKDIR /work
