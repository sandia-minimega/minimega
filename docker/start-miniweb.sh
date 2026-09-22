#!/bin/bash

set -o pipefail

# Check if there are values in /etc/default/minimega
source /load-defaults.sh

# Final default assignment (if these are not set already)
: "${MINIWEB_ROOT:=/opt/minimega/web}"
: "${MINIWEB_HOST:=0.0.0.0}"
: "${MINIWEB_PORT:=9001}"
: "${MINIWEB_BASE:=/tmp/minimega}"
: "${MINIWEB_LOGLEVEL:=info}"

# console-attach.sh runs in a separate process spawned by miniweb, so the base
# directory has to be in its environment.
export MINIWEB_BASE

: "${MINIWEB_LOGFILE:=}"
: "${MINIWEB_NAMESPACE:=}"
: "${MINIWEB_PASSWORDS:=}"
: "${MINIWEB_CERT:=}"
: "${MINIWEB_KEY:=}"
: "${MINIWEB_CONSOLE:=}"
: "${MINIWEB_APPEND:=}"

MINIWEB_ARGS=(
  -root="${MINIWEB_ROOT}"
  -addr="${MINIWEB_HOST}:${MINIWEB_PORT}"
  -base="${MINIWEB_BASE}"
  -level="${MINIWEB_LOGLEVEL}"
)

# Only pass the optional flags that have a value so miniweb keeps its own
# defaults for everything left unset.
if [[ -n "${MINIWEB_LOGFILE}" ]]; then
  MINIWEB_ARGS+=(-logfile="${MINIWEB_LOGFILE}")
fi

if [[ -n "${MINIWEB_NAMESPACE}" ]]; then
  MINIWEB_ARGS+=(-namespace="${MINIWEB_NAMESPACE}")
fi

if [[ -n "${MINIWEB_PASSWORDS}" ]]; then
  MINIWEB_ARGS+=(-passwords="${MINIWEB_PASSWORDS}")
fi

if [[ -n "${MINIWEB_CERT}" ]]; then
  MINIWEB_ARGS+=(-cert="${MINIWEB_CERT}")
fi

if [[ -n "${MINIWEB_KEY}" ]]; then
  MINIWEB_ARGS+=(-key="${MINIWEB_KEY}")
fi

# The web console gives anyone who can reach miniweb a full minimega command
# line, so it stays off unless a path is provided. /console-attach.sh is the
# wrapper shipped in this image.
if [[ -n "${MINIWEB_CONSOLE}" ]]; then
  MINIWEB_ARGS+=(-console="${MINIWEB_CONSOLE}")
fi

echo "[$(date --rfc-3339=seconds)] starting miniweb on ${MINIWEB_HOST}:${MINIWEB_PORT}..."

# Replace this script with miniweb so that it runs as PID 1 and receives
# signals from Docker directly.
exec /opt/minimega/bin/miniweb "${MINIWEB_ARGS[@]}" ${MINIWEB_APPEND}
