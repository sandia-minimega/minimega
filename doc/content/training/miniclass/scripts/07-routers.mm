# Chapter 7: routers and services
# Rebuilds the sandwich with a static lease and a DNS name for vm-right. Then it
# adds router2, net_far, and vm-far. It runs OSPF between the two routers.
clear namespace sandwich
namespace sandwich
clear cc responses

# clients. vm-right gets a fixed MAC so the router can give it a static lease
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 3072
vm config net net_left
vm launch kvm vm-left
vm config net net_right,00:00:00:00:02:01
vm launch kvm vm-right
vm config net net_far
vm launch kvm vm-far

# routers
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config net net_left net_right
vm launch kvm router
vm config net net_right net_far
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

# router2: net_right is its transit link and net_far sits behind it
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
# wait until every VM has a connected agent and an address (about 90 s)
shell timeout 300 sh -c "until minimega -e .annotate false .columns cc_active,ip namespace sandwich vm info 2>/dev/null | grep -c 'true.*10[.]' | grep -qx 5; do sleep 3; done"
# OSPF needs a little while to form its adjacency and exchange routes
shell sleep 30

.annotate false .columns name,vlan vm info
router router
router router2

# vm-left reaches router2's far interface through both routers, by name too
cc filter name=vm-left
cc exec ping -c 3 10.0.2.1
cc exec ping -c 3 right.sandwich
# vm-far reaches vm-right's static address the other way
cc filter name=vm-far
cc exec ping -c 3 10.0.1.10
clear cc filter
shell sleep 15
cc responses all

# router drops ICMP to vm-right. The first ping fails but the second still works.
# -W 1 keeps the wait for the lost replies short. A commit restarts the routing
# daemon, so give OSPF time to exchange the routes again
clear cc responses
router router fw drop out 1 10.0.1.10 icmp
router router commit
shell sleep 30
cc filter name=vm-left
cc exec ping -c 3 -W 1 10.0.1.10
cc exec ping -c 3 10.0.2.1
clear cc filter
shell sleep 20
cc responses all
# remove the rule again
clear router router fw
router router commit
