# minimega live integration test topology: containers
#
# Five containers and one KVM VM on two VLANs. One container runs minirouter
# and routes between the VLANs, so traffic crosses containers, the KVM VM, and
# Open vSwitch. minimega limits each container with cgroups (v1 or v2,
# whichever the host has), which tests/live/containers checks on the host,
# and the device filter also from inside the containers.
#
#   lan-a 10.20.1.0/24  ctr-a1..a3      dynamic DHCP from ctr-rtr
#   lan-b 10.20.2.0/24  ctr-b1, kvm-b1  static DHCP (by MAC) from ctr-rtr
#
#   ctr-rtr  lan-a .1, lan-b .1   (minirouter in a container)
#
# Requirements (staged by tests/live/run.sh):
#   - live.kernel and live.initrd in the minimega files directory
#     (-filepath), under live/, and the initrd unpacked in live/rootfs as the
#     filesystem of the containers
#   - MM_FILEPATH, the files directory, in the environment of minimega
#
# Load it with:
#   mm read /tmp/minimega/files/live/topology.mm
#
# Remove everything it creates with:
#   clear namespace live

namespace live

# Queue VM launches and let the scheduler place them in one pass.
ns queueing true

vlans add lan-a 1101

# --- common VM configuration -------------------------------------------------

clear vm config
vm config snapshot true
vm config tags topology live
vm config save live-base

# Containers share one filesystem, each with an overlay of its own (snapshot).
# The init program takes the role of the container as its argument.
vm config filesystem live/rootfs
vm config init /usr/local/sbin/live-container host
vm config memory 128
vm config vcpus 1
vm config save ctr-base

# --- router ------------------------------------------------------------------

# ctr-rtr: minirouter. A container cannot write /proc/sys, so a preinit
# program, which minimega runs on the host, turns on forwarding.
vm config init /usr/local/sbin/live-container router
vm config preinit ${MM_FILEPATH}/live/rootfs/usr/local/sbin/live-preinit
vm config memory 256
vm config tags role router
vm config net lan-a lan-b
vm launch container ctr-rtr

# --- lan-a: dynamic DHCP clients ---------------------------------------------

vm config restore ctr-base
vm config tags role client
vm config tags lan a
vm config net lan-a

# ctr-a1: its own hostname and UUID, two CPUs, a volume from the host, and a
# named pipe to the host.
vm config hostname alpha
vm config uuid 4c1e0000-0000-4000-8000-0000000000a1
vm config memory 192
vm config vcpus 2
vm config fifos 1
vm config volume /scratch ${MM_FILEPATH}/live/scratch-a1
vm launch container ctr-a1

# ctr-a2, ctr-a3: launched as a range from one configuration (the hostname of
# each is its VM name).
vm config restore ctr-base
vm config tags role client
vm config tags lan a
vm config net lan-a
vm launch container ctr-a[2-3]

# --- lan-b: static DHCP by MAC, container and KVM ----------------------------

vm config restore ctr-base
vm config tags role server
vm config tags lan b
vm config net lan-b,00:1f:53:00:12:11
vm launch container ctr-b1

# kvm-b1: a KVM VM next to the containers.
vm config restore live-base
vm config kernel live/live.kernel
vm config initrd live/live.initrd
vm config memory 384
vm config tags role server
vm config tags lan b
vm config append live.role=host live.hostname=kvm-b1
vm config net lan-b,00:1f:53:00:12:21,virtio-net-pci
vm launch kvm kvm-b1

# --- schedule, start ---------------------------------------------------------

clear vm config

# Flush the queue: the scheduler places all queued VMs, then launches them.
vm launch
vm start all

# --- router ------------------------------------------------------------------
# Router configuration is delivered over miniccc once the router checks in.

router ctr-rtr interface 0 10.20.1.1/24
router ctr-rtr interface 1 10.20.2.1/24
router ctr-rtr dhcp 10.20.1.1 range 10.20.1.100 10.20.1.199
router ctr-rtr dhcp 10.20.1.1 router 10.20.1.1
router ctr-rtr dhcp 10.20.2.1 router 10.20.2.1
router ctr-rtr dhcp 10.20.2.1 static 00:1f:53:00:12:11 10.20.2.11
router ctr-rtr dhcp 10.20.2.1 static 00:1f:53:00:12:21 10.20.2.21
router ctr-rtr commit

# --- guest services ----------------------------------------------------------
# protonuke serves HTTP from the container ctr-b1.

cc filter name=ctr-b1
cc background protonuke -serve -http
clear cc filter
