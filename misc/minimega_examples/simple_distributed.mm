# minimega example
#
# This script launches a single type of VM across a network of minimega
# instances. All VMs will be networked on a single flat network, with DHCP on
# this node.
#
# This script assumes that a cluster of minimega instances are running, and 
# are all connected. An easy way to do this is to launch minimega on each node
# and ask minimega to solicit connections from other instances:
# 	minimega -degree 2
# This will cause minimega to solicit at least 2 connections from other nodes.
# You can verify this in minimega using the mesh_status and mesh_list commands.
#
# launch in minimega with
# 	read simple_distributed.mm

# create a host tap with ip 10.0.0.1 on VLAN 100
host_tap 100 10.0.0.1/16

# start a DHCP server locally that listens on the host tap and serves IPs 
# between 10.0.0.2 and 10.0.254.254
dhcp start 10.0.0.1 10.0.0.2 10.0.254.254

# define our VM parameters, which will be used for any VMs we launch until
# we change the parameters again. We will distribute these commands to all
# minimega instances

# vlan 100, same as our host tap and DHCP server
mesh_broadcast vm_net 100

# 512 MB of memory per image
mesh_broadcast vm_memory 512

# set the initrd and kernel
mesh_broadcast vm_initrd default_amd64.initrd
mesh_broadcast vm_kernel default_amd64.kernel

# all of the images are launched in the paused state, so now we just start them
mesh_broadcast vm_start

# we'd also like to see our VMs, so we'll start a VNC proxy and browse to
# localhost:8080
# The VNC proxy will forward connections from other minimega instances to the
# this node automatically.
vnc serve
