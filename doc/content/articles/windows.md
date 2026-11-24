# Windows guests

Windows runs under minimega like any other KVM guest. Three things need
attention that Linux images handle on their own. The first is the virtio
drivers, which give the guest a fast serial channel, disk, and network. The
second is running miniccc as a Windows service. The third is getting files into
an NTFS image. This page covers these topics in order and ends with the problems
that people most often have.

It assumes that you already installed Windows into a qcow2 image, as [Creating
VMs from install media](newvm.md) describes. It also assumes that you have a
`miniccc.exe` built from the same minimega release as your server. A source
build with `scripts/build.bash` produces `bin/miniccc.exe`.

!!! note "Verified on an older release"
    The driver, service, and injection steps on this page were originally
    verified on Windows 7. This page describes them for Windows 10 and 11, where
    the mechanisms are the same. They were not re-run on a current Windows image
    yet. If you find a difference, please report it.

## Configuring the VM

This configuration installs a Windows 10 or 11 guest into a new image. If you
already have an installed image, do not run `disk create`, because it overwrites
`win-base.qc2`. Also leave out `vm config cdrom`, because the CD starts the
installer:

```minimega
minimega$ disk create qcow2 win-base.qc2 64G
minimega$ vm config disks win-base.qc2
minimega$ vm config cdrom windows.iso
minimega$ vm config snapshot false
minimega$ vm config memory 4096
minimega$ vm config vcpus 2
minimega$ vm config machine q35
minimega$ vm config vga std
minimega$ vm config usb-use-xhci true
minimega$ vm launch kvm win
minimega$ vm start win
```

- `vm config machine` selects the QEMU machine type. minimega leaves it at
  QEMU's default. `q35` is the modern PCI Express chipset. People usually
  install current Windows releases on `q35`. Run `qemu-system-x86_64 -M help`
  for the list.
- `vm config vga` defaults to `std`, which works with the Windows installer
  and the built-in display driver. `cirrus` is the fallback for very old
  installers.
- `vm config usb-use-xhci` defaults to `true`. This gives the guest an xHCI (USB
  3.0) controller, which Windows 8 and later support natively. To get an EHCI
  controller on older releases, set it to `false`.
- `vm config bidirectional-copy-paste true` shares the clipboard between your
  VNC viewer and the guest. It requires QEMU 6.1 or newer on the host, built
  with the `qemu-vdagent` chardev. It also requires the SPICE guest agent in the
  VM. The virtio-win guest tools below install this agent. Without this setting,
  pasting into the guest still works in one direction.
- Windows 11's installer checks for a TPM and UEFI firmware. minimega can attach
  a software TPM. To do this, run a TPM emulator such as `swtpm` on the host.
  Then point `vm config tpm-socket` at its socket. minimega does not select
  firmware. Pass an OVMF image through `vm config qemu-append`. For example, use
  `vm config qemu-append -bios /usr/share/ovmf/OVMF.fd` on Debian with the
  `ovmf` package installed.

For the installation, leave the disk on the default `ide` interface. Leave the
network on the default `e1000` driver. Windows has drivers for both. minimega
attaches only one CD at a time, so Windows cannot load the virtio drivers during
setup. If you want the extra performance, switch to `virtio` after you install
the drivers.

## Installing the virtio drivers

miniccc talks to minimega over a virtio-serial port. Windows does not know
about this port until you install the driver. The Fedora project publishes the
drivers as the
[virtio-win ISO](https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso).
Older builds are in the
[archive](https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/archive-virtio/).
Put the ISO in the files directory. After Windows is installed, replace the
installation ISO with it:

```minimega
minimega$ vm cdrom change win /tmp/minimega/files/virtio-win.iso
```

Inside the guest, run `virtio-win-guest-tools.exe` from the CD. It installs
every virtio driver and the SPICE guest agent in one pass. The drivers are
`vioserial` for the serial port, `viostor` for a virtio disk, and `NetKVM` for a
virtio NIC.

To install only the serial driver, follow these steps:

1. Open Device Manager.
2. Find the device listed as **PCI Simple Communications Controller**.
3. Choose *Update driver*.
4. Browse the CD.

The ISO is organized as `<driver>\<Windows version>\<architecture>`, so the
serial driver for 64-bit Windows 11 is under `vioserial\w11\amd64`.

When you are done, eject the CD:

```minimega
minimega$ vm cdrom eject win
```

## Running miniccc as a service

