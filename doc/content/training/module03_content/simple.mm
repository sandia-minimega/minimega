# As an exercise, let's launch a few VMs of various types

# Let's start by setting some properties
vm config vcpu 4
vm config memory 8196 # MB
vm config net 100 # vlan 100
vm config disk foo.qc2 # defaults to /tmp/minimega/files/foo.qc2
vm launch kvm foo[1-5]
# Now to use a different image
clear vm config disk
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm launch kvm foo[6-10]
# Now some containers
vm config memory 4098
vm config tags container=true
vm config filesystem /tmp/minimega/files/minicccfs
# both filesystem and kernel/initrd are set, but that is ok
vm launch container bar[1-5]
vm start all
