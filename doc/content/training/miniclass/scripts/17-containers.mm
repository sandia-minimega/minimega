# Chapter 17: Containers
# Rebuilds the router sandwich from chapter 3 and adds 100 container VMs,
# 50 on each network. Needs miniccc.kernel, miniccc.initrd,
# minirouter.kernel, minirouter.initrd and the minicccfs container
# filesystem in /tmp/minimega/files (chapters 2 and 17), and a host that
# boots with cgroup v1 (see chapter 17).
clear namespace sandwich
namespace sandwich

# The router, exactly as in chapter 3
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config networks net_left net_right
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

# The two KVM clients from chapter 3
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config networks net_left
vm launch kvm vm_left
vm config networks net_right
vm launch kvm vm_right

# Containers share the host kernel, so they need a root filesystem instead
# of a kernel and initrd. The KVM fields left in the template are ignored.
vm config filesystem /tmp/minimega/files/minicccfs
vm config memory 256
shell mkdir -p /tmp/minimega/scratch
vm config volume /scratch /tmp/minimega/scratch
vm config networks net_left
vm launch container left[0-49]
vm config networks net_right
vm launch container right[0-49]

vm start all
shell sleep 30
.columns name,type,state,vlan,ip,cc_active .filter type=container vm info

# Containers use cc exactly like KVM guests
cc filter name=left0
cc exec ls /scratch
cc exec ping -c 3 10.0.1.1
shell sleep 5
cc responses all
clear cc filter
