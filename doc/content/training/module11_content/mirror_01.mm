# create two VMs, each with a hardcoded UUID
vm config kernel $images/miniccc.kernel
vm config initrd $images/miniccc.initrd
vm config net LAN
vm config uuid 11111111-1111-1111-1111-111111111111
vm launch kvm A
vm config uuid 22222222-2222-2222-2222-222222222222
vm launch kvm B

# create a VM to monitor the other two, also with a hardcoded UUID
vm config net 0
vm config uuid 33333333-3333-3333-3333-333333333333
vm launch kvm monitor
