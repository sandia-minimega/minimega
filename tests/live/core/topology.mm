# minimega live integration test topology
#
# 22 KVM VMs across 4 VLANs, routed by two minirouter VMs that run DHCP, DNS,
# OSPF, and a firewall. The topology deliberately touches as many "vm config"
# options as possible so that a regression in any of them shows up as a failed
# launch or a failed assertion in tests/live/core.
#
#   lan-a 10.10.1.0/24  host-a1..a5  dynamic DHCP from rtr-ab
#   lan-b 10.10.2.0/24  host-b1..b5  static DHCP (by MAC) from rtr-ab; OSPF transit
#   lan-c 10.10.3.0/24  host-c1..c5  static DHCP from rtr-cd; disks, machine types
#   lan-d 10.10.4.0/24  host-d1..d5  static DHCP from rtr-cd; firewall, no-cc host
#
#   rtr-ab  lan-a .1, lan-b .1   (OSPF area 0, router id 1.1.1.1)
#   rtr-cd  lan-b .2, lan-c .1, lan-d .1   (OSPF area 0, router id 2.2.2.2)
#
# Requirements (staged by tests/live/run.sh):
#   - live.kernel and live.initrd from tests/live/image/build.sh in the
#     minimega files directory (-filepath), under live/
#   - the container hostname is "minimega" (used by "vm config schedule")
#
# Load it with:
#   mm read /tmp/minimega/files/live/topology.mm
#
# Remove everything it creates with:
#   clear namespace live

namespace live

# Queue VM launches and let the scheduler place them in one pass, the same
# way phenix and FIREWHEEL drive minimega.
ns queueing true

# VLAN aliases: pin lan-a to a known VLAN, let minimega allocate the rest.
vlans add lan-a 1001

# Scratch disk shared (in snapshot mode) by several lan-c hosts.
disk create qcow2 live/data.qcow2 64M

# --- common VM configuration -------------------------------------------------

clear vm config
vm config kernel live/live.kernel
vm config initrd live/live.initrd
vm config memory 384
vm config vcpus 1
vm config snapshot true
vm config tags topology live
vm config save live-base

# --- routers -----------------------------------------------------------------

vm config memory 448
vm config schedule minimega
vm config tags role router

vm config append live.role=router live.hostname=rtr-ab
vm config net lan-a,virtio-net-pci lan-b,virtio-net-pci
vm launch kvm rtr-ab

vm config append live.role=router live.hostname=rtr-cd
vm config net lan-b,virtio-net-pci lan-c,virtio-net-pci lan-d,virtio-net-pci
vm launch kvm rtr-cd

# --- lan-a: dynamic DHCP clients ---------------------------------------------

vm config restore live-base
vm config tags role client
vm config tags lan a
vm config net lan-a,virtio-net-pci

# host-a1: extra serial port with a heartbeat, a named virtio-serial port,
# and it must land on the same node as its router.
vm config append live.role=host live.hostname=host-a1 live.serial=1
vm config serial-ports 1
vm config virtio-ports live.port0
vm config colocate rtr-ab
vm launch kvm host-a1

# host-a2..a5: launched as a range from one configuration (hostnames are left
# at the guest default so miniccc reports "live-host").
vm config serial-ports 0
clear vm config virtio-ports
clear vm config colocate
vm config append live.role=host
vm launch kvm host-a[2-5]

# --- lan-b: static DHCP by MAC, mixed NIC drivers and SMP --------------------

vm config restore live-base
vm config memory 352
vm config tags role client
vm config tags lan b

vm config append live.role=host live.hostname=host-b1
vm config net lan-b,00:1f:53:00:02:11,e1000
vm launch kvm host-b1

vm config append live.role=host live.hostname=host-b2
vm config net lan-b,00:1f:53:00:02:12,e1000
vm launch kvm host-b2

vm config append live.role=host live.hostname=host-b3
vm config net lan-b,00:1f:53:00:02:13,virtio-net-pci
vm config vcpus 2
vm config cores 2
vm launch kvm host-b3

vm config vcpus 1
clear vm config cores
vm config append live.role=host live.hostname=host-b4
vm config net lan-b,00:1f:53:00:02:14,virtio-net-pci
vm launch kvm host-b4

vm config append live.role=host live.hostname=host-b5
vm config net lan-b,00:1f:53:00:02:15,virtio-net-pci
vm launch kvm host-b5

# --- lan-c: disks, QEMU passthrough, machine types ---------------------------

vm config restore live-base
vm config tags role server
vm config tags lan c

# host-c1: virtio data disk plus raw QEMU arguments (an SMBIOS serial the
# guest can read back).
vm config append live.role=host live.hostname=host-c1
vm config net lan-c,00:1f:53:00:03:11,virtio-net-pci
vm config disks live/data.qcow2,virtio
vm config qemu-append -smbios type=1,serial=LIVE-C1
vm launch kvm host-c1

# host-c2: the same base disk over IDE with an explicit cache mode.
clear vm config qemu-append
vm config append live.role=host live.hostname=host-c2
vm config net lan-c,00:1f:53:00:03:12,virtio-net-pci
vm config disks live/data.qcow2,ide,writeback
vm launch kvm host-c2

# host-c3: no disks; target for "vm hotplug".
clear vm config disks
vm config append live.role=host live.hostname=host-c3
vm config net lan-c,00:1f:53:00:03:13,virtio-net-pci
vm launch kvm host-c3

# host-c4: q35 machine type.
vm config machine q35
vm config append live.role=host live.hostname=host-c4
vm config net lan-c,00:1f:53:00:03:14,virtio-net-pci
vm launch kvm host-c4

