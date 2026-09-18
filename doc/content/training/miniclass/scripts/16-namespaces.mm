# 16-namespaces.mm: a second sandwich in namespace sandwich2, scheduled
# through the queue, then destroyed. Needs the chapter 2 images in
# /tmp/minimega/files.
clear namespace sandwich
namespace sandwich

# the sandwich from chapter 3 with fixed client addresses (chapter 11)
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config networks net_left,00:00:00:00:01:01
vm launch kvm vm_left
vm config networks net_right,00:00:00:00:02:01
vm launch kvm vm_right
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config networks net_left net_right
vm launch kvm router
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.0 range 10.0.0.2 10.0.0.254
router router dhcp 10.0.0.0 router 10.0.0.1
router router dhcp 10.0.0.0 static 00:00:00:00:01:01 10.0.0.10
router router interface 1 10.0.1.1/24
router router dhcp 10.0.1.0 range 10.0.1.2 10.0.1.254
router router dhcp 10.0.1.0 router 10.0.1.1
router router dhcp 10.0.1.0 static 00:00:00:00:02:01 10.0.1.10
router router commit
vm start router
shell sleep 10
vm start all
shell sleep 30

# where we are
.annotate false namespace
.annotate false vlans

# the same sandwich again in sandwich2, queued instead of launched
clear namespace sandwich2
namespace sandwich2
ns hosts
ns queueing true
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config networks net_left,00:00:00:00:01:01
vm launch kvm vm_left
vm config networks net_right,00:00:00:00:02:01
vm launch kvm vm_right
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config networks net_left net_right
vm launch kvm router
ns queue

# ask the scheduler for a placement, then let it launch
.annotate false ns schedule dry-run
ns schedule
.annotate false .columns state,launched,failures,total,hosts ns schedule status
.annotate false .columns name,state,vlan vm info
.annotate false vlans

# configure and start the second router and its clients
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.0 range 10.0.0.2 10.0.0.254
router router dhcp 10.0.0.0 router 10.0.0.1
router router dhcp 10.0.0.0 static 00:00:00:00:01:01 10.0.0.10
router router interface 1 10.0.1.1/24
router router dhcp 10.0.1.0 range 10.0.1.2 10.0.1.254
router router dhcp 10.0.1.0 router 10.0.1.1
router router dhcp 10.0.1.0 static 00:00:00:00:02:01 10.0.1.10
router router commit
vm start router
shell sleep 10
vm start all
shell sleep 30

# same addresses as the first sandwich, separate network: this ping
# reaches sandwich2's vm_right only
cc filter name=vm_left
cc exec ping -c 3 10.0.1.10
shell sleep 10
cc responses 4
clear cc filter

# look at the first sandwich without leaving this namespace
.annotate false namespace sandwich .columns name,state,vlan vm info

# destroy the copy; the first sandwich is untouched
clear namespace sandwich2
.annotate false namespace
