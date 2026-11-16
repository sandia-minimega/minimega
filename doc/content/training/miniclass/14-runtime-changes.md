# Chapter 14: Runtime changes

Real machines get unplugged, slow links, CDs and USB sticks. In this lab you do
all four to running VMs without restarting anything. You pull the cable of the
router to `net_right` and plug it back in with `vm net`. You degrade the link to
`vm-left` with `qos`. You swap an ISO in and out of the CD drive of `vm-right`
with `vm cdrom`. You attach a disk image as a USB drive with `vm hotplug`.

Prerequisites: Part I, in particular [Chapter 3](03-router-sandwich.md),
[Chapter 4](04-vms.md) for VM targets, [Chapter 6](06-networking.md) for
netspecs and VLANs, and [Chapter 8](08-cc.md) for `cc`.

## Rebuild the sandwich

Start from the sandwich with fixed client addresses, as in
[chapter 11](11-traffic.md#rebuild-the-sandwich-with-fixed-addresses). `vm-left`
is `10.0.0.10`, and `vm-right` is `10.0.1.10`. The script at the end rebuilds
the sandwich. A ping from `vm-left` is the baseline for everything that follows.
Give the commands a prefix, so that their responses are easy to find:

```minimega
minimega[sandwich]$ cc filter name=vm-left
minimega[sandwich]$ cc prefix baseline
minimega[sandwich]$ cc exec ping -c 3 10.0.1.10
minimega[sandwich]$ shell sleep 10
minimega[sandwich]$ cc responses baseline
```

Three replies, each after a few milliseconds at most.

## Unplug the router

Interfaces are addressed by their zero-based position in `vm config networks` of
the VM, so the `net_right` side of the router is interface 1. A disconnect
removes the port from the bridge. The interface stays on the VM, and the guest
keeps its address and routes:

```minimega
minimega[sandwich]$ vm net disconnect router 1
minimega[sandwich]$ .annotate false .columns name,vlan vm info
name     | vlan
router   | [net_left (101), disconnected]
vm-left  | [net_left (101)]
vm-right | [net_right (102)]
```

Now the ping has nowhere to go past the router. `-W 1` limits each attempt to
one second. minimega records the exit code of a foreground command, so
`cc exitcode <id> vm-left` reports `1` where the baseline reported `0`:

```minimega
minimega[sandwich]$ cc prefix unplugged
minimega[sandwich]$ cc exec ping -c 3 -W 1 10.0.1.10
minimega[sandwich]$ shell sleep 10
minimega[sandwich]$ cc responses unplugged
```

Plug it back in by naming the VLAN to connect the interface to. The same command
moves an interface to a different VLAN. With a fourth argument, it moves the
interface to another bridge. You use this to model a machine that moves between
switch ports:

```minimega
minimega[sandwich]$ vm net connect router 1 net_right
```

The ping works again. `vm net add <vm> <netspec>` hot-plugs a new interface into
the running VM. It uses the same netspec syntax as `vm config networks`. The
guest sees the interface as a new NIC.

## Shape the link

`qos add` applies Linux traffic control to the host side of one VM interface.
`delay` adds latency, `loss` drops a percentage of packets, and `rate` caps
bandwidth in `kbit`, `mbit` or `gbit`:

```minimega
minimega[sandwich]$ qos add vm-left 0 delay 100ms
minimega[sandwich]$ cc prefix delayed
minimega[sandwich]$ cc exec ping -c 3 10.0.1.10
```

Each reply now takes a little over 100 ms. Shaping applies to what the VM
*receives*, which is the egress side of its host tap. The requests of `vm-left`
leave at full speed, and only the replies that come back to it are delayed. For
this reason, the round trip grows by one delay, not two. To slow both directions
of a link, shape the interface at each end.

`loss` and `delay` are both netem parameters, and they stack. The argument to
`loss` is a percentage:

```minimega
minimega[sandwich]$ qos add vm-left 0 loss 25
minimega[sandwich]$ .annotate false .columns name,qos vm info
name     | qos
router   | []
vm-left  | [0: loss 25 delay 100ms]
vm-right | []
```

A longer ping, `ping -c 20 10.0.1.10`, now reports roughly a quarter of the
replies missing. `rate` uses a different queue and cannot coexist with the other
two. `qos add vm-left 0 rate 1 mbit` replaces the delay and loss, with a warning
in the log. If you add loss or delay again, minimega removes the rate. Clear one
interface or all of them:

```minimega
minimega[sandwich]$ clear qos vm-left all
```

## Swap the CD

Every KVM VM that is not a bare-metal guest gets an IDE CD-ROM drive. The drive
is empty, unless `vm config cdrom` named an ISO at launch. In that case, the ISO
is also the boot device. `vm cdrom change` inserts a new medium, and
`vm cdrom eject` removes it, both on a running VM.

You need an ISO to insert. Any ISO works. To make a small one on the host from a
directory, use `genisoimage` or `xorriso -as mkisofs`. Use the tool that your
distribution packages. Put the ISO in the files directory and give its absolute
path, because the path must be valid on the host where minimega runs:

```minimega
minimega[sandwich]$ vm cdrom change vm-right /tmp/minimega/files/lab.iso
minimega[sandwich]$ .annotate false .columns name,cdrom vm info
name     | cdrom
router   |
vm-left  |
vm-right | /tmp/minimega/files/lab.iso
minimega[sandwich]$ vm cdrom eject vm-right
```

`vm cdrom change` first ejects whatever was inserted. If the guest locked the
tray, add `force` to either command. Inside the guest, the medium is on the
first IDE CD drive, `/dev/sr0`. The client image does not run udev, so it does
not load the IDE and CD-ROM drivers by itself. You can need to `modprobe` them
before the device appears.

## Plug in a USB drive

`vm hotplug add` attaches a disk image to a running KVM VM as a USB mass storage
device. Make an empty image with the `disk` API, which writes into the files
directory. Attach the image by absolute path:

```minimega
minimega[sandwich]$ disk create raw usb.img 64M
minimega[sandwich]$ vm hotplug add vm-right /tmp/minimega/files/usb.img
minimega[sandwich]$ .annotate false vm hotplug
name     | id | file                       | version
vm-right | 0  | /tmp/minimega/files/usb.img | 1.1
```

Without a version, the drive lands on the USB 1.1 bus. `2.0` and `3.0` put the
drive on the xHCI controller that VMs get by default (`vm config usb-use-xhci`,
which is `true` unless you change it). If you set it to `false`, the VM has an
EHCI controller instead. Then `2.0` goes there, and `3.0` is refused.

`serial <string>` before the version sets the serial number that the guest sees.
This is useful for udev rules. The guest side is the same as for the CD. The
disk appears as `/dev/sda` on a diskless client after the USB host controller
and `usb-storage` drivers are loaded.

Remove drives by the id in the listing, or all at once:

```minimega
minimega[sandwich]$ vm hotplug remove vm-right 0
minimega[sandwich]$ vm hotplug remove vm-right all
```

Ids are per VM, one above the highest drive that is still attached. You can also
pass a real USB device on the host to the VM with `vm config qemu-append` and
the `usb-host` device of QEMU. That is a launch-time setting, not a hotplug.

## What you built

- A router whose `net_right` interface was unplugged and reconnected, with
  the ping that proved it each time.
- A shaped link into `vm-left`: 100 ms of delay and 25% loss, then cleared.
- An ISO inserted into and ejected from `vm-right`, and a 64 MB USB drive
  attached to and removed from it.

## Where to read more

- [Host networking](../../articles/networking.md#changing-interfaces-on-a-running-vm)
  for `vm net`, and [quality of service](../../articles/networking.md#quality-of-service).
- [VM lifecycle](../../articles/vm-lifecycle.md#changing-a-running-vm) for
  the list of everything you can change after launch.
- [Disk images](../../articles/disk-images.md#creating-and-inspecting-images)
  for `disk create` and the rest of the `disk` API.
- Reference: [`vm net`](../../reference/minimega.md#vm-net),
  [`qos`](../../reference/minimega.md#qos),
  [`clear qos`](../../reference/minimega.md#clear-qos),
  [`vm cdrom`](../../reference/minimega.md#vm-cdrom),
  [`vm config cdrom`](../../reference/minimega.md#vm-config-cdrom),
  [`vm hotplug`](../../reference/minimega.md#vm-hotplug),
  [`vm config usb-use-xhci`](../../reference/minimega.md#vm-config-usb-use-xhci),
  [`disk create`](../../reference/minimega.md#disk-create).

## Script

The script expects `/tmp/minimega/files/lab.iso` to exist. If it does not exist,
the two `vm cdrom` lines report an error, and the rest of the script continues.

```minimega title="14-runtime-changes.mm"
--8<-- "training/miniclass/scripts/14-runtime-changes.mm"
```

[Download this example](scripts/14-runtime-changes.mm){ download="14-runtime-changes.mm" }
