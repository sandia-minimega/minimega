# and a router
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd

# notice the router has two networks!
vm config net net_left net_right
vm launch kvm router

# configure the router
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.0 range 10.0.0.2 10.0.0.2
router router dhcp 10.0.0.0 dns 10.0.0.2
router router interface 1 20.0.0.1/24
router router dhcp 20.0.0.0 range 20.0.0.2 20.0.0.2
router router dhcp 20.0.0.0 dns 20.0.0.2
router router route ospf 0 0
router router route ospf 0 1
router router commit


