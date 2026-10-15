#!/bin/sh
#
# Prepare the shared log volume, then hand over to tinyproxy (exec, so it
# becomes PID 1 and receives signals directly).
#
# Why this exists:
#   tinyproxy opens its log file *after* dropping privileges, using
#   create_file_safely() which creates it with mode 0600 owned by the
#   unprivileged user (src/utils.c, src/main.c). An exporter container running
#   with a different UID could therefore never read it. We pre-create the file
#   with 0644 so both containers can use it: tinyproxy appends (it opens the
#   existing file with O_APPEND), the exporter reads.
#
set -eu

log_dir="${TINYPROXY_LOG_DIR:-/var/log/tinyproxy}"
log_file="${TINYPROXY_LOG_FILE:-${log_dir}/tinyproxy.log}"
run_user="${TINYPROXY_RUN_USER:-tinyproxy}"
run_group="${TINYPROXY_RUN_GROUP:-tinyproxy}"

say() {
    printf '%s [entrypoint] %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$*"
}

mkdir -p "$log_dir"

if [ ! -e "$log_file" ]; then
    : > "$log_file"
fi

if [ "$(id -u)" = "0" ]; then
    if chown -R "$run_user:$run_group" "$log_dir" 2>/dev/null; then
        chmod 0755 "$log_dir"
        chmod 0644 "$log_file"
    else
        say "warning: unable to chown ${log_dir} to ${run_user}:${run_group}"
        chmod 0755 "$log_dir" 2>/dev/null || true
        chmod 0644 "$log_file" 2>/dev/null || true
    fi
else
    say "running unprivileged (uid $(id -u)); skipping ownership fixes"
    chmod 0644 "$log_file" 2>/dev/null || true
fi

say "log file ready: ${log_file} ($(ls -l "$log_file" 2>/dev/null || echo 'missing'))"
say "starting: $*"

exec "$@"
