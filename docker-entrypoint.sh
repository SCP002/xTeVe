#!/bin/sh
set -eu

# XTEVE_PORT overrides the port from the settings; without it the port set in the web UI is used.
if [ -n "${XTEVE_PORT:-}" ]; then
    set -- -port="${XTEVE_PORT}" "$@"
fi

# exec makes xTeVe PID 1, so it receives the stop signal from Docker directly.
exec xteve -config="${XTEVE_CONF}" "$@"
