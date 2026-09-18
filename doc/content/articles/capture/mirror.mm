# Two VMs talking on "lan", and a third VM that receives a copy of
# everything the first one sends and receives.
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config networks lan
vm launch kvm A
vm launch kvm B

# The monitor needs a port on the same bridge; VLAN 0 keeps it off "lan".
vm config networks 0
vm launch kvm monitor
vm start all

# Address A and B through the cc channel (miniccc is in the image).
cc filter name=A
cc exec ip addr add 10.0.0.1/24 dev eth0
cc filter name=B
cc exec ip addr add 10.0.0.2/24 dev eth0
clear cc filter

# Mirror interface 0 of A to interface 0 of monitor.
tap mirror A 0 monitor 0
