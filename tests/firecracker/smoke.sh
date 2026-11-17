#!/usr/bin/env bash
#
# Functional checks for Firecracker VMs against a running minimega.
#
# Usage: sudo -E smoke.sh <workdir>
#
# <workdir> holds the files from prepare-guest.sh: vmlinux, rootfs.qcow2, and
# rootfs-part.raw. <workdir>/minimal holds rootfs.squashfs from
# misc/firecracker/guest/build-rootfs.sh.
#
# Environment:
#   FS       xfs or ext4: the filesystem of FC_DIR (required)
#   MM       minimega binary (default: bin/minimega)
#   MM_BASE  minimega -base (default: /tmp/mm)
#   MM_LOG   minimega -logfile (default: /tmp/mm.log)
#   MM_FILES minimega -filepath (default: $MM_BASE/files)
#   FC_DIR   minimega -firecracker-dir (default: /var/lib/mmfc)
#   MINIMAL  minimal guest root image (default: <workdir>/minimal/rootfs.squashfs)
#   RATE_VMS number of VMs in the launch rate check, check 15 (default: 100)
#
# fc1, fc2, and fc3 use vm config backchannel true. The guest root filesystem
# starts miniccc over vsock (prepare-guest.sh), and the cc checks use it.
#
# Guest addresses: the guest root filesystem runs fcnet-setup.sh from the
# Firecracker CI images. It reads a MAC of the form 06:00:AA:BB:CC:DD, sets
# the address AA.BB.CC.DD/30 on that NIC, and adds no routes. So each guest
# gets its own /30, and the host tap holds the other address of each /30:
#
#   VM    MAC                guest        host tap
#   fc1   06:00:ac:10:00:02  172.16.0.2   172.16.0.1/30
#   fc2   06:00:ac:10:00:06  172.16.0.6   172.16.0.5/30
#   fc3   06:00:ac:10:00:0a  172.16.0.10  172.16.0.9/30
#   fc4   06:00:ac:10:00:0e  172.16.0.14  172.16.0.13/30
#
# Check 14 boots the minimal guest, which reads its address from the kernel
# command line (mm.ip= and mm.gw=). VM roN gets 172.16.1.(4N-2)/30, and the
# host tap holds 172.16.1.(4N-3)/30.

set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "usage: $0 <workdir>" >&2
    exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root_dir="$(cd "${script_dir}/../.." && pwd)"

WORKDIR="$(cd "$1" && pwd)"
FS="${FS:-}"
MM="${MM:-${root_dir}/bin/minimega}"
MM_BASE="${MM_BASE:-/tmp/mm}"
MM_LOG="${MM_LOG:-/tmp/mm.log}"
MM_FILES="${MM_FILES:-${MM_BASE}/files}"
FC_DIR="${FC_DIR:-/var/lib/mmfc}"

KERNEL="${WORKDIR}/vmlinux"
QCOW2="${WORKDIR}/rootfs.qcow2"
PART="${WORKDIR}/rootfs-part.raw"
MINIMAL="${MINIMAL:-${WORKDIR}/minimal/rootfs.squashfs}"
RATE_VMS="${RATE_VMS:-100}"
if ! [[ "$RATE_VMS" =~ ^[1-9][0-9]*$ ]]; then
    echo "RATE_VMS must be a positive integer, not '${RATE_VMS}'" >&2
    exit 2
fi
RO_VMS=10

VLAN=LAN
BRIDGE=mega_bridge
HOST_TAP=mmfc_host
BEACON="the miniature beacons of gondor have been lit, it's a minifire!"
BANNER="minifire: Firecracker microVMs for minimega"

TMP="$(mktemp -d)"
INJECT_LOOP=""
INJECT_MNT=""

# The file for cc send and cc recv, in the minimega files directory.
CC_SEND_NAME="minifire-cc-$$"
CC_SEND_SRC=""

# The path prefix of the vm save of fc1, in the minimega files directory.
SNAP=""

# The path prefix of the vm save of csrc (check 16), in the files directory.
CSNAP=""

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

step() {
    echo
    echo "==== $*"
}

cleanup() {
    local rc=$?

    if [[ -n "$INJECT_MNT" ]] && mountpoint -q "$INJECT_MNT"; then
        umount "$INJECT_MNT" || true
    fi
    if [[ -n "$INJECT_LOOP" ]]; then
        losetup -d "$INJECT_LOOP" || true
    fi
    if [[ -n "$CC_SEND_SRC" ]]; then
        rm -f "$CC_SEND_SRC" || true
    fi
    if [[ -n "$SNAP" ]]; then
        rm -f "${SNAP}.state" "${SNAP}.mem" "${SNAP}.hdd" "${SNAP}.json" || true
    fi
    if [[ -n "$CSNAP" ]]; then
        rm -f "${CSNAP}.state" "${CSNAP}.mem" "${CSNAP}.json" || true
    fi

    if [[ $rc -ne 0 ]]; then
        echo
        echo "==== last 50 lines of ${MM_LOG}"
        tail -n 50 "$MM_LOG" || true
    fi

    rm -rf "$TMP"
    exit "$rc"
}
trap cleanup EXIT

# mm runs one minimega command. minimega -e exits 0 even when the command
# fails, so mm returns 1 when the error output has an "Error (host)" line.
mm() {
    local rc=0
    local out

    out="$("$MM" -base "$MM_BASE" -e "$*" 2>"${TMP}/mm.err")" || rc=$?
    if [[ $rc -ne 0 ]] || grep -q 'Error (' "${TMP}/mm.err"; then
        [[ -n "$out" ]] && printf '%s\n' "$out" >&2
        cat "${TMP}/mm.err" >&2
        echo "minimega command failed: $*" >&2
        return 1
    fi

    [[ -n "$out" ]] && printf '%s\n' "$out"
    return 0
}

# vm_field prints one vm info column of one VM.
vm_field() {
    mm ".annotate false .headers false .csv true .columns $2 .filter name=$1 vm info" | head -n 1 | tr -d '[:space:]'
}

vm_id() {
    local id
    id="$(vm_field "$1" id)"
    [[ "$id" =~ ^[0-9]+$ ]] || fail "no VM ID for $1: '$id'"
    echo "$id"
}

vm_absent() {
    [[ -z "$(vm_field "$1" name 2>/dev/null)" ]]
}

state_is() {
    [[ "$(vm_field "$1" state 2>/dev/null)" == "$2" ]]
}

# wait_for runs a command once each second until it succeeds or the timeout
# (seconds) passes.
wait_for() {
    local timeout=$1 desc=$2
    shift 2

    local deadline=$((SECONDS + timeout))
    until "$@"; do
        if ((SECONDS >= deadline)); then
            fail "$desc: not true after ${timeout}s"
        fi
        sleep 1
    done
}

