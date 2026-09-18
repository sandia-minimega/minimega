# Chapter 19: Specialty KVM guests

The KVM guests so far have been plain Linux machines booted from a kernel
and initrd. This chapter covers four things you reach for when a guest needs
more than that: a virtual TPM through swtpm, a shared clipboard in both
directions, direct access to QEMU's monitor with `vm qmp`, and the
bare-metal mode for firmware images that expect no PC hardware at all. It
finishes with where to go for Windows guests. At the end `vm_left` is
running with a TPM the guest can see, and you have queried QEMU about it
directly.

It assumes the sandwich from [Chapter 3](03-router-sandwich.md), the VNC
console from [Chapter 5](05-miniweb.md) and [Chapter 13](13-vnc.md), and
`cc` from [Chapter 8](08-cc.md). The TPM section needs the `swtpm` package
on the host.

## A virtual TPM

A Trusted Platform Module is what BitLocker, measured boot, virtual smart
cards, and the Windows 11 installer ask for. minimega attaches one through
[swtpm](https://github.com/stefanberger/swtpm), a software TPM emulator that
exposes a control socket QEMU connects to. Each VM needs its own swtpm
instance with its own state directory and socket. Start one from a host
shell:

```bash
$ sudo mkdir -p /tmp/minimega/swtpm/vm_left
$ sudo swtpm socket --tpm2 --tpmstate dir=/tmp/minimega/swtpm/vm_left \
      --ctrl type=unixio,path=/tmp/minimega/swtpm/vm_left/swtpm-sock &
```

or, as the chapter script does, from minimega with `background`, which runs
a host program and returns at once:

```minimega
minimega$ shell mkdir -p /tmp/minimega/swtpm/vm_left
minimega$ background swtpm socket --tpm2 --tpmstate dir=/tmp/minimega/swtpm/vm_left --ctrl type=unixio,path=/tmp/minimega/swtpm/vm_left/swtpm-sock
Started background process with id 1
```

`--tpm2` gives a TPM 2.0 device; leave it out for TPM 1.2. Then point the
VM at the socket before launching it. Set the field for `vm_left` only and
clear it before launching `vm_right`, since two VMs must not share a
socket:

```minimega
minimega$ vm config tpm-socket /tmp/minimega/swtpm/vm_left/swtpm-sock
minimega$ vm launch kvm vm_left
minimega$ clear vm config tpm-socket
```

minimega adds `-tpmdev emulator` and a `tpm-tis` device to QEMU's command
line. The path shows up in `vm info`:

```minimega
minimega$ .columns name,state,tpm-socket vm info
host  | name     | state   | tpm-socket
node1 | router   | RUNNING |
node1 | vm_left  | RUNNING | /tmp/minimega/swtpm/vm_left/swtpm-sock
node1 | vm_right | RUNNING |
```

Inside the guest the device is an ordinary `/dev/tpm0` once the `tpm_tis`
driver is loaded, and the Debian kernel in the vmbetter image has the
driver:

```minimega
minimega$ cc filter name=vm_left
minimega$ cc exec modprobe tpm_tis
minimega$ cc exec ls -l /dev/tpm0
minimega$ cc responses all
```

A software TPM gives none of the guarantees of a hardware one, which is fine
for an experiment that needs the interface rather than the trust. Bare-metal
VMs (below) cannot have one. The swtpm process outlives the VM; find its PID
with `background-status` and kill it when you are done.

## Copy and paste in both directions

Over VNC, every KVM guest accepts pasted text: whatever you put in the noVNC
clipboard in miniweb is typed into the guest as keystrokes. Nothing comes
back out. With

```minimega
minimega$ vm config bidirectional-copy-paste true
```

the clipboard is synchronised in both directions through a SPICE vdagent
channel: pasting puts text on the guest's clipboard, and copying in the
guest updates the noVNC clipboard. Two things have to be in place, and
minimega checks the first at launch and refuses to start the VM if it is
missing:

- QEMU 6.1 or newer with the `qemu-vdagent` character device on the host.
  `kvm -chardev help` lists it if you have it; recent Debian and Ubuntu
  packages generally do.
- The SPICE guest agent in the VM: the `spice-vdagent` package on Linux, or
  the vdagent from [spice-space.org](https://www.spice-space.org/download.html)
  on Windows, which the virtio-win guest tools install. The miniccc image
  from Chapter 2 does not include it, which is why the chapter script leaves
  this field alone; add `spice-vdagent` to the packages of a vmbetter
  configuration that inherits from `miniccc.conf` if you want it in your
  clients.

The one-way mode needs nothing in the guest, which is why it is the default
and why it works on a bare console. Bare-metal VMs can use neither.

## Talking to QEMU directly

minimega manages each KVM VM through QEMU's monitor protocol, QMP:
starting, stopping, saving, screenshots, hotplug, and CD-ROM changes are all
QMP commands underneath. `vm qmp` sends any QMP command you write yourself
and returns QEMU's JSON reply:

```minimega
minimega$ vm qmp vm_left '{ "execute": "query-status" }'
{"return":{"running":true,"singlestep":false,"status":"running"}}
minimega$ vm stop vm_left
minimega$ vm qmp vm_left '{ "execute": "query-status" }'
{"return":{"running":false,"singlestep":false,"status":"paused"}}
minimega$ vm start vm_left
```

The reply for a VM that is launched but not yet started reports
`"status": "prelaunch"`, which is what the `BUILDING` state means to QEMU.
`query-block` lists the disks and their snapshot overlays, `query-version`
tells you which QEMU you are running, and the full command set is in the
[QEMU QMP reference](https://www.qemu.org/docs/master/interop/qemu-qmp-ref.html).

!!! warning
    `vm qmp` goes behind minimega's back. Commands that change the VM, such
    as `block-commit` to write a snapshot's changes into its backing image,
    can corrupt an image that other VMs are using. Prefer `vm save` and the
    `disk` API for anything you want to keep; see
    [Virtual machine types](../../articles/vmtypes.md#qmp-access).

## Bare-metal firmware guests

`vm config baremetal true` turns a KVM VM into a minimal QEMU machine for
firmware and RTOS images built for a board with no PC hardware. minimega
leaves out VNC, VGA, USB (including the tablet), CD-ROM, RTC, the PCI
bridge, and the miniccc backchannel, and keeps QMP control, kernel loading,
serial sockets, and tap networking. Such a VM must set `vm config kernel` and
`vm config backchannel false`, must ask for its serial ports explicitly, and
cannot use disks, saved state, a TPM, virtio ports, or copy and paste.

Because the network controller is part of the board rather than a PCI
device, you also name it with `vm config baremetal-network-driver` before
using it in the netspec, and you usually select a non-x86 QEMU binary with
`vm config qemu` and a board with `vm config machine`. The firmware's first
UART becomes `serial0` in the instance directory, and `vm serial <name>`
reads a bounded snapshot of it, since there is no VNC console:

```minimega
minimega$ vm serial freertos
```

[Virtual machine types](../../articles/vmtypes.md#bare-metal-firmware) has
a complete example for an Arm Cortex-M3 image on QEMU's MPS2 AN385 board.
There is no script for this section because it needs a firmware image built
for a specific board.

## Windows guests

Windows is a KVM guest like any other, with three extra steps: the virtio
drivers, so the guest has the serial channel miniccc uses; miniccc installed
as a Windows service with `miniccc.exe -install`; and `disk inject` to get
files into the NTFS image before first boot. The TPM above is what the
Windows 11 installer wants, together with UEFI firmware passed through
`vm config qemu-append`. [Windows guests](../../articles/windows.md) walks
through all of it, starting from an image installed as in
[Creating VMs from install media](../../articles/newvm.md).

## The script

The script rebuilds the sandwich with the TPM on `vm_left`, queries QEMU,
and looks for the device in the guest.

```minimega title="19-specialty-guests.mm"
--8<-- "training/miniclass/scripts/19-specialty-guests.mm"
```

[Download this example](scripts/19-specialty-guests.mm){ download="19-specialty-guests.mm" }

## What you built

- `vm_left` with a TPM 2.0 device backed by swtpm, visible inside the guest
  as `/dev/tpm0`.
- A QMP conversation with QEMU through `vm qmp`, and an understanding of
  what minimega itself does over that socket.
- The requirements for bidirectional copy and paste and for bare-metal
  firmware guests, and where the Windows guide picks up.

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
