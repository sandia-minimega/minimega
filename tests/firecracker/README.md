# Firecracker functional tests

These scripts boot real Firecracker VMs through minimega. They check the VM
lifecycle, the network, cc over vsock, the disk paths, and the cleanup of host
objects.

The GitHub Actions workflow
[`.github/workflows/firecracker.yml`](../../.github/workflows/firecracker.yml)
runs them on a hosted x86_64 Ubuntu 24.04 runner. The job runs two times:

- `xfs`: the Firecracker directory is on XFS with reflink. Each VM gets a
  reflink copy of the cached disk.
- `ext4`: the Firecracker directory is on ext4. Each VM gets a device-mapper
  snapshot of the cached disk.

The workflow installs Firecracker v1.17.0. It downloads a guest kernel and an
Ubuntu root filesystem from the Firecracker CI artifact bucket and verifies
their SHA-256 sums. It also builds the minimal guest root image with
[`misc/firecracker/guest/build-rootfs.sh`](../../misc/firecracker/guest/build-rootfs.sh).
When a test fails, the workflow uploads the minimega log and the VM logs as an
artifact.

## Memory dump and save checks

Check 10a of `smoke.sh` writes an ELF memory dump of `fc3` with
`vm memory-dump`. It polls `vm memory-dump` until the dump is `completed`.
Then it makes sure that `readelf -l` shows at least one `LOAD` segment and
that the file is not smaller than the 256 MiB of guest memory.

Check 10b saves `fc1` with `vm save fc1 fc1snap` and polls `vm save` until the
save is `completed`. It makes sure of these items:

- The files directory has `fc1snap.state`, `fc1snap.mem`, `fc1snap.hdd`, and
  `fc1snap.json`, and `fc1snap.json` has `"vm_type": "firecracker"`.
- `fc1` is `PAUSED` after the save, as a KVM VM is after `vm save`.

Then the check kills and flushes `fc1`. It launches `fcr` with the
configuration of `fc1`, `vm config disk <files>/fc1snap.hdd`, and
`vm config firecracker-snapshot <files>/fc1snap`, and starts it. `fcr` must
be `RUNNING`, answer ping at the address of `fc1` in 30 seconds, and be a cc
client in 90 seconds. The test removes the `fc1snap` files at the end.

## Shared read-only root check

Check 14 of `smoke.sh` boots 10 VMs, `ro1` to `ro10`, from the minimal guest
`rootfs.squashfs` with `vm config disk <image>,ro`. The even VMs also get
`vm config firecracker-scratch 16M`. The check makes sure of these items:

- All 10 VMs are `RUNNING`, and each console log shows `init: ready` and a
  read-only squashfs root.
- Each VM answers ping at its `mm.ip` address.
- The jail of each VM has a hard link to the one cached copy of the image.
  The cached copy has mode `0444` and at least 11 links.
- No VM has a `disk-0.*` file, and no VM has a device-mapper target or a loop
  device.
- Only the even VMs have `scratch.raw`. It is 16 MiB, it uses less than 1 MiB,
  and the guest kernel finds it as `vdb`.
- Each jail has `firecracker.log` as a named pipe, and each instance directory
  has a `firecracker.log` file that is not empty.
- After `vm kill all` and `vm flush`, no jail is left and the cached copy has
  one link.

## Launch rate check

Check 15 of `smoke.sh` measures the launch rate. It launches `N` VMs,
`scale1` to `scaleN`, from the minimal guest `rootfs.squashfs` with
`vm config disk <image>,ro`, 64 MiB of memory, and no NIC. So the check
measures the disk, jail, VMM, and boot steps, not the network setup. `N` is
the value of `RATE_VMS`. The default is 100.

