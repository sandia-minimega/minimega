# start this experiment in a namespace
namespace sandwich

# describe vm-left
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 5096
vm config net net_left
vm launch kvm vm-left

# describe vm-right
# We don't need to specify kernel/initrd/memory again
# Our config carries over
vm config net net_right
vm launch kvm vm-right
