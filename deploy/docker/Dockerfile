# syntax=docker/dockerfile:1

FROM node:24-bookworm-slim AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-bookworm AS backend
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libvirt-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/web/dist ./web/dist

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=1 go build -trimpath \
    -ldflags "-s -w -X 'virtmgr/internal/version.Version=${VERSION}' -X 'virtmgr/internal/version.Commit=${COMMIT}' -X 'virtmgr/internal/version.Date=${BUILD_DATE}'" \
    -o /out/virtmgr ./cmd/virtmgr

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl libvirt0 tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system virtmgr \
    && useradd --system --gid virtmgr --home-dir /var/lib/virtmgr --create-home --shell /usr/sbin/nologin virtmgr \
    && mkdir -p /etc/virtmgr /var/lib/virtmgr \
    && chown -R virtmgr:virtmgr /var/lib/virtmgr /etc/virtmgr
COPY --from=backend /out/virtmgr /usr/local/bin/virtmgr

ENV VIRTMGR_LISTEN=[::]:8443 \
    VIRTMGR_LIBVIRT_URI=qemu:///system \
    VIRTMGR_DATABASE_PATH=/var/lib/virtmgr/virtmgr.db \
    VIRTMGR_READ_ONLY=true \
    VIRTMGR_COOKIE_SECURE=false

EXPOSE 8443
VOLUME ["/var/lib/virtmgr"]
USER virtmgr
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD ["curl", "-fsS", "http://127.0.0.1:8443/healthz"]
ENTRYPOINT ["/usr/local/bin/virtmgr"]
