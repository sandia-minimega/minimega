#
# example of launching a vyatta router in minimega
#
# the config script on the floppy disk image provided will bridge two networks with the ip configuration:
# 	eth0 10.0.0.254/24
#	eth1 20.0.0.254/24
#
# the vyatta iso can be obtained from vyatta.com for free
vm_net 100 200
vm_memory 2048
vm_qemu_append -fda disk.img
vm_cdrom vyatta-livecd_VC6.5R1_amd64.iso
vm_launch 1
vm_start