wait_state() {
    local name=$1 state=$2 timeout=$3
    if ! wait_for_quiet "$timeout" state_is "$name" "$state"; then
        echo "vm info for ${name}:" >&2
        mm ".filter name=${name} vm info" >&2 || true
        fail "${name} is not ${state} after ${timeout}s (state: $(vm_field "$name" state || true))"
    fi
}

# wait_for_quiet is wait_for, but returns 1 at the timeout.
wait_for_quiet() {
    local timeout=$1
    shift

    local deadline=$((SECONDS + timeout))
    until "$@"; do
        if ((SECONDS >= deadline)); then
            return 1
        fi
        sleep 1
    done
}

ping_ok() {
    ping -c 1 -W 1 "$1" >/dev/null 2>&1
}

ping_fails() {
    ! ping_ok "$1"
}

guest_mac() {
    printf '06:00:ac:10:00:%02x' $((4 * $1 - 2))
}

guest_ip() {
    echo "172.16.0.$((4 * $1 - 2))"
}

host_ip() {
    echo "172.16.0.$((4 * $1 - 3))"
}

# cc_clients prints one "uuid,mac" line for each cc client.
cc_clients() {
    mm ".annotate false .headers false .csv true .columns uuid,mac cc clients" 2>/dev/null
}

cc_has_client() {
    grep -qiF "$1," <<<"$(cc_clients)"
}

cc_no_client() {
    local out
    out="$(cc_clients)" || return 1
    ! grep -qiF "$1," <<<"$out"
}

# cc_client_has_mac returns 0 when the cc client with UUID $1 reports MAC $2.
cc_client_has_mac() {
    local row
    row="$(grep -iF "$1," <<<"$(cc_clients)")" || return 1
    grep -qiF "$2" <<<"$row"
}

# cc_raw prints the raw responses to the cc commands with prefix $1. The host
# annotation is off, because it puts "<host>: " before the first line.
cc_raw() {
    mm ".annotate false cc responses $1 raw" 2>/dev/null
}

# cc_response_has returns 0 when the responses to the cc commands with prefix
# $1 contain the text $2.
cc_response_has() {
    grep -qF -- "$2" <<<"$(cc_raw "$1")"
}

# cc_response_line returns 0 when the responses to the cc commands with
# prefix $1 have a line that is exactly $2.
cc_response_line() {
    grep -qxF -- "$2" <<<"$(cc_raw "$1")"
}

# cc_dump prints the raw responses and the cc commands rows of prefix $1 to
# stderr.
cc_dump() {
    {
        echo "---- cc responses $1 raw"
        mm ".annotate false cc responses $1 raw" || true
        echo "---- cc commands with prefix $1"
        mm ".annotate false .filter prefix=$1 cc commands" || true
        echo "----"
    } >&2
}

# wait_cc is wait_for for a cc response check: wait_cc <timeout> <desc>
# <check> <prefix> <text>. At the timeout, it prints the responses and the
# commands of the prefix, and fails the test.
wait_cc() {
    local timeout=$1 desc=$2 check=$3 prefix=$4 text=$5

    if ! wait_for_quiet "$timeout" "$check" "$prefix" "$text"; then
        cc_dump "$prefix"
        fail "$desc: not true after ${timeout}s"
    fi
}

# cc_recv_path prints the path of the file that cc recv got from the VM with
# UUID $1.
cc_recv_path() {
    find "${MM_FILES}/miniccc_responses" -type f -path "*/$1/tmp/miniccc/files/${CC_SEND_NAME}" 2>/dev/null | head -n 1 || true
}

cc_recv_matches() {
    local p
    p="$(cc_recv_path "$1")"
    [[ -n "$p" ]] && cmp -s "$CC_SEND_SRC" "$p"
}

ro_mac() {
    printf '06:00:ac:10:01:%02x' $((4 * $1 - 2))
}

ro_ip() {
    echo "172.16.1.$((4 * $1 - 2))"
}

ro_host_ip() {
    echo "172.16.1.$((4 * $1 - 3))"
}

ro_running() {
    local n
    n="$(mm ".annotate false .headers false .csv true .columns name,state vm info" | grep -c '^ro[0-9]*,RUNNING' || true)"
    [[ "$n" -eq "$RO_VMS" ]]
}

# jail_root prints the jail root directory of VM id.
jail_root() {
    find "${FC_DIR}/jail" -mindepth 3 -maxdepth 3 -type d -path "*/vm$1/root" | head -n 1
}

# instance_disk prints the host path of disk 0 of VM id: a file on xfs, a
# device-mapper snapshot on ext4.
instance_disk() {
    local id=$1
    if [[ "$FS" == xfs ]]; then
        echo "${FC_DIR}/vm/${id}/disk-0.raw"
        return
    fi

    local name
    name="$(dmsetup ls | awk -v re="^mm-fc-[0-9a-f]+-${id}-0\$" '$1 ~ re {print $1}')"
    [[ -n "$name" ]] || fail "no device-mapper target for VM ${id}"
    echo "/dev/mapper/${name}"
}

base_config() {
    mm "clear vm config"
    mm "vm config vcpus 1"
    mm "vm config memory 256"
    mm "vm config kernel ${KERNEL}"
    mm "vm config append root=/dev/vda"
    mm "vm config disk ${QCOW2}"
    mm "vm config snapshot true"
    mm "vm config backchannel false"
}

# leftovers prints one line for each host object that a flushed Firecracker
# VM left behind.
leftovers() {
    local ns dm port
    ns="$(ip netns list | awk '{print $1}' | grep '^mm-fc-' || true)"
    [[ -z "$ns" ]] || echo "netns: ${ns//$'\n'/ }"

    dm="$(dmsetup ls 2>/dev/null | awk '{print $1}' | grep '^mm-fc-' || true)"
    [[ -z "$dm" ]] || echo "device-mapper targets: ${dm//$'\n'/ }"

    if [[ -d "${FC_DIR}/vm" ]] && [[ -n "$(find "${FC_DIR}/vm" -mindepth 1 -maxdepth 1)" ]]; then
        echo "instance directories: $(find "${FC_DIR}/vm" -mindepth 1 -maxdepth 1 | tr '\n' ' ')"
    fi

    if [[ -d "${FC_DIR}/jail" ]] && [[ -n "$(find "${FC_DIR}/jail" -mindepth 2 -maxdepth 2)" ]]; then
        echo "jails: $(find "${FC_DIR}/jail" -mindepth 2 -maxdepth 2 | tr '\n' ' ')"
    fi

    local loops
    loops="$(losetup -a | grep -F "${FC_DIR}/" || true)"
    [[ -z "$loops" ]] || echo "loop devices: ${loops//$'\n'/ }"

    port="$(ovs-vsctl list-ports "$BRIDGE" 2>/dev/null | grep -vx "$HOST_TAP" || true)"
    [[ -z "$port" ]] || echo "OVS ports on ${BRIDGE}: ${port//$'\n'/ }"
}

