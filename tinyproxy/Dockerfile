# Tinyproxy, tuned for container use and for Prometheus scraping.
#
# The image intentionally stays on a "root" PID 1: tinyproxy binds the
# configured port and then drops privileges itself through the `User`/`Group`
# directives in tinyproxy.conf.
FROM alpine:3.20

LABEL org.opencontainers.image.title="tinyproxy" \
      org.opencontainers.image.description="Tinyproxy HTTP proxy with StatHost/StatFile enabled for Prometheus scraping" \
      org.opencontainers.image.source="https://github.com/tinyproxy/tinyproxy"

# Alpine's CDN (dl-cdn.alpinelinux.org) is unreachable from some locked down
# networks. Point APK_MIRROR at a mirror to work around that, for example:
#   docker build --build-arg APK_MIRROR=https://mirrors.aliyun.com/alpine .
ARG APK_MIRROR=""

# tinyproxy lives in Alpine's `community` repository, which is enabled by
# default. curl is only used by the container health check.
RUN if [ -n "${APK_MIRROR}" ]; then \
        sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${APK_MIRROR}|g" /etc/apk/repositories; \
    fi \
 && apk add --no-cache tinyproxy curl \
 && (grep -q '^tinyproxy:' /etc/group  || addgroup -S tinyproxy) \
 && (grep -q '^tinyproxy:' /etc/passwd || adduser -S -D -H -s /sbin/nologin -G tinyproxy tinyproxy) \
 && mkdir -p /var/log/tinyproxy \
 && chown -R tinyproxy:tinyproxy /var/log/tinyproxy \
 && chmod 0755 /var/log/tinyproxy

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
