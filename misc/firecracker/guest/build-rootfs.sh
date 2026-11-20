#!/bin/bash
#
# Build the read-only root image of the minimal Firecracker guest.
#
# Usage: build-rootfs.sh <out-dir> [--miniccc <path>] [--busybox <path>]
#
# The root tree has a static BusyBox (from the Alpine busybox-static package)
# with its applet links, /init, a minimal /etc, the udhcpc event script, and a
# static miniccc in /usr/local/bin. The script writes <out-dir>/rootfs.squashfs
# and <out-dir>/rootfs.ext4 and prints their sizes.
#
# Without --miniccc, the script builds a static miniccc from this repository
# with "go build". The image work runs in a Docker container from
# ALPINE_IMAGE. On Alpine Linux as root, it runs on the host instead.

set -euo pipefail

# alpine:3.22.2, pinned by the digest of its multi-arch index.
ALPINE_IMAGE="${ALPINE_IMAGE:-alpine:3.22.2@sha256:4b7ce07002c69e8f3d704a9c5d6fd3053be500b7f1c69fc0d80990c2ad8dd412}"
SQUASHFS_COMP="${SQUASHFS_COMP:-zstd}"
ALPINE_PACKAGES="bash busybox-static e2fsprogs file squashfs-tools"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

usage() {
    echo "usage: $0 <out-dir> [--miniccc <path>] [--busybox <path>]" >&2
    exit 2
}

# stage builds the images. It runs as root on Alpine. /in holds the inputs and
# /out gets the images.
stage() {
    local in="$1" out="$2" root
    root="$(mktemp -d)"

    local bb="/bin/busybox.static"
    [[ -f "${in}/busybox" ]] && bb="${in}/busybox"

    local f desc
    for f in "${bb}" "${in}/miniccc"; do
        desc="$(file -b "${f}")"
        if [[ "${desc}" != *"statically linked"* && "${desc}" != *"static-pie linked"* ]]; then
            echo "error: ${f} is not a static binary: ${desc}" >&2
            exit 1
        fi
    done

    mkdir -p "${root}"/{bin,sbin,usr/bin,usr/sbin,usr/local/bin,usr/share/udhcpc}
    mkdir -p "${root}"/{dev,etc,proc,root,run,sys,tmp,var}
    install -m 0755 "${bb}" "${root}/bin/busybox"

    local applet
    while IFS= read -r applet; do
        applet="${applet#/}"
        [[ -n "${applet}" && ! -e "${root}/${applet}" ]] || continue
        ln -s /bin/busybox "${root}/${applet}"
    done < <("${root}/bin/busybox" --list-full)

    # The kernel runs /sbin/init when the command line has no init=.
    rm -f "${root}/sbin/init"
    ln -s /init "${root}/sbin/init"

    install -m 0755 "${in}/init" "${root}/init"
    install -m 0755 "${in}/udhcpc.script" "${root}/usr/share/udhcpc/default.script"
    install -m 0755 "${in}/miniccc" "${root}/usr/local/bin/miniccc"

    printf 'root:x:0:0:root:/root:/bin/sh\n' > "${root}/etc/passwd"
    printf 'root:x:0:\ntty:x:5:\n' > "${root}/etc/group"
    printf '127.0.0.1\tlocalhost\n' > "${root}/etc/hosts"
    ln -s /run/resolv.conf "${root}/etc/resolv.conf"
    ln -s /proc/mounts "${root}/etc/mtab"

    chown -R 0:0 "${root}"
    chmod 0755 "${root}"
    chmod 0700 "${root}/root"
    chmod 1777 "${root}/tmp"

    rm -f "${out}/rootfs.squashfs" "${out}/rootfs.ext4"
    mksquashfs "${root}" "${out}/rootfs.squashfs" -comp "${SQUASHFS_COMP}" -noappend -all-root -quiet

    # Size the ext4 image to the tree plus 10%, plus 4 MiB for metadata.
    local kb
    kb="$(du -sk "${root}" | cut -f1)"
    kb=$((kb + kb / 10 + 4096))
    dd if=/dev/zero of="${out}/rootfs.ext4" bs=1024 count=0 seek="${kb}" 2>/dev/null
    mkfs.ext4 -q -F -L rootfs -m 0 -O ^has_journal -d "${root}" "${out}/rootfs.ext4"

    rm -rf "${root}"

    if [[ -n "${HOST_UID:-}" ]]; then
        chown "${HOST_UID}:${HOST_GID:-${HOST_UID}}" "${out}/rootfs.squashfs" "${out}/rootfs.ext4"
    fi

    for f in "${out}/rootfs.squashfs" "${out}/rootfs.ext4"; do
        echo "$(basename "${f}"): $(wc -c < "${f}") bytes"
    done
}

if [[ "${1:-}" == "--stage" ]]; then
    [[ $# -eq 3 ]] || usage
    stage "$2" "$3"
    exit 0
fi

[[ $# -ge 1 ]] || usage
out_dir="$1"
shift

miniccc=""
busybox=""
while [[ $# -gt 0 ]]; do
    case "$1" in
    --miniccc)
        [[ $# -ge 2 ]] || usage
        miniccc="$2"
        shift 2
        ;;
    --busybox)
        [[ $# -ge 2 ]] || usage
        busybox="$2"
        shift 2
        ;;
    *)
        usage
        ;;
    esac
done

mkdir -p "${out_dir}"
out_dir="$(cd "${out_dir}" && pwd)"

in_dir="$(mktemp -d)"
trap 'rm -rf "${in_dir}"' EXIT

cp "${SCRIPT_DIR}/init" "${SCRIPT_DIR}/udhcpc.script" "${in_dir}/"
[[ -z "${busybox}" ]] || cp "${busybox}" "${in_dir}/busybox"

if [[ -n "${miniccc}" ]]; then
    cp "${miniccc}" "${in_dir}/miniccc"
else
    echo "building a static miniccc from ${REPO_DIR}"
    (cd "${REPO_DIR}" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -trimpath -o "${in_dir}/miniccc" ./cmd/miniccc)
fi

if [[ -f /etc/alpine-release && "$(id -u)" -eq 0 ]]; then
    # shellcheck disable=SC2086
    apk add --no-cache ${ALPINE_PACKAGES} >/dev/null
    stage "${in_dir}" "${out_dir}"
    exit 0
fi

chmod -R a+rX "${in_dir}"
docker run --rm \
    -e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" -e SQUASHFS_COMP="${SQUASHFS_COMP}" \
    -v "${in_dir}:/in:ro" \
    -v "${out_dir}:/out" \
    -v "${SCRIPT_DIR}/build-rootfs.sh:/build-rootfs.sh:ro" \
    "${ALPINE_IMAGE}" \
    sh -c "apk add --no-cache ${ALPINE_PACKAGES} >/dev/null && bash /build-rootfs.sh --stage /in /out"
