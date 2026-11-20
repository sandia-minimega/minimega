#!/usr/bin/env bash
# Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
# Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
# rights in this software.
#
# Check that the containers of minimega stop with minimega on a cgroup v2 host:
# when minimega stops, and when it is killed. minimega runs from a Docker image,
# with a private cgroup namespace or with --cgroupns host, or as the
# minimega.service of an installed deb or rpm package.
#
#   sudo tests/live/stop.sh docker [image]
#   sudo tests/live/stop.sh docker-host-cgroupns [image]
#   sudo tests/live/stop.sh systemd
#
# Each mode launches three containers, pauses one of them, and checks that
# their cgroups are below the cgroup of the Docker container or of the service.
# It then stops minimega (docker stop, systemctl stop) and checks that no
# process of a container is left and that the cgroup is gone, and does the same
# once more with SIGKILL to minimega. The containers run sleep from a small
# filesystem copied from the host.
#
# Requirements: Linux with cgroup v2, root, and Docker (docker modes, which also
# need the openvswitch kernel module), or systemd and the installed package
# (systemd mode). The systemd mode starts and stops minimega.service with a
# drop-in that sets its base directory, and leaves it stopped. Do not start
# containers with another minimega while the check runs: the cleanup kills
# every container of minimega on the host.
#
# Environment:
#   LIVE_IMAGE        image for the docker modes (default: the argument after
#                     the mode, then minimega:dev)
#   LIVE_ARTIFACTS    output directory (default: ./live-artifacts); logs go to
#                     stop-<mode> in it

set -uo pipefail

MODE="${1:-}"
IMAGE="${2:-${LIVE_IMAGE:-minimega:dev}}"
CONTAINER=minimega-stop
UNIT=minimega.service
MM_BASE=/tmp/minimega-stop
ROOTFS="${MM_BASE}/files/rootfs"
CGROUP=/sys/fs/cgroup

log() { echo "==> [$(date +%H:%M:%S)] $*"; }
die() { echo "ERROR: $*" >&2; exit 1; }

failed=0
# check <description> <command...> runs a check and records a failure
check() {
    if "${@:2}"; then
        echo "ok: $1"
    else
        echo "FAIL: $1"
        failed=1
    fi
}

case "${MODE}" in
docker | docker-host-cgroupns | systemd) ;;
*) die "usage: $0 docker|docker-host-cgroupns|systemd [image]" ;;
esac

[[ "$(id -u)" == "0" ]] || die "run as root"
[[ "$(stat -f -c %T "${CGROUP}")" == "cgroup2fs" ]] || die "${CGROUP} is not cgroup v2"
if [[ "${MODE}" == "systemd" ]]; then
    [[ -d /run/systemd/system ]] || die "systemd is not running"
    systemctl cat "${UNIT}" >/dev/null || die "${UNIT} is not installed"
    systemctl is-active -q "${UNIT}" && die "${UNIT} is running; stop it first"
else
    command -v docker >/dev/null || die "docker not found"
    docker inspect "${CONTAINER}" >/dev/null 2>&1 && die "container ${CONTAINER} exists"
fi
[[ -e "${MM_BASE}" ]] && die "${MM_BASE} exists"

ARTIFACTS="$(mkdir -p "${LIVE_ARTIFACTS:-${PWD}/live-artifacts}" && cd "${LIVE_ARTIFACTS:-${PWD}/live-artifacts}" && pwd)/stop-${MODE}"
rm -rf "${ARTIFACTS}"
mkdir -p "${ARTIFACTS}"

if [[ "${MODE}" == "systemd" ]]; then
    mm() { minimega -base="${MM_BASE}" -e "$@"; }
else
    mm() { docker exec -i "${CONTAINER}" minimega -base="${MM_BASE}" -e "$@"; }
fi

# --- setup and teardown --------------------------------------------------------

DROPIN="/run/systemd/system/${UNIT}.d/50-stop-test.conf"
ENVFILE=/run/minimega-stop-test.env
# ExecStartPre of the unit creates it; remove it again if this run did
SAVED=/etc/minimega/saved_vms
[[ -e "${SAVED}" ]] && SAVED=""

# container_cgroups prints the cgroup of every container of minimega on the host
container_cgroups() {
    find "${CGROUP}" -path '*/minimega/[0-9]*' -name cgroup.procs -printf '%h\n' 2>/dev/null | sort
}

