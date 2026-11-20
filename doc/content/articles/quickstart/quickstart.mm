# quickstart.mm: two Linux VMs on a private network, with DHCP from the host.
# Build miniccc.kernel and miniccc.initrd with vmbetter first and put them in
# the files directory, /tmp/minimega/files.

vm config kernel /tmp/minimega/files/miniccc.kernel
vm config initrd /tmp/minimega/files/miniccc.initrd
# 3072 MB: the image unpacks into RAM, and 2048 MB is not sufficient
vm config memory 3072
vm config networks LAN

tap create LAN ip 10.0.0.1/24
dnsmasq start 10.0.0.1 10.0.0.2 10.0.0.254

vm launch kvm 2
vm start all