# host-c5: xHCI USB controller and a cirrus VGA adapter.
clear vm config machine
vm config usb-use-xhci true
vm config vga cirrus
vm config append live.role=host live.hostname=host-c5
vm config net lan-c,00:1f:53:00:03:15,virtio-net-pci
vm launch kvm host-c5

# --- lan-d: firewall target, no-backchannel host, dual-homed host ------------

vm config restore live-base
vm config tags role server
vm config tags lan d

vm config append live.role=host live.hostname=host-d1
vm config net lan-d,00:1f:53:00:04:11,virtio-net-pci
vm launch kvm host-d1

vm config append live.role=host live.hostname=host-d2
vm config net lan-d,00:1f:53:00:04:12,virtio-net-pci
vm launch kvm host-d2

vm config append live.role=host live.hostname=host-d3
vm config net lan-d,00:1f:53:00:04:13,virtio-net-pci
vm launch kvm host-d3

# host-d4: no miniccc backchannel; only reachable over the network.
vm config backchannel false
vm config append live.role=host live.hostname=host-d4
vm config net lan-d,00:1f:53:00:04:14,virtio-net-pci
vm launch kvm host-d4

# host-d5: dual-homed on lan-d and lan-c with a fixed UUID.
vm config backchannel true
vm config uuid 4c1e0000-0000-4000-8000-0000000000d5
vm config append live.role=host live.hostname=host-d5
vm config net lan-d,00:1f:53:00:04:15,virtio-net-pci lan-c,00:1f:53:00:03:16,virtio-net-pci
vm launch kvm host-d5

# --- schedule, start ---------------------------------------------------------

clear vm config

# Flush the queue: the scheduler places all queued VMs, then launches them.
vm launch
vm start all

# --- routers -----------------------------------------------------------------
# Router configuration is delivered over miniccc once each router checks in.

router rtr-ab rid 1.1.1.1
router rtr-ab interface 0 10.10.1.1/24
router rtr-ab interface 1 10.10.2.1/24
router rtr-ab dhcp 10.10.1.1 range 10.10.1.100 10.10.1.199
router rtr-ab dhcp 10.10.1.1 router 10.10.1.1
router rtr-ab dhcp 10.10.1.1 dns 10.10.1.1
router rtr-ab dhcp 10.10.2.1 router 10.10.2.1
router rtr-ab dhcp 10.10.2.1 dns 10.10.2.1
router rtr-ab dhcp 10.10.2.1 static 00:1f:53:00:02:11 10.10.2.11
router rtr-ab dhcp 10.10.2.1 static 00:1f:53:00:02:12 10.10.2.12
router rtr-ab dhcp 10.10.2.1 static 00:1f:53:00:02:13 10.10.2.13
router rtr-ab dhcp 10.10.2.1 static 00:1f:53:00:02:14 10.10.2.14
router rtr-ab dhcp 10.10.2.1 static 00:1f:53:00:02:15 10.10.2.15
router rtr-ab dns 10.10.3.11 web.live
router rtr-ab dns 10.10.4.11 fw.live
router rtr-ab route ospf 0 0
router rtr-ab route ospf 0 1
router rtr-ab commit

router rtr-cd rid 2.2.2.2
router rtr-cd interface 0 10.10.2.2/24
router rtr-cd interface 1 10.10.3.1/24
router rtr-cd interface 2 10.10.4.1/24
router rtr-cd dhcp 10.10.3.1 router 10.10.3.1
router rtr-cd dhcp 10.10.3.1 dns 10.10.3.1
router rtr-cd dhcp 10.10.3.1 static 00:1f:53:00:03:11 10.10.3.11
router rtr-cd dhcp 10.10.3.1 static 00:1f:53:00:03:12 10.10.3.12
router rtr-cd dhcp 10.10.3.1 static 00:1f:53:00:03:13 10.10.3.13
router rtr-cd dhcp 10.10.3.1 static 00:1f:53:00:03:14 10.10.3.14
router rtr-cd dhcp 10.10.3.1 static 00:1f:53:00:03:15 10.10.3.15
router rtr-cd dhcp 10.10.3.1 static 00:1f:53:00:03:16 10.10.3.16
router rtr-cd dhcp 10.10.4.1 router 10.10.4.1
router rtr-cd dhcp 10.10.4.1 dns 10.10.4.1
router rtr-cd dhcp 10.10.4.1 static 00:1f:53:00:04:11 10.10.4.11
router rtr-cd dhcp 10.10.4.1 static 00:1f:53:00:04:12 10.10.4.12
router rtr-cd dhcp 10.10.4.1 static 00:1f:53:00:04:13 10.10.4.13
router rtr-cd dhcp 10.10.4.1 static 00:1f:53:00:04:14 10.10.4.14
router rtr-cd dhcp 10.10.4.1 static 00:1f:53:00:04:15 10.10.4.15
router rtr-cd dns 10.10.3.11 web.live
router rtr-cd dns 10.10.4.11 fw.live
router rtr-cd route ospf 0 0
router rtr-cd route ospf 0 1
router rtr-cd route ospf 0 2
# Block TCP/8080 to host-d1 at its gateway; TCP/80 stays open.
router rtr-cd fw drop out 2 10.10.4.11:8080 tcp
router rtr-cd commit

# --- host access -------------------------------------------------------------
# A host tap on lan-a lets the minimega node itself reach the experiment.

tap create lan-a ip 10.10.1.250/24 live-mgmt

# --- guest services ----------------------------------------------------------
# protonuke serves HTTP on host-c1 (web.live) and host-d1 (fw.live); busybox
# httpd listens on 8080 on host-d1 behind the rtr-cd firewall rule.

cc filter name=host-c1
cc background protonuke -serve -http
cc filter name=host-d1
cc background protonuke -serve -http
cc background httpd -f -p 8080 -h /etc
clear cc filter
