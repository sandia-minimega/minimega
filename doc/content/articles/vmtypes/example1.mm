# set a disk image
vm config disks server.qc2

# set some other common parameters
vm config memory 4096
vm config networks 100

# launch one VM, named server
vm launch kvm server

# launch 10 more, named client1, client2, client3...
vm launch kvm client[1-10]
