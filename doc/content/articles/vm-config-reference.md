# VM configuration reference

This page lists every `vm config` field grouped by the VM type it applies to,
with its type, default, and a one-line meaning, followed by the columns that
`vm info` reports. It is maintained by hand and mirrors the `vm config` help
text and the `vmconfiger` code generator that produces most of the handlers;
when in doubt, the generated [command reference](../reference/minimega.md) is
authoritative. Read [VM lifecycle](vm-lifecycle.md) for how the configuration
is used and [Virtual machine types](vmtypes.md) for what each type does with
it.

## Using the fields

- `vm config <field>` prints the current value; `vm config <field> <value>`
  sets it. `vm config` alone prints the whole template.
- `clear vm config <field>` restores the default listed below;
  `clear vm config` restores all of them. Saved configurations
  (`vm config save`) are not affected.
- The parser accepts any unambiguous prefix of a keyword, so `vm config net`
  and `vm config disk` are accepted short forms of `networks` and `disks`.
  The canonical names are the ones shown here and in the generated reference.
- `state` also answers to its old name `migrate`.
- Relative paths for images, kernels, filesystems, and sockets are resolved
  under the files directory (`-filepath`). Absolute paths are used as is, and
  a `file:` prefix fetches the file from another host first (see
  [File management](file.md)). The Android host-tool paths are the exception;
  they are interpreted on the host as given.
- Fields of one type are ignored when launching another type.

## All VM types

