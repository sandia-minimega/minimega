# Creating VMs from install media

This page explains how to install an operating system from an ISO into a disk
image. The image then serves as a reusable base for many VMs. The page also
explains how to add miniccc to the image. When you need a full distribution,
vendor software, or a desktop that the minimal images from
[vmbetter](vmbetter.md) do not provide, use this page. For Windows-specific
driver and service steps, continue with [Windows guests](windows.md).

You need a running minimega (see [Installation](installing.md)), an install
ISO in minimega's files directory (`/tmp/minimega/files` by default), and a
way to see the console. Use [miniweb](miniweb.md) or a [VNC](vnc.md) client.

## Create the disk and boot the installer

Create an empty qcow2 image. Attach the image and the ISO. Turn snapshot mode
off, so that the installer writes to the image and not to a temporary overlay:

```minimega
minimega$ disk create qcow2 debian-base.qc2 20G
minimega$ vm config disks debian-base.qc2
minimega$ vm config cdrom debian-13.iso
minimega$ vm config snapshot false
minimega$ vm launch kvm installer
minimega$ vm start installer
```

The qcow2 file starts small and grows as the guest writes. `20G` is only the
size that the guest sees. When `vm config cdrom` is set, the CD is
automatically the boot device, so the installer starts first. If the installer
needs more than the defaults (2048 MB and one vCPU), adjust `vm config memory`
and `vm config vcpus`.

Open the VM's console in miniweb. Complete the installation as you do on
hardware. If the installer needs network access to fetch packages, launch the
VM with `vm config net 100`. Then give VLAN 100 a tap, DHCP, and NAT, as
[Host networking](networking.md) describes.

When the installer asks to reboot, eject the ISO so that the guest boots from
disk. Or shut down the VM and launch it again without the `cdrom` setting. This
command ejects the ISO:

```minimega
minimega$ vm cdrom eject installer
```

## Add miniccc

Most experiments want miniccc in the image, so that the
[command and control](cc.md) API works. The simplest way is to shut down the
VM. Then use `disk inject` to put the binary and a service unit in the disk
image. This method needs no network and no console:

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
`/etc/systemd/system/multi-user.target.wants/`. To enable the unit, use one of
these methods:

- Boot the VM once more with `snapshot false` and run
  `systemctl enable miniccc`.
- Inject the symlink. Create it on the host with
  `ln -s /etc/systemd/system/miniccc.service /tmp/miniccc.link`. Then add
  `/tmp/miniccc.link:/etc/systemd/system/multi-user.target.wants/miniccc.service`
  to the `files` list.

If the image has only one partition, `disk inject` picks the first partition.
Otherwise, name the partition with a suffix such as `debian-base.qc2:2`. The
full rules are in [Disk images and the disk API](disk-images.md).

If you prefer to copy files into a running guest, use one of these methods:

- `vm hotplug add <vm> /tmp/minimega/files/usb.img` attaches a USB disk. The
  hotplug path is not relative to the files directory. Build the disk with
  `disk create raw usb.img 64M`. Then format it on the host with
  `mkfs.vfat /tmp/minimega/files/usb.img`. Then put the files on it with
  `disk inject usb.img:none files ...`.
- If the VM has an interface, use a network path. Fetch the files from a web
  server on the host tap, or copy them with `scp`.
- If you are building an image and not installing one, use the
  [vmbetter](vmbetter.md) overlays.

Use the `miniccc` from the same minimega release as the server. Mismatched
versions log a warning at connect time and can misbehave.

## Finish the image

Before shutting down for the last time:

- Remove package caches (`apt clean`) and anything else you do not want in
  every clone.
- Give clones distinct identities. Clear `/etc/machine-id`, so that the guest
  makes a new ID at first boot. Remove `/etc/ssh/ssh_host_*`. A Debian guest
  does not make new host keys at boot, so add a first-boot step that runs
  `ssh-keygen -A` before `sshd` starts. On Windows, run Sysprep instead (see
  [Windows guests](windows.md)).
- If the guest uses systemd's predictable interface names, its NIC can be
  `ens3` on one `vm config` and `enp0s4` on another. The name follows the PCI
  slot. To keep the NIC as `eth0`, add `net.ifnames=0` to the kernel command
  line of the guest.

Shut down the guest from inside with its own shutdown command. Wait for
`vm info` to show the state `QUIT`, so that the filesystem is consistent. Then
flush the VM. When you zero-fill the free space and then convert the image, the
file shrinks. See [Disk images and the disk API](disk-images.md).

```minimega
minimega$ vm flush
```

The image is now a base. The template still has the installer settings, so
reset the template first. Snapshot mode is then on again, and any number of VMs
can share the image without modifying it:

```minimega
minimega$ clear vm config
minimega$ vm config disks debian-base.qc2
minimega$ vm launch kvm node[1-8]
minimega$ vm start all
```

To change the base later, launch it again with `vm config snapshot false`.
Then make the change and shut down the VM cleanly. To keep the original
untouched, take a `disk snapshot` first. Then modify the snapshot instead.

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
