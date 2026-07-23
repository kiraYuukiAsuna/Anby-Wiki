# syntax=docker/dockerfile:1.7

ARG GO_IMAGE=golang:1.26.5-alpine
ARG NODE_IMAGE=node:22-alpine3.22
ARG RUNTIME_IMAGE=alpine:3.22

FROM ${GO_IMAGE} AS go-builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/wiki-api ./cmd/api && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/wiki-worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/wiki-migrate ./cmd/migrate && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/wiki-doctor ./cmd/doctor && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/wiki-perf ./cmd/perf

FROM ${NODE_IMAGE} AS web-builder
ARG API_BASE_URL=http://api:8080
ENV API_BASE_URL=${API_BASE_URL}
ENV NEXT_TELEMETRY_DISABLED=1
WORKDIR /src/apps/web
COPY apps/web/package.json apps/web/package-lock.json ./
RUN npm ci
COPY apps/web/ ./
COPY contracts/ /src/contracts/
RUN npm run build

FROM ${RUNTIME_IMAGE} AS go-runtime
RUN apk add --no-cache ca-certificates wget && \
    addgroup -S -g 10001 wiki && adduser -S -D -H -u 10001 -G wiki wiki
COPY --chmod=0555 infra/deploy/container-entrypoint.sh /usr/local/bin/container-entrypoint
USER 10001:10001
WORKDIR /app
ENTRYPOINT ["/usr/local/bin/container-entrypoint"]

FROM go-runtime AS api
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="Anby Wiki API" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"
COPY --from=go-builder /out/wiki-api /usr/local/bin/wiki-api
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=6 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/readyz || exit 1
CMD ["wiki-api"]

FROM go-runtime AS worker
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="Anby Wiki Worker" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"
COPY --from=go-builder /out/wiki-worker /usr/local/bin/wiki-worker
EXPOSE 9091
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=6 \
  CMD wget -q -O /dev/null http://127.0.0.1:9091/metrics || exit 1
CMD ["wiki-worker"]

FROM go-runtime AS migrate
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="Anby Wiki Migration Tools" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"
COPY --from=go-builder /out/wiki-migrate /usr/local/bin/wiki-migrate
COPY --from=go-builder /out/wiki-doctor /usr/local/bin/wiki-doctor
COPY --from=go-builder /out/wiki-perf /usr/local/bin/wiki-perf
COPY backend/migrations/ /app/migrations/
CMD ["wiki-migrate"]

FROM ${NODE_IMAGE} AS web
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="Anby Wiki Web" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
WORKDIR /app/apps/web
COPY --chmod=0555 infra/deploy/container-entrypoint.sh /usr/local/bin/container-entrypoint
COPY --from=web-builder --chown=node:node /src/apps/web/.next/standalone /app/
COPY --from=web-builder --chown=node:node /src/apps/web/public ./public
COPY --from=web-builder --chown=node:node /src/apps/web/.next/static ./.next/static
USER node
EXPOSE 3000
ENTRYPOINT ["/usr/local/bin/container-entrypoint"]
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=6 \
  CMD node -e "const n=require('net').connect(3000,'127.0.0.1',()=>{n.end();process.exit(0)});n.on('error',()=>process.exit(1))"
CMD ["node", "server.js"]