| Field | Type | Default | Meaning | Reference |
|---|---|---|---|---|
| `memory` | integer (MB) | `2048` | Memory to allocate. | [`vm config memory`](../reference/minimega.md#vm-config-memory) |
| `vcpus` | integer | `1` | Number of virtual CPUs. | [`vm config vcpus`](../reference/minimega.md#vm-config-vcpus) |
| `networks` | list of netspecs | none | Interfaces, one netspec each: `<bridge>,<VLAN>,<MAC>,<driver>,<qinq>` with only the VLAN (number or alias) required. | [`vm config networks`](../reference/minimega.md#vm-config-networks) |
| `bonds` | list of bondspecs | none | Bonds of configured interfaces: `<interface indexes>,<bond mode>,<lacp mode>,<no-lacp-fallback>,<qinq>,<bond name>`. | [`vm config bonds`](../reference/minimega.md#vm-config-bonds) |
| `snapshot` | boolean | `true` | Run from a temporary overlay so writes are discarded; lets many VMs share one image or filesystem. | [`vm config snapshot`](../reference/minimega.md#vm-config-snapshot) |
| `uuid` | string | random at launch | System UUID. When set, only one VM can be launched from the template. | [`vm config uuid`](../reference/minimega.md#vm-config-uuid) |
| `backchannel` | boolean | `true` | Enable the miniccc command-and-control channel. | [`vm config backchannel`](../reference/minimega.md#vm-config-backchannel) |
| `tags <key> [value]` | map | empty | Tags applied to every VM launched from the template; `clear vm config tag <key>` removes one. | [`vm config tags`](../reference/minimega.md#vm-config-tags) |
| `schedule` | hostname | none | Host the scheduler must place the VM on. Cannot be combined with `colocate`. | [`vm config schedule`](../reference/minimega.md#vm-config-schedule) |
| `colocate` | VM name | none | Place the VM on the same host as another launched or queued VM. Cannot be combined with `schedule`. | [`vm config colocate`](../reference/minimega.md#vm-config-colocate) |
| `coschedule` | integer | `-1` | Maximum number of other VMs on the same host; `0` means alone, `-1` no limit. Namespace scheduling only. | [`vm config coschedule`](../reference/minimega.md#vm-config-coschedule) |

## KVM

| Field | Type | Default | Meaning | Reference |
|---|---|---|---|---|
| `disks` | list of diskspecs | none | Disks to attach: `<path>[,<interface>[,<cache>]]`. Interfaces: `ahci`, `ide` (default), `scsi`, `sd`, `mtd`, `floppy`, `pflash`, `virtio`. Cache modes: `none`, `writeback`, `unsafe`, `directsync`, `writethrough`; default `unsafe` in snapshot mode, `writeback` otherwise. | [`vm config disks`](../reference/minimega.md#vm-config-disks) |
| `cdrom` | path | none | CD-ROM image; becomes the boot device. | [`vm config cdrom`](../reference/minimega.md#vm-config-cdrom) |
| `kernel` | path | none | Kernel image; boots from it instead of any disk. | [`vm config kernel`](../reference/minimega.md#vm-config-kernel) |
| `initrd` | path | none | Initrd passed with the kernel. | [`vm config initrd`](../reference/minimega.md#vm-config-initrd) |
| `append` | list of strings | none | Kernel command line; requires `kernel`. | [`vm config append`](../reference/minimega.md#vm-config-append) |
| `state` (alias `migrate`) | path | none | Saved memory state from `vm save` to resume from; needs the matching disk, kernel, or CD-ROM. | [`vm config`](../reference/minimega.md#vm-config) |
| `qemu` | path or name | `kvm` | QEMU binary to run; looked up on `PATH` unless absolute. | [`vm config qemu`](../reference/minimega.md#vm-config-qemu) |
| `cpu` | string | `host` | CPU model; must be one the configured QEMU binary lists with `-cpu help`. | [`vm config cpu`](../reference/minimega.md#vm-config-cpu) |
| `sockets` | integer | `0` | CPU sockets; `0` lets QEMU derive it from `vcpus`, `cores`, and `threads`. | [`vm config sockets`](../reference/minimega.md#vm-config-sockets) |
| `cores` | integer | `0` | Cores per socket; `0` lets QEMU derive it. | [`vm config cores`](../reference/minimega.md#vm-config-cores) |
| `threads` | integer | `0` | Threads per core; `0` lets QEMU derive it. | [`vm config threads`](../reference/minimega.md#vm-config-threads) |
| `machine` | string | QEMU default | Machine type; must be one the configured QEMU binary lists with `-M help`. | [`vm config machine`](../reference/minimega.md#vm-config-machine) |
| `vga` | string | `std` | Emulated graphics adapter; `std` or `cirrus` suit most guests. | [`vm config vga`](../reference/minimega.md#vm-config-vga) |
| `serial-ports` | integer | `0` | Serial ports, exposed as `serial<n>` sockets in the instance directory. | [`vm config serial-ports`](../reference/minimega.md#vm-config-serial-ports) |
| `virtio-ports` | integer or names | none | Virtio-serial ports, by count or as a comma-separated list of names. | [`vm config virtio-ports`](../reference/minimega.md#vm-config-virtio-ports) |
| `usb-use-xhci` | boolean | `true` | Use an xHCI USB controller (USB 3.0 capable) instead of EHCI. | [`vm config usb-use-xhci`](../reference/minimega.md#vm-config-usb-use-xhci) |
| `tpm-socket` | path | none | Unix socket of a software TPM (swtpm) to attach. | [`vm config tpm-socket`](../reference/minimega.md#vm-config-tpm-socket) |
| `bidirectional-copy-paste` | boolean | `false` | Share the clipboard both ways through a SPICE vdagent; needs QEMU 6.1+ with `qemu-vdagent` and the guest agent. | [`vm config bidirectional-copy-paste`](../reference/minimega.md#vm-config-bidirectional-copy-paste) |
| `qemu-append` | list of strings | none | Extra arguments appended to the QEMU command line. | [`vm config qemu-append`](../reference/minimega.md#vm-config-qemu-append) |
| `qemu-override <match> <replacement>` | list of pairs | none | Rewrite parts of the generated QEMU command line; applied in order. | [`vm config qemu-override`](../reference/minimega.md#vm-config-qemu-override) |
| `baremetal` | boolean | `false` | Launch a bare-metal firmware machine; see the [bare metal](#bare-metal) group. | [`vm config baremetal`](../reference/minimega.md#vm-config-baremetal) |
| `baremetal-network-driver` | string | none | Board-integrated NIC model to accept in netspecs; only used when `baremetal` is on. | [`vm config baremetal-network-driver`](../reference/minimega.md#vm-config-baremetal-network-driver) |

## Container

| Field | Type | Default | Meaning | Reference |
|---|---|---|---|---|
| `filesystem` | path | none (required) | Root filesystem directory to run in. | [`vm config filesystem`](../reference/minimega.md#vm-config-filesystem) |
| `init` | program and arguments | `/init` | Program to exec as PID 1, relative to the filesystem root. | [`vm config init`](../reference/minimega.md#vm-config-init) |
| `preinit` | program | none | Program run as root before isolation (cgroups, capabilities, chroot) is applied; must exit before the VM can start. | [`vm config preinit`](../reference/minimega.md#vm-config-preinit) |
| `hostname` | string | VM name | Hostname set before `init` runs. | [`vm config hostname`](../reference/minimega.md#vm-config-hostname) |
| `fifos` | integer | `0` | Named pipes shared with the host: `fifo<n>` in the instance directory, `/dev/fifos/fifo<n>` in the container. | [`vm config fifos`](../reference/minimega.md#vm-config-fifos) |
| `volume <target> <source>` | map | empty | Host directory to bind-mount at `<target>` inside the container; a repeated target replaces the earlier mapping. | [`vm config volume`](../reference/minimega.md#vm-config-volume) |

`networks` applies to containers too; the driver part of the netspec is
ignored because containers use `veth` pairs.

## Android

All Android fields start with `android-`. The tool and directory paths are
host paths, not files-directory paths.

| Field | Type | Default | Meaning | Reference |
|---|---|---|---|---|
| `android-avd` | string | none (required) | Name of the AVD to boot. | [`vm config android-avd`](../reference/minimega.md#vm-config-android-avd) |
| `android-sdk` | host path | none | Android SDK root. | [`vm config android-sdk`](../reference/minimega.md#vm-config-android-sdk) |
| `android-emulator` | host path or name | `emulator` on `PATH` | Emulator binary. | [`vm config android-emulator`](../reference/minimega.md#vm-config-android-emulator) |
| `android-adb` | host path or name | `adb` on `PATH` | adb binary. | [`vm config android-adb`](../reference/minimega.md#vm-config-android-adb) |
| `android-avd-dir` | host path | none | Directory holding `<avd-name>.avd`. | [`vm config android-avd-dir`](../reference/minimega.md#vm-config-android-avd-dir) |
| `android-no-window` | boolean | `true` | Run without a local emulator window. | [`vm config android-no-window`](../reference/minimega.md#vm-config-android-no-window) |
| `android-console-base-port` | integer | `0` | Preferred first console port to try; even, 5554 to 5680, or `0` for the start of the range. The ADB port is console + 1. | [`vm config android-console-base-port`](../reference/minimega.md#vm-config-android-console-base-port) |
| `android-grpc-base-port` | integer | `0` | Preferred first gRPC port to try; 8554 to 8617, or `0` for the start of the range. | [`vm config android-grpc-base-port`](../reference/minimega.md#vm-config-android-grpc-base-port) |
| `android-extra-args` | list of strings | none | Raw arguments appended to the emulator command line. | [`vm config android-extra-args`](../reference/minimega.md#vm-config-android-extra-args) |
| `android-writable-system` | boolean | `false` | Request a writable system partition. | [`vm config android-writable-system`](../reference/minimega.md#vm-config-android-writable-system) |

Android VMs also honour the common fields and, for the backend QEMU, the KVM
`disks` and `snapshot` behaviour.

## Bare metal

Bare-metal firmware guests are KVM VMs with two extra switches and a stricter
set of rules:

| Field | Type | Default | Meaning | Reference |
|---|---|---|---|---|
| `baremetal` | boolean | `false` | Drop the PC devices (display, VNC, USB, CD-ROM, keyboard, RTC, PCI bridge, backchannel) and launch a firmware-only machine. | [`vm config baremetal`](../reference/minimega.md#vm-config-baremetal) |
| `baremetal-network-driver` | string | none | NIC model built into the board (for example `lan9118` on `mps2-an385`), which QEMU does not report through device discovery. Required when a bare-metal VM has any `networks`. | [`vm config baremetal-network-driver`](../reference/minimega.md#vm-config-baremetal-network-driver) |

With `baremetal true`, `kernel` is required, `backchannel` must be `false`,
and `serial-ports` must be set explicitly for any UART you want. `disks`,
`cdrom`, `state`, `tpm-socket`, `virtio-ports`, and
`bidirectional-copy-paste` are rejected. `qemu`, `machine`, `cpu`, `memory`,
`vcpus`, `networks`, `append`, and `qemu-append` are used as for any KVM VM.

## `vm info` columns

`vm info` prints these columns in this order. `.columns` selects a subset,
`vm info summary` prints only the ones marked with `*`, and the `host` column
that appears first in the table comes from the `.annotate` builtin rather than
from `vm info`. Type-specific columns show `N/A` for other types.

| Column | Applies to | Meaning |
|---|---|---|
| `id`* | all | Per-host VM ID. |
| `name`* | all | VM name. |
| `state`* | all | `BUILDING`, `RUNNING`, `PAUSED`, `QUIT`, or `ERROR`. |
| `uptime` | all | Time since launch. |
| `type`* | all | `kvm`, `container`, or `android`. |
| `uuid`* | all | System UUID. |
| `cc_active`* | all | Whether a miniccc client is connected. |
| `pid` | all | PID of QEMU, the container's init, or the emulator. |
| `vlan`* | all | VLAN of each interface, or `disconnected`. |
| `bridge` | all | Bridge of each interface. |
| `tap` | all | Host tap name of each interface. |
| `mac` | all | MAC address of each interface. |
| `ip` | all | IPv4 address of each interface, learned from traffic. |
| `ip6` | all | IPv6 address of each interface, learned from traffic. |
| `qos` | all | Quality-of-service settings on each interface. |
| `qinq` | all | Interfaces and bonds in dot1q-tunnel mode, with their outer VLAN. |
| `bond` | all | Bonds and the taps they contain. |
| `memory` | all | Memory in MB. |
| `vcpus` | all | Virtual CPUs. |
| `disks` | kvm | Configured diskspecs. |
| `snapshot` | all | Whether snapshot mode is on. |
| `initrd` | kvm | Initrd image. |
| `kernel` | kvm | Kernel image. |
| `cdrom` | kvm | CD-ROM image. |
| `save` | kvm | Save information. |
| `append` | kvm | Kernel command line. |
| `serial-ports` | kvm | Number of serial ports. |
| `virtio-ports` | kvm | Virtio-serial ports. |
| `vnc_port` | kvm | Port of the VNC shim (`0` for bare-metal VMs). |
| `usb-use-xhci` | kvm | USB controller selection. |
| `tpm-socket` | kvm | TPM socket path. |
| `bidirectional-copy-paste` | kvm | Whether bidirectional clipboard is on. |
| `android_avd` | android | AVD name. |
| `android_console_port` | android | Emulator console port. |
| `android_adb_port` | android | ADB port (console + 1). |
| `android_serial` | android | adb serial, for example `emulator-5554`. |
| `android_grpc_port` | android | Emulator gRPC control port. |
| `filesystem` | container | Root filesystem. |
| `hostname` | container | Hostname. |
| `init` | container | Init program and arguments. |
| `preinit` | container | Preinit program. |
| `fifo` | container | Number of fifos. |
| `volume` | container | Volume mappings. |
| `console_port` | container | TCP port of the console shim. |
| `tags` | all | Tags as a JSON object. |

## See also

- [VM lifecycle](vm-lifecycle.md)
- [Virtual machine types](vmtypes.md)
- [Android VMs](android.md)
- [Host networking](networking.md) for netspecs, bonds, and `vm net`
- Reference: [`vm config`](../reference/minimega.md#vm-config),
  [`clear vm config`](../reference/minimega.md#clear-vm-config),
  [`vm info`](../reference/minimega.md#vm-info)
