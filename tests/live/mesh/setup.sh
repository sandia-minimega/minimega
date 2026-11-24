# Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
# Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
# rights in this software.
#
# shellcheck shell=bash
#
# Sourced by run.sh for the mesh group: start a second minimega container from
# the same image, join it to the mesh, and stage a container filesystem tarball
# (the KVM guest's initramfs) on the first node only.

PEER="${CONTAINER}-peer"
PEER_HOST=minimega-peer
# The peer gets its own base directory, mounted at the same path in its
# container. It lives inside MM_BASE, which run.sh has checked and emptied.
PEER_BASE="${MM_BASE}/peer"

if docker inspect "${PEER}" >/dev/null 2>&1; then
    [[ "${LIVE_FORCE:-0}" == "1" ]] || die "container ${PEER} exists; set LIVE_FORCE=1 to replace it"
    docker rm -f "${PEER}" >/dev/null 2>&1
fi
mkdir -p "${PEER_BASE}" "${ARTIFACTS}/logs/peer" || die "unable to create ${PEER_BASE}"

group_cleanup() {
    docker logs "${PEER}" > "${ARTIFACTS}/logs/peer/container.log" 2>&1 || true
    docker rm -f "${PEER}" >/dev/null 2>&1 || true
}

# Same flags as the first node, without the published ports. A private cgroup
# namespace keeps the two daemons' container cgroups apart on cgroup v1 hosts.
log "starting ${PEER}"
docker run -d \
    --name "${PEER}" \
    --hostname "${PEER_HOST}" \
    --privileged \
    --cap-add ALL \
    --cgroupns private \
    -v /dev:/dev \
    -v /lib/modules:/lib/modules:ro \
    -v "${ARTIFACTS}/logs/peer:/var/log/minimega" \
    -v "${PEER_BASE}:${MM_BASE}" \
    -e MM_BASE="${MM_BASE}" \
    -e MM_FILEPATH="${MM_FILES}" \
    -e MM_LOGFILE=/var/log/minimega/minimega.log \
    -e MM_LOGLEVEL="${LIVE_LOGLEVEL:-info}" \
    "${IMAGE}" >/dev/null || die "unable to start ${PEER}"

for _ in $(seq 1 60); do
    docker exec "${PEER}" minimega -e version >/dev/null 2>&1 && break
    sleep 2
done
docker exec "${PEER}" minimega -e version || die "minimega did not start in ${PEER}"

peer_ip="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "${PEER}")"
[[ -n "${peer_ip}" ]] || die "no IP address for ${PEER}"
# the nodes may already have found each other by broadcast (MM_DEGREE=1)
log "joining ${PEER_HOST} (${peer_ip}) to the mesh"
mm mesh list 2>/dev/null | grep -q "${PEER_HOST}" || mm mesh dial "${peer_ip}"
for _ in $(seq 1 30); do
    mm mesh list 2>/dev/null | grep -q "${PEER_HOST}" && break
    sleep 1
done
mm mesh list | grep -q "${PEER_HOST}" || die "${PEER_HOST} did not join the mesh"
mm mesh status || die "mesh status failed"

# The container filesystem is the KVM guest's initramfs, packed as a tarball
# with a single top-level directory as the tar: prefix requires.
log "staging the container filesystem tarball"
fs_dir="$(mktemp -d)" || die "unable to create a temporary directory"
mkdir "${fs_dir}/containerfs" || die "unable to create ${fs_dir}/containerfs"
(cd "${fs_dir}/containerfs" && gzip -dc "${STAGE}/live.initrd" | cpio -idm --quiet) || die "unable to unpack live.initrd"
tar -C "${fs_dir}" -czf "${STAGE}/containerfs.tar.gz" containerfs || die "unable to pack containerfs.tar.gz"
rm -rf "${fs_dir}"
du -h "${STAGE}/containerfs.tar.gz"

export LIVE_PEER_HOST="${PEER_HOST}" LIVE_PEER_FILES="${PEER_BASE}/files"
