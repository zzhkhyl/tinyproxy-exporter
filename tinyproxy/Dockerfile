# Tinyproxy, tuned for container use and for Prometheus scraping.
#
# The image intentionally stays on a "root" PID 1: tinyproxy binds the
# configured port and then drops privileges itself through the `User`/`Group`
# directives in tinyproxy.conf.
FROM alpine:3.24

LABEL org.opencontainers.image.title="tinyproxy" \
      org.opencontainers.image.description="Tinyproxy HTTP proxy with StatHost/StatFile enabled for Prometheus scraping" \
      org.opencontainers.image.source="https://github.com/tinyproxy/tinyproxy"

# Alpine's CDN (dl-cdn.alpinelinux.org) is unreachable from some locked down
# networks. Point APK_MIRROR at a mirror to work around that, for example:
#   docker build --build-arg APK_MIRROR=https://mirrors.aliyun.com/alpine .
ARG APK_MIRROR=""

# Upstream tinyproxy version, e.g. "1.11.1". tinyproxy is installed with `apk
# add`, so its version is whatever the base image's repository carries -- not
# something this repository decides. CI resolves that version first and passes
# it in here; it becomes the GHCR tag and the OCI version label.
#
# Because the tag is a promise about the image contents, the RUN below compares
# it against what actually got installed and fails the build on a mismatch.
# Leave it empty (local `make images`) to skip both the label and the check.
ARG TINYPROXY_VERSION=""

LABEL org.opencontainers.image.version="${TINYPROXY_VERSION}"

# tinyproxy lives in Alpine's `community` repository, which is enabled by
# default. curl is only used by the container health check.
RUN set -eu; \
    : "${APK_MIRROR:=}"; \
    : "${TINYPROXY_VERSION:=}"; \
    if [ -n "${APK_MIRROR}" ]; then \
        sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${APK_MIRROR}|g" /etc/apk/repositories; \
    fi; \
    apk add --no-cache tinyproxy curl; \
    installed="$(apk list --installed tinyproxy | head -n1 | cut -d' ' -f1)"; \
    installed="${installed#tinyproxy-}"; \
    installed="${installed%-r*}"; \
    if [ -n "${TINYPROXY_VERSION}" ] && [ "${TINYPROXY_VERSION%-r*}" != "${installed}" ]; then \
        echo "TINYPROXY_VERSION=${TINYPROXY_VERSION} but the base image installed tinyproxy ${installed}" >&2; \
        echo "the published tag would misdescribe the image; bump the expected version" >&2; \
        exit 1; \
    fi; \
    printf '%s\n' "${installed}" > /etc/tinyproxy/VERSION; \
    grep -q '^tinyproxy:' /etc/group  || addgroup -S tinyproxy; \
    grep -q '^tinyproxy:' /etc/passwd || adduser -S -D -H -s /sbin/nologin -G tinyproxy tinyproxy; \
    mkdir -p /var/log/tinyproxy; \
    chown -R tinyproxy:tinyproxy /var/log/tinyproxy; \
    chmod 0755 /var/log/tinyproxy

COPY tinyproxy.conf /etc/tinyproxy/tinyproxy.conf
COPY statpage.txt   /etc/tinyproxy/statpage.txt
COPY filter         /etc/tinyproxy/filter
COPY entrypoint.sh  /usr/local/bin/entrypoint.sh

RUN chmod 0755 /usr/local/bin/entrypoint.sh \
 && chmod 0644 /etc/tinyproxy/tinyproxy.conf /etc/tinyproxy/statpage.txt /etc/tinyproxy/filter

# Proxy port. Change it in tinyproxy.conf and re-expose if you need another one.
EXPOSE 8888

# Shared with the exporter container so it can tail tinyproxy.log.
VOLUME ["/var/log/tinyproxy"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD /bin/sh -c 'curl -fsS --max-time 3 -x http://127.0.0.1:8888 http://tinyproxy.stats/ -o /dev/null || exit 1'

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["tinyproxy", "-d", "-c", "/etc/tinyproxy/tinyproxy.conf"]
