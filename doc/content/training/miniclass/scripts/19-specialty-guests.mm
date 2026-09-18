# Chapter 19: Specialty KVM guests
# Rebuilds the router sandwich from chapter 3 with a software TPM attached
# to vm_left, then talks to QEMU directly over QMP. Needs the chapter 2
# images and the swtpm package on the host.
clear namespace sandwich
namespace sandwich

# One swtpm instance per VM, started as a host process from minimega
shell mkdir -p /tmp/minimega/swtpm/vm_left
background swtpm socket --tpm2 --tpmstate dir=/tmp/minimega/swtpm/vm_left --ctrl type=unixio,path=/tmp/minimega/swtpm/vm_left/swtpm-sock
shell sleep 2

# vm_left gets the TPM; vm_right is a plain client
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config networks net_left
vm config tpm-socket /tmp/minimega/swtpm/vm_left/swtpm-sock
vm launch kvm vm_left
clear vm config tpm-socket
vm config networks net_right
vm launch kvm vm_right

# The router, as in chapter 3
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
vm start all
shell sleep 20
.columns name,state,tpm-socket vm info

# Ask QEMU directly over its QMP socket
vm qmp vm_left '{ "execute": "query-status" }'

# The guest sees an ordinary TPM device
cc filter name=vm_left
cc exec modprobe tpm_tis
cc exec ls -l /dev/tpm0
shell sleep 5
cc responses all
clear cc filter
