# One minirouter container between an outside network and a client LAN.
# uminirouterfs is the root filesystem built by misc/uminirouter/build.bash.
vm config filesystem /root/uminirouterfs
vm config preinit /root/uminirouterfs/preinit
vm config networks wan lan
vm launch container r0
vm start r0

# Address both interfaces, use 10.0.0.1 on the wan side as gateway and
# resolver, serve DHCP and DNS on the LAN, then push it all to the router.
router r0 interface 0 10.0.0.2/24
router r0 interface 1 192.168.1.1/24
router r0 gw 10.0.0.1
router r0 upstream 10.0.0.1
router r0 dhcp 192.168.1.0 range 192.168.1.100 192.168.1.200
router r0 dhcp 192.168.1.0 router 192.168.1.1
router r0 dhcp 192.168.1.0 dns 192.168.1.1
router r0 dhcp 192.168.1.0 static 00:11:22:33:44:55 192.168.1.10
router r0 dns 192.168.1.10 fileserver.lan
router r0 commit
