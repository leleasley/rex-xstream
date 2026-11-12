FROM golang:1.27-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/rex-xstream ./cmd/rex-xstream

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata ffmpeg wget && \
    addgroup -S -g 1000 app && adduser -S -D -H -u 1000 -G app app && \
    mkdir -p /data && chown -R app:app /data
WORKDIR /app
COPY --from=build /out/rex-xstream /app/rex-xstream
COPY assets /app/assets
COPY docs /app/docs
ENV APP_HOST=0.0.0.0 \
    APP_PORT=9981 \
    DATABASE_PATH=/data/rex-xstream.db \
    FFPROBE_PATH=ffprobe \
    LOG_LEVEL=info
USER app
EXPOSE 9981
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:9981/api/v1/health || exit 1
ENTRYPOINT ["/app/rex-xstream"]
