#!/bin/bash

set -o pipefail

# Check if there are values in /etc/default/minimega
#   The order of precedence is:
#     1. Existing environment variables
#     2. Variables in /etc/default/minimega
#     3. A set of defaults in this script
if [[ -f "/etc/default/minimega" ]]; then
  # Check if any variables are already set
  while IFS='=' read -r key value; do
    # Skip empty lines and comments
    if [[ -n "$key" && -n "$value" && "$key" != \#* ]]; then
      # Remove surrounding quotes
      value="${value%\"}"
      value="${value#\"}"

      # Only set the variable if it is not already set
      if [[ -z "${!key}" ]]; then
        export "${key}=${value}"
      fi
    fi
  done < <(grep -v '^#' "/etc/default/minimega")
fi

# Final default assignment (if these are not set already)
: "${MINIWEB_ROOT:=/opt/minimega/web}"
: "${MINIWEB_HOST:=0.0.0.0}"
: "${MINIWEB_PORT:=9001}"

: "${MM_BASE:=/tmp/minimega}"
: "${MM_FILEPATH:=/tmp/minimega/files}"
: "${MM_BROADCAST:=255.255.255.255}"
: "${MM_VLANRANGE:=101-4096}"
: "${MM_PORT:=9000}"
: "${MM_DEGREE:=1}"
: "${MM_CONTEXT:=minimega}"
: "${MM_LOGLEVEL:=info}"
: "${MM_LOGFILE:=/var/log/minimega.log}"
: "${MM_FORCE:=true}"
: "${MM_RECOVER:=false}"
: "${MM_CGROUP:=/sys/fs/cgroup}"
: "${MM_ABSSNAPSHOT:=false}"
: "${MM_APPEND:=}"

: "${OVS_APPEND:=}"
: "${OVS_HOST_IFACE:=}"

MM_SOCKET="${MM_BASE}/minimega"
MM_PIDFILE="${MM_BASE}/minimega.pid"
MINIWEB_PID=""
MINIMEGA_PID=""

# Remove stale minimega socket/PID state left behind by crashes or container stops.
cleanup_stale_minimega_state() {
  if [[ -f "${MM_PIDFILE}" ]]; then
    local pid exe

    pid="$(<"${MM_PIDFILE}")"
    exe="$(readlink "/proc/${pid}/exe" 2>/dev/null || true)"
    # If the PID is missing or no longer belongs to minimega, discard stale state.
    if [[ ! "${pid}" =~ ^[0-9]+$ || "${exe}" != "/opt/minimega/bin/minimega" ]]; then
      rm -f "${MM_PIDFILE}" "${MM_SOCKET}"
    fi
  elif [[ -S "${MM_SOCKET}" ]]; then
    # A socket without a valid pidfile means the old daemon is gone but leftovers remain.
    rm -f "${MM_SOCKET}"
  fi
}

# Stop the background processes we started and clean any stale state before exit.
shutdown() {
  if [[ -n "${MINIMEGA_PID}" ]] && kill -0 "${MINIMEGA_PID}" 2>/dev/null; then
    kill "${MINIMEGA_PID}"
    wait "${MINIMEGA_PID}"
  fi

  if [[ -n "${MINIWEB_PID}" ]] && kill -0 "${MINIWEB_PID}" 2>/dev/null; then
    kill "${MINIWEB_PID}"
    wait "${MINIWEB_PID}"
  fi

  cleanup_stale_minimega_state
}

trap shutdown EXIT
trap 'exit 143' TERM INT

cleanup_stale_minimega_state

# Start Open vSwitch
/usr/share/openvswitch/scripts/ovs-ctl start ${OVS_APPEND} |& tee -a ${MM_LOGFILE}
if [ ${PIPESTATUS[0]} -ne 0 ]; then
  echo "failed to start Open vSwitch" | tee -a ${MM_LOGFILE}
  exit 1
fi

# Ensure Open vSwitch is available
TIMEOUT=30
INTERVAL=1
START=$(date +%s)

echo "waiting for Open vSwitch to become available (timeout: ${TIMEOUT}s)..." | tee -a ${MM_LOGFILE}
while true; do
  current=$(date +%s)
  elapsed=$((current - START))

  if [[ "$elapsed" -ge "$TIMEOUT" ]]; then
    echo "failed to connect to Open vSwitch" | tee -a ${MM_LOGFILE}
    exit 1
  fi

  if ovs-vsctl show &>/dev/null; then
    break
  else
    sleep $INTERVAL
  fi
done

# Check if there are bridge:port values to add
if [[ -v "OVS_HOST_IFACE" ]]; then
  iface=(${OVS_HOST_IFACE//:/ })
  bridge=${iface[0]}

  if [[ -n "${bridge}" ]]; then
    echo -e "\tadding '${bridge}' bridge..." | tee -a ${MM_LOGFILE}
    /usr/bin/ovs-vsctl --may-exist add-br ${bridge}
    ip link set dev ${bridge} up
  fi

  if [[ -n "${iface[1]}" ]]; then
    ports=(${iface[1]//,/ })

    for port in "${ports[@]}"; do
      echo -e "\tadding '${port}' port to '${bridge}' bridge..." | tee -a ${MM_LOGFILE}
      /usr/bin/ovs-vsctl --may-exist add-port ${bridge} ${port}
    done
  fi
fi

echo "starting miniweb..." | tee -a ${MM_LOGFILE}
/opt/minimega/bin/miniweb -root=${MINIWEB_ROOT} -addr=${MINIWEB_HOST}:${MINIWEB_PORT} &
MINIWEB_PID=$!
echo "miniweb started on ${MINIWEB_HOST}:${MINIWEB_PORT}" | tee -a ${MM_LOGFILE}

echo "starting minimega..." | tee -a ${MM_LOGFILE}
/opt/minimega/bin/minimega \
  -nostdin \
  -force=${MM_FORCE} \
  -recover=${MM_RECOVER} \
  -base=${MM_BASE} \
  -filepath=${MM_FILEPATH} \
  -broadcast=${MM_BROADCAST} \
  -vlanrange=${MM_VLANRANGE} \
  -port=${MM_PORT} \
  -degree=${MM_DEGREE} \
  -context=${MM_CONTEXT} \
  -level=${MM_LOGLEVEL} \
  -logfile=${MM_LOGFILE} \
  -cgroup=${MM_CGROUP} \
  -abssnapshot=${MM_ABSSNAPSHOT} \
  ${MM_APPEND} &
MINIMEGA_PID=$!
wait "${MINIMEGA_PID}"
