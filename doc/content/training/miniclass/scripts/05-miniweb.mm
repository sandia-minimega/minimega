# 05-miniweb.mm: rebuild the router sandwich and capture its consoles from
# the prompt. miniweb itself is started from a host shell; see the chapter.
clear namespace sandwich
namespace sandwich

vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config networks net_left
vm launch kvm vm_left
vm config networks net_right
vm launch kvm vm_right

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
.annotate false .columns name,state vm info

# a full-size screenshot in vm_left's instance directory, and a 400-pixel
# thumbnail of vm_right in the files directory, where the Files page serves it
vm screenshot vm_left
vm screenshot vm_right file /tmp/minimega/files/vm_right.png 400
