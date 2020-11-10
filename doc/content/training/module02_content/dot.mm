minimega$ vm info
host   | id | name        | state    | uptime          | type       | uuid                                 | cc_active | pid | vlan | bridge | tap | mac | ip | ip6 | qos | memory | vcpus | disk           | snapshot | initrd | kernel | cdrom | migrate | append | serial-ports | virtio-ports | vnc_port | filesystem | hostname | init | preinit | fifo | volume | console_port | tags
myhost | 0  | kvm1        | BUILDING | 3m30.511919113s | kvm        | 2a270748-9dfd-47ca-8302-b8cc64206e1f | false     | 0   | []   | []     | []  | []  | [] | []  | []  | 2048   | 1     | [mydisk.qcow]  | true     |        |        |       |         | []     | 0            | 0            | 34671    | N/A        | N/A      | N/A  | N/A     | N/A  | N/A    | N/A          | {}

# That's a lot of data!! Let's try to narrow it down:

minimega$ .column host,name,state,type .filter state=building .filter type!=container vm info 
host   | hostname | name | state    | type
myhost | N/A      | kvm1  | BUILDING | kvm

# Better! But that is a bit lengthy to type repeatedly... try aliasing!

minimega$ .alias vms=.column host,name,state,type .filter state=building .filter type!=container vm info
minimega$ vms
host   | hostname | name | state    | type
myhost | N/A      | kvm1  | BUILDING | kvm

# perfect!