no_leftovers() {
    [[ -z "$(leftovers)" ]]
}

check_clean() {
    if ! wait_for_quiet 30 no_leftovers; then
        leftovers >&2
        fail "flushed VMs left host objects behind"
    fi
    echo "no netns, device-mapper targets, instance directories, jails, loop devices, or OVS ports are left"
}

all_quit() {
    local s
    s="$(mm ".annotate false .headers false .csv true .columns state vm info" | tr -d '[:blank:]' | grep -v '^$' | grep -vx QUIT || true)"
    [[ -z "$s" ]]
}

no_vms() {
    [[ -z "$(mm ".annotate false .headers false .csv true .columns name vm info" | tr -d '[:space:]')" ]]
}

kill_flush_all() {
    mm "vm kill all"
    wait_for 60 "all VMs are QUIT" all_quit
    mm "vm flush"
    wait_for 30 "vm info has no VMs" no_vms
}

# job_status prints the status of VM $2 in the progress table of command $1,
# "vm save" or "vm memory-dump" with no arguments.
job_status() {
    mm ".annotate false .headers false .csv true .columns name,status $1" | awk -F, -v n="$2" '$1 == n {print $2}' | tr -d '[:space:]'
}

# job_completed returns 0 when the job of VM $2 in command $1 is completed, and
# fails the test when it failed.
job_completed() {
    local s
    s="$(job_status "$1" "$2")"
    if [[ "$s" == failed ]]; then
        fail "$1 of $2 failed"
    fi
    [[ "$s" == completed ]]
}

case "$FS" in
xfs | ext4) ;;
*) fail "set FS to xfs or ext4" ;;
esac

[[ $EUID -eq 0 ]] || fail "run $0 as root"
for f in "$MM" "$KERNEL" "$QCOW2" "$PART" "$MINIMAL"; do
    [[ -s "$f" ]] || fail "$f is missing or empty"
done
MINIMAL="$(readlink -f "$MINIMAL")"
"$MM" -base "$MM_BASE" -e version >/dev/null || fail "minimega does not answer on ${MM_BASE}"

# ---------------------------------------------------------------------------
step "1. check reports Firecracker ${FS}"

start_line=$(($(wc -l <"$MM_LOG") + 1))
mm "check"
sleep 1
tail -n "+${start_line}" "$MM_LOG" >"${TMP}/check.log"
grep -E 'firecracker|jailer|cgroup version|/dev/kvm|reflink|loop-control|mapper/control|qemu-img|dmsetup|setfacl' "${TMP}/check.log" || true
grep -qE 'firecracker and jailer versions match: v?1\.17\.0' "${TMP}/check.log" ||
    fail "check does not report matching firecracker and jailer v1.17.0"
if grep -q 'firecracker unavailable' "${TMP}/check.log"; then
    fail "check reports that firecracker is unavailable"
fi
want_reflink=false
[[ "$FS" == xfs ]] && want_reflink=true
grep -qF "${FC_DIR} filesystem: ${FS}, reflink: ${want_reflink}" "${TMP}/check.log" ||
    fail "check does not report ${FC_DIR} as ${FS} with reflink ${want_reflink}"

# ---------------------------------------------------------------------------
step "2. host tap on VLAN ${VLAN}"

mm "tap create ${VLAN} ip $(host_ip 1)/30 ${HOST_TAP}"
for i in 2 3 4; do
    ip addr add "$(host_ip "$i")/30" dev "$HOST_TAP"
done
ip link set "$HOST_TAP" up
ip -br addr show dev "$HOST_TAP"

# ---------------------------------------------------------------------------
step "3. launch fc1, fc2, and fc3 from rootfs.qcow2"

base_config
mm "vm config backchannel true"
for i in 1 2 3; do
    mm "vm config net ${VLAN},$(guest_mac "$i")"
    mm "vm launch firecracker fc${i}"
    wait_state "fc${i}" BUILDING 60
done
mm "vm config backchannel false"
mm ".columns name,state,type,vlan,mac,tap vm info"

# ---------------------------------------------------------------------------
step "4. start fc1, fc2, and fc3"

mm "vm start all"
for i in 1 2 3; do
    wait_state "fc${i}" RUNNING 30
done
for i in 1 2 3; do
    wait_for 90 "fc${i} answers ping at $(guest_ip "$i")" ping_ok "$(guest_ip "$i")"
    echo "fc${i} answers at $(guest_ip "$i")"
done

# ---------------------------------------------------------------------------
step "4a. cc clients over vsock"

for i in 1 2 3; do
    u="$(vm_field "fc${i}" uuid)"
    [[ -n "$u" ]] || fail "no UUID for fc${i}"
    wait_for 90 "fc${i} (${u}) is a cc client" cc_has_client "$u"
    wait_for 30 "the cc client of fc${i} reports MAC $(guest_mac "$i")" cc_client_has_mac "$u" "$(guest_mac "$i")"
    echo "fc${i} is a cc client with MAC $(guest_mac "$i")"
done
mm "cc clients"

# ---------------------------------------------------------------------------
step "4b. cc exec on fc1"

mm "cc filter name=fc1"
mm "cc prefix smoke-exec"
mm "cc exec ip -br link"
wait_cc 60 "cc exec on fc1 returns the MAC of fc1" cc_response_has smoke-exec "$(guest_mac 1)"
mm "cc responses smoke-exec"
for i in 2 3; do
    if cc_response_has smoke-exec "$(guest_mac "$i")"; then
        cc_dump smoke-exec
        fail "cc exec with cc filter name=fc1 ran on fc${i}"
    fi
done

# ---------------------------------------------------------------------------
step "4c. cc send and cc recv with fc1"

mkdir -p "$MM_FILES"
CC_SEND_SRC="${MM_FILES}/${CC_SEND_NAME}"
echo "minifire cc send test $(date +%s) $$" >"$CC_SEND_SRC"
send_text="$(cat "$CC_SEND_SRC")"
guest_file="/tmp/miniccc/files/${CC_SEND_NAME}"

