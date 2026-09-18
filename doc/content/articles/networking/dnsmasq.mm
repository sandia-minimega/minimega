# Serve DHCP and DNS from the host to VMs on the "lan" network.
# The disk path is relative to the iomeshage files directory.
tap create lan ip 10.0.0.1/24
dnsmasq start 10.0.0.1 10.0.0.2 10.0.0.254
vm config disks client.qc2
vm config memory 1024
vm config networks lan
vm launch kvm client[1-5]
vm start all