# reap kills the containers and removes the cgroups and overlays that minimega
# left, so that the next run starts clean
reap() {
    local d
    for d in $(container_cgroups); do
        echo 0 > "${d}/cgroup.freeze"
        xargs -r kill -KILL < "${d}/cgroup.procs"
    done
    find "${CGROUP}" -depth -type d \( -path '*/minimega/*' -o -name minimega \) \
        -exec rmdir {} + 2>/dev/null
    grep "${MM_BASE}" /proc/mounts | awk '{ print $2 }' | sort -r | xargs -r umount -l
    # the network namespace links of the killed containers
    find /var/run/netns -maxdepth 1 -name 'meganet_*' -xtype l -delete 2>/dev/null
}

cleanup() {
    if [[ "${MODE}" == "systemd" ]]; then
        systemctl stop "${UNIT}" 2>/dev/null
        systemctl reset-failed "${UNIT}" 2>/dev/null
        rm -f "${DROPIN}" "${ENVFILE}"
        rmdir "$(dirname "${DROPIN}")" 2>/dev/null
        [[ -n "${SAVED}" ]] && rmdir "${SAVED}" 2>/dev/null
        systemctl daemon-reload
    else
        docker rm -f "${CONTAINER}" >/dev/null 2>&1
    fi
    reap
    rm -rf "${MM_BASE}"
}

# Only containers of this run may be in cgroups of minimega, because reap kills
# whatever is in them.
[[ -z "$(container_cgroups)" ]] || die "containers of another minimega are running"
trap cleanup EXIT

# The containers run sleep from the host: copy it, a shell, and their libraries.
mkdir -p "${ROOTFS}"/{bin,dev,etc,proc,sys,tmp}
for bin in sh sleep; do
    bin="$(command -v "${bin}")"
    cp -L "${bin}" "${ROOTFS}/bin/"
    for lib in $(ldd "${bin}" | grep -o '/[^ ]*'); do
        mkdir -p "${ROOTFS}$(dirname "${lib}")"
        cp -L "${lib}" "${ROOTFS}${lib}"
    done
done
printf '#!/bin/sh\nexec sleep 100000\n' > "${ROOTFS}/init"
chmod +x "${ROOTFS}/init"

if [[ "${MODE}" == "systemd" ]]; then
    # a second environment file overrides the settings of the first
    printf 'MM_BASE=%s\nMM_FILEPATH=%s\nMM_LOGFILE=%s\nMM_LOGLEVEL=debug\n' \
        "${MM_BASE}" "${MM_BASE}/files" "${ARTIFACTS}/minimega.log" > "${ENVFILE}"
    mkdir -p "$(dirname "${DROPIN}")"
    printf '[Service]\nEnvironmentFile=%s\n' "${ENVFILE}" > "${DROPIN}"
    systemctl daemon-reload
    systemctl show -p Delegate -p KillMode "${UNIT}"
else
    log "Docker cgroup driver: $(docker info -f '{{.CgroupDriver}}')"
fi

# start_minimega starts minimega and waits for its command socket
start_minimega() {
    if [[ "${MODE}" == "systemd" ]]; then
        systemctl start "${UNIT}" || return 1
    else
        local cgroupns=private
        [[ "${MODE}" == "docker-host-cgroupns" ]] && cgroupns=host
        # the flags of docker/README.md, without the ports
        docker run -d \
            --name "${CONTAINER}" \
            --hostname minimega \
            --privileged \
            --cap-add ALL \
            --cgroupns "${cgroupns}" \
            -v /dev:/dev \
            -v /lib/modules:/lib/modules:ro \
            -v "${ARTIFACTS}:/var/log/minimega" \
            -v "${MM_BASE}:${MM_BASE}" \
            -e MM_BASE="${MM_BASE}" \
            -e MM_FILEPATH="${MM_BASE}/files" \
            -e MM_LOGFILE=/var/log/minimega/minimega.log \
            -e MM_LOGLEVEL=debug \
            "${IMAGE}" >/dev/null || return 1
    fi

    for _ in $(seq 1 60); do
        mm version >/dev/null 2>&1 && return 0
        sleep 1
    done
    mm version
}

