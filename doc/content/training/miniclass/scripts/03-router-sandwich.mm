# 03-router-sandwich.mm: the router sandwich. Two clients on separate
# networks, joined by a minirouter that serves DHCP on both of them.
# Needs miniccc.kernel/.initrd and minirouter.kernel/.initrd from chapter 2
# in the files directory, /tmp/minimega/files.
clear namespace sandwich
namespace sandwich

# the two clients: one description, two launches
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config networks net_left
vm launch kvm vm_left
vm config networks net_right
vm launch kvm vm_right

# the router, with one interface on each network
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config networks net_left net_right
vm launch kvm router

# describe the router, then commit the description
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.0 range 10.0.0.2 10.0.0.254
router router dhcp 10.0.0.0 router 10.0.0.1
router router interface 1 10.0.1.1/24
router router dhcp 10.0.1.0 range 10.0.1.2 10.0.1.254
router router dhcp 10.0.1.0 router 10.0.1.1
router router commit

# start the router first so DHCP is serving before the clients ask
vm start router
shell sleep 10
vm start all
shell sleep 20
.columns name,state,vlan,ip vm info
