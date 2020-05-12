# set a disk image
vm config disk foo.qcow2

# set some other common parameters
vm config memory 4096
vm config net 100

# launch one VM, named foo
vm launch kvm foo

# launch 10 more, named bar1, bar2, bar3...
vm launch kvm bar[1-10]
