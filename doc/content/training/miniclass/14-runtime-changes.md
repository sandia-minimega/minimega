# Chapter 14: Runtime changes

Real machines get unplugged, slow links, CDs and USB sticks. In this lab you
do all four to running VMs without restarting anything: pull the router's
cable to `net_right` and plug it back in with `vm net`, degrade the link to
`vm_left` with `qos`, swap an ISO in and out of `vm_right`'s CD drive with
`vm cdrom`, and attach a disk image as a USB drive with `vm hotplug`.

Prerequisites: Part I, in particular [Chapter 3](03-router-sandwich.md),
[Chapter 4](04-vms.md) for VM targets, [Chapter 6](06-networking.md) for
netspecs and VLANs, and [Chapter 8](08-cc.md) for `cc`.

## Rebuild the sandwich

Start from the sandwich with fixed client addresses, as in
[chapter 11](11-traffic.md#rebuild-the-sandwich-with-fixed-addresses):
`vm_left` is `10.0.0.10`, `vm_right` is `10.0.1.10`. The script at the end
rebuilds it. A ping from `vm_left` is the yardstick for everything that
follows; give the commands a prefix so their responses are easy to find:

```minimega
minimega[sandwich]$ cc filter name=vm_left
minimega[sandwich]$ cc prefix baseline
minimega[sandwich]$ cc exec ping -c 3 10.0.1.10
minimega[sandwich]$ shell sleep 10
minimega[sandwich]$ cc responses baseline
```

Three replies, each well under a millisecond.

## Unplug the router

Interfaces are addressed by their zero-based position in the VM's
`vm config networks`, so the router's `net_right` side is interface 1.
Disconnecting it removes the port from the bridge; the interface stays on the
VM, and the guest keeps its address and routes:

```minimega
minimega[sandwich]$ vm net disconnect router 1
minimega[sandwich]$ .annotate false .columns name,vlan vm info
name     | vlan
router   | [net_left (101), disconnected]
vm_left  | [net_left (101)]
vm_right | [net_right (102)]
```

Now the ping has nowhere to go past the router. `-W 1` keeps each attempt to
one second, and the exit code of a foreground command is recorded, so
`cc exitcode <id> vm_left` reports `1` where the baseline reported `0`:

```minimega
minimega[sandwich]$ cc prefix unplugged
minimega[sandwich]$ cc exec ping -c 3 -W 1 10.0.1.10
minimega[sandwich]$ shell sleep 10
minimega[sandwich]$ cc responses unplugged
```

Plug it back in by naming the VLAN to connect the interface to. The same
command moves an interface to a different VLAN, or to another bridge with a
fourth argument, which is how you model a machine being moved between
switch ports:

```minimega
minimega[sandwich]$ vm net connect router 1 net_right
```

The ping works again. `vm net add <vm> <netspec>` goes one step further and
hot-plugs a new interface, using the same netspec syntax as
`vm config networks`; the guest sees it as a new NIC.

## Shape the link

`qos add` applies Linux traffic control to the host side of one VM interface.
`delay` adds latency, `loss` drops a percentage of packets, and `rate` caps
bandwidth in `kbit`, `mbit` or `gbit`:

```minimega
minimega[sandwich]$ qos add vm_left 0 delay 100ms
minimega[sandwich]$ cc prefix delayed
minimega[sandwich]$ cc exec ping -c 3 10.0.1.10
```

Each reply now takes a little over 100 ms. Shaping applies to what the VM
*receives*, the egress side of its host tap; `vm_left`'s requests leave at
full speed and only the replies coming back to it are delayed, which is why
the round trip grows by one delay rather than two. To slow both directions
of a link, shape the interface at each end.

`loss` and `delay` are both netem parameters and stack; the argument to
`loss` is a percentage:

```minimega
minimega[sandwich]$ qos add vm_left 0 loss 25
minimega[sandwich]$ .annotate false .columns name,qos vm info
name     | qos
router   | []
vm_left  | [0: delay 100ms loss 25]
vm_right | []
```

A longer ping, `ping -c 20 10.0.1.10`, now reports roughly a quarter of the
replies missing. `rate` uses a different queue and cannot coexist with the
other two: `qos add vm_left 0 rate 1 mbit` replaces the delay and loss, with
a warning in the log, and adding loss or delay again removes the rate. Clear
one interface or all of them:

```minimega
minimega[sandwich]$ clear qos vm_left all
```

## Swap the CD

Every KVM VM that is not a bare-metal guest gets an IDE CD-ROM drive, empty
unless `vm config cdrom` named an ISO at launch (in which case it is also the
boot device). `vm cdrom change` inserts a new medium and `vm cdrom eject`
removes it, both on a running VM. You need an ISO to insert; any one will
do, and a small one is quick to make on the host from a directory with
`genisoimage` or `xorriso -as mkisofs`, whichever your distribution
packages. Put it in the files directory and give its absolute path, since the
path is checked on the host where minimega runs:

```minimega
minimega[sandwich]$ vm cdrom change vm_right /tmp/minimega/files/lab.iso
minimega[sandwich]$ .annotate false .columns name,cdrom vm info
name     | cdrom
router   |
vm_left  |
vm_right | /tmp/minimega/files/lab.iso
minimega[sandwich]$ vm cdrom eject vm_right
```

`vm cdrom change` ejects whatever was inserted first. If the guest has
locked the tray, add `force` to either command. Inside the guest the medium
is on the first IDE CD drive, `/dev/sr0`; the client image does not run
udev, so it will not load the IDE and CD-ROM drivers by itself, and you may
need to `modprobe` them before the device appears.

## Plug in a USB drive

`vm hotplug add` attaches a disk image to a running KVM VM as a USB mass
storage device. Make an empty image with the `disk` API, which writes into
the files directory, and attach it by absolute path:

```minimega
minimega[sandwich]$ disk create raw usb.img 64M
minimega[sandwich]$ vm hotplug add vm_right /tmp/minimega/files/usb.img
minimega[sandwich]$ .annotate false vm hotplug
name     | id | file                       | version
vm_right | 0  | /tmp/minimega/files/usb.img | 1.1
```

Without a version the drive lands on the USB 1.1 bus. `2.0` and `3.0` put it
on the xHCI controller that VMs get by default (`vm config usb-use-xhci`,
`true` unless you change it; with `false` the VM has an EHCI controller
instead, `2.0` goes there and `3.0` is refused). `serial <string>` before the
version sets the serial number the guest sees, useful for udev rules. The
guest side is the same story as the CD: the disk appears as `/dev/sda` on a
diskless client once the USB host controller and `usb-storage` drivers are
loaded.

Remove drives by the id in the listing, or all at once:

```minimega
minimega[sandwich]$ vm hotplug remove vm_right 0
minimega[sandwich]$ vm hotplug remove vm_right all
```

Ids are per VM, one above the highest drive still attached. A real USB
device on the host can also be passed through with `vm config qemu-append`
and QEMU's `usb-host` device; that is a launch-time setting, not a hotplug.

## What you built

- A router whose `net_right` interface was unplugged and reconnected, with
  the ping that proved it each time.
- A shaped link into `vm_left`: 100 ms of delay and 25% loss, then cleared.
- An ISO inserted into and ejected from `vm_right`, and a 64 MB USB drive
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

The script expects `/tmp/minimega/files/lab.iso` to exist; the two
`vm cdrom` lines report an error and the rest continues if it does not.

```minimega title="14-runtime-changes.mm"
--8<-- "training/miniclass/scripts/14-runtime-changes.mm"
```

[Download this example](scripts/14-runtime-changes.mm){ download="14-runtime-changes.mm" }
