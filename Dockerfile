# syntax=docker/dockerfile:1.7
# Netward – multi-stage build: web UI (Node) → static Go binary → scratch image.

FROM --platform=$BUILDPLATFORM node:22-alpine@sha256:fe4f6d74e20370acb04ad889077d14d0f64a9c8ea6a42bc60cddf1e844699914 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts --no-audit --no-fund
COPY web/ ./
RUN npm run check && npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web /src/web/dist ./cmd/netward/web
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/netward ./cmd/netward
RUN mkdir -p /out/data \
 && echo "netward:x:10001:10001:Netward:/data:/sbin/nologin" > /out/passwd \
 && echo "netward:x:10001:" > /out/group

FROM scratch
ARG VERSION=dev
LABEL org.opencontainers.image.title="Netward" \
      org.opencontainers.image.description="Self-hosted network monitor: discovers every device on your LAN, tracks presence and alerts you." \
      org.opencontainers.image.source="https://github.com/Pyzyi/Netward" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/passwd /etc/passwd
COPY --from=build /out/group /etc/group
COPY --from=build --chown=10001:10001 --chmod=0700 /out/data /data
COPY --from=build /out/netward /netward
ENV NETWARD_DATA=/data
VOLUME /data
EXPOSE 8840
# Starts as root only to hand /data to the runtime user and open raw sockets;
# the process then switches to UID 10001 and keeps no capabilities.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/netward", "healthcheck"]
ENTRYPOINT ["/netward"]
CMD ["serve"]
