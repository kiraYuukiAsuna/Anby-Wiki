# syntax=docker/dockerfile:1.7

ARG GO_IMAGE=golang:1.26.6-alpine
ARG NODE_IMAGE=node:22-alpine3.22
ARG RUNTIME_IMAGE=alpine:3.22
ARG PYTHON_IMAGE=python:3.12.11-slim-bookworm

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
  go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/anby-wiki ./cmd/anby-wiki && \
  CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/wiki-migrate ./cmd/migrate && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/wiki-doctor ./cmd/doctor && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/wiki-ai-config-import-env ./cmd/ai-config-import-env && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/wiki-perf ./cmd/perf

FROM ${NODE_IMAGE} AS web-builder
ARG API_BASE_URL=http://api:8080
ENV API_BASE_URL=${API_BASE_URL}
ENV NEXT_TELEMETRY_DISABLED=1
WORKDIR /src/apps/web
COPY apps/web/package.json apps/web/package-lock.json ./
COPY apps/web/scripts/patch-minimatch-brace-expansion.mjs ./scripts/
RUN npm ci
COPY apps/web/ ./
COPY contracts/ /src/contracts/
RUN npm run build

FROM ${RUNTIME_IMAGE} AS go-runtime
RUN apk add --no-cache ca-certificates wget && \
    addgroup -S -g 10001 wiki && adduser -S -D -H -u 10001 -G wiki wiki
USER 10001:10001
WORKDIR /app

FROM go-runtime AS api
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="Anby Wiki API" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"
COPY --from=go-builder /out/wiki-api /usr/local/bin/wiki-api
ENV API_PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=6 \
  CMD wget -q -O /dev/null "http://127.0.0.1:${API_PORT}/readyz" || exit 1
CMD ["wiki-api"]

FROM go-runtime AS worker
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
USER root
RUN apk add --no-cache \
    poppler-utils \
    tesseract-ocr \
    tesseract-ocr-data-chi_sim \
    tesseract-ocr-data-eng
USER 10001:10001
LABEL org.opencontainers.image.title="Anby Wiki Worker" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"
COPY --from=go-builder /out/wiki-worker /usr/local/bin/wiki-worker
EXPOSE 9091
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=6 \
  CMD wget -q -O /dev/null http://127.0.0.1:9091/metrics || exit 1
CMD ["wiki-worker"]

FROM go-runtime AS cli
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="Anby Wiki Agent CLI" \
  org.opencontainers.image.version="${VERSION}" \
  org.opencontainers.image.revision="${VCS_REF}" \
  org.opencontainers.image.created="${BUILD_DATE}"
COPY --from=go-builder /out/anby-wiki /usr/local/bin/anby-wiki
ENTRYPOINT ["anby-wiki"]

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
COPY --from=go-builder /out/wiki-ai-config-import-env /usr/local/bin/wiki-ai-config-import-env
COPY --from=go-builder /out/wiki-perf /usr/local/bin/wiki-perf
COPY backend/migrations/ /app/migrations/
CMD ["wiki-migrate"]

FROM ${PYTHON_IMAGE} AS ai-kernel
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="Anby Wiki AI Kernel" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PORT=8090
RUN groupadd --gid 10002 kernel && \
    useradd --uid 10002 --gid kernel --no-create-home --shell /usr/sbin/nologin kernel
WORKDIR /app
COPY services/ai-kernel/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir --requirement requirements.txt && \
    pip install --no-cache-dir --no-deps semantic-kernel==1.44.1
COPY services/ai-kernel/app.py ./app.py
USER 10002:10002
EXPOSE 8090
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=6 \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8090/healthz', timeout=2)" || exit 1
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8090", "--no-access-log"]

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
COPY --from=web-builder --chown=node:node /src/apps/web/.next/standalone /app/
COPY --from=web-builder --chown=node:node /src/apps/web/public ./public
COPY --from=web-builder --chown=node:node /src/apps/web/.next/static ./.next/static
USER node
EXPOSE 3000
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=6 \
  CMD node -e "const n=require('net').connect(3000,'127.0.0.1',()=>{n.end();process.exit(0)});n.on('error',()=>process.exit(1))"
CMD ["node", "server.js"]