On Windows, miniccc registers itself as a service. Copy `miniccc.exe` into the
guest. The [next section](#injecting-files-into-the-image) shows how to do this
without a network. Then run these commands from a command prompt that you open
as administrator:

```text
C:\> mkdir C:\miniccc
C:\> C:\miniccc\miniccc.exe -install auto-start -logfile C:\miniccc\miniccc.log -level info
C:\> sc start miniccc
```

`-install` creates a service named `miniccc` (display name "minimega Agent").
The service runs `miniccc.exe -serial \\.\Global\cc` with the `-logfile` and
`-level` that you gave at install time. The service restarts five seconds after
a failure. `-install` also registers an event log source. `auto-start` starts
the service at boot. `manual-start` (or any other value) registers the service
without starting it automatically.

`\\.\Global\cc` is the Windows name of the virtio-serial port that minimega
creates for the VM's backchannel (`vm config backchannel`, on by default). On
Linux, the same port is `/dev/virtio-ports/cc`.

The installed service always uses the serial port. To connect over TCP with
`-parent` instead, register the service yourself. Use
[`sc create`](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/sc-create)
with the command line that you want.

Files sent with `cc send` go under the `-path` of miniccc. `-path` defaults to
`/tmp/miniccc`, which resolves to `\tmp\miniccc` on the system drive. To change
it, pass `-path C:\miniccc` in the service command line.

The older approach of starting a batch file from Task Scheduler still works.
`-install` supersedes this approach and also gives you automatic restarts.

One behavior is specific to the Windows client. When the serial connection
drops, Windows returns *Access denied* on any attempt to reopen the port from
the same process. For example, when minimega restarts or the VM is saved and
restored, the connection drops. Therefore, miniccc exits, and the recovery
action of the service starts a fresh copy. If you ran `miniccc.exe`
interactively, you must restart it by hand.

While minimega is unreachable at start-up, every miniccc, not only the Windows
client, retries for up to two hours (480 attempts, fifteen seconds apart). Then
miniccc gives up.

miniccc reports the VM's SMBIOS UUID. QEMU takes the UUID from `vm config uuid`.
If you do not set it, the value is random per launch. Cloned images therefore do
not collide in `cc clients`. Sysprep does not affect the UUID.

Once the service is running, the usual `cc` commands work. Filter on the OS
and run commands through `cmd`:

```minimega
minimega$ cc filter os=windows
minimega$ cc exec cmd /c "ver"
minimega$ cc mount win /tmp/minimega/winmnt
```

`cc mount` serves the guest's filesystem over the command and control
connection. On Windows, it exposes the system drive. The mount point then shows
items such as `Windows` and `Users` at its root. For `cc mount`, nothing on the
host needs to understand NTFS. This is different from `disk inject`.

## Injecting files into the image

With the VM shut down, `disk inject` mounts a partition of the image on the host
and copies files in. This is the simplest way to place `miniccc.exe` before the
first boot. Windows creates more than one partition, so you must say which
partition holds the system volume.

On a BIOS/MBR installation, the system volume is normally partition 2. Partition
1 is the small system-reserved volume. On a UEFI/GPT installation, the system
volume is normally partition 3.

Destination paths are relative to the root of that partition. They use forward
slashes and cannot contain a colon:

```minimega
minimega$ disk inject win-base.qc2:2 files /tmp/minimega/files/miniccc.exe:miniccc/miniccc.exe
```

If you are unsure of the layout, attach the image with `qemu-nbd`. Then list it
with `fdisk -l /dev/nbd0`, as [Disk images and the disk API](disk-images.md)
describes.

Writing to NTFS depends on `ntfs-3g` on the host. The minimega packages depend
on `ntfs-3g` (from EPEL on RHEL-family systems). If the mount of the kernel
fails, `disk inject` falls back to `ntfs-3g` automatically.

Windows leaves the filesystem marked dirty after hibernation. If Fast Startup is
enabled, Windows also leaves it marked dirty after a normal shutdown. `ntfs-3g`
then refuses to mount the filesystem read-write. Before you inject, disable Fast
Startup in the power options. Or shut down with `shutdown /s /t 0` from an
administrator prompt.

## Networking notes

The `net.ifnames` discussion for Linux guests does not apply. Windows names
adapters in detection order, for example `Ethernet` and `Ethernet 2`, regardless
of the PCI slot. The default `e1000` NIC needs no extra driver. A `virtio` NIC
(`vm config net 100,virtio-net-pci`) needs the `NetKVM` driver from the
virtio-win ISO. DHCP from a host `dnsmasq` works as for any other guest. See
[Host networking](networking.md).

## Preparing an image for cloning

Before you turn an installed image into a base for many VMs, run Sysprep with
the *generalize* option
([Microsoft's instructions](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/sysprep--generalize--a-windows-installation)).
Clones then do not share a machine identity. Shut down cleanly. After that,
launch with the default `vm config snapshot true`. With this setting, you
can boot as many copies as you need without modifying the image.

## Common problems

| Symptom | Likely cause |
|---|---|
| The service starts but `cc clients` never lists the VM | The `vioserial` driver is not installed, or a service you registered yourself with `sc create` omits `-serial \\.\Global\cc` (`-install` always passes it). Read `C:\miniccc\miniccc.log`. |
| Commands run but responses are wrong or missing | `miniccc.exe` is from a different minimega release than the server. minimega logs `mismatched miniccc version` at connect time. |
| `disk inject` fails with a mount error | Wrong partition number, `ntfs-3g` missing, or a dirty NTFS volume from Fast Startup or hibernation. |
| The guest shows a blank or garbled display | Try `vm config vga cirrus` for the installer. Then return to `std` after the display driver is installed. |
| `cc exec` reports the executable is not found | Wrap the command in `cmd /c` so that the shell resolves it. |
| A restored VM's service is stopped | This is expected. When the serial port drops, the service exits, and Windows restarts it after five seconds. |

## See also

- [Creating VMs from install media](newvm.md)
- [Disk images and the disk API](disk-images.md)
- [Command and control](cc.md)
- [VNC](vnc.md) and [miniweb](miniweb.md) for the console
- Reference: [`vm config machine`](../reference/minimega.md#vm-config-machine),
  [`vm config vga`](../reference/minimega.md#vm-config-vga),
  [`vm config usb-use-xhci`](../reference/minimega.md#vm-config-usb-use-xhci),
  [`vm config bidirectional-copy-paste`](../reference/minimega.md#vm-config-bidirectional-copy-paste),
  [`vm config tpm-socket`](../reference/minimega.md#vm-config-tpm-socket),
  [`vm cdrom`](../reference/minimega.md#vm-cdrom)
