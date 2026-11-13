# Two routers joined by a transit link, each with its own client LAN,
# reaching each other's LAN through static routes.
vm config filesystem /root/uminirouterfs
vm config preinit /root/uminirouterfs/preinit
vm config networks transit lan1
vm launch container r0
vm config networks transit lan2
vm launch container r1

clear vm config
vm config disks client.qc2
vm config memory 512
vm config networks lan1
vm launch kvm a[1-3]
vm config networks lan2
vm launch kvm b[1-3]
vm start all

router r0 interface 0 10.0.0.1/24
router r0 interface 1 192.168.1.1/24
router r0 dhcp 192.168.1.0 range 192.168.1.2 192.168.1.254
router r0 route static 192.168.2.0/24 10.0.0.2
router r0 commit

router r1 interface 0 10.0.0.2/24
router r1 interface 1 192.168.2.1/24
router r1 dhcp 192.168.2.0 range 192.168.2.2 192.168.2.254
router r1 route static 192.168.1.0/24 10.0.0.1
router r1 commit
