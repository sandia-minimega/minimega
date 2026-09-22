# Three routers in a triangle, all interfaces in OSPF area 0.
# Link ab is 10.0.0.0/24, ac is 10.0.1.0/24 and bc is 10.0.2.0/24.
vm config filesystem /root/uminirouterfs
vm config preinit /root/uminirouterfs/preinit
vm config networks ab ac
vm launch container routerA
vm config networks ab bc
vm launch container routerB
vm config networks ac bc
vm launch container routerC
vm start all

router routerA interface 0 10.0.0.1/24
router routerA interface 1 10.0.1.1/24
router routerA route ospf 0 0
router routerA route ospf 0 1
router routerA commit

router routerB interface 0 10.0.0.2/24
router routerB interface 1 10.0.2.1/24
router routerB route ospf 0 0
router routerB route ospf 0 1
router routerB commit

router routerC interface 0 10.0.1.2/24
router routerC interface 1 10.0.2.2/24
router routerC route ospf 0 0
router routerC route ospf 0 1
router routerC commit