mm "cc prefix smoke-send"
mm "cc send ${CC_SEND_NAME}"
mm "cc prefix smoke-cat"
mm "cc exec cat ${guest_file}"
wait_cc 60 "fc1 has ${guest_file} from cc send" cc_response_has smoke-cat "$send_text"
echo "fc1 has ${guest_file} from cc send"

fc1_uuid="$(vm_field fc1 uuid)"
mm "cc prefix smoke-recv"
mm "cc recv ${guest_file}"
wait_for 60 "cc recv of ${guest_file} from fc1 matches ${CC_SEND_SRC}" cc_recv_matches "$fc1_uuid"
echo "cc recv from fc1 wrote $(cc_recv_path "$fc1_uuid")"

mm "clear cc prefix"
mm "clear cc filter"

# ---------------------------------------------------------------------------
step "5. stop and resume fc1"

mm "vm stop fc1"
wait_state fc1 PAUSED 10
wait_for 5 "ping to paused fc1 fails" ping_fails "$(guest_ip 1)"
echo "paused fc1 does not answer"
mm "vm start fc1"
wait_state fc1 RUNNING 10
wait_for 30 "resumed fc1 answers ping" ping_ok "$(guest_ip 1)"
echo "resumed fc1 answers"

# ---------------------------------------------------------------------------
step "6. kill and relaunch fc2"

fc2_uuid="$(vm_field fc2 uuid)"
mm "vm kill fc2"
wait_state fc2 QUIT 30
wait_for 20 "the cc client of killed fc2 is gone" cc_no_client "$fc2_uuid"
echo "killed fc2 is not a cc client"
mm "vm start fc2"
wait_state fc2 RUNNING 30
wait_for 90 "relaunched fc2 answers ping" ping_ok "$(guest_ip 2)"
echo "relaunched fc2 answers"

# ---------------------------------------------------------------------------
step "6a. cc client of relaunched fc2"

wait_for 90 "relaunched fc2 (${fc2_uuid}) is a cc client" cc_has_client "$fc2_uuid"
echo "relaunched fc2 is a cc client again"

# ---------------------------------------------------------------------------
step "7. console logs"

for i in 1 2 3; do
    id="$(vm_id "fc${i}")"
    console="${FC_DIR}/vm/${id}/console.log"
    [[ -s "$console" ]] || fail "${console} of fc${i} is missing or empty"
    grep -m 1 'Linux version' "$console" || fail "${console} of fc${i} has no 'Linux version' line"
done

# ---------------------------------------------------------------------------
step "8. disk path for ${FS}"

mm "disk cache"
qcow2_entries="$(mm ".annotate false .headers false .csv true disk cache" | awk -F, -v src="$QCOW2" '$2 == src && $3 == "qcow2"')"
[[ "$(grep -c . <<<"$qcow2_entries")" -eq 1 ]] ||
    fail "disk cache has $(grep -c . <<<"$qcow2_entries") entries for ${QCOW2}, not 1"
inuse="$(awk -F, '{print $6}' <<<"$qcow2_entries" | tr -d '[:space:]')"
[[ "$inuse" == 3 ]] || fail "the cache entry of ${QCOW2} has ${inuse} users, not 3"

for i in 1 2 3; do
    id="$(vm_id "fc${i}")"
    if [[ "$FS" == xfs ]]; then
        disk="${FC_DIR}/vm/${id}/disk-0.raw"
        [[ -f "$disk" && ! -L "$disk" ]] || fail "${disk} of fc${i} is not a regular file"
        [[ "$(stat -c %u "$disk")" != 0 ]] || fail "${disk} of fc${i} is owned by root, not by the VM uid"
        frag="$(filefrag -v "$disk" 2>&1 || true)"
        if grep -q shared <<<"$frag"; then
            echo "fc${i}: ${disk} has shared (reflink) extents"
        else
            echo "fc${i}: filefrag shows no shared extents for ${disk}"
        fi
    else
        disk="$(instance_disk "$id")"
        [[ -b "$disk" ]] || fail "${disk} of fc${i} is not a block device"
        [[ -f "${FC_DIR}/vm/${id}/disk-0.cow" ]] || fail "fc${i} has no copy-on-write file"
        echo "fc${i}: ${disk}"
    fi
done
if [[ "$FS" == ext4 ]]; then
    dmsetup ls | grep '^mm-fc-'
fi

# ---------------------------------------------------------------------------
step "9. snapshot false refuses a shared image and a qcow2 image"

mm "clear vm config net"
mm "vm config snapshot false"

# fc1, fc2, and fc3 use rootfs.qcow2, so a VM that writes to it conflicts.
start_line=$(($(wc -l <"$MM_LOG") + 1))
mm "vm launch firecracker bad0" || true
vm_absent bad0 || fail "bad0 launched with the disk that fc1 uses"
tail -n "+${start_line}" "$MM_LOG" >"${TMP}/bad0.log"
grep -q "disk conflict" "${TMP}/bad0.log" || fail "the log has no disk conflict for bad0"
echo "bad0 did not launch: disk conflict"

# A qcow2 image that no other VM uses.
cp "$QCOW2" "${TMP}/bad1.qcow2"
mm "vm config disk ${TMP}/bad1.qcow2"
launch_out="$(mm "vm launch firecracker bad1" 2>&1)" || true
echo "$launch_out"
wait_state bad1 ERROR 30
tags="$(mm ".annotate false .headers false .columns tags .filter name=bad1 vm info" || true)"
echo "bad1 tags: ${tags}"
grep -qi raw <<<"${launch_out} ${tags}" || fail "the error for bad1 does not mention raw"
mm "vm flush"
wait_for 30 "bad1 is flushed" vm_absent bad1
mm "vm config snapshot true"

# ---------------------------------------------------------------------------
step "10. inject a file into a partitioned raw image"

inject_src="${TMP}/minifire-test"
echo "minifire inject test $(date +%s) $$" >"$inject_src"

mm "vm config disk ${PART}"
mm "vm config append root=/dev/vda1"
mm "vm config inject 1 ${inject_src}:/etc/minifire-test"
mm "vm config net ${VLAN},$(guest_mac 4)"
mm "vm launch firecracker fc4"
wait_state fc4 BUILDING 120
mm "vm start fc4"
wait_state fc4 RUNNING 30
wait_for 90 "fc4 answers ping" ping_ok "$(guest_ip 4)"
echo "fc4 answers at $(guest_ip 4)"
mm "vm kill fc4"
wait_state fc4 QUIT 30

