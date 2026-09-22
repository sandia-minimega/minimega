# 04-vms.mm: rebuild the router sandwich, then walk it through the VM
# lifecycle and launch extra VMs from a saved configuration.
clear namespace sandwich
namespace sandwich

# the client description, saved for later launches
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config networks net_left
vm config save client
vm launch kvm vm_left
vm config networks net_right
vm launch kvm vm_right

# the router description, also saved
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config networks net_left net_right
vm config save router
vm launch kvm router

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

# pause one client and resume it; memory is kept while it is PAUSED
vm stop vm_left
.annotate false .columns name,state vm info
vm start vm_left

# kill the other client: "vm start all" leaves it in QUIT, naming it restarts it
vm kill vm_right
vm start all
.annotate false .columns name,state vm info
vm start vm_right

# two spare clients from the saved description, tagged as a group
vm launch kvm spare[1-2] client
vm tag spare[1-2] role spare
vm start spare[1-2]
.annotate false .columns name,state,tags .filter tags=spare vm info

# the spares are not part of the sandwich; remove them and forget them
vm kill spare[1-2]
vm flush
.annotate false .columns name,state vm info