The workflow sets `RATE_VMS` to 25. GitHub Actions can run the job on Azure or
on AWS ([changelog](https://github.blog/changelog/2026-10-28-github-actions-adds-multi-cloud-compute-to-improve-reliability/)).
On the AWS-hosted runners, which use nested KVM on Intel CPUs, many VMs that
boot at the same time can fail with `KVM_EXIT_FAIL_ENTRY`. 100 VMs pass on the
Azure-hosted runners.

The check runs `vm launch firecracker scale[1-N]` in the background, and
runs `vm start all` one time each second until the launch ends. Then it
measures the seconds until all `N` VMs are `RUNNING` and until each console
log shows `init: ready`. All `N` VMs must be ready in 240 seconds. The check
prints these items:

- The `minifire: N microVMs lit in ...` line from the minimega log. This
  line gives the launch time of the batch, the p50 and p95 times of the disk,
  network, and vmm stages, and the p95 time that the launches waited for a
  stage.
- The seconds until the launch ended, until all VMs were `RUNNING`, and until
  all VMs were ready.
- The launch rate in VMs each second: `N` divided by the seconds until all
  VMs were ready.

Then the check kills and flushes the VMs, and makes sure that no host objects
are left, as check 12 does.

The check then does the same steps for 20 VMs, `scalenic1` to `scalenic20`,
with one NIC each on VLAN `LAN`. This part sends the netlink setup and the OVS
port adds of many VMs at the same time. Each of these guests gets the static
address `192.0.2.1/32`, so that it does not start DHCP. The check does not use
the guest addresses.

## Snapshot clone check

Check 16 of `smoke.sh` boots `csrc` from the minimal guest with
`vm config backchannel true`, one NIC on VLAN `LAN`, and the static address
`192.0.2.10/24`. It saves `csrc` with `vm save csrc csnap`, then kills and
flushes it. Then it launches five clones, `clone1` to `clone5`, with
`vm config firecracker-snapshot <files>/csnap`,
`vm config firecracker-clone true`, and a different MAC for each clone. The
check makes sure of these items:

- Before `vm start all`, each clone shows `disconnected` in the `vlan` column,
  and its tap is not on a bridge.
- In 120 seconds after `vm start all`, each clone has the tag
  `clone_identity=ok`.
- The tap of each clone has the OVS tag of VLAN `LAN`.
- In each clone, `ip link` shows the MAC of the clone and not the MAC of
  `csrc`, `hostname` shows the name of the clone, and `ip route` still has the
  default route of `csrc`.
- The jail of each clone has `/snapshot.mem`, which is the same file as
  `csnap.mem`. All VMMs of the clones have their own uid and the same gid,
  which is not a uid of a VMM. `csnap.state` and `csnap.mem` are
  `root:<gid>` with mode `0640` and no ACL entries for the clones.
- The jail root of each clone is owned by the uid of the clone with mode
  `0700`. The cc socket in the jail is owned by that uid with mode `0600`.
- After `vm kill all` and `vm flush`, no host objects are left, and
  `csnap.state` and `csnap.mem` are `root:root` with mode `0600`.

The check prints the seconds from `vm start all` until all clones have their
identity. It also prints the sum of the RSS and PSS of the five VMMs, and the
sum of the guest memory, for information.

## Files

| File | Use |
|---|---|
| `prepare-guest.sh` | Builds `rootfs.ext4`, `rootfs.qcow2`, and `rootfs-part.raw` from a kernel and a squashfs root filesystem. It adds miniccc and a systemd unit that starts `miniccc -family vsock -port 9002`. It downloads nothing. |
| `smoke.sh` | Runs the checks against a running minimega. |

## Guest network addresses

The guest root filesystem sets the address of each NIC from its MAC address.
A MAC `06:00:AA:BB:CC:DD` gives the address `AA.BB.CC.DD/30`. `smoke.sh` gives
each VM a MAC in its own `/30` and adds the other address of that `/30` to a
host tap.

## Run the tests on a Linux host

Use a Linux host with KVM that you can change. The tests run as root, and they
create taps, network namespaces, loop devices, and device-mapper targets.

1. Install the packages:

   ```bash
   sudo apt-get install build-essential dnsmasq iproute2 isc-dhcp-client libpcap-dev \
     ntfs-3g openssh-client openvswitch-switch qemu-kvm qemu-utils squashfs-tools \
     e2fsprogs xfsprogs acl dmsetup fdisk iputils-ping binutils
   ```

2. Install `firecracker` and `jailer` v1.17.0 as `/usr/local/bin/firecracker`
   and `/usr/local/bin/jailer`, with owner `root` and mode `0755`.

3. Make a work directory. Put the guest kernel in it as `vmlinux` and the
   squashfs root filesystem in it as `rootfs.squashfs`. The workflow file
   gives the URLs and the SHA-256 sums.

4. Build minimega and miniccc from the repository root:

   ```bash
   source scripts/env.bash
   ./scripts/version.bash
   go build -o bin/minimega ./cmd/minimega
   CGO_ENABLED=0 go build -o bin/miniccc ./cmd/miniccc
   ```

5. Build the guest images:

   ```bash
   sudo MINICCC="$PWD/bin/miniccc" tests/firecracker/prepare-guest.sh <workdir>
   misc/firecracker/guest/build-rootfs.sh <workdir>/minimal --miniccc bin/miniccc
   ```

6. Make the Firecracker directory `/var/lib/mmfc`. The directory and its
   parent directories must not be writable by the group or by other users.
   The directory must not be on a `nodev` or `noexec` mount.

7. Start minimega:

   ```bash
   sudo bin/minimega -nostdin -base /tmp/mm -firecracker-dir /var/lib/mmfc \
     -level debug -logfile /tmp/mm.log &
   ```

8. Run the checks. Set `FS` to the filesystem of `/var/lib/mmfc`, `xfs` or
   `ext4`:

   ```bash
   sudo FS=ext4 tests/firecracker/smoke.sh <workdir>
   ```

The checks use VLAN `LAN`, the host tap `mmfc_host`, and the addresses in
`172.16.0.0/28` and `172.16.1.0/26`. The cc checks put one file in the minimega files
directory (`<base>/files`). Check 10b puts the `fc1snap` files there, and
check 16 puts the `csnap` files there. The test removes them at the end. Use a minimega instance
that has no other VMs. To use other paths, set `MM`, `MM_BASE`, `MM_LOG`,
`MM_FILES`, `FC_DIR`, and `MINIMAL`.
