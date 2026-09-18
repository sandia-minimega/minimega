# Chapter 7: routers and services
# Rebuilds the sandwich with a static lease and a DNS name for vm_right, then
# adds router2, net_far and vm_far and runs OSPF between the two routers.
clear namespace sandwich
namespace sandwich

# clients; vm_right gets a fixed MAC so the router can give it a static lease
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config networks net_left
vm launch kvm vm_left
vm config networks net_right,00:00:00:00:02:01
vm launch kvm vm_right
vm config networks net_far
vm launch kvm vm_far

# routers
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config networks net_left net_right
vm launch kvm router
vm config networks net_right net_far
vm launch kvm router2

# router: addresses, DHCP with a static lease, a name, OSPF on both interfaces
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.0 range 10.0.0.2 10.0.0.254
router router dhcp 10.0.0.0 router 10.0.0.1
router router dhcp 10.0.0.0 dns 10.0.0.1
router router interface 1 10.0.1.1/24
router router dhcp 10.0.1.0 range 10.0.1.2 10.0.1.254
router router dhcp 10.0.1.0 router 10.0.1.1
router router dhcp 10.0.1.0 dns 10.0.1.1
router router dhcp 10.0.1.0 static 00:00:00:00:02:01 10.0.1.10
router router dns 10.0.1.10 right.sandwich
router router route ospf 0 0
router router route ospf 0 1
router router commit

# router2: net_right is its transit link, net_far sits behind it
router router2 interface 0 10.0.1.2/24
router router2 interface 1 10.0.2.1/24
router router2 dhcp 10.0.2.0 range 10.0.2.2 10.0.2.254
router router2 dhcp 10.0.2.0 router 10.0.2.1
router router2 route ospf 0 0
router router2 route ospf 0 1
router router2 commit

vm start router,router2
shell sleep 15
vm start all
# OSPF needs a little while to form its adjacency and exchange routes
shell sleep 45

.annotate false .columns name,vlan vm info
router router
router router2

# vm_left reaches router2's far interface through both routers, by name too
cc filter name=vm_left
cc exec ping -c 3 10.0.2.1
cc exec ping -c 3 right.sandwich
# vm_far reaches vm_right's static address the other way
cc filter name=vm_far
cc exec ping -c 3 10.0.1.10
clear cc filter
shell sleep 15
cc responses all
