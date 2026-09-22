#!/bin/bash

set -o pipefail

# Check if there are values in /etc/default/minimega
source /load-defaults.sh

# Final default assignment (if these are not set already)
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

echo "[$(date --rfc-3339=seconds)] starting minimega..." | tee -a ${MM_LOGFILE}

# Replace this script with minimega so that it runs as PID 1 and receives
# signals from Docker directly. minimega handles SIGTERM itself: it tears down
# namespaces and bridges, then removes its socket and PID file. Any state left
# behind by a crash or a SIGKILL is cleaned up by the startup check above.
exec /opt/minimega/bin/minimega \
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
  ${MM_APPEND}
