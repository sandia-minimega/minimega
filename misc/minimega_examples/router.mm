# minimega example
#
# This script launches two subnets on 10.0.0.0/24 and 11.0.0.0/24, with 5
# VMs each. It also creates a quagga router image that bridges the two
# subnets.
#
# See the quagga vmbetter example for quagga configuration.
#
# launch in minimega with
# 	read router.mm

# create a host tap with ip 10.0.0.2 on VLAN 100 and 11.0.0.2 on VLAN 200
# the quagga router will take 10.0.0.1 and 11.0.0.1
host_tap 100 10.0.0.2/24
host_tap 200 11.0.0.2/24

# start a DHCP server locally that listens on the host tap and serves IPs 
# between 10.0.0.3 and 10.0.0.254
dhcp start 10.0.0.2 10.0.0.3 10.0.0.254

# start a second DHCP server for the 11.0.0.0/24 subnet
dhcp start 11.0.0.2 11.0.0.3 11.0.0.254

# first define the VM parameters for the router and launch it
vm_net 100 200
vm_memory 2048
vm_initrd quagga.initrd
vm_kernel quagga.kernel
vm_launch 1

# now launch 5 VMs on the 10.0.0.0/24 subnet, which is VLAN 100
vm_net 100
vm_initrd default_amd64.initrd
vm_kernel default_amd64.kernel
vm_launch 5

# finally launch 5 VMs on the other subnet, VLAN 200.
# Note we only have to change the vm_net parameter, as we're using the
# same image.
vm_net 200
vm_launch 5

# all of the images are launched in the paused state, so now we just start them
vm_start

# we'd also like to see our VMs, so we'll start a VNC proxy and browse to
# localhost:8080
vnc serve
