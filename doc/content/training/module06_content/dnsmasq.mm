tap create 100 ip 10.0.0.1/24
vm config disk foo.qc2
vm config memory 128
vm config net 100
dnsmasq start 10.0.0.1 10.0.0.2 10.0.0.254
vm launch kvm linux[1-10]
vm start all
