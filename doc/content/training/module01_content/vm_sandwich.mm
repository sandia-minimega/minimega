# start this experiment in a namespace
namespace sandwich

# describe vm_left
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 5096
vm config net net_left
vm launch kvm vm_left

# describe vm_right
# We don't need to specify kernel/initrd/memory again
# Our config carries over
vm config net net_right
vm launch kvm vm_right

# and a router
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd

# notice the router has two networks!
vm config net net_left net_right
vm launch kvm router

# configure the router
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.0 range 10.0.0.2 10.0.0.2
router router dhcp 10.0.0.0 dns 10.0.0.2
router router interface 1 20.0.0.1/24
router router dhcp 20.0.0.0 range 20.0.0.2 20.0.0.2
router router dhcp 20.0.0.0 dns 20.0.0.2
router router route ospf 0 0
router router route ospf 0 1
router router commit

shell echo "start the router first, and give it a chance to configure"
vm start router
shell sleep 10

# "all" is a special keyword
vm start all
shell echo "give VMs 20 seconds to obtain IP..."
shell sleep 20

# make sure everything booted and connected
shell echo "vm info:"
.column id,name,state,vlan,ip,qos vm info

# collect some control data
shell echo "vm top:"
vm top
# notice there is no data traveling across the network (rx/tx == 0.00)
# Let's fix that!

# Let's use miniccc to push out a tool to our VMs
cc send file:protonuke

# We will have vm_left server traffic
cc filter name=vm_left
cc background /tmp/miniccc/files/protonuke -serve -http
cc background /tmp/miniccc/files/protonuke -serve -https

# We will have vm_right request http/https
clear cc filter
cc filter name=vm_right
cc background /tmp/miniccc/files/protonuke -level info -logfile proto.log -http -https 10.0.0.2

shell echo "Now let's check our traffic levels:"
vm top
# Notice that we have values in rx/tx now
# Notice, also the difference in values for the roles each are playing

# Now let's turn on qos to manipulate the network traffic
qos add vm_left 0 loss 0.5
# this will force the interface for vm_right to drop hald the packets it receives to simulate packet loss

# let's see how this changes what we see over the network:
shell sleep 20
vm top

# clean up 
#clear namespace sandwich
