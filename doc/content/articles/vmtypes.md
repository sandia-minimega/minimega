# Virtual machine types

minimega launches three kinds of virtual machine through one `vm config` /
`vm launch` API: QEMU/KVM virtual machines (`kvm`), lightweight full-system
Linux containers (`container`), and Android emulator instances (`android`).
Bare-metal firmware guests are a variant of the `kvm` type. An experiment can
mix all of them.

This page describes what each type needs on the host, which configuration
fields matter for it, and what minimega does when it launches one. Read
[VM lifecycle](vm-lifecycle.md) first for the configure, launch, start model
that all types share; the field-by-field details are in the
[VM configuration reference](vm-config-reference.md).

## A quick example

Launching a VM means describing it and then asking minimega to launch one or
more copies of that description. A KVM VM needs at least a disk image (or a
kernel and initrd):

```minimega title="example1.mm"
--8<-- "articles/vmtypes/example1.mm"
```

[Download this example](vmtypes/example1.mm){ download="example1.mm" }

The disk is configured once, but eleven VMs are launched from it. minimega
always launches from the *current* configuration, so the type keyword in
`vm launch` (`kvm`, `container`, or `android`) is what decides which fields
are used.

## Common configuration

Some fields apply to every VM type: `memory`, `vcpus`, `networks`, `bonds`,
`snapshot`, `uuid`, `backchannel`, `tags`, and the scheduler hints
`schedule`, `colocate`, and `coschedule`. The rest are type specific. Fields
that do not apply to the type being launched are ignored, so it is safe to
leave `vm config disks` set when you launch a container.

