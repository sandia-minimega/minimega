# Chapter 19: Specialty KVM guests

Until now, the KVM guests were plain Linux machines that boot from a kernel
and initrd. This chapter covers four features for a guest that needs more
than that:

- A virtual TPM through swtpm.
- A shared clipboard in both directions.
- Direct access to the QEMU monitor with `vm qmp`.
- The bare-metal mode for firmware images that expect no PC hardware at all.

The chapter finishes with where to go for Windows guests. At the end,
`vm-left` runs with a TPM that the guest can see, and you queried QEMU about
it directly.

This chapter needs the sandwich from [Chapter 3](03-router-sandwich.md), the
VNC console from [Chapter 5](05-miniweb.md) and [Chapter 13](13-vnc.md), and
`cc` from [Chapter 8](08-cc.md). The TPM section needs the `swtpm` package
on the host.

## A virtual TPM

A Trusted Platform Module is what BitLocker, measured boot, virtual smart
cards, and the Windows 11 installer ask for. minimega attaches one through
[swtpm](https://github.com/stefanberger/swtpm), a software TPM emulator that
exposes a control socket that QEMU connects to. Each VM needs its own swtpm
instance with its own state directory and socket. Start one from a host
shell:

```bash
$ sudo mkdir -p /tmp/minimega/swtpm/vm-left
$ sudo swtpm socket --tpm2 --tpmstate dir=/tmp/minimega/swtpm/vm-left \
      --ctrl type=unixio,path=/tmp/minimega/swtpm/vm-left/swtpm-sock &
```

Or start one from minimega with `background`, as the chapter script does.
`background` runs a host program and returns at once:

```minimega
minimega$ shell mkdir -p /tmp/minimega/swtpm/vm-left
minimega$ background swtpm socket --tpm2 --tpmstate dir=/tmp/minimega/swtpm/vm-left --ctrl type=unixio,path=/tmp/minimega/swtpm/vm-left/swtpm-sock
Started background process with id 1
```

`--tpm2` gives a TPM 2.0 device. For TPM 1.2, leave it out. Then point the
VM at the socket before you launch it. A VM that exists cannot get a TPM. Clear
the namespace and build the sandwich again as in chapter 3, as the chapter
script does. In that build, set the field for `vm-left` only. Clear it before
you launch `vm-right`, because two VMs must not share a socket:

```minimega
minimega$ vm config tpm-socket /tmp/minimega/swtpm/vm-left/swtpm-sock
minimega$ vm launch kvm vm-left
minimega$ clear vm config tpm-socket
```

minimega adds `-tpmdev emulator` and a `tpm-tis` device to the QEMU command
line. The path appears in `vm info`:

```minimega
minimega$ .columns name,state,tpm-socket vm info
host  | name     | state   | tpm-socket
node1 | router   | RUNNING |
node1 | vm-left  | RUNNING | /tmp/minimega/swtpm/vm-left/swtpm-sock
node1 | vm-right | RUNNING |
```

Inside the guest, the device is an ordinary `/dev/tpm0` after the guest
loads the `tpm_tis` driver. The Debian kernel in the vmbetter image has the
driver:

```minimega
minimega$ cc filter name=vm-left
minimega$ cc exec modprobe tpm_tis
minimega$ cc exec ls -l /dev/tpm0
minimega$ cc responses all
```

A software TPM gives none of the guarantees of a hardware TPM. This is fine
for an experiment that needs the interface rather than the trust. Bare-metal
VMs (below) cannot have a TPM. The swtpm process outlives the VM. When you
are done, find its PID with `background-status` and kill it.

## Copy and paste in both directions

Over VNC, every KVM guest accepts pasted text. Whatever you put in the noVNC
clipboard in miniweb is typed into the guest as keystrokes. Nothing comes
back out. With

```minimega
minimega$ vm config bidirectional-copy-paste true
```

the clipboard is synchronized in both directions through a SPICE vdagent
channel. Pasting puts text on the guest's clipboard, and copying in the
guest updates the noVNC clipboard. Two things must be in place. minimega
checks the first at launch. If the first is missing, minimega refuses to
start the VM:

- QEMU 6.1 or newer with the `qemu-vdagent` character device on the host.
  If you have it, `kvm -chardev help` lists it. Recent Debian and Ubuntu
  packages generally have it.
- The SPICE guest agent in the VM: the `spice-vdagent` package on Linux, or
  the vdagent from [spice-space.org](https://www.spice-space.org/download.html)
  on Windows. The virtio-win guest tools install the vdagent on Windows. The
  miniccc image from Chapter 2 does not include the agent, so the chapter
  script leaves this field alone. If you want the agent in your clients, add
  `spice-vdagent` to the packages of a vmbetter configuration that inherits
  from `miniccc.conf`.

The one-way mode needs nothing in the guest. So it is the default, and it
works on a bare console. Bare-metal VMs can use neither mode.

## Talking to QEMU directly

minimega manages each KVM VM through the monitor protocol of QEMU, QMP.
Starting, stopping, saving, screenshots, hotplug, and CD-ROM changes are all
QMP commands underneath. `vm qmp` sends any QMP command that you write
yourself and returns the JSON reply of QEMU:

```minimega
minimega$ vm qmp vm-left '{ "execute": "query-status" }'
{"return":{"running":true,"singlestep":false,"status":"running"}}
minimega$ vm stop vm-left
minimega$ vm qmp vm-left '{ "execute": "query-status" }'
{"return":{"running":false,"singlestep":false,"status":"paused"}}
minimega$ vm start vm-left
```

The reply for a VM that is launched but not yet started reports
`"status": "prelaunch"`. This is what the `BUILDING` state means to QEMU.
`query-block` lists the disks and their snapshot overlays. `query-version`
tells you which QEMU you are running. The full command set is in the
[QEMU QMP reference](https://www.qemu.org/docs/master/interop/qemu-qmp-ref.html).

!!! warning
    `vm qmp` bypasses minimega. Commands that change the VM can corrupt an
    image that other VMs are using. An example is `block-commit`, which
    writes the changes of a snapshot into its backing image. For anything
    that you want to keep, we recommend `vm save` and the `disk` API. See
    [Virtual machine types](../../articles/vmtypes.md#qmp-access).

## Bare-metal firmware guests

`vm config baremetal true` turns a KVM VM into a minimal QEMU machine for
firmware and RTOS images built for a board with no PC hardware. minimega
leaves out VNC, VGA, USB (including the tablet), CD-ROM, RTC, the PCI
bridge, and the miniccc backchannel. It keeps QMP control, kernel loading,
serial sockets, and tap networking. Such a VM must set `vm config kernel` and
`vm config backchannel false`. It must also ask for its serial ports
explicitly. It cannot use disks, saved state, a TPM, virtio ports, or copy
and paste.

The network controller is part of the board and not a PCI device. So you
also name the controller with `vm config baremetal-network-driver` before
you use it in the netspec. You usually select a non-x86 QEMU binary with
`vm config qemu` and a board with `vm config machine`. The first UART of the
firmware becomes `serial0` in the instance directory. `vm serial <name>`
reads a bounded snapshot of it, because there is no VNC console:

```minimega
minimega$ vm serial freertos
```

[Virtual machine types](../../articles/vmtypes.md#bare-metal-firmware) has
a complete example for an Arm Cortex-M3 image on QEMU's MPS2 AN385 board.
There is no script for this section because it needs a firmware image built
for a specific board.

## Windows guests

Windows is a KVM guest like any other, with three extra steps:

- Install the virtio drivers, so that the guest has the serial channel that
  miniccc uses.
- Install miniccc as a Windows service with `miniccc.exe -install`.
- Use `disk inject` to put files into the NTFS image before the first boot.

The Windows 11 installer wants the TPM above and UEFI firmware. You pass the
firmware through `vm config qemu-append`. [Windows guests](../../articles/windows.md)
describes all of it, starting from an image that you installed as in
[Creating VMs from install media](../../articles/newvm.md).

## The script

The script rebuilds the sandwich with the TPM on `vm-left`, queries QEMU,
and looks for the device in the guest.

```minimega title="19-specialty-guests.mm"
--8<-- "training/miniclass/scripts/19-specialty-guests.mm"
```

[Download this example](scripts/19-specialty-guests.mm){ download="19-specialty-guests.mm" }

## What you built

- `vm-left` with a TPM 2.0 device backed by swtpm, visible inside the guest
  as `/dev/tpm0`.
- A QMP conversation with QEMU through `vm qmp`, and an understanding of
  what minimega itself does over that socket.
- The requirements for bidirectional copy and paste and for bare-metal
  firmware guests, and where the Windows guide continues.

## Where to read more

- [Virtual machine types](../../articles/vmtypes.md): TPM, copy and paste,
  QMP, and the full bare-metal example.
- [Windows guests](../../articles/windows.md)
- [VNC](../../articles/vnc.md) for the clipboard in the VNC console.
- [Command line and scripting](../../articles/cli.md#host-processes-from-minimega)
  for `background`.
- Reference: [`vm config tpm-socket`](../../reference/minimega.md#vm-config-tpm-socket),
  [`vm config bidirectional-copy-paste`](../../reference/minimega.md#vm-config-bidirectional-copy-paste),
  [`vm config baremetal`](../../reference/minimega.md#vm-config-baremetal),
  [`vm config baremetal-network-driver`](../../reference/minimega.md#vm-config-baremetal-network-driver),
  [`vm qmp`](../../reference/minimega.md#vm-qmp),
  [`vm serial`](../../reference/minimega.md#vm-serial).