id="$(vm_id fc4)"
disk="$(instance_disk "$id")"
INJECT_LOOP="$(losetup -f -P -r --show "$disk")"
wait_for 10 "${INJECT_LOOP}p1 exists" test -b "${INJECT_LOOP}p1"
INJECT_MNT="${TMP}/mnt"
mkdir -p "$INJECT_MNT"
mount -o ro,noload "${INJECT_LOOP}p1" "$INJECT_MNT"
cmp "$inject_src" "${INJECT_MNT}/etc/minifire-test" ||
    fail "/etc/minifire-test in the disk of fc4 does not match ${inject_src}"
echo "fc4 disk ${disk} has /etc/minifire-test"
umount "$INJECT_MNT"
INJECT_MNT=""
losetup -d "$INJECT_LOOP"
INJECT_LOOP=""

mm "clear vm config inject"
mm "vm flush"
wait_for 30 "fc4 is flushed" vm_absent fc4

# ---------------------------------------------------------------------------
step "10a. memory dump of fc3"

command -v readelf >/dev/null || fail "readelf is missing: install binutils"
dump="${TMP}/fc3.elf"
mm "vm memory-dump fc3 ${dump}"
wait_for 120 "the memory dump of fc3 is completed" job_completed "vm memory-dump" fc3
mm "vm memory-dump"
readelf -l -W "$dump" >"${TMP}/fc3.readelf"
loads="$(grep -c '^ *LOAD ' "${TMP}/fc3.readelf" || true)"
((loads >= 1)) || fail "${dump} has no LOAD segment"
dump_size="$(stat -c %s "$dump")"
((dump_size >= 256 << 20)) || fail "${dump} is ${dump_size} bytes, less than the 256 MiB of fc3"
echo "${dump} has ${loads} LOAD segments and ${dump_size} bytes"
wait_state fc3 RUNNING 10
rm -f "$dump"

# ---------------------------------------------------------------------------
step "10b. save fc1 and restore it as fcr"

SNAP="${MM_FILES}/fc1snap"
mm "vm save fc1 fc1snap"
wait_for 300 "vm save of fc1 is completed" job_completed "vm save" fc1
mm "vm save"
for ext in state mem hdd json; do
    [[ -s "${SNAP}.${ext}" ]] || fail "${SNAP}.${ext} is missing or empty"
done
grep -qF '"vm_type": "firecracker"' "${SNAP}.json" || fail "${SNAP}.json does not have \"vm_type\": \"firecracker\""
state_is fc1 PAUSED || fail "fc1 is $(vm_field fc1 state) after vm save, not PAUSED"
echo "vm save wrote ${SNAP}.state, .mem, .hdd, and .json, and fc1 is PAUSED"

mm "vm kill fc1"
wait_state fc1 QUIT 30
mm "vm flush fc1"
wait_for 30 "fc1 is flushed" vm_absent fc1

base_config
mm "vm config disk ${SNAP}.hdd"
mm "vm config backchannel true"
mm "vm config net ${VLAN},$(guest_mac 1)"
mm "vm config firecracker-snapshot ${SNAP}"
mm "vm launch firecracker fcr"
wait_state fcr BUILDING 120
mm "vm start fcr"
wait_state fcr RUNNING 30
wait_for 30 "restored fcr answers ping at $(guest_ip 1)" ping_ok "$(guest_ip 1)"
echo "restored fcr answers at $(guest_ip 1)"
fcr_uuid="$(vm_field fcr uuid)"
[[ -n "$fcr_uuid" ]] || fail "no UUID for fcr"
wait_for 90 "restored fcr (${fcr_uuid}) is a cc client" cc_has_client "$fcr_uuid"
echo "restored fcr is a cc client"
mm "clear vm config firecracker-snapshot"

# ---------------------------------------------------------------------------
step "11. launch and start 20 VMs"

base_config
mm "clear vm config net"
mm "vm config memory 128"

"$MM" -base "$MM_BASE" -e "vm launch firecracker scale[1-20]" >"${TMP}/launch.out" 2>&1 &
launch_pid=$!

answered=0
probes=0
while kill -0 "$launch_pid" 2>/dev/null; do
    probes=$((probes + 1))
    if timeout 5 "$MM" -base "$MM_BASE" -e "vm info summary" >/dev/null 2>&1 && kill -0 "$launch_pid" 2>/dev/null; then
        answered=$((answered + 1))
        break
    fi
    sleep 1
done
if ! wait "$launch_pid"; then
    cat "${TMP}/launch.out" >&2
    fail "vm launch firecracker scale[1-20] did not exit 0"
fi
cat "${TMP}/launch.out"
if grep -q 'Error (' "${TMP}/launch.out"; then
    fail "vm launch firecracker scale[1-20] failed"
fi
[[ $answered -ge 1 ]] || fail "vm info did not answer within 5s during the launch (${probes} probes)"
echo "vm info answered during the launch"

scale_running() {
    local n
    n="$(mm ".annotate false .headers false .csv true .columns name,state vm info" | grep -c '^scale[0-9]*,RUNNING' || true)"
    [[ "$n" -eq 20 ]]
}

mm "vm start all"
if ! wait_for_quiet 120 scale_running; then
    mm ".columns name,state,tags vm info" >&2 || true
    fail "not all 20 scale VMs are RUNNING after 120s"
fi
echo "20 scale VMs are RUNNING"

# ---------------------------------------------------------------------------
step "12. kill and flush all VMs"

kill_flush_all
check_clean

# ---------------------------------------------------------------------------
step "13. minifire"

banner="$(mm "minifire")"
echo "$banner"
grep -qF "$BANNER" <<<"$banner" || fail "minifire does not print the banner"

start_line=$(($(wc -l <"$MM_LOG") + 1))
base_config
mm "clear vm config net"
mm "vm launch firecracker minifire"
beacon_lit() {
    tail -n "+${start_line}" "$MM_LOG" | grep -qF "$BEACON"
}
wait_for 30 "the log has the beacon message" beacon_lit
echo "the log has the beacon message"
kill_flush_all
check_clean

# ---------------------------------------------------------------------------
step "14. shared read-only root"

ro_start=$SECONDS
for i in $(seq 1 "$RO_VMS"); do
    ip addr add "$(ro_host_ip "$i")/30" dev "$HOST_TAP"
done

mm "clear vm config"
mm "vm config vcpus 1"
mm "vm config memory 64"
mm "vm config kernel ${KERNEL}"
mm "vm config disk ${MINIMAL},ro"
mm "vm config backchannel false"
for i in $(seq 1 "$RO_VMS"); do
    mm "vm config append root=/dev/vda ro rootfstype=squashfs init=/init mm.ip=$(ro_ip "$i")/30 mm.gw=$(ro_host_ip "$i") mm.hostname=ro${i}"
    if ((i % 2 == 0)); then
        mm "vm config firecracker-scratch 16M"
    else
        mm "clear vm config firecracker-scratch"
    fi
    mm "vm config net ${VLAN},$(ro_mac "$i")"
    mm "vm launch firecracker ro${i}"
