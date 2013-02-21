# minimega example
#
# This script launches 10 VMs locally with a simple, flat network and DHCP.
# Two different images will be launched, 5 of each.
#
# launch in minimega with
# 	read local.mm

# create a host tap with ip 10.0.0.1 on VLAN 100
host_tap 100 10.0.0.1/24

# start a DHCP server locally that listens on the host tap and serves IPs 
# between 10.0.0.2 and 10.0.0.254
dhcp start 10.0.0.1 10.0.0.2 10.0.0.254

# define our VM parameters, which will be used for any VMs we launch until
# we change the parameters again. 

# vlan 100, same as our host tap and DHCP server
vm_net 100

# 512 MB of memory per image
vm_memory 512

# set the initrd and kernel
vm_initrd default_amd64.initrd
vm_kernel default_amd64.kernel

# the first VM is configured, so now just launch 5 of them
vm_launch 5

# now update the VM parameters to use a different initrd and kernel, but leave
# all of the other parameters the same as before
vm_initrd other_image.initrd
vm_kernel other_kernel.kernel

# and launch 5 of these
vm_launch 5

# all of the images are launched in the paused state, so now we just start them
vm_start

# we'd also like to see our VMs, so we'll start a VNC proxy and browse to
# localhost:8080
vnc serve
