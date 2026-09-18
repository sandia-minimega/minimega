# Chapter 20: Clusters
# Run this on the head node (node1) of a two-host mesh (node1 and node2)
# once "mesh status" reports size 2. It spreads the router sandwich over
# both hosts, joined by a VXLAN tunnel. It cannot run on a single machine.
# Needs the chapter 2 images in node1's files directory only; node2
# fetches what it needs over the mesh.
clear namespace sandwich
namespace sandwich

# A new namespace holds every mesh peer except the head node; add it too
ns add-hosts localhost
ns hosts

# A private bridge on every host of the namespace, joined by VXLAN tunnels
ns bridge sandwich vxlan

# Queue the VMs so the scheduler sees the whole experiment before placing it
ns queueing true
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config networks sandwich,net_left sandwich,net_right
vm config schedule node1
vm launch kvm router
clear vm config schedule
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config networks sandwich,net_left
vm launch kvm vm_left
vm config networks sandwich,net_right
vm launch kvm vm_right

# Look at the placement, then force the clients onto opposite hosts
ns schedule dry-run
ns schedule mv vm_left node2
ns schedule mv vm_right node1
ns schedule dump
ns schedule
shell sleep 5
.columns name,state,vlan vm info

# Configure and start the router on node1, then the clients
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.0 range 10.0.0.2 10.0.0.254
router router dhcp 10.0.0.0 router 10.0.0.1
router router interface 1 10.0.1.1/24
router router dhcp 10.0.1.0 range 10.0.1.2 10.0.1.254
router router dhcp 10.0.1.0 router 10.0.1.1
router router commit
vm start router
shell sleep 10
vm start all
shell sleep 20
.columns name,state,vlan,ip vm info

# vm_left on node2 got its lease from the router on node1 through the tunnel
cc filter name=vm_left
cc exec ping -c 3 10.0.0.1
shell sleep 5
cc responses all
clear cc filter

# node2 fetched the images it needed from node1 with iomeshage
mesh send node2 file list
ns schedule status
