# Windows guests

Windows runs under minimega like any other KVM guest, but three things need
attention that Linux images handle on their own: the virtio drivers that give
the guest a fast serial channel, disk, and network; running miniccc as a
Windows service; and getting files into an NTFS image. This page covers those
in order and finishes with the problems people most often hit.

It assumes you have already installed Windows into a qcow2 image by following
[Creating VMs from install media](newvm.md), and that you have a `miniccc.exe`
built from the same minimega release as your server (a source build with
`scripts/build.bash` produces `bin/miniccc.exe`).

!!! note "Verified on an older release"
    The driver, service, and injection steps on this page were originally
    verified on Windows 7 and are written here for Windows 10 and 11, where
    the mechanisms are the same. They have not yet been re-run on a current
    Windows image; if you find a difference, please report it.

## Configuring the VM

A reasonable configuration for a Windows 10 or 11 guest:

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
  QEMU's default; `q35` is the modern PCI Express chipset and is what current
  Windows releases are usually installed on. Run `qemu-system-x86_64 -M help`
  for the list.
- `vm config vga` defaults to `std`, which works with the Windows installer
  and the built-in display driver. `cirrus` is the fallback for very old
  installers.
- `vm config usb-use-xhci` defaults to `true`, giving the guest an xHCI (USB
  3.0) controller, which Windows 8 and later support natively. Set it to
  `false` for an EHCI controller on older releases.
- `vm config bidirectional-copy-paste true` shares the clipboard between your
  VNC viewer and the guest. It requires QEMU 6.1 or newer built with the
  `qemu-vdagent` chardev on the host and the SPICE guest agent in the VM,
  which the virtio-win guest tools below install. Without it, pasting into the
  guest still works in one direction.
- Windows 11's installer checks for a TPM and UEFI firmware. minimega can
  attach a software TPM: run a TPM emulator such as `swtpm` on the host and
  point `vm config tpm-socket` at its socket. It does not select firmware;
  pass an OVMF image through `vm config qemu-append`, for example
  `vm config qemu-append -bios /usr/share/ovmf/OVMF.fd` on Debian with the
  `ovmf` package installed.

Leave the disk on the default `ide` interface and the network on the default
`e1000` driver for the installation. Windows has drivers for both, and
minimega attaches only one CD at a time, so the virtio drivers cannot be loaded
during setup. Switch to `virtio` after installing the drivers if you want the
extra performance.

## Installing the virtio drivers

miniccc talks to minimega over a virtio-serial port, which Windows does not
know about until you install the driver. The drivers are published by the
Fedora project as the
[virtio-win ISO](https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso)
(older builds are in the
[archive](https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/archive-virtio/)).
Put it in the files directory and swap it in for the installation ISO once
Windows is installed:

```minimega
minimega$ vm cdrom change win virtio-win.iso
```

Inside the guest, run `virtio-win-guest-tools.exe` from the CD. It installs
every virtio driver (`vioserial` for the serial port, `viostor` for a virtio
disk, `NetKVM` for a virtio NIC) and the SPICE guest agent in one pass. If you
prefer to install only the serial driver, open Device Manager, find the
device listed as **PCI Simple Communications Controller**, choose *Update
driver*, and browse the CD; the ISO is organised as
`<driver>\<Windows version>\<architecture>`, so the serial driver for 64-bit
Windows 11 is under `vioserial\w11\amd64`.

Eject the CD when you are done:

```minimega
minimega$ vm cdrom eject win
```

## Running miniccc as a service

