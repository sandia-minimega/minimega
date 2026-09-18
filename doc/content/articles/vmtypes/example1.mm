# set a disk image
vm config disks foo.qc2

# set some other common parameters
vm config memory 4096
vm config networks 100

# launch one VM, named foo
vm launch kvm foo

# launch 10 more, named bar1, bar2, bar3...
vm launch kvm bar[1-10]
