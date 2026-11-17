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
: "${MINIWEB_ENABLED:=true}"

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
: "${MM_FIRECRACKER_DIR:=/var/lib/minimega/firecracker}"
: "${MM_FIRECRACKER_UID_BASE:=2000000000}"
: "${MM_FIRECRACKER_LAUNCH_PARALLEL:=0}"
: "${MM_FIRECRACKER_START_PARALLEL:=0}"
: "${MM_APPEND:=}"
: "${MM_OOM_SCORE_ADJ:=}"

: "${OVS_APPEND:=}"
: "${OVS_HOST_IFACE:=}"

case "${MINIWEB_ENABLED}" in
  true | false) ;;
  *)
    echo "MINIWEB_ENABLED must be true or false, not '${MINIWEB_ENABLED}'" >&2
    exit 1
    ;;
esac

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

log() {
  echo "$*" | tee -a "${MM_LOGFILE}"
}

# On cgroup v2, a cgroup that holds processes cannot enable controllers for its
# children. Move the container processes into a child cgroup so that the
# parent cgroup of the Firecracker jails can get the cpu, memory, and pids
# controllers. The host root cgroup has no cgroup.type and is not changed.
setup_cgroup() {
  local root=/sys/fs/cgroup pid ctrl

  [[ -f "${root}/cgroup.controllers" && -f "${root}/cgroup.type" ]] || return 0
  [[ -n "$(<"${root}/cgroup.procs")" ]] || return 0

  if ! mkdir -p "${root}/init"; then
    log "unable to create ${root}/init; firecracker VMs may not get cgroup controllers"
    return 0
  fi

  # Processes can start while this runs, so do more than one pass.
  for _ in 1 2 3 4 5; do
    while read -r pid; do
      echo "${pid}" > "${root}/init/cgroup.procs" 2>/dev/null || true
    done < "${root}/cgroup.procs"
    [[ -n "$(<"${root}/cgroup.procs")" ]] || break
  done

  for ctrl in cpu memory pids; do
    if ! grep -qw "${ctrl}" "${root}/cgroup.controllers"; then
      log "cgroup controller ${ctrl} is not available in ${root}"
    elif ! echo "+${ctrl}" > "${root}/cgroup.subtree_control" 2>/dev/null; then
      log "unable to enable the ${ctrl} cgroup controller in ${root}"
    fi
  done
}

# Each Firecracker VM uses several file descriptors in minimega.
raise_nofile() {
  local hard

  hard="$(ulimit -Hn)"
  if ! ulimit -n "${hard}" 2>/dev/null; then
    log "unable to raise the open file limit to ${hard}"
  fi

  if [[ "$(ulimit -n)" != "unlimited" && "$(ulimit -n)" -lt 100000 ]]; then
    log "WARNING: the open file limit is $(ulimit -n); firecracker VMs need more (docker run --ulimit nofile=1048576:1048576)"
  fi
}

# Keep the OOM killer away from the Open vSwitch daemons.
protect_ovs() {
  local f pid

  for f in /var/run/openvswitch/ovsdb-server.pid /var/run/openvswitch/ovs-vswitchd.pid; do
    [[ -f "${f}" ]] || continue
    pid="$(<"${f}")"
    echo -1000 > "/proc/${pid}/oom_score_adj" 2>/dev/null || log "unable to set oom_score_adj of ${f}"
  done
}

trap shutdown EXIT
trap 'exit 143' TERM INT

cleanup_stale_minimega_state
setup_cgroup
raise_nofile

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

protect_ovs

# Check if there are bridge:port values to add
if [[ -v "OVS_HOST_IFACE" ]]; then
  IFS=: read -r -a iface <<< "${OVS_HOST_IFACE}"
  bridge=${iface[0]}

  if [[ -n "${bridge}" ]]; then
    echo -e "\tadding '${bridge}' bridge..." | tee -a ${MM_LOGFILE}
    /usr/bin/ovs-vsctl --may-exist add-br ${bridge}
    ip link set dev ${bridge} up
  fi

  if [[ -n "${iface[1]}" ]]; then
    IFS=, read -r -a ports <<< "${iface[1]}"

    for port in "${ports[@]}"; do
      echo -e "\tadding '${port}' port to '${bridge}' bridge..." | tee -a ${MM_LOGFILE}
      /usr/bin/ovs-vsctl --may-exist add-port ${bridge} ${port}
    done
  fi
fi

if [[ "${MINIWEB_ENABLED}" == "true" ]]; then
  echo "[$(date --rfc-3339=seconds)] starting miniweb..." | tee -a ${MM_LOGFILE}
  /opt/minimega/bin/miniweb -root=${MINIWEB_ROOT} -addr=${MINIWEB_HOST}:${MINIWEB_PORT} &
  MINIWEB_PID=$!
  echo "[$(date --rfc-3339=seconds)] miniweb started on ${MINIWEB_HOST}:${MINIWEB_PORT}" | tee -a ${MM_LOGFILE}
else
  log "[$(date --rfc-3339=seconds)] miniweb is disabled (MINIWEB_ENABLED=false)"
fi

echo "[$(date --rfc-3339=seconds)] starting minimega..." | tee -a ${MM_LOGFILE}
# The VMs that minimega starts inherit its oom_score_adj, so it is opt-in.
if [[ -n "${MM_OOM_SCORE_ADJ}" ]]; then
  echo "${MM_OOM_SCORE_ADJ}" > /proc/self/oom_score_adj 2>/dev/null \
    || log "unable to set the oom_score_adj of minimega to ${MM_OOM_SCORE_ADJ}"
fi
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
  -firecracker-dir=${MM_FIRECRACKER_DIR} \
  -firecracker-uid-base=${MM_FIRECRACKER_UID_BASE} \
  -firecracker-launch-parallel=${MM_FIRECRACKER_LAUNCH_PARALLEL} \
  -firecracker-start-parallel=${MM_FIRECRACKER_START_PARALLEL} \
  ${MM_APPEND} &
MINIMEGA_PID=$!
echo "[$(date --rfc-3339=seconds)] minimega started with PID ${MINIMEGA_PID}" | tee -a ${MM_LOGFILE}
wait "${MINIMEGA_PID}"