done
mm "vm start all"
if ! wait_for_quiet 30 ro_running; then
    mm ".columns name,state,tags vm info" >&2 || true
    fail "not all ${RO_VMS} ro VMs are RUNNING after 30s"
fi
echo "${RO_VMS} ro VMs are RUNNING"

ro_entry="$(mm ".annotate false .headers false .csv true disk cache" | awk -F, -v src="$MINIMAL" '$2 == src')"
[[ "$(grep -c . <<<"$ro_entry")" -eq 1 ]] ||
    fail "disk cache has $(grep -c . <<<"$ro_entry") entries for ${MINIMAL}, not 1"
ro_inuse="$(cut -d, -f6 <<<"$ro_entry" | tr -d '[:space:]')"
[[ "$ro_inuse" == "$RO_VMS" ]] || fail "the cache entry of ${MINIMAL} has ${ro_inuse} users, not ${RO_VMS}"
cached="${FC_DIR}/cache/$(cut -d, -f1 <<<"$ro_entry" | tr -d '[:space:]').raw"
[[ "$(stat -c %a "$cached")" == 444 ]] || fail "${cached} has mode $(stat -c %a "$cached"), not 444"

for i in $(seq 1 "$RO_VMS"); do
    name="ro${i}"
    id="$(vm_id "$name")"
    inst="${FC_DIR}/vm/${id}"
    console="${inst}/console.log"
    jail="$(jail_root "$id")"
    [[ -n "$jail" ]] || fail "no jail for ${name}"

    wait_for 60 "the console of ${name} shows 'init: ready'" grep -q 'init: ready' "$console"
    grep -q 'Mounted root (squashfs filesystem) readonly' "$console" ||
        fail "${name} did not mount a read-only squashfs root"
    [[ "${jail}/disk0" -ef "$cached" ]] || fail "${jail}/disk0 of ${name} is not a hard link to ${cached}"
    if compgen -G "${inst}/disk-0.*" >/dev/null; then
        fail "${name} has per-VM disk files: $(echo "${inst}"/disk-0.*)"
    fi
    [[ -p "${jail}/firecracker.log" ]] || fail "${jail}/firecracker.log of ${name} is not a named pipe"
    [[ -s "${inst}/firecracker.log" ]] || fail "${inst}/firecracker.log of ${name} is empty"

    scratch="${inst}/scratch.raw"
    if ((i % 2 == 0)); then
        [[ -f "$scratch" ]] || fail "${name} has no scratch.raw"
        [[ "$(stat -c %s "$scratch")" -eq $((16 << 20)) ]] || fail "scratch.raw of ${name} is $(stat -c %s "$scratch") bytes, not 16 MiB"
        blocks="$(stat -c %b "$scratch")"
        bsize="$(stat -c %B "$scratch")"
        ((blocks * bsize < 1 << 20)) || fail "scratch.raw of ${name} uses $((blocks * bsize)) bytes"
        [[ "${jail}/scratch" -ef "$scratch" ]] || fail "${jail}/scratch of ${name} is not a hard link to ${scratch}"
        grep -q '\[vdb\]' "$console" || fail "the kernel of ${name} did not find the scratch disk vdb"
        echo "${name}: scratch.raw uses $((blocks * bsize)) bytes"
    else
        [[ ! -e "$scratch" ]] || fail "${name} has a scratch.raw without firecracker-scratch"
        if grep -q '\[vdb\]' "$console"; then
            fail "the kernel of ${name} found a vdb without firecracker-scratch"
        fi
    fi

    wait_for 30 "${name} answers ping at $(ro_ip "$i")" ping_ok "$(ro_ip "$i")"
    echo "${name} booted from ${cached} and answers at $(ro_ip "$i")"
done

links="$(stat -c %h "$cached")"
((links >= RO_VMS + 1)) || fail "${cached} has ${links} links, not at least $((RO_VMS + 1))"
echo "${cached} has ${links} links"

dm="$(dmsetup ls 2>/dev/null | awk '{print $1}' | grep '^mm-fc-' || true)"
[[ -z "$dm" ]] || fail "read-only VMs have device-mapper targets: ${dm//$'\n'/ }"
loops="$(losetup -a | grep -F "${FC_DIR}/" || true)"
[[ -z "$loops" ]] || fail "read-only VMs have loop devices: ${loops//$'\n'/ }"
echo "no device-mapper targets or loop devices for the ro VMs"

kill_flush_all
check_clean
links="$(stat -c %h "$cached")"
[[ "$links" -eq 1 ]] || fail "${cached} has ${links} links after the flush, not 1"
for i in $(seq 1 "$RO_VMS"); do
    ip addr del "$(ro_host_ip "$i")/30" dev "$HOST_TAP"
done
echo "check 14 took $((SECONDS - ro_start))s"

# ---------------------------------------------------------------------------
step "15. launch rate"

# now prints the time in seconds, with nanoseconds.
now() {
    date +%s.%N
}

# since prints the seconds from time $1 to now, with one decimal.
since() {
    awk -v a="$1" -v b="$(now)" 'BEGIN { printf "%.1f", b - a }'
}

# count_state prints the number of VMs named $1<number> that are in state $2.
count_state() {
    mm ".annotate false .headers false .csv true .columns name,state vm info" | grep -c "^$1[0-9]*,$2" || true
}

# ready_count prints the number of VMs in file $1, one VM ID on each line,
# whose console log shows 'init: ready'.
ready_count() {
    local n=0 id
    while read -r id; do
        if grep -qs 'init: ready' "${FC_DIR}/vm/${id}/console.log"; then
            n=$((n + 1))
        fi
    done <"$1"
    echo "$n"
}

