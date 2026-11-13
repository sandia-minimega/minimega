# quickstart.mm: two Linux VMs on a private network, with DHCP from the host.
# Build miniccc.kernel and miniccc.initrd with vmbetter first and put them in
# the files directory, /tmp/minimega/files.

vm config kernel /tmp/minimega/files/miniccc.kernel
vm config initrd /tmp/minimega/files/miniccc.initrd
vm config networks LAN

tap create LAN ip 10.0.0.1/24
dnsmasq start 10.0.0.1 10.0.0.2 10.0.0.254

vm launch kvm 2
vm start all