# daemon_pid prints the host PID of minimega, or of the entrypoint of its Docker container
daemon_pid() {
    if [[ "${MODE}" == "systemd" ]]; then
        systemctl show -p MainPID --value "${UNIT}"
    else
        docker inspect -f '{{.State.Pid}}' "${CONTAINER}"
    fi
}

# kill_minimega stops minimega, or kills it with SIGKILL
kill_minimega() {
    case "${MODE}:$1" in
    systemd:stop) systemctl stop "${UNIT}" ;;
    systemd:kill) kill -KILL "$(daemon_pid)" ;;
    *:stop) docker stop -t 30 "${CONTAINER}" >/dev/null ;;
    *:kill) docker exec "${CONTAINER}" sh -c "kill -KILL \$(cat ${MM_BASE}/minimega.pid)" ;;
    esac
}

# gone <path...> waits up to 30 seconds until no path exists
gone() {
    local p left
    for _ in $(seq 1 30); do
        left=""
        for p in "$@"; do
            [[ -e "${p}" ]] && left+="${p} "
        done
        [[ -z "${left}" ]] && return 0
        sleep 1
    done
    echo "still there: ${left}"
    return 1
}

# --- run ---------------------------------------------------------------------

# run <stop|kill> launches containers, stops minimega with it, and checks
run() {
    local how=$1 top pid own dirs pids d

    log "${MODE}: starting minimega"
    start_minimega || die "minimega did not start"

    log "${MODE}: launching containers"
    mm vm config filesystem "${ROOTFS}" &&
        mm vm launch container stop[0-2] &&
        mm vm start all &&
        mm vm stop stop2 || die "unable to launch the containers"
    mm .columns id,name,state vm info | tee "${ARTIFACTS}/vm-info-${how}.txt"

    # the cgroup of the service or the Docker container
    if [[ "${MODE}" == "systemd" ]]; then
        top="${CGROUP}$(systemctl show -p ControlGroup --value "${UNIT}")"
    else
        top="${CGROUP}$(sed -n 's/^0:://p' "/proc/$(daemon_pid)/cgroup")"
        top="${top%/init}"
    fi
    # minimega (at its first container) or the entrypoint moved to a child cgroup init
    pid="$(daemon_pid)"
    own="${CGROUP}$(sed -n 's/^0:://p' "/proc/${pid}/cgroup")"
    check "${pid} is in ${top}/init" test "${own}" == "${top}/init"

    # the containers are in cgroups of their own below it
    dirs="$(container_cgroups)"
    check "three container cgroups" test "$(grep -c . <<< "${dirs}")" == 3
    check "the container cgroups are in ${top}/minimega" \
        test "$(grep -c "^${top}/minimega/[0-9]*$" <<< "${dirs}")" == 3
    pids="$(for d in ${dirs}; do cat "${d}/cgroup.procs"; done)"
    check "every container has a process" test "$(wc -w <<< "${pids}")" == 3
    check "one container is frozen" test "$(for d in ${dirs}; do grep -l '^frozen 1' "${d}/cgroup.events"; done | wc -l)" == 1
    find "${top}" -name cgroup.procs -exec grep -H . {} + > "${ARTIFACTS}/cgroups-${how}.txt"

    log "${MODE}: minimega ${how}"
    kill_minimega "${how}"

    # shellcheck disable=SC2046
    check "no process of a container is left after ${how}" gone $(printf '/proc/%s ' ${pids})
    check "the cgroup ${top} is gone after ${how}" gone "${top}"

    if [[ "${MODE}" == "systemd" ]]; then
        systemctl status --no-pager "${UNIT}" > "${ARTIFACTS}/status-${how}.txt" 2>&1
        systemctl reset-failed "${UNIT}" 2>/dev/null
    else
        docker logs "${CONTAINER}" > "${ARTIFACTS}/container-${how}.log" 2>&1
        docker rm -f "${CONTAINER}" >/dev/null
    fi
    # minimega does not clean up after SIGKILL
    reap
    find "${MM_BASE}" -mindepth 1 -maxdepth 1 ! -name files -exec rm -rf {} +
}

run stop
run kill

if [[ ${failed} -eq 0 ]]; then
    log "PASS"
else
    log "FAIL (see ${ARTIFACTS})"
fi
exit "${failed}"