# rate_run launches VMs $1[1-$2] with the current vm config in the
# background and starts them while they launch. It waits until all are
# RUNNING and each console log shows 'init: ready', then prints the minifire
# timing line and the launch rate.
rate_run() {
    local prefix=$1 n=$2
    local log_start t0 deadline launch_pid launch_s running_s ready_s ids line

    log_start=$(($(wc -l <"$MM_LOG") + 1))
    t0="$(now)"
    deadline=$((SECONDS + RATE_TIMEOUT))

    "$MM" -base "$MM_BASE" -e "vm launch firecracker ${prefix}[1-${n}]" >"${TMP}/${prefix}.out" 2>&1 &
    launch_pid=$!
    while kill -0 "$launch_pid" 2>/dev/null; do
        ((SECONDS < deadline)) || fail "vm launch firecracker ${prefix}[1-${n}] did not end in ${RATE_TIMEOUT}s"
        mm "vm start all" >/dev/null
        sleep 1
    done
    if ! wait "$launch_pid"; then
        cat "${TMP}/${prefix}.out" >&2
        fail "vm launch firecracker ${prefix}[1-${n}] did not exit 0"
    fi
    launch_s="$(since "$t0")"
    if grep -q 'Error (' "${TMP}/${prefix}.out"; then
        cat "${TMP}/${prefix}.out" >&2
        fail "vm launch firecracker ${prefix}[1-${n}] failed"
    fi
    mm "vm start all"

    until [[ "$(count_state "$prefix" RUNNING)" -eq "$n" ]]; do
        if ((SECONDS >= deadline)); then
            mm ".columns name,state,tags vm info" >&2 || true
            fail "not all ${n} ${prefix} VMs are RUNNING after ${RATE_TIMEOUT}s"
        fi
        sleep 0.5
    done
    running_s="$(since "$t0")"

    ids="${TMP}/${prefix}.ids"
    mm ".annotate false .headers false .csv true .columns id,name vm info" |
        awk -F, -v p="$prefix" '$2 ~ "^" p "[0-9]+$" {print $1}' >"$ids"
    [[ "$(grep -c . "$ids")" -eq "$n" ]] || fail "vm info has $(grep -c . "$ids") ${prefix} VMs, not ${n}"
    until [[ "$(ready_count "$ids")" -eq "$n" ]]; do
        if ((SECONDS >= deadline)); then
            fail "$(ready_count "$ids") of ${n} ${prefix} consoles show 'init: ready' after ${RATE_TIMEOUT}s"
        fi
        sleep 0.5
    done
    ready_s="$(since "$t0")"

    line="$(tail -n "+${log_start}" "$MM_LOG" | grep -F "minifire: ${n} microVMs lit in" | tail -n 1 || true)"
    [[ -n "$line" ]] || fail "the minimega log has no 'minifire: ${n} microVMs lit in' line"
    echo "$line"
    echo "${n} ${prefix} VMs: launched in ${launch_s}s, RUNNING in ${running_s}s, ready in ${ready_s}s"
    echo "launch rate: $(awk -v n="$n" -v s="$ready_s" 'BEGIN { printf "%.1f", (s > 0 ? n / s : 0) }') VMs/s"
}

RATE_NIC_VMS=20
RATE_TIMEOUT=240

mm "clear vm config"
mm "vm config vcpus 1"
mm "vm config memory 64"
mm "vm config kernel ${KERNEL}"
mm "vm config disk ${MINIMAL},ro"
mm "vm config backchannel false"
mm "vm config append root=/dev/vda ro rootfstype=squashfs init=/init"
rate_run scale "$RATE_VMS"
kill_flush_all
check_clean

# A static address keeps udhcpc out of the boot. The check does not use it.
mm "vm config append root=/dev/vda ro rootfstype=squashfs init=/init mm.ip=192.0.2.1/32"
mm "vm config net ${VLAN}"
rate_run scalenic "$RATE_NIC_VMS"
kill_flush_all
check_clean

# ---------------------------------------------------------------------------
step "16. snapshot clones"

CLONES=5
CSRC_MAC="06:00:ac:10:02:00"
CSNAP="${MM_FILES}/csnap"

clone_mac() {
    printf '06:00:ac:10:02:%02x' "$1"
}

# clone_tag prints the clone_identity tag of VM $1.
clone_tag() {
    mm ".annotate false .headers false .columns tags .filter name=$1 vm info" 2>/dev/null |
        sed -n 's/.*"clone_identity":"\([^"]*\)".*/\1/p'
}

# clones_done returns 0 when each clone has clone_identity=ok. It fails the
# test when a clone has an identity error.
clones_done() {
    local i v
    for i in $(seq 1 "$CLONES"); do
        v="$(clone_tag "clone${i}")"
        case "$v" in
        ok) ;;
        "" | pending) return 1 ;;
        *) fail "the identity of clone${i} failed: ${v}" ;;
        esac
    done
}

# acl_users prints the number of named user entries in the ACL of file $1.
acl_users() {
    getfacl -cn "$1" 2>/dev/null | grep -c '^user:[0-9]' || true
}

lan_vlan="$(mm ".annotate false .headers false .csv true vlans" | awk -F, -v a="$VLAN" '$1 == a || $1 ~ ("//" a "$") {print $2}' | tr -d '[:space:]')"
[[ "$lan_vlan" =~ ^[0-9]+$ ]] || fail "vlans has no VLAN number for ${VLAN}: '${lan_vlan}'"
host_tag="$(ovs-vsctl get port "$HOST_TAP" tag)"
[[ "$host_tag" == "$lan_vlan" ]] || fail "${HOST_TAP} has OVS tag ${host_tag}, but vlans gives ${lan_vlan} for ${VLAN}"
echo "${VLAN} is VLAN ${lan_vlan}"

mm "clear vm config"
mm "vm config vcpus 1"
mm "vm config memory 64"
mm "vm config kernel ${KERNEL}"
mm "vm config disk ${MINIMAL},ro"
mm "vm config backchannel true"
mm "vm config append root=/dev/vda ro rootfstype=squashfs init=/init mm.ip=192.0.2.10/24 mm.gw=192.0.2.1 mm.hostname=csrc"
mm "vm config net ${VLAN},${CSRC_MAC}"
mm "vm launch firecracker csrc"
wait_state csrc BUILDING 60
mm "vm start csrc"
wait_state csrc RUNNING 30
csrc_uuid="$(vm_field csrc uuid)"
[[ -n "$csrc_uuid" ]] || fail "no UUID for csrc"
wait_for 90 "csrc (${csrc_uuid}) is a cc client" cc_has_client "$csrc_uuid"

mm "vm save csrc csnap"
wait_for 120 "vm save of csrc is completed" job_completed "vm save" csrc
for ext in state mem json; do
    [[ -s "${CSNAP}.${ext}" ]] || fail "${CSNAP}.${ext} is missing or empty"
done
mm "vm kill csrc"
wait_state csrc QUIT 30
mm "vm flush csrc"
wait_for 30 "csrc is flushed" vm_absent csrc
echo "vm save wrote ${CSNAP}.state, .mem, and .json"

mm "clear vm config"
mm "vm config vcpus 1"
mm "vm config memory 64"
mm "vm config kernel ${KERNEL}"
mm "vm config disk ${MINIMAL},ro"
mm "vm config backchannel true"
mm "vm config append root=/dev/vda ro rootfstype=squashfs init=/init"
mm "vm config firecracker-snapshot ${CSNAP}"
mm "vm config firecracker-clone true"
for i in $(seq 1 "$CLONES"); do
    mm "vm config net ${VLAN},$(clone_mac "$i")"
    mm "vm launch firecracker clone${i}"
