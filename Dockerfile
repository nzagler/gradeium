# syntax=docker/dockerfile:1.7

FROM node:24-bookworm-slim AS frontend-build
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26.5-bookworm AS backend-build
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build \
    -buildvcs=false \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/gradeium \
    ./cmd/gradeium

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 gradeium \
    && useradd --system --uid 10001 --gid gradeium --home-dir /app --no-create-home --shell /usr/sbin/nologin gradeium \
    && install -d -o gradeium -g gradeium /app /config /backups

WORKDIR /app
COPY --from=backend-build --chown=gradeium:gradeium /out/gradeium /app/gradeium
COPY --from=frontend-build --chown=gradeium:gradeium /src/frontend/dist /app/frontend

ENV GRADEIUM_LISTEN_ADDRESS=:8080 \
    GRADEIUM_CONFIG_DIR=/config \
    GRADEIUM_BACKUPS_DIR=/backups \
    GRADEIUM_WEB_DIR=/app/frontend \
    GRADEIUM_LOG_LEVEL=info

USER 10001:10001
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/gradeium", "healthcheck", "http://127.0.0.1:8080/api/healthz"]

ENTRYPOINT ["/app/gradeium"]