On Windows, miniccc registers itself as a service. Copy `miniccc.exe` into the
guest (the [next section](#injecting-files-into-the-image) shows how to do
that without a network), then from an elevated command prompt:

```text
C:\> mkdir C:\miniccc
C:\> C:\miniccc\miniccc.exe -install auto-start -logfile C:\miniccc\miniccc.log -level info
C:\> sc start miniccc
```

`-install` creates a service named `miniccc` (display name "minimega Agent")
that runs `miniccc.exe -serial \\.\Global\cc` with the `-logfile` and `-level`
you gave at install time, configured to restart five seconds after a failure,
and registers an event log source. `auto-start` starts it at boot;
`manual-start` (or any other value) registers it without starting it
automatically.
`\\.\Global\cc` is the Windows name of the virtio-serial port minimega creates
for the VM's backchannel (`vm config backchannel`, on by default); on Linux
the same port is `/dev/virtio-ports/cc`.

The installed service always uses the serial port. To connect over TCP with
`-parent` instead, register the service yourself with
[`sc create`](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/sc-create)
and the command line you want. Files sent with `cc send` land under miniccc's
`-path`, which defaults to `/tmp/miniccc` and therefore resolves to
`\tmp\miniccc` on the system drive; pass `-path C:\miniccc` in the service
command line to change it.

The older approach of starting a batch file from Task Scheduler still works
but is superseded by `-install`, which also gets you automatic restarts.

Two behaviours are specific to the Windows client:

- When the serial connection drops, for instance because minimega restarted or
  the VM was saved and restored, Windows returns *Access denied* on any attempt
  to reopen the port from the same process. miniccc therefore exits, and the
  service's recovery action starts a fresh copy. If you ran `miniccc.exe`
  interactively, you have to restart it by hand.
- While minimega is unreachable at start-up, miniccc retries for up to two
  hours (480 attempts, fifteen seconds apart) before giving up.

miniccc reports the VM's SMBIOS UUID, which QEMU takes from `vm config uuid`
(random per launch unless you set it). Cloned images therefore do not collide
in `cc clients`, and Sysprep does not affect it.

Once the service is running, the usual `cc` commands work. Filter on the OS
and run commands through `cmd`:

```minimega
minimega$ cc filter os=windows
minimega$ cc exec cmd /c "ver"
minimega$ cc mount win /tmp/minimega/winmnt
```

`cc mount` serves the guest's filesystem over the command and control
connection; on Windows it exposes the system drive, so the mount point shows
`Windows`, `Users`, and so on at its root. Nothing on the host needs to
understand NTFS for this to work, in contrast to `disk inject`.

## Injecting files into the image

With the VM shut down, `disk inject` mounts a partition of the image on the
host and copies files in, which is the simplest way to place `miniccc.exe`
before the first boot. Windows creates more than one partition, so you must
say which one holds the system volume. On a BIOS/MBR installation it is
normally partition 2 (partition 1 is the small system-reserved volume); on a
UEFI/GPT installation it is normally partition 3. Destination paths are
relative to the root of that partition, use forward slashes, and cannot
contain a colon:

```minimega
minimega$ disk inject win-base.qc2:2 files /tmp/minimega/files/miniccc.exe:miniccc/miniccc.exe
```

If you are unsure of the layout, attach the image with `qemu-nbd` and list it
with `fdisk -l /dev/nbd0`, as described in
[Disk images and the disk API](disk-images.md). Writing to NTFS depends on
`ntfs-3g` on the host; the minimega packages depend on it (from EPEL on
RHEL-family systems), and `disk inject` falls back to it automatically when
the kernel's own mount fails.

Windows leaves the filesystem marked dirty after a normal shutdown when Fast
Startup is enabled, and after hibernation, and `ntfs-3g` then refuses to
mount it read-write. Disable Fast Startup in the power options, or shut down
with `shutdown /s /t 0` from an administrator prompt, before injecting.

## Networking notes

The `net.ifnames` discussion for Linux guests does not apply: Windows names
adapters `Ethernet`, `Ethernet 2`, and so on in detection order regardless of
the PCI slot. The default `e1000` NIC needs no extra driver. A `virtio` NIC
(`vm config networks 100,virtio`) needs the `NetKVM` driver from the virtio-win
ISO. DHCP from a host `dnsmasq` works as for any other guest; see
[Host networking](networking.md).

## Preparing an image for cloning

Before you turn an installed image into a base for many VMs, run Sysprep with
the *generalize* option
([Microsoft's instructions](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/sysprep--generalize--a-windows-installation))
so clones do not share a machine identity, and shut down cleanly. Then launch
with the default `vm config snapshot true` to boot as many copies as you need
without modifying the image.

## Common problems

| Symptom | Likely cause |
|---|---|
| The service starts but `cc clients` never lists the VM | The `vioserial` driver is not installed, or a service you registered yourself with `sc create` omits `-serial \\.\Global\cc` (`-install` always passes it); check `C:\miniccc\miniccc.log`. |
| Commands run but responses are wrong or missing | `miniccc.exe` is from a different minimega release than the server; minimega logs `mismatched miniccc version` at connect time. |
| `disk inject` fails with a mount error | Wrong partition number, `ntfs-3g` missing, or a dirty NTFS volume from Fast Startup or hibernation. |
| The guest shows a blank or garbled display | Try `vm config vga cirrus` for the installer, then return to `std` once the display driver is installed. |
| `cc exec` reports the executable is not found | Wrap the command in `cmd /c` so the shell resolves it. |
| A restored VM's service is stopped | Expected; the service exits when the serial port drops and Windows restarts it after five seconds. |

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