done
for i in $(seq 1 "$CLONES"); do
    name="clone${i}"
    wait_state "$name" BUILDING 60
    v="$(vm_field "$name" vlan)"
    [[ "$v" == "[disconnected]" ]] || fail "${name} has VLAN ${v} before its identity, not [disconnected]"
    tap="$(vm_field "$name" tap | tr -d '[]')"
    if ovs-vsctl port-to-br "$tap" >/dev/null 2>&1; then
        fail "the tap ${tap} of ${name} is on a bridge before its identity"
    fi
done
mm ".columns name,state,vlan,mac,tap vm info"
echo "the taps of the ${CLONES} clones are not on a bridge"

t0="$(now)"
mm "vm start all"
if ! wait_for_quiet 120 clones_done; then
    mm ".columns name,state,vlan,tags vm info" >&2 || true
    fail "not all ${CLONES} clones have clone_identity=ok after 120s"
fi
echo "${CLONES} clones have their identity $(since "$t0")s after vm start all"
mm ".columns name,state,vlan,mac,tap,tags vm info"

for i in $(seq 1 "$CLONES"); do
    name="clone${i}"
    mac="$(clone_mac "$i")"

    tap="$(vm_field "$name" tap | tr -d '[]')"
    tag="$(ovs-vsctl get port "$tap" tag)"
    [[ "$tag" == "$lan_vlan" ]] || fail "the tap ${tap} of ${name} has OVS tag ${tag}, not ${lan_vlan}"

    mm "cc filter name=${name}"
    # BusyBox ip has no -br option.
    mm "cc prefix ${name}-link"
    mm "cc exec ip link"
    mm "cc prefix ${name}-host"
    mm "cc exec hostname"
    mm "cc prefix ${name}-route"
    mm "cc exec ip route"
    wait_cc 60 "${name} reports MAC ${mac}" cc_response_has "${name}-link" "$mac"
    wait_cc 60 "${name} reports host name ${name}" cc_response_line "${name}-host" "$name"
    wait_cc 60 "${name} keeps the default route of csrc" cc_response_has "${name}-route" "default via 192.0.2.1"
    if cc_response_has "${name}-link" "$CSRC_MAC"; then
        cc_dump "${name}-link"
        fail "${name} still has the MAC ${CSRC_MAC} of csrc"
    fi
    echo "${name}: MAC ${mac}, host name ${name}, OVS tag ${tag}"
done
mm "clear cc prefix"
mm "clear cc filter"

rss_kb=0
pss_kb=0
snap_gid=""
for i in $(seq 1 "$CLONES"); do
    name="clone${i}"
    id="$(vm_id "$name")"
    jail="$(jail_root "$id")"
    [[ -n "$jail" ]] || fail "no jail for ${name}"
    [[ "${jail}/snapshot.mem" -ef "${CSNAP}.mem" ]] || fail "${jail}/snapshot.mem of ${name} is not ${CSNAP}.mem"

    # Each VMM has its own uid (uid base + VM ID) and the gid of the snapshot.
    pid="$(vm_field "$name" pid)"
    vmm_uid="$(awk '/^Uid:/ {print $2}' "/proc/${pid}/status" 2>/dev/null || true)"
    vmm_gid="$(awk '/^Gid:/ {print $2}' "/proc/${pid}/status" 2>/dev/null || true)"
    [[ "$vmm_uid" =~ ^[0-9]+$ && "$vmm_gid" =~ ^[0-9]+$ ]] || fail "no uid and gid for the VMM of ${name} (pid ${pid})"
    uid_base=$((vmm_uid - id))
    ((vmm_gid < uid_base && vmm_gid >= uid_base - 65536)) || fail "the VMM of ${name} has gid ${vmm_gid}, not a snapshot gid below the uid base ${uid_base}"
    [[ -z "$snap_gid" || "$vmm_gid" == "$snap_gid" ]] || fail "the VMM of ${name} has gid ${vmm_gid}, but another clone has gid ${snap_gid}"
    snap_gid="$vmm_gid"

    # The jail root and the cc socket stay with the uid of the clone.
    [[ "$(stat -c '%u %a' "$jail")" == "${vmm_uid} 700" ]] ||
        fail "the jail root of ${name} is $(stat -c '%u:%g mode %a' "$jail"), not owned by ${vmm_uid} with mode 700"
    sock="${jail}/v.sock_9002"
    [[ "$(stat -c '%u %a' "$sock")" == "${vmm_uid} 600" ]] ||
        fail "the cc socket of ${name} is $(stat -c '%u:%g mode %a' "$sock"), not owned by ${vmm_uid} with mode 600"

    r="$(awk '/^VmRSS:/ {print $2}' "/proc/${pid}/status" 2>/dev/null || true)"
    p="$(awk '/^Pss:/ {print $2}' "/proc/${pid}/smaps_rollup" 2>/dev/null || true)"
    rss_kb=$((rss_kb + ${r:-0}))
    pss_kb=$((pss_kb + ${p:-0}))
done
for f in "${CSNAP}.state" "${CSNAP}.mem"; do
    [[ "$(stat -c '%u %g %a' "$f")" == "0 ${snap_gid} 640" ]] ||
        fail "${f} is $(stat -c '%u:%g mode %a' "$f"), not 0:${snap_gid} with mode 640"
    acl="$(acl_users "$f")"
    [[ "$acl" -eq 0 ]] || fail "${f} has ${acl} named user ACL entries, not 0"
done
echo "each clone maps ${CSNAP}.mem; the VMMs share gid ${snap_gid}, and ${CSNAP}.mem is 0:${snap_gid} with mode 640 and no ACL entries"
echo "VMM memory of ${CLONES} clones: RSS ${rss_kb} KiB, PSS ${pss_kb} KiB; guest memory ${CLONES} x 64 MiB = $((CLONES * 65536)) KiB"

kill_flush_all
check_clean
for f in "${CSNAP}.state" "${CSNAP}.mem"; do
    [[ "$(stat -c '%u %g %a' "$f")" == "0 0 600" ]] ||
        fail "${f} is $(stat -c '%u:%g mode %a' "$f") after the flush, not 0:0 with mode 600"
done
rm -f "${CSNAP}.state" "${CSNAP}.mem" "${CSNAP}.json"
CSNAP=""

mm "tap delete ${HOST_TAP}"

echo
echo "PASS: all Firecracker smoke checks passed on ${FS}"
