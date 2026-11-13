# Six routers, nine transit links and six client LANs, all in OSPF area 0.
# Link aliases: A 10.1.0.0/24, B 10.21.0.0/24, B2 10.22.0.0/24, B3 10.23.0.0/24,
# C 10.3.0.0/24, D 10.4.0.0/24, E 10.5.0.0/24, F 10.6.0.0/24, G 10.71.0.0/24,
# G2 10.72.0.0/24, G3 10.73.0.0/24, H 10.8.0.0/24, I 10.9.0.0/24, J 10.10.0.0/24,
# K 10.11.0.0/24.
vlans range 1500 1600

vm config filesystem /root/uminirouterfs
vm config preinit /root/uminirouterfs/preinit
vm config networks A B B2 C
vm launch container r0
vm config networks D B B3 E
vm launch container r1
vm config networks E G G2 F
vm launch container r2
vm config networks C G3 G2 H I
vm launch container r3
vm config networks B3 K G G3 B2
vm launch container r4
vm config networks I J
vm launch container r5

clear vm config
vm config disks client.qc2
vm config memory 512
vm config networks A
vm launch kvm l1
vm config networks D
vm launch kvm l2
vm config networks K
vm launch kvm l3
vm config networks F
vm launch kvm l4
vm config networks H
vm launch kvm l5
vm config networks J
vm launch kvm l6
vm start all

router r0 interface 0 10.1.0.1/24
router r0 interface 1 10.21.0.1/24
router r0 interface 2 10.22.0.1/24
router r0 interface 3 10.3.0.1/24
router r0 dhcp 10.1.0.0 range 10.1.0.2 10.1.0.254
router r0 route ospf 0 0
router r0 route ospf 0 1
router r0 route ospf 0 2
router r0 route ospf 0 3
router r0 commit

router r1 interface 0 10.4.0.1/24
router r1 interface 1 10.21.0.2/24
router r1 interface 2 10.23.0.1/24
router r1 interface 3 10.5.0.1/24
router r1 dhcp 10.4.0.0 range 10.4.0.2 10.4.0.254
router r1 route ospf 0 0
router r1 route ospf 0 1
router r1 route ospf 0 2
router r1 route ospf 0 3
router r1 commit

router r2 interface 0 10.5.0.2/24
router r2 interface 1 10.71.0.1/24
router r2 interface 2 10.72.0.1/24
router r2 interface 3 10.6.0.1/24
router r2 dhcp 10.6.0.0 range 10.6.0.2 10.6.0.254
router r2 route ospf 0 0
router r2 route ospf 0 1
router r2 route ospf 0 2
router r2 route ospf 0 3
router r2 commit

router r3 interface 0 10.3.0.2/24
router r3 interface 1 10.73.0.1/24
router r3 interface 2 10.72.0.2/24
router r3 interface 3 10.8.0.1/24
router r3 interface 4 10.9.0.1/24
router r3 dhcp 10.8.0.0 range 10.8.0.2 10.8.0.254
router r3 route ospf 0 0
router r3 route ospf 0 1
router r3 route ospf 0 2
router r3 route ospf 0 3
router r3 route ospf 0 4
router r3 commit

router r4 interface 0 10.23.0.2/24
router r4 interface 1 10.11.0.1/24
router r4 interface 2 10.71.0.2/24
router r4 interface 3 10.73.0.2/24
router r4 interface 4 10.22.0.2/24
router r4 dhcp 10.11.0.0 range 10.11.0.2 10.11.0.254
router r4 route ospf 0 0
router r4 route ospf 0 1
router r4 route ospf 0 2
router r4 route ospf 0 3
router r4 route ospf 0 4
router r4 commit

router r5 interface 0 10.9.0.2/24
router r5 interface 1 10.10.0.1/24
router r5 dhcp 10.10.0.0 range 10.10.0.2 10.10.0.254
router r5 route ospf 0 0
router r5 route ospf 0 1
router r5 commit
