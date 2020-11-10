# start all the VMs
vm start all

# set static IP on A
cc filter uuid=11111111-1111-1111-1111-111111111111
cc exec ip addr add 10.0.0.1/24 dev eth0

# set static IP on B
cc filter uuid=22222222-2222-2222-2222-222222222222
cc exec ip addr add 10.0.0.2/24 dev eth0