| Type | Fields you will usually set | Details |
|---|---|---|
| `kvm` | `disks` or `kernel`/`initrd`/`append` or `cdrom`; `cpu`, `machine`, `vga`, `serial-ports`, `virtio-ports`, `state` | [KVM virtual machines](#kvm-virtual-machines) |
| `kvm` (bare metal) | `baremetal true`, `baremetal-network-driver`, `kernel`, `qemu`, `machine`, `cpu`, `serial-ports`, `backchannel false` | [Bare-metal firmware](#bare-metal-firmware) |
| `container` | `filesystem` (required), `init`, `preinit`, `hostname`, `fifos`, `volume` | [Containers](#containers) |
| `android` | `android-avd` (required), `android-sdk`, `android-emulator`, `android-adb`, `android-avd-dir` | [Android virtual machines](#android-virtual-machines) |

The [VM configuration reference](vm-config-reference.md) lists every field
with its type, default, and a link into the generated command reference.

## KVM virtual machines

`kvm` VMs are QEMU processes. minimega translates the configuration into a
QEMU command line, creates the network taps and attaches them to Open vSwitch,
starts QEMU, and then manages it through its QMP socket. When QEMU exits or
crashes, `vm info` reflects it in the `state` column.

minimega requires QEMU 1.6 or newer (`check` verifies the installed version).
The QEMU binary defaults to `kvm`; set `vm config qemu` to use another binary,
for example `qemu-system-arm` for non-x86 firmware.

### Boot media and disks

A KVM VM boots from one of:

- one or more disks (`vm config disks <path>[,<interface>[,<cache>]] ...`),
- a kernel and initrd (`vm config kernel`, `vm config initrd`, with kernel
  parameters in `vm config append`), which take precedence over disks, or
- a CD-ROM image (`vm config cdrom`), which becomes the boot device.

With snapshot mode on (the default), minimega creates a qcow2 overlay in the
VM's instance directory for every disk, so any number of VMs can share one
image and their writes are discarded when they are flushed. With
`vm config snapshot false` the VM writes to the image itself, and only one VM
may use it. Building, resizing, and injecting files into images is covered in
[Disk images and the disk API](disk-images.md), creating a fresh image from
install media in [Creating VMs from install media](newvm.md), and Windows
specifics in [Windows guests](windows.md).

### CPU, machine, and devices

`vm config cpu` (default `host`) and `vm config machine` are passed to QEMU;
both accept only values the configured QEMU binary reports (`qemu -cpu help`,
`qemu -M help`), and the prompt tab-completes them. `sockets`, `cores`, and
`threads` shape the topology of the `vcpus` you allocate. `vm config vga`
(default `std`) selects the display adapter, and `vm config usb-use-xhci`
(default `true`) selects an xHCI rather than EHCI USB controller, which also
decides whether `vm hotplug` can attach USB 3.0 drives.

`vm config serial-ports <n>` creates serial ports that appear as
`/dev/ttyS<n>` in a Linux guest and as Unix sockets `serial<n>` in the
instance directory; `vm serial <name>` reads a bounded snapshot from one.
`vm config virtio-ports <n or names>` does the same with virtio-serial ports,
exposed as `/dev/virtio-ports/<name>` in the guest. The miniccc backchannel
uses its own virtio-serial port and is enabled by `vm config backchannel`
(default `true`); see [Command and control](cc.md).

For anything the configuration does not cover, `vm config qemu-append`
appends raw arguments to the QEMU command line, and
`vm config qemu-override <match> <replacement>` rewrites parts of the
generated command line before QEMU starts.

### What happens at launch

1. The VM is created with a copy of the configuration and an instance
   directory under the base path.
2. If snapshot mode is on, a qcow2 overlay is created for each disk.
3. Network taps are created and attached to their bridges; bonds are built.
4. The QEMU command line is generated and `qemu-override` rules are applied.
5. QEMU is started. If the binary is not an absolute path it is looked up on
   `PATH`.
6. minimega connects to the QMP socket. If the connection fails, QEMU is killed
   and the VM enters `ERROR` with QEMU's output in the `error` tag.
7. The VNC shim is connected (not for bare-metal VMs).

The VM stays in `BUILDING` until you `vm start` it.

### QMP access

`vm qmp <name> '<json>'` sends any QMP command to a VM's monitor and returns
the JSON reply. minimega uses the same socket for start, stop, save,
screenshots, hotplug, and CD-ROM changes, so `vm qmp` is for the rare cases
where you need something QEMU offers but minimega does not expose:

```minimega
minimega$ vm qmp web1 '{ "execute": "query-status" }'
{"return":{"running":true,"singlestep":false,"status":"running"}}
```

One practical use is persisting the writes of a VM that was launched in
snapshot mode. `query-block` lists the block devices; the first IDE disk is
`ide0-hd0`, and `block-commit` merges its overlay back into the backing image:

```minimega
minimega$ vm qmp web1 '{ "execute": "query-block" }'
minimega$ vm qmp web1 '{ "execute": "block-commit", "arguments": { "device": "ide0-hd0" } }'
minimega$ vm qmp web1 '{ "execute": "query-block-jobs" }'
```

`query-block-jobs` returns an empty list once the commit has finished.

!!! warning
    `block-commit` writes into the backing image behind minimega's back. If any
    other VM is running from the same image, or the image is a backing file
    for other snapshots, this can corrupt them. Prefer `vm save` or the `disk`
    API (see [Disk images and the disk API](disk-images.md)) when you want a
    reusable result.

The [QEMU QMP reference](https://www.qemu.org/docs/master/interop/qemu-qmp-ref.html)
documents the available commands.

### Trusted Platform Module

A KVM VM can be given a virtual TPM through a socket provided by
[swtpm](https://github.com/stefanberger/swtpm). Start one swtpm instance per
VM, each with its own state directory and socket, then point the VM at it:

```bash
$ mkdir -p /var/lib/swtpm/web1
$ swtpm socket --tpm2 --tpmstate dir=/var/lib/swtpm/web1 \
    --ctrl type=unixio,path=/var/lib/swtpm/web1/swtpm-sock
```

```minimega
minimega$ vm config tpm-socket /var/lib/swtpm/web1/swtpm-sock
minimega$ vm launch kvm web1
```

Omit `--tpm2` for a TPM 1.2 device. The guest sees a normal TPM and can be
configured to use it (BitLocker, measured boot, virtual smart cards, and so
on). A software TPM does not offer the guarantees of a hardware module. The
[QEMU TPM documentation](https://www.qemu.org/docs/master/specs/tpm.html)
describes the device in detail.

### Copy and paste

Two clipboard modes are available over VNC. By default, text pasted into the
VNC client's clipboard is typed into the guest as keystrokes; nothing can be
copied out. With

```minimega
minimega$ vm config bidirectional-copy-paste true
```

the clipboard is shared in both directions through a SPICE vdagent channel.
This needs:

- QEMU 6.1 or newer built with the `qemu-vdagent` character device
  (`kvm -chardev help` should list `qemu-vdagent`). minimega checks both at
  launch and refuses to launch the VM if either is missing.
- The SPICE guest agent inside the VM: the `spice-vdagent` package on Linux,
  or the [Windows vdagent](https://www.spice-space.org/download.html) on
  Windows, which relies on the same virtio-serial device as miniccc.

Bare-metal VMs cannot use bidirectional copy and paste. See [VNC](vnc.md) for
the VNC side of this.

### Bare-metal firmware

`vm config baremetal true` turns a KVM VM into a minimal QEMU machine for
firmware and RTOS images whose board has no PC hardware. It removes the VNC
display, VGA, USB, CD-ROM, keyboard, RTC, PCI bridge, and virtio backchannel
devices that minimega normally adds. QMP lifecycle control, PID tracking,
kernel loading, serial sockets, and tap networking remain.

A bare-metal VM must set `vm config kernel` and `vm config backchannel false`,
and must request any serial ports explicitly. It cannot use disks, a CD-ROM,
saved state, a TPM socket, virtio-serial ports, or bidirectional copy and
paste; minimega rejects the configuration at launch otherwise. Serial ports
map to the board's native UARTs and are exposed as `serial<n>` sockets in the
instance directory.

Network interfaces use QEMU's board-oriented `-net nic,model=...` form rather
than a PCI device, so the driver must be implemented by both the selected
machine and the firmware. QEMU does not report board-integrated controllers
through its device discovery, so a networked bare-metal VM must name the model
with `vm config baremetal-network-driver` before it appears in the netspec.

An Arm Cortex-M3 image built for QEMU's MPS2 AN385 board, for example:

```minimega
vm config qemu /usr/bin/qemu-system-arm
vm config machine mps2-an385
vm config cpu cortex-m3
vm config memory 16
vm config vcpus 1
vm config kernel /absolute/path/RTOSDemo.out
vm config baremetal true
vm config baremetal-network-driver lan9118
vm config backchannel false
vm config serial-ports 1
vm config networks 378,52:54:00:12:34:ad,lan9118
vm launch kvm freertos-mps2
vm start freertos-mps2
vm serial freertos-mps2
```

The firmware's first UART is `serial0` in the instance directory, and
`vm serial` reads from it. There is no framebuffer, so no VNC port is created
and `vnc_port` is `0`. QEMU's own stdout and stderr go to `qemu.log` in the
instance directory. Firmware that uses Arm semihosting can enable it with:

```minimega
vm config qemu-append -semihosting -semihosting-config enable=on,target=native
```

## Containers

`container` VMs are full-system Linux containers built into minimega rather
than an external runtime. Each one gets its own PID, network, mount, and IPC
namespaces, a chroot into a root filesystem, cgroup limits, and a reduced set
of root capabilities. A container needs at minimum a root filesystem
(`vm config filesystem`) and an executable inside it to run as PID 1
(`vm config init`, default `/init`, which may be a shell script). The
filesystem must contain the usual top-level directories (`/dev`, `/proc`,
`/sys`, and so on).

The repository ships two ways to build suitable filesystems: busybox-based
ones from `misc/uminiccc/build.bash` and `misc/uminirouter/build.bash`, and
vmbetter configurations `misc/vmbetter_configs/miniccc_container.conf` and
`misc/vmbetter_configs/minirouter_container.conf` (see
[Building images with vmbetter](vmbetter.md)).

### Host requirements

- A kernel with overlayfs, which minimega uses for snapshot mode. Linux 3.18
  or newer has it built in.
- The cgroup **v1** controllers `freezer`, `memory`, `devices`, and `cpu`,
  each mounted as its own hierarchy under the cgroup mount point (`-cgroup`,
  default `/sys/fs/cgroup`; `MM_CGROUP` in `/etc/minimega/minimega.conf` for
  the packaged service). At first launch minimega creates a `minimega`
  subtree in each of them and enables `cgroup.clone_children` and
  `memory.use_hierarchy`, which only exist in v1.

The unified cgroup v2 hierarchy is not supported. Hosts that boot with only
cgroup v2 (the default on current Debian, Ubuntu, and RHEL-family releases)
fail the first container launch with `cgroups are not initialized, cannot
continue`. Boot such hosts with `systemd.unified_cgroup_hierarchy=0` on the
kernel command line to get the legacy hierarchy back (see the
[systemd kernel command line options](https://www.freedesktop.org/software/systemd/man/latest/systemd.html)).
Debian-family kernels also need `cgroup_enable=memory` on the command line
before the memory controller is available; add both to
`GRUB_CMDLINE_LINUX_DEFAULT` in `/etc/default/grub`, run `update-grub`, and
reboot.

### Container fields

| Field | Purpose |
|---|---|
| `filesystem` | Path to the root filesystem. Required. |
| `init` | Program and arguments to exec as PID 1, relative to the filesystem root. Default `/init`. |
| `preinit` | Program run as root *before* isolation is applied (after namespaces and mounts, before cgroups, capabilities, and the chroot). Use it for one-off privileged setup such as enabling IP forwarding. It must exit before the container can start. |
| `hostname` | Hostname set before `init` runs. Defaults to the VM name. |
| `fifos` | Number of named pipes shared with the host: `fifo<n>` in the instance directory, `/dev/fifos/fifo<n>` inside. |
| `volume <target> <source>` | Bind-mount a host directory into the container. Repeat for several volumes; the same target overwrites an earlier one. |
| `snapshot` | With the default `true`, the filesystem is mounted through an overlay and changes are discarded. With `false`, the container writes to the filesystem directly, and no other container may use it. |

`networks` uses the same netspec as KVM; the device driver field is ignored
because containers get `veth` pairs. `memory` and `vcpus` become cgroup
limits.

### What happens at launch

1. On the first container launch, the cgroup subtrees are created.
2. The instance directory is created and, in snapshot mode, an overlayfs
   mount is set up inside it.
3. Pipes for the container's stdio and for the pre-`init` handshake are
   created.
4. A shim (a copy of minimega with special arguments) starts in fresh
   namespaces, mounts `/dev`, `/proc`, `/sys`, and any volumes, sets the
   hostname, runs `preinit`, applies cgroups and capabilities, and chroots
   into the filesystem.
5. minimega creates the `veth` pairs inside the new network namespace and
   attaches the host ends to Open vSwitch.
6. The shim is frozen with the freezer cgroup and the VM is left in
   `BUILDING`.

`vm start` thaws the freezer and the shim execs `init`. `vm stop` freezes the
whole container again. Each container exposes a console on a local TCP port
(the `console_port` column of `vm info`), which miniweb uses for its browser
terminal. A miniccc client inside a container talks to minimega over a Unix
socket (`cc` in the container's root) rather than the virtio-serial port KVM
guests use; see [Command and control](cc.md).

### Running many containers

Every container is a process tree on the host, so limits on open files and
processes are reached long before memory runs out. The packaged systemd unit
(`misc/daemon/minimega.service`) raises both:

```text
LimitNOFILE=1024000
LimitNPROC=4096000
```

If you start minimega by hand, raise them in the shell first
(`ulimit -n 1024000 -u 4096000`). Hosts that run hundreds of containers may
also need more inotify instances, for example
`sysctl -w fs.inotify.max_user_instances=8192`.

## Android virtual machines

`android` VMs run the official Android emulator under KVM. minimega reserves a
console/ADB port pair and a gRPC port for each, builds the emulator command
line, starts it with the configured SDK environment, and manages it over QMP
like a KVM VM. miniweb provides a browser console with live display, input,
hardware buttons, GPS, and screenshots through the gRPC port.

An Android VM needs `vm config android-avd` (an AVD created with `avdmanager`
or Android Studio) and discoverable `emulator` and `adb` binaries, which
`android-sdk`, `android-emulator`, `android-adb`, and `android-avd-dir` can
override. Network interfaces use the `virtio-net-pci` driver and appear in the
guest as extra, unconfigured interfaces. A host can run at most 64 Android VMs
at once, and `vm save` is not supported for them. Everything else, including
port assignment, the web console, and troubleshooting, is in
[Android VMs](android.md).

## See also

- [VM lifecycle](vm-lifecycle.md)
- [VM configuration reference](vm-config-reference.md)
- [Android VMs](android.md)
- [Disk images and the disk API](disk-images.md), [Creating VMs from install
  media](newvm.md), [Building images with vmbetter](vmbetter.md), and
  [Windows guests](windows.md)
- [Saving and restoring experiments](save-restore.md)
- Reference: [`vm launch`](../reference/minimega.md#vm-launch),
  [`vm qmp`](../reference/minimega.md#vm-qmp),
  [`vm serial`](../reference/minimega.md#vm-serial),
  [`vm config baremetal`](../reference/minimega.md#vm-config-baremetal),
  [`vm config tpm-socket`](../reference/minimega.md#vm-config-tpm-socket),
  [`vm config bidirectional-copy-paste`](../reference/minimega.md#vm-config-bidirectional-copy-paste),
  [`vm config filesystem`](../reference/minimega.md#vm-config-filesystem)
