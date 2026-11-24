# Firecracker Virtual Machines


<a id="TOC_1."></a>

## Introduction

minimega can launch Linux guests as `firecracker` type VMs. Each VM runs in a
[Firecracker](https://firecracker-microvm.github.io/) microVM. The Firecracker
jailer confines the virtual machine monitor (VMM) of each VM.

Use Firecracker VMs for many small Linux nodes, or for guests that run code
that you do not trust:

- Firecracker has a smaller device model than QEMU. It has no graphics, USB,
  or legacy PC devices.
- The jailer gives each VMM its own chroot, user ID, cgroup, network
  namespace, and seccomp filter. No VMM runs as root.
- Firecracker boots Linux quickly and uses little memory for each VM.
- cc uses vsock, a channel between the host and the guest that does not use
  the experiment network.

Firecracker VMs attach to the same Open vSwitch VLANs as KVM VMs, so one
experiment can use both types. The usual commands and features keep their
meaning: `vm launch`, `vm start`, `vm stop`, `vm kill`, `vm flush`, `vm info`,
tags, namespaces, and cluster scheduling.

<a id="TOC_2."></a>

## Host requirements

Each minimega host that runs Firecracker VMs needs these items:

- An x86_64 Linux host with `/dev/kvm`.
- `firecracker` and `jailer` v1.17.0 or later. Use the static binaries of a
  Firecracker release. The two binaries must have the same version.
- Binaries and parent directories that the group and other users cannot write
  to.
- Open vSwitch, as for KVM VMs.
- `qemu-img`, to convert qcow2 images to raw images.
- `losetup`, `dmsetup`, `/dev/loop-control`, and `/dev/mapper/control`, for
  device-mapper disk copies and for `vm config inject`.
- `setfacl`, for disks with `vm config snapshot false`.

The guests must be Linux guests. Firecracker boots the kernel directly. It does
not use firmware or a boot loader, so a boot loader in the disk image has no
effect. The Firecracker project tests its releases on the host kernels 5.10,
6.1, and 6.18.

The minimega Docker image includes Firecracker and the jailer. For the build
arguments that set the version, see the Firecracker section of the
[Docker guide](https://github.com/sandia-minimega/minimega/blob/master/docker/README.md#firecracker-vms).

<a id="TOC_2.1."></a>

### The Firecracker directory

The `-firecracker-dir` startup flag sets the directory for Firecracker VMs. The
default is `/var/lib/minimega/firecracker`. It holds three directories:

- `cache`: the image cache. Each entry is a raw copy of a disk image, a
  kernel, or an initrd.
- `vm`: one instance directory for each VM, `vm/<ID>`. The usual instance path
  `<base>/<ID>` is a link to it.
- `jail`: the jail of each VM. minimega builds a new jail at each launch, and
  removes it after the VMM stops.

The directory must obey these rules:

- All three directories must be on one filesystem. minimega puts hard links
  into each jail, and a hard link cannot go across two filesystems.
- On XFS or btrfs, each VM gets a reflink copy of a cached image. On ext4, each
  VM gets a device-mapper snapshot of a cached image.
- The directory must not be on a `nodev` or `noexec` mount. The jailer creates
  device nodes in the jail, and it runs its copy of the Firecracker binary from
  the jail.
- The group and other users must not have write access to the directory, to
  its parent directories, or to `/var/run/netns`.
- The path must be short. A Unix socket path in a jail must be shorter than
  108 bytes, and a longer path stops the launch.

<a id="TOC_2.2."></a>

### Startup flags

| Flag | Default | Use |
|---|---|---|
| `-firecracker-dir` | `/var/lib/minimega/firecracker` | The directory for the jails, the instance directories, and the image cache. |
| `-firecracker-uid-base` | `2000000000` | The first uid and gid of the VMMs. Each VMM runs as the base plus its VM ID. The VMMs that restore one snapshot share a gid below the base. |
| `-firecracker-launch-parallel` | `0` | The maximum number of VMs that prepare at the same time. `0` is the number of CPUs. |
| `-firecracker-start-parallel` | `0` | The maximum number of VMs that `vm start` boots at the same time. `0` is the number of CPUs. |

The environment variables `MM_FIRECRACKER_DIR`, `MM_FIRECRACKER_UID_BASE`,
`MM_FIRECRACKER_LAUNCH_PARALLEL`, and `MM_FIRECRACKER_START_PARALLEL` set the
same values.

At startup, minimega logs a warning when `/etc/passwd` or `/etc/group` has an
ID from the uid base to the uid base plus 65535.

The 65536 gids below the uid base are the snapshot gids. Each snapshot that a
VM restores gets one of these gids while a VM that restores it exists.
minimega logs a warning at startup when `/etc/passwd` or `/etc/group` has an
ID in this range, and does not use that ID.

Two minimega instances on one host must use different `-firecracker-dir` and
`-firecracker-uid-base` values. Two instances can give the same VM ID to two
VMs. With the same values, these VMs then get the same jail and uid.

<a id="TOC_2.3."></a>

### Check the host

minimega checks the host at startup and when you run the `check` command. It
writes the results to the minimega log:

- The path and the version of `firecracker` and `jailer`, if each binary is
  static, and if the two versions are the same.
- The host cgroup version, and `/dev/kvm`.
- The filesystem of `-firecracker-dir`, and its reflink support.
- `/dev/loop-control`, `/dev/mapper/control`, `qemu-img`, `dmsetup`, and
  `setfacl`.
- A `firecracker host tuning` warning for each host setting that makes many
  VMs slow. The
  [Host settings](https://github.com/sandia-minimega/minimega/blob/master/docker/README.md#host-settings)
  section of the Docker guide gives the fixes.

Firecracker is optional, so a failed check only writes a warning. The check
uses the `firecracker` and `jailer` binaries in `$PATH`. A launch uses
`vm config firecracker-path` and `vm config jailer-path`, and it fails when a
check fails.

<a id="TOC_3."></a>

## Launching Firecracker VMs

Do these steps to launch a Firecracker VM:

1. Clear the old VM configuration with `clear vm config`.
2. Set the guest kernel with `vm config kernel`. A kernel is necessary.
3. Optional: set an initrd with `vm config initrd`.
4. Optional: set the kernel arguments with `vm config append`.
5. Set the disk with `vm config disk`.
6. Set the network with `vm config net`.
7. Launch the VM with `vm launch firecracker`.
8. Boot the VM with `vm start`.

For example:

```text
clear vm config
vm config kernel vmlinux
vm config disk rootfs.ext4
vm config net LAN
vm launch firecracker fc0
vm start fc0
```

minimega resolves a relative path in the files directory (`-filepath`).
`vm launch` prepares the disk and the network, and it configures the VMM. It
does not boot the guest. The VM stays in the building state until `vm start`.

The guest must run miniccc to use cc. See
[Command and control over vsock](#command-and-control-over-vsock).

<a id="TOC_3.1."></a>

### Example: 100 VMs from one configuration

This script sets one configuration and launches 100 VMs from it on VLAN `lan`:

```text
clear vm config
vm config vcpus 1
vm config memory 512
vm config kernel fc/vmlinux-6.1
vm config initrd fc/initrd-6.1.img
vm config append root=/dev/vda1 rw
vm config disk debian-12.qcow2
vm config snapshot true
vm config net lan
vm config backchannel true
vm config firecracker-entropy true

vm launch firecracker fc[1-100]
vm start all

.columns name,state,vlan vm info
```

- The first launch converts `debian-12.qcow2` to a raw image in the cache one
  time. The other 99 VMs get a reflink copy or a device-mapper snapshot of the
  cached image. A copy uses almost no disk space until the guest writes.
- Each VM uses 4 of its 17 devices: the disk, the NIC, the vsock device for
  cc, and the entropy device.
- The host has 100 jails, 100 network namespaces, and 100 veth host ends on
  the OVS bridge. Each VMM runs as the uid base plus its VM ID.
- The guest memory totals 50 GiB. Each VMM also uses a small quantity of host
  memory.
- In a cluster, run `ns queueing true` before the script, and run `vm launch`
  with no arguments after it. The scheduler then puts the VMs on the hosts.

<a id="TOC_3.2."></a>

### A shared read-only root image

All VMs can boot from one read-only image. The
[minimal guest](https://github.com/sandia-minimega/minimega/blob/master/misc/firecracker/guest/README.md)
in `misc/firecracker/guest` builds such an image with BusyBox and miniccc. It
also builds a small guest kernel. The minimal guest README gives the full
configuration.

```text
vm config disk rootfs.squashfs,ro
vm config firecracker-scratch 64M
```

- The `ro` option attaches the disk read-only. All VMs that use the image share
  one cached copy, through hard links. The `snapshot` setting does not apply
  to a disk with `ro`.
- minimega makes no copy, loop device, or device-mapper target for each VM.
  The host page cache holds one copy of the image for all VMs.
- `vm config firecracker-scratch` adds an empty writable disk as the last disk.
  It is a sparse file, so it uses only the blocks that the guest writes.

<a id="TOC_4."></a>

## Configuration

Firecracker VMs use these common `vm config` parameters: `vcpus` (1 to 32),
`memory`, `disk`, `kernel`, `initrd`, `append`, `net`, `snapshot`,
`backchannel`, `cc-policy`, `uuid`, `tags`, `schedule`, and `coschedule`. See
the [minimega API](../reference/minimega.md) for each parameter.

<a id="TOC_4.1."></a>

### Firecracker-specific configuration parameters

| Parameter | Default | Use |
|---|---|---|
| `firecracker-path` | `firecracker` | The Firecracker binary: a path, or a name in `$PATH`. |
| `jailer-path` | `jailer` | The jailer binary. It must have the same version as the Firecracker binary. |
| `firecracker-pci` | `false` | Use the PCI transport for the devices. The guest kernel must support PCI. |
| `firecracker-entropy` | `true` | Add a virtio entropy device. |
| `vsock-cid` | `3` | The guest CID of the vsock device, from 3 to 4294967294. |
| `firecracker-scratch` | none | Add an empty writable disk of this size, for example `64M`. |
| `firecracker-snapshot` | none | Load a snapshot that `vm save` wrote. See [Save and restore](#save-and-restore). |
| `firecracker-clone` | `false` | Give a VM that loads `firecracker-snapshot` its own MACs and host name. See [Clones](#clones). |
| `inject` | none | Copy files into the first disk, or remove files from it, at launch. |

<a id="TOC_4.2."></a>

### Disks

Firecracker reads raw images only. minimega accepts the same `vm config disk`
values as for KVM VMs, and it makes a raw disk for each VM:

- With `snapshot true`, each VM gets its own disk in its instance directory.
  minimega copies or converts the image into the cache one time. Then each VM gets
  a reflink copy or a device-mapper snapshot of the cached image. When neither
  is available, each VM gets a full sparse copy.
- After minimega creates a device-mapper snapshot, it runs `dmsetup mknodes`.
  Then it waits up to 5 seconds for the `/dev/mapper` node. A container without
  udev needs this step. When the node does not appear, minimega returns an
  error.
- A qcow2 overlay with one backing file, as `disk snapshot` makes, gets a copy
  of its cached backing file. Then minimega commits the overlay into the copy.
  A longer chain of backing files gets a full `qemu-img convert`.
- With `snapshot false`, the disk must be a raw image. The VM writes to the
  source image. minimega gives the uid of the VMM access to the image with an
  ACL. As for KVM VMs, a launch fails when another VM of any type uses the
  same image and one of the two VMs writes to it.
- With the `ro` option, the VM gets the disk read-only. All VMs share the
  cached copy. The image must not have a backing file.
- The disk interface does not apply. Firecracker always uses virtio-blk.
- The cache mode `unsafe` sets the Firecracker cache type `Unsafe`. All other
  cache modes set `Writeback`.

`vm config inject` copies files into the first disk of each VM at launch.
minimega changes the disk of the VM. It does not change the source image. This
parameter needs `snapshot true` and a disk 0 without `ro`.

`disk cache` shows the entries of the image cache. `disk cache clear` removes
the entries that no VM uses. It also removes the temporary files of cache
copies that did not complete.

<a id="TOC_4.3."></a>

### Boot arguments

minimega gives `vm config append` to Firecracker as the kernel command line. It
adds `console=ttyS0`, `reboot=k`, and `panic=1` when the arguments do not set
them. A `reboot=` value other than `k` stops the launch. Firecracker then adds
`pci=off` without PCI, and one `virtio_mmio.device=` entry for each device.

- When the arguments have no `root=`, disk 0 is the root device. Firecracker
  then adds the root device to the command line. Use this for an image that has
  a filesystem and no partition table.
- When the arguments have `root=`, no disk is the root device. minimega adds
  `rw` when the arguments have no `ro` and no `rw`. Use this for an image with
  a partition table, for example with `root=/dev/vda1`.
- On x86_64, the command line must be shorter than 2048 bytes. Each device
  adds about 30 bytes. minimega checks the length at launch.

The guest kernel must have these drivers, built in or in the initrd:

- `CONFIG_ACPI`, `CONFIG_PCI`, `CONFIG_KVM_GUEST`, and `CONFIG_VIRTIO_MMIO`.
- `CONFIG_VIRTIO_BLK`, `CONFIG_VIRTIO_NET`, and the driver of the root
  filesystem.
- The 8250 serial console.
- The virtio vsock transport, `CONFIG_VIRTIO_VSOCKETS`, for cc.
- `CONFIG_HW_RANDOM_VIRTIO`, for the entropy device.
- `CONFIG_SERIO_I8042` and `CONFIG_KEYBOARD_ATKBD`, for `vm shutdown`.
- For `firecracker-pci`, also `CONFIG_PCI_MSI`, `CONFIG_PCI_MMCONFIG`, and
  `CONFIG_VIRTIO_PCI`.

A distribution kernel builds some of these drivers as modules, so it needs a
matching initrd. The minimal guest has a kernel configuration fragment for a
small guest kernel.

<a id="TOC_4.4."></a>

### Device limit

On x86_64, a Firecracker VM can have at most 17 devices. Each virtio-MMIO device
needs its own legacy interrupt line, and Firecracker has 17 lines for these
devices. With `vm config firecracker-pci true`, the devices use the PCI
transport and MSI-X, and the limit is 31 devices.

Each of these items uses one device:

- Each disk of `vm config disk`, also a disk with `ro`.
- The scratch disk of `vm config firecracker-scratch`.
- Each NIC of `vm config net`.
- The vsock device, when `vm config backchannel` is true. The default is true.
- The entropy device, when `vm config firecracker-entropy` is true. The default
  is true.

When a VM needs more devices, `vm launch` fails and shows the count. For
example:

```text
VM web1 needs 19 devices (2 disks, 15 NICs, vsock, entropy); the limit is 17 without firecracker-pci
```

<a id="TOC_4.5."></a>

### Ignored and unsupported settings

`vm config` keeps its values across launches, so a setting for another VM type
can stay from an earlier launch.

minimega ignores these settings for a Firecracker VM, and it logs a warning
for each one that is not at its default value:

- `qemu`, `cpu`, `sockets`, `cores`, `threads`, `machine`, and `vga`.
- `qemu-append` and `qemu-override`.
- `baremetal`, `baremetal-network-driver`, `usb-use-xhci`, `tpm-socket`, and
  `bidirectional-copy-paste`.
- A disk interface other than virtio, and a NIC driver other than virtio.
- The container settings and the Android settings.
- `optimize hugepages`. Huge pages for Firecracker VMs are not supported.

`vm launch` fails for these settings:

- `vm config cdrom`, `vm config serial-ports`, and `vm config virtio-ports`.
- `vm config state`. Use `vm config firecracker-snapshot` instead.
- A NIC with QinQ, and bonds.
- No kernel, and fewer than 1 or more than 32 vCPUs.

<a id="TOC_5."></a>

## VM lifecycle

| Command or event | Result | State after |
|---|---|---|
| `vm launch firecracker` | minimega prepares the disks, the network, and a new jail. It runs the jailer and configures the VMM. The guest does not boot. | building |
| `vm start` from building | The guest boots. | running |
| `vm stop` | The VMM pauses the guest. | paused |
| `vm start` from paused | The guest continues. | running |
| The guest reboots | The VMM stops with exit code 0. | quit |
| The guest kernel panics | The VMM stops. minimega finds `Kernel panic` at the end of the console log. | error |
| The VMM stops with an error | minimega records the exit status and the last lines of the Firecracker log. | error |
| `vm kill` | minimega kills the VMM. | quit |
| `vm start` from quit or error | minimega builds a new jail and runs a new VMM with the same disk. The guest boots. | running |
| `vm flush` | minimega removes the VM and its host objects. | none |

A guest reboot keeps the disk changes of the guest. The next `vm start` boots
the same disk, as a cold boot does. Firecracker has no ACPI power-off. The
result of a power-off in the guest depends on the guest kernel and its init.

`vm flush` removes the jail, the instance directory, the disks, the network
namespace, the veth pair, the OVS port, the device-mapper target, and the loop
device of the VM. The image cache stays.

`vm info` shows `firecracker` in the `type` column. For a Firecracker VM, it
also has the `append`, `disks`, `kernel`, `initrd`, and `pid` columns. The
`vnc_port` column is empty.

<a id="TOC_5.1."></a>

### Shut down, reset, and export disks

These commands are new for Firecracker VMs:

- `vm shutdown <vm target>` sends Ctrl-Alt-Del to a running VM. The guest
  reboots, and the VM goes to the quit state. The guest kernel must have the
  i8042 and AT keyboard drivers. The Firecracker CI kernels do not have them.
- `vm reset-disk <vm target>` builds the disks of a VM in the quit or error
  state again, as `vm launch` did. All disk changes of the guest are lost.
  minimega applies `vm config inject` again, and the scratch disk is empty
  again. The VM must have `snapshot true`.
- `vm export-disk <vm name> <dst> [raw,qcow2]` writes the first disk of a VM to
  an image file. The default format is qcow2. The VM must be in the running,
  paused, or quit state. minimega pauses a running VM only for a fast copy of
  the disk. A fast copy is possible only when the disk is a device-mapper
  snapshot or the Firecracker directory supports reflink. Otherwise, minimega
  refuses the export of a running VM, so pause or stop the VM first. The image
  is crash-consistent.

`vm export-disk` with no arguments shows the progress of the exports, with the
same columns as `vm save`. A relative destination is relative to the files
directory. The destination must not exist.

`vm shutdown` also works for KVM VMs. For a KVM VM, it sends an ACPI power
button event.

<a id="TOC_5.2."></a>

### Console and logs

A Firecracker VM has one serial port, the console. minimega writes the console
output to `console.log` in the instance directory, `<base>/<ID>/console.log`.
At each launch, minimega renames the old file to `console.log.1`. It keeps at
most 16 MiB of console output for each launch, then writes one truncation line.

To see the last console output, use `vm serial <vm name> 0`. It returns the
last 64 KiB of `console.log`. The `bytes` argument of `vm serial` sets a
different size. minimega does not supply an interactive console.

The instance directory also holds these logs:

- `firecracker.log`: the log of the VMM, at most 4 MiB for each launch.
- `jailer.log`: the output of the jailer.

<a id="TOC_6."></a>

## Command and control over vsock

KVM VMs use a virtio-serial port for cc. A Firecracker VM has only one serial
port, the console, so cc uses vsock.

- With `vm config backchannel true`, minimega adds a vsock device to the VM.
  This is the default.
- In the guest, miniccc connects to the host, CID 2, port 9002.
- The VMM sends the connection to a Unix socket in the jail of the VM.
  minimega listens on that socket.
- The guest has no DMI UUID. minimega binds the socket to the UUID of the VM.
- The guest opens each connection, so miniccc connects again after a reboot,
  a restore, a memory dump, or a save.

Start miniccc in the guest with:

```bash
miniccc -family vsock -port 9002
```

`vm config vsock-cid` sets the CID of the guest. Each VM has its own vsock
device, so all VMs can use the same CID.

<a id="TOC_6.1."></a>

### cc policy

`vm config cc-policy` sets the policy for requests from the guest. The values
are `strict` and `legacy`. An empty value uses the default of the VM type:
`strict` for Firecracker VMs, and `legacy` for the other VM types.

With `strict`, minimega applies these rules to tunnel requests from the guest:

- minimega refuses a request that makes the host listen on a port.
- minimega refuses a request that makes the host connect to an address. The
  only exception is a connection for a reverse tunnel that the operator made
  for that VM.
- The tunnels that the operator makes with `cc tunnel` work.

With both policies, minimega applies these rules to each cc client:

- A connection that minimega binds to a VM accepts only the UUID of that VM,
  or an empty UUID.
- minimega closes a connection that sends a message that is not valid.
- minimega refuses a file path from the guest that is absolute, or that goes
  out of its directory.
- A message can be at most 16 MiB. The first message (the handshake) can be at
  most 256 KiB. A file that the guest sends can be at most 1 GiB.
- minimega decodes at most two messages larger than 1 MiB at the same time,
  for all clients. A client must send each of these messages in 30 seconds.
- minimega closes a client that does not read a message in 15 seconds.
- A client can write at most 4 GiB of responses each hour. In each hour, it
  can start at most 65536 files and directories. It can open at most 256
  tunnel streams and 64 pipes.
- minimega drops responses and files for a command that does not exist, for
  example a command that the operator deleted.
- The clients can set at most 1024 tags on a VM, with a total size of 1 MiB.

<a id="TOC_7."></a>

## Memory dumps

`vm memory-dump <vm name> <file> [elf,raw]` writes the guest memory of a
running or paused VM to a file:

- `elf` is the default. The file is an ELF core file with one `PT_LOAD`
  segment for each range of guest physical memory. It has no notes with the
  CPU registers.
- `raw` is for Firecracker VMs only. It writes the memory file of a
  Firecracker snapshot. minimega also writes `<file>.json` with the address,
  the size, and the file offset of each memory region.

minimega pauses a running VM while Firecracker writes the memory, then resumes
the VM. The pause time increases with the memory size. For `elf`, the dump
uses disk space for two copies of the guest memory for a short time.

The dump resets all vsock connections of the guest. cc commands, file
transfers, and tunnels that are in progress fail. miniccc then connects again.
`vm memory-dump` with no arguments shows the progress of the dumps.

<a id="TOC_8."></a>

## Save and restore

`vm save <vm name> <filename>` saves a running or paused VM. minimega pauses
the VM, Firecracker writes a full snapshot, and minimega copies the writable
disks. The VM stays paused, as a KVM VM does after `vm save`. Use `vm start` to
resume it. A relative `<filename>` is relative to the files directory.
minimega writes these files:

- `<filename>.state`: the Firecracker state file.
- `<filename>.mem`: the guest memory. It is as large as the memory of the VM.
- `<filename>.hdd`: disk 0, as a raw image. Other writable disks go to
  `<filename>.hdd<N>`, and the scratch disk goes to `<filename>.scratch`.
- `<filename>.json`: the facts that a restore checks. minimega writes it last.

minimega does not copy a disk with the `ro` option. `<filename>.json` records
its path.

Do these steps to restore a saved VM:

1. Set the same `vm config` as the saved VM had.
2. Set `vm config disk` to `<filename>.hdd`. For more disks, add the other
   disk files of the save, in the same order. Keep each `ro` disk.
3. Set `vm config firecracker-snapshot <filename>`.
4. Keep `vm config kernel` set.
5. Launch the VM with `vm launch firecracker`.
6. Resume the VM with `vm start`.

The launch compares the configuration and the host with `<filename>.json`, and
fails when they are different. The VM gets the NIC MACs of the snapshot,
unless it is a [clone](#clones). After
a guest reboot, the VM boots its disk with `vm config kernel`. It does not load
the snapshot again.

`ns save` saves each running or paused Firecracker VM with `vm save`. It waits
until minimega writes the files of each save, and fails when a save fails. Its
launch script restores the VM with `vm config firecracker-snapshot`. For other
Firecracker VMs, `ns save` records only the configuration.

Only one save to a `<filename>` can run at a time. A launch that restores
`<filename>` waits while a save renames its files into place.

<a id="TOC_8.1."></a>

### Restore limits

- A snapshot loads only when its format version has the same major number as
  the format of the Firecracker binary. Its minor number must not be higher.
  A Firecracker upgrade usually changes the format version.
- Restore a snapshot only on the same kind of host: the same architecture, CPU
  model, and host kernel.
- The vCPUs, the memory, and the devices must be the same as in the snapshot.
- The snapshot does not include the disks. Use the disk files that `vm save`
  wrote, and do not change them.
- Do not change `<filename>.mem`. A restored VM uses that file for all its
  life. A new `vm save` to the same name writes new files, and does not change
  the files that a running VM uses.
- `vm config inject` does not apply to a restore.
- After a restore, guest TCP sessions with other nodes break. The vsock
  connections reset in the restored VM. The save also resets them in the
  source VM. miniccc connects again.
- The guest clock is late after a restore until the guest sets it, for example
  with NTP. minimega does not set the `clock_realtime` option of the snapshot
  load.
- All restores of one snapshot have the same MACs, IP addresses, host name, and
  memory secrets. Do not run two of them on one VLAN, unless the guest changes
  its address. A [clone](#clones) gets its own MACs and host name.
- When the source VM continues after `vm save`, it also shares these secrets.
  Linux 5.18 and later guests use VMGenID to reseed their random number
  generator after a restore.
- A KVM VM cannot load a Firecracker snapshot. A Firecracker VM cannot load a
  KVM snapshot.

<a id="TOC_8.2."></a>

### Clones

A clone is a VM that loads a snapshot and then gets its own identity. Use
clones to start many VMs from one saved VM. Set
`vm config firecracker-clone true` together with
`vm config firecracker-snapshot`.

minimega does these steps for each clone:

1. `vm launch` keeps the configured MAC of each NIC. It creates each NIC
   disconnected, so the `vlan` column of `vm info` shows `disconnected`.
2. `vm start` resumes the guest. The guest still has the MACs and the host
   name of the saved VM, but no NIC is on a VLAN.
3. When miniccc in the guest connects, minimega runs one cc command in the
   guest. For each NIC, the command finds the guest interface that has the MAC
   of the snapshot, and sets the configured MAC on it. Then the command sets
   the first 63 bytes of the VM name as the host name. When the guest refuses
   the host name, the command continues.
4. minimega connects each NIC to its configured VLAN.
5. When `udhcpc` runs in the guest, minimega then runs `udhcpc` one time on
   each NIC, to get a lease for the new MAC.

The `clone_identity` tag shows the result. The value is `pending` until step
4, `ok` after step 4, or an error message. minimega waits at most 120 seconds
for the cc client, and then at most 120 seconds for the result of the command.
The time while the VM is paused does not count. After an error, the NICs stay
disconnected. Correct the guest, then connect the NICs with `vm net connect`.
Or use `vm kill` and `vm start`: the VM then boots its disk with the
configured MACs, and minimega connects the NICs. When the VM quits before the
identity is set, for example after `vm kill`, minimega also connects the NICs.

`vm save` and `ns save` refuse a clone until its `clone_identity` tag is
`ok`, or until a `vm kill` and `vm start` of the clone.

For example, these commands save `src`, and then start 100 clones of it on
VLAN `lan`:

```text
vm save src base
vm kill src

clear vm config
vm config vcpus 1
vm config memory 64
vm config kernel vmlinux
vm config disk rootfs.squashfs,ro
vm config net lan
vm config firecracker-snapshot base
vm config firecracker-clone true
vm launch firecracker clone[1-100]
vm start all
```

Each clone gets a random MAC, because `vm config net` gives no MAC. The clones
share memory on the host:

- The jail of each clone gets a hard link to `<filename>.mem`, or a bind mount
  when the file is on a different filesystem. Firecracker maps the file as
  private memory. Thus all clones share the pages that they do not change, in
  the host page cache.
- All VMs that restore one snapshot run with the same snapshot gid. minimega
  gives `<filename>.state` and `<filename>.mem` to `root:<gid>` with mode
  `0640`. After `vm flush` of the last of these VMs, the files are `root:root`
  with mode `0600` again. When the filesystem cannot change the group, each VM
  gets an ACL entry. ext4 holds only about 500 ACL entries on a file.
- The jail, the console and log pipes, the tap, and the cc socket of each VM
  stay with the uid of the VM, so that one VMM cannot use the objects of
  another VMM.
- A disk with `ro` is one shared cached image for all clones. A writable disk
  gets its own copy of `<filename>.hdd` for each clone.

These limits apply to clones:

- A clone needs `vm config backchannel true`, and the guest must run miniccc.
- The VM name must be 64 bytes or shorter, the limit of a Linux host name.
- The guest must have `sh`, `ip`, `hostname`, and `/sys/class/net`. The
  minimal guest has them.
- A clone keeps the static IP addresses of the saved VM. Give the saved VM no
  static address and use `udhcpc`, or set the addresses through cc.
- Clones share the memory secrets of the saved VM. Linux 5.18 and later
  guests use VMGenID to reseed their random number generator after a restore.

<a id="TOC_9."></a>

## Unsupported features

Firecracker VMs do not support these features:

- VNC and `vm screenshot`. A Firecracker VM has no graphics device.
- CD-ROMs and `vm cdrom`.
- Serial ports other than the console, and virtio-serial ports.
- `vm hotplug`.
- `vm net add`. A Firecracker VM gets all its NICs at launch.
- `vm net bond` and QinQ.
- `vm qmp`.

A command that names a Firecracker VM for one of these features returns an
error. For `vm hotplug` and `vm cdrom`, a wildcard target skips the Firecracker
VMs. `vm net connect` and `vm net disconnect` work. They move the host end of
the veth pair of the NIC.

Firecracker VMs do not use huge pages. With `optimize hugepages`, minimega logs
a warning at launch. `optimize ksm` has no effect, because Firecracker does not
mark guest memory as mergeable.

<a id="TOC_10."></a>

## Recovery, nuke, and clusters

- `-recover` does not recover Firecracker VMs after a minimega restart.
  minimega logs a message and skips each Firecracker VM.
- `nuke` kills the VMM and jailer processes of this minimega instance. It
  removes their jails, instance directories, network namespaces,
  device-mapper targets, loop devices, and cgroups. It keeps the image cache.
- At startup, minimega removes the Firecracker state that an earlier minimega
  process with the same `-base` and `-firecracker-dir` left. It does the
  Firecracker steps of `nuke`, and it also removes the `<base>/<ID>` links to
  the instance directories. It keeps the image cache and the other entries of
  the base directory. minimega logs each removal and a summary. It does this
  before it accepts commands and before `-recover` runs. When the Firecracker
  directory does not exist, it does nothing.
- Do not use `-force` while a minimega process with the same `-base` runs. The
  startup cleanup kills the Firecracker VMs of that process.
- In a cluster, each host that can get a Firecracker VM needs the Firecracker
  and jailer binaries. The scheduler does not know which hosts have them.

<a id="TOC_11."></a>

## Launches at scale

minimega launches Firecracker VMs in parallel, in three stages. Each stage has
its own limit. `P` is the value of `-firecracker-launch-parallel`, or the
number of CPUs when the value is 0.

| Stage | Work | Limit |
|---|---|---|
| disk | The disks, `vm config inject`, and the kernel and initrd cache entries. | `P` |
| network | The network namespace, the veth pairs, the taps, and the tc filters. | `P`, at most 8 |
| vmm | The jail, the jailer, and the configuration of the VMM. | `2 × P` |

At most `4 × P` first launches can be in progress at the same time. Each first
launch counts until it completes the vmm stage. A launch skips a stage that has
no work. For example, a relaunch from the quit state skips the disk and network
stages.

`vm start` sends the boot request (`InstanceStart`) of at most `S` VMs at the
same time. `S` is the value of `-firecracker-start-parallel`, or the number of
CPUs when the value is 0. The guests that boot use the host CPUs, so a value
of `S` that is much larger than the number of CPUs makes each request slower,
but does not boot the VMs faster. The other VMs wait for a place. The timeout of each request is 60
seconds, and it starts when the request gets a place. The guests that boot at
the same time use the host CPUs, so a request can take longer than other API
requests.

- minimega creates the links, the taps, and the tc filters through netlink. It
  does not run `ip` or `tc` for them.
- minimega holds each network namespace open as a file descriptor. The
  namespaces have no name in `/run/netns`, so `ip netns list` does not show
  them.
- minimega adds the OVS ports in groups. One `ovs-vsctl` transaction adds at
  most 256 ports. A port add does not wait for other requests. The port adds
  that the launches request while a transaction runs go into the next
  transaction.

After each `vm launch` with Firecracker VMs, minimega writes one line to the
log:

```text
minifire: <N> microVMs lit in <time> (disk p50/p95 <t>/<t>, network p50/p95 <t>/<t>, vmm p50/p95 <t>/<t>; waits p95 <t>)
```

The line gives the time of the batch, and the median and 95th percentile time
of each stage. `waits` is the time that the launches waited for the stages. At
the debug level, minimega also logs the launch times of each VM.

For a host with many VMs, do these steps:

1. Apply the host settings in the Firecracker section of the
   [Docker guide](https://github.com/sandia-minimega/minimega/blob/master/docker/README.md#host-settings).
2. Set the open file limit of minimega to 100000 or more.
3. Make sure that the host has enough memory for the sum of the guest memory,
   plus a small quantity for each VMM.
4. On ext4, make sure that the host has enough loop devices. Each cached image
   uses one, and each VM uses one more.

The [Firecracker functional tests](https://github.com/sandia-minimega/minimega/blob/master/tests/firecracker/README.md)
measure the launch rate of 100 VMs by default.

GitHub Actions can run a Linux job on Azure or on AWS
([changelog](https://github.blog/changelog/2026-10-28-github-actions-adds-multi-cloud-compute-to-improve-reliability/)).
On the AWS-hosted runners, which use nested KVM on Intel CPUs, many
Firecracker VMs that boot at the same time can fail with
`KVM_EXIT_FAIL_ENTRY`. Thus, the launch rate check in GitHub Actions boots 25
VMs. 100 VMs pass on the Azure-hosted runners.

<a id="TOC_12."></a>

## Technical details

When minimega launches a `firecracker` type VM, these steps occur, in order:

- minimega creates a new VM handler with a copy of the VM configuration.
- minimega creates the instance directory in `-firecracker-dir`, and links
  `<base>/<ID>` to it.
- minimega checks the configuration, the binaries, and the Firecracker
  directory.
- In the disk stage, minimega makes the disks and applies `vm config inject`.
  It adds the kernel and the initrd to the cache.
- In the network stage, minimega creates the network namespace of the VM. For
  each NIC, it creates a veth pair. The host end gets a minimega tap name. The
  peer goes into the namespace with a tap that the VMM uid owns. A tc redirect
  connects the peer and the tap.
- minimega adds the host end of each veth pair to the OVS bridge as an access
  port of the VLAN.
- In the vmm stage, minimega builds a new jail and hard-links the disks, the
  kernel, and the initrd into it. It creates the named pipes for the console
  and the VMM log. On a relaunch, it also creates the cc socket.
- minimega runs the jailer. The jailer creates the chroot, enters the network
  namespace and the cgroup, drops to the VMM uid, and runs Firecracker.
- minimega configures the VMM through its API socket: the logger, the machine,
  the boot source, the drives, the NICs, the vsock device, the entropy device,
  and the serial console.
- The VM goes to the building state, and minimega returns control to the user.
- On the first launch, minimega creates the cc socket when it connects the VM
  to cc, after the first `vm start`.

The network namespace has no addresses and no routes. The VMM does not need a
network. Packet capture, QoS, and tap statistics use the host end of the veth
pair, as they do for containers.
