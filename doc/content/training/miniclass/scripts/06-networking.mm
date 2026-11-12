# Chapter 6: networking basics
# Rebuilds the router sandwich from chapter 3, inspects its VLANs, gives the
# host a tap into net_left, unplugs and replugs vm_right, and tries qos.
clear namespace sandwich
namespace sandwich
clear cc responses

# the two clients
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 3072
vm config networks net_left
vm launch kvm vm_left
vm config networks net_right
vm launch kvm vm_right

# the router; the net_left range stops at .200 to leave room for the host tap
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config networks net_left net_right
vm launch kvm router
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.0 range 10.0.0.2 10.0.0.200
router router dhcp 10.0.0.0 router 10.0.0.1
router router interface 1 10.0.1.1/24
router router dhcp 10.0.1.0 range 10.0.1.2 10.0.1.254
router router dhcp 10.0.1.0 router 10.0.1.1
router router commit
vm start router
shell sleep 10
vm start all
# wait until every VM has a connected agent and an address (about 90 s)
shell timeout 300 sh -c "until minimega -e .annotate false .columns cc_active,ip namespace sandwich vm info 2>/dev/null | grep -c 'true.*10[.]' | grep -qx 3; do sleep 3; done"

# the VLAN aliases the sandwich allocated, and which interface is on which
vlans
.annotate false .columns name,vlan vm info

# a host tap on net_left: the host is now 10.0.0.254 on that network
tap create net_left ip 10.0.0.254/24
tap
# prove it from the guest side: the Docker image has no ping on the host
cc filter name=vm_left
cc prefix tap
cc exec ping -c 3 10.0.0.254
shell sleep 8
cc responses tap
clear cc prefix
clear cc filter

# unplug vm_right, look, plug it back in
vm net disconnect vm_right 0
.annotate false .columns name,vlan vm info
vm net connect vm_right 0 net_right

# drop a quarter of what vm_left receives, look, then remove the constraint
qos add vm_left 0 loss 25
.annotate false .columns name,qos vm info
clear qos vm_left all
