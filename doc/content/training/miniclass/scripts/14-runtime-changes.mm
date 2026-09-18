# 14-runtime-changes.mm: unplug, shape, swap a CD and plug in a USB drive.
# Needs miniccc.kernel/.initrd and minirouter.kernel/.initrd from chapter 2
# in /tmp/minimega/files, plus an ISO at /tmp/minimega/files/lab.iso.
clear namespace sandwich
namespace sandwich

# the sandwich from chapter 3 with fixed client addresses (chapter 11)
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config networks net_left,00:00:00:00:01:01
vm launch kvm vm_left
vm config networks net_right,00:00:00:00:02:01
vm launch kvm vm_right
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config networks net_left net_right
vm launch kvm router
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.0 range 10.0.0.2 10.0.0.254
router router dhcp 10.0.0.0 router 10.0.0.1
router router dhcp 10.0.0.0 static 00:00:00:00:01:01 10.0.0.10
router router interface 1 10.0.1.1/24
router router dhcp 10.0.1.0 range 10.0.1.2 10.0.1.254
router router dhcp 10.0.1.0 router 10.0.1.1
router router dhcp 10.0.1.0 static 00:00:00:00:02:01 10.0.1.10
router router commit
vm start router
shell sleep 10
vm start all
shell sleep 30

# baseline: vm_left pings vm_right through the router
cc filter name=vm_left
cc prefix baseline
cc exec ping -c 3 10.0.1.10
shell sleep 10
cc responses baseline

# unplug the router's net_right side (interface 1) and try again
vm net disconnect router 1
.annotate false .columns name,vlan vm info
cc prefix unplugged
cc exec ping -c 3 -W 1 10.0.1.10
shell sleep 10
cc responses unplugged

# plug it back in
vm net connect router 1 net_right
cc prefix replugged
cc exec ping -c 3 10.0.1.10
shell sleep 10
cc responses replugged

# shape what vm_left receives: 100 ms of delay, then 25% loss on top
qos add vm_left 0 delay 100ms
cc prefix delayed
cc exec ping -c 3 10.0.1.10
shell sleep 10
cc responses delayed
qos add vm_left 0 loss 25
.annotate false .columns name,qos vm info
cc prefix lossy
cc exec ping -c 20 10.0.1.10
shell sleep 30
cc responses lossy
clear qos vm_left all
clear cc prefix
clear cc filter

# swap an ISO into vm_right's CD drive and eject it again
vm cdrom change vm_right /tmp/minimega/files/lab.iso
.annotate false .columns name,cdrom vm info
vm cdrom eject vm_right

# attach a 64 MB image as a USB drive, list it, remove it
disk create raw usb.img 64M
vm hotplug add vm_right /tmp/minimega/files/usb.img
.annotate false vm hotplug
vm hotplug remove vm_right 0
.annotate false vm hotplug
