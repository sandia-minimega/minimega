#!/bin/bash
#
# Build the minimal Firecracker guest kernel.
#
# Usage: build-kernel.sh <kernel-src> <out-dir> --fc-config <path>
#
# <kernel-src> is a Linux 6.18 source tree. --fc-config is Firecracker's
# resources/guest_configs/microvm-kernel-ci-x86_64-6.18.config (or set
# FC_CONFIG). The script copies that config into the tree, merges
# kernel/minimal-6.18.fragment onto it, runs "make olddefconfig", and builds
# vmlinux. It stops before the build when olddefconfig dropped a fragment
# option. It writes <out-dir>/vmlinux and <out-dir>/config.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRAGMENT="${SCRIPT_DIR}/kernel/minimal-6.18.fragment"

usage() {
    echo "usage: $0 <kernel-src> <out-dir> --fc-config <path>" >&2
    exit 2
}

[[ $# -ge 2 ]] || usage
src="$1"
out="$2"
shift 2

fc_config="${FC_CONFIG:-}"
while [[ $# -gt 0 ]]; do
    case "$1" in
    --fc-config)
        [[ $# -ge 2 ]] || usage
        fc_config="$2"
        shift 2
        ;;
    *)
        usage
        ;;
    esac
done

if [[ -z "${fc_config}" ]]; then
    echo "error: give the Firecracker guest config with --fc-config or FC_CONFIG" >&2
    usage
fi
for f in "${fc_config}" "${FRAGMENT}" "${src}/Makefile" "${src}/scripts/kconfig/merge_config.sh"; do
    if [[ ! -f "${f}" ]]; then
        echo "error: ${f} not found" >&2
        exit 1
    fi
done

mkdir -p "${out}"
njobs="${JOBS:-$(nproc 2>/dev/null || echo 4)}"

cp "${fc_config}" "${src}/.config"
(cd "${src}" && ./scripts/kconfig/merge_config.sh -m .config "${FRAGMENT}")
make -C "${src}" olddefconfig

# Compare each fragment option with the final .config.
dropped=0
while IFS= read -r line; do
    if [[ "${line}" =~ ^(CONFIG_[A-Za-z0-9_]+)=(.*)$ ]]; then
        opt="${BASH_REMATCH[1]}"
        want="${BASH_REMATCH[2]}"
    elif [[ "${line}" =~ ^#\ (CONFIG_[A-Za-z0-9_]+)\ is\ not\ set$ ]]; then
        opt="${BASH_REMATCH[1]}"
        want="n"
    else
        continue
    fi

    got="$(grep -E "^${opt}=" "${src}/.config" | head -n 1 | cut -d= -f2- || true)"
    [[ -n "${got}" ]] || got="n"

    if [[ "${got}" != "${want}" ]]; then
        echo "dropped: ${opt} wanted ${want}, olddefconfig set ${got}" >&2
        dropped=1
    fi
done < "${FRAGMENT}"

if [[ "${dropped}" -ne 0 ]]; then
    echo "error: olddefconfig dropped fragment options; fix their dependencies first" >&2
    exit 1
fi

make -C "${src}" -j"${njobs}" vmlinux

cp "${src}/vmlinux" "${out}/vmlinux"
cp "${src}/.config" "${out}/config"
echo "wrote ${out}/vmlinux ($(wc -c < "${out}/vmlinux") bytes) and ${out}/config"
