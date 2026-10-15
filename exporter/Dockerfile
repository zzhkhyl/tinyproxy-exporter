# Multi stage build: static Go binary on top of a minimal Alpine runtime.
#
# Build (from the repository root):
#   docker build -t tinyproxy-exporter:local ./exporter
#
# Locked down networks may block dl-cdn.alpinelinux.org and proxy.golang.org.
# Both are overridable:
#   docker build \
#     --build-arg APK_MIRROR=https://mirrors.aliyun.com/alpine \
#     --build-arg GOPROXY=https://goproxy.cn,direct \
#     -t tinyproxy-exporter:local ./exporter
# See also Dockerfile.slim for a fully offline fallback.
FROM golang:1.26-alpine AS builder

# Module proxy used by `go mod download`.
ARG GOPROXY="https://proxy.golang.org,direct"
ENV GOPROXY=${GOPROXY}

WORKDIR /src

# Dependencies first: this layer is cached as long as go.mod/go.sum are stable.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG REVISION=unknown

# CGO off => a fully static binary, so the runtime stage needs no libc.
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags "-s -w -X main.version=${VERSION} -X main.revision=${REVISION}" \
        -o /out/tinyproxy-exporter .

FROM alpine:3.24

# Alpine's CDN is unreachable from some locked down networks; see above.
ARG APK_MIRROR=""

# wget is used by the health check, ca-certificates by outbound TLS (only needed
# if the tinyproxy endpoint is ever reached over https).
RUN if [ -n "${APK_MIRROR}" ]; then \
        sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${APK_MIRROR}|g" /etc/apk/repositories; \
    fi \
 && apk add --no-cache ca-certificates wget \
 && adduser -D -H -u 10001 -s /sbin/nologin exporter

COPY --from=builder /out/tinyproxy-exporter /usr/local/bin/tinyproxy-exporter

USER exporter
EXPOSE 9119

# The check targets the exporter itself, not tinyproxy: a degraded tinyproxy is
# reported through tinyproxy_up / tinyproxy_exporter_source_up, not by killing
# the container that is supposed to report it.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD /bin/sh -c 'port="${EXPORTER_LISTEN_ADDRESS##*:}"; wget -q -O /dev/null "http://127.0.0.1:${port:-9119}${EXPORTER_METRICS_PATH:-/metrics}" || exit 1'

ENTRYPOINT ["/usr/local/bin/tinyproxy-exporter"]
