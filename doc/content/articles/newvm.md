# Creating VMs from install media

This page walks through installing an operating system from an ISO into a disk
image that then serves as a reusable base for many VMs, and through adding
miniccc to the result. Use it when you need a full distribution, vendor
software, or a desktop that the minimal images from
[vmbetter](vmbetter.md) do not provide. For Windows-specific driver and
service steps, continue with [Windows guests](windows.md).

You need a running minimega (see [Installation](installing.md)), an install
ISO in minimega's files directory (`/tmp/minimega/files` by default), and a
way to see the console: [miniweb](miniweb.md) or a [VNC](vnc.md) client.

## Create the disk and boot the installer

Create an empty qcow2 image, attach it and the ISO, and turn snapshot mode off
so the installer's writes land in the image rather than in a temporary
overlay:

```minimega
minimega$ disk create qcow2 debian-base.qc2 20G
minimega$ vm config disks debian-base.qc2
minimega$ vm config cdrom debian-13.iso
minimega$ vm config snapshot false
minimega$ vm launch kvm installer
minimega$ vm start installer
```

The qcow2 file starts small and grows as the guest writes; `20G` is only the
size the guest sees. When `vm config cdrom` is set, the CD is automatically the
boot device, so the installer comes up first. Adjust `vm config memory` and
`vm config vcpus` if the installer needs more than the defaults (2048 MB and
one vCPU).

Open the VM's console in miniweb and complete the installation as you would on
hardware. If the installer needs network access to fetch packages, launch
the VM with `vm config networks 100` and give VLAN 100 a tap, DHCP, and NAT as
described in [Host networking](networking.md).

When the installer asks to reboot, either eject the ISO so the guest boots from
disk, or shut down and relaunch without the `cdrom` setting:

```minimega
minimega$ vm cdrom eject installer
```

## Add miniccc

Most experiments want miniccc in the image so that the [command and
control](cc.md) API works. The simplest way is to shut the VM down and inject
the binary and a service unit straight into the disk image with
`disk inject`, which needs no network and no console:

```text title="miniccc.service"
[Unit]
Description=miniccc

[Service]
ExecStart=/usr/local/bin/miniccc -v=false -serial /dev/virtio-ports/cc -logfile /var/log/miniccc.log

[Install]
WantedBy=multi-user.target
```

```minimega
minimega$ disk inject debian-base.qc2 files /opt/minimega/bin/miniccc:/usr/local/bin/miniccc /tmp/minimega/files/miniccc.service:/etc/systemd/system/miniccc.service
```

systemd enables a unit through a symlink in
`/etc/systemd/system/multi-user.target.wants/`. Either boot the VM once more
with `snapshot false` and run `systemctl enable miniccc`, or inject the symlink
too: create it on the host with
`ln -s /etc/systemd/system/miniccc.service /tmp/miniccc.link` and add
`/tmp/miniccc.link:/etc/systemd/system/multi-user.target.wants/miniccc.service`
to the `files` list. `disk inject` picks the first partition when the image
has only one; otherwise name it with a suffix such as `debian-base.qc2:2`.
The full rules are in [Disk images and the disk API](disk-images.md).

If you would rather copy files into a running guest, the alternatives are:

- `vm hotplug add <vm> <image>` attaches a USB disk. Build one with
  `disk create raw usb.img 64M`, format it (`mkfs.vfat usb.img`), and put the
  files on it with `disk inject usb.img:none files ...`.
- A network path, when the VM has an interface: fetch from a web server on the
  host tap, or `scp` in.
- The [vmbetter](vmbetter.md) overlays, if you are building rather than
  installing.

Use the `miniccc` from the same minimega release as the server; mismatched
versions log a warning at connect time and may misbehave.

## Finish the image

Before shutting down for the last time:

- Remove package caches (`apt clean`) and anything else you do not want in
  every clone.
- Give clones distinct identities: clear `/etc/machine-id` and remove
  `/etc/ssh/ssh_host_*` so they regenerate on first boot. On Windows, run
  Sysprep instead (see [Windows guests](windows.md)).
- If the guest uses systemd's predictable interface names, its NIC may be
  `ens3` on one `vm config` and `enp0s4` on another because the name follows
  the PCI slot. Adding `net.ifnames=0` to the guest's kernel command line keeps
  it `eth0`.

Shut the guest down from inside using its own shutdown command and wait for
`vm info` to show state `QUIT` so the filesystem is consistent, then flush the
VM. Zero-filling free space and converting the image afterwards shrinks the
file; see [Disk images and the disk API](disk-images.md).

```minimega
minimega$ vm flush
```

The image is now a base. Snapshot mode is on by default, so any number of VMs
can share it without modifying it:

```minimega
minimega$ vm config disks debian-base.qc2
minimega$ vm launch kvm node[1-8]
minimega$ vm start all
```

To change the base later, launch it once more with `vm config snapshot false`,
make the change, and shut down cleanly. To keep the original untouched, take a
`disk snapshot` first and modify the snapshot instead.

## See also

- [Disk images and the disk API](disk-images.md)
- [Windows guests](windows.md)
- [Building images with vmbetter](vmbetter.md)
- [Command and control](cc.md)
- [Host networking](networking.md)
- Reference: [`vm config cdrom`](../reference/minimega.md#vm-config-cdrom),
  [`vm config disks`](../reference/minimega.md#vm-config-disks),
  [`vm config snapshot`](../reference/minimega.md#vm-config-snapshot),
  [`vm cdrom`](../reference/minimega.md#vm-cdrom),
  [`vm hotplug`](../reference/minimega.md#vm-hotplug)
