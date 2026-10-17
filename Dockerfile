# syntax=docker/dockerfile:1.7
#
# REA Importify images. Targets:
#   api        Go API (and rea-worker) with pg_dump for backups      -> deploy/docker-compose.yml "api"/"worker"
#   web        Caddy serving the dashboard and proxying /api          -> deploy/docker-compose.yml "web"
#   extension  the packed browser extension, for `--output type=local,dest=out`
#
# Base images are pinned to exact versions. Behind a TLS-intercepting proxy, add
#   --secret id=extra_ca,src=/path/to/proxy-ca.crt
ARG GO_IMAGE=golang:1.27.2-alpine3.24
ARG NODE_IMAGE=node:22.23.3-alpine3.24
ARG ALPINE_IMAGE=alpine:3.24.2
ARG CADDY_IMAGE=caddy:2.11.7-alpine
ARG PG_IMAGE=postgres:18.6-alpine3.24
ARG PNPM_VERSION=10.28.0

# ---- Go binaries ----
FROM ${GO_IMAGE} AS go-build
ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-trimpath
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=bind,source=deploy/docker/with-ca.sh,target=/usr/local/bin/with-ca \
    --mount=type=secret,id=extra_ca \
    --mount=type=cache,target=/go/pkg/mod \
    with-ca go mod download
COPY db ./db
COPY services ./services
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -tags timetzdata -ldflags "-s -w -X main.version=${VERSION}" -o /out/rea-api ./services/api && \
    go build -tags timetzdata -ldflags "-s -w -X main.version=${VERSION}" -o /out/rea-worker ./services/worker

# ---- JavaScript workspace (dashboard and extension) ----
FROM ${NODE_IMAGE} AS js-deps
ARG PNPM_VERSION
WORKDIR /src
RUN --mount=type=bind,source=deploy/docker/with-ca.sh,target=/usr/local/bin/with-ca \
    --mount=type=secret,id=extra_ca \
    with-ca npm install -g --no-fund --no-audit pnpm@${PNPM_VERSION}
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/
COPY apps/extension/package.json apps/extension/
COPY packages/api-types/package.json packages/api-types/
RUN --mount=type=bind,source=deploy/docker/with-ca.sh,target=/usr/local/bin/with-ca \
    --mount=type=secret,id=extra_ca \
    --mount=type=cache,target=/root/.local/share/pnpm/store \
    with-ca pnpm install --frozen-lockfile
COPY packages/api-types ./packages/api-types
COPY testdata ./testdata

FROM js-deps AS web-build
COPY apps/web ./apps/web
RUN pnpm --filter @rea/web build

FROM js-deps AS extension-build
COPY apps/extension ./apps/extension
RUN pnpm --filter @rea/extension build

FROM scratch AS extension
COPY --from=extension-build /src/apps/extension/dist /rea-extension

# ---- PostgreSQL client tools ----
# pg_dump/pg_restore/psql are taken from the same image the database runs, so their major version
# always matches the server. ldd collects exactly the shared libraries they need.
FROM ${PG_IMAGE} AS pgclient
RUN set -e; mkdir -p /out/bin /out/lib; \
    for b in pg_dump pg_restore psql; do \
      cp /usr/local/bin/$b /out/bin/; \
      ldd /usr/local/bin/$b | awk '$3 ~ /^\// {print $3}' | while read -r lib; do cp -L "$lib" /out/lib/; done; \
    done; \
    rm -f /out/lib/ld-musl-* /out/lib/libc.musl-*

# ---- API runtime ----
FROM ${ALPINE_IMAGE} AS api
COPY --from=pgclient /out/bin/ /usr/local/bin/
COPY --from=pgclient /out/lib/ /usr/local/lib/
RUN pg_dump --version && \
    addgroup -S -g 10001 rea && adduser -S -D -H -u 10001 -G rea -h /data rea && \
    mkdir -p /data && chown rea:rea /data
COPY --from=go-build /out/rea-api /out/rea-worker /usr/local/bin/
ENV REA_HTTP_ADDR=0.0.0.0:8080 \
    REA_DATA_DIR=/data \
    REA_LOG_FORMAT=json \
    REA_PG_DUMP=/usr/local/bin/pg_dump
USER rea:rea
WORKDIR /data
VOLUME ["/data"]
EXPOSE 8080
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=15s --timeout=5s --start-period=40s --retries=5 CMD ["rea-api", "healthcheck"]
ENTRYPOINT ["rea-api"]
CMD ["serve"]

# ---- Web server ----
FROM ${CADDY_IMAGE} AS web
COPY deploy/proxy/Caddyfile /etc/caddy/Caddyfile
COPY --from=web-build /src/apps/web/dist /srv/rea
EXPOSE 80 443
