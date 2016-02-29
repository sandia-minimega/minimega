# Specify your interface IPs
vyatta interfaces 5.5.5.5/24 192.168.0.1/24

# Enable OSPF
vyatta ospf 5.5.5.0/24 192.168.0.1/24

# Enable internal DHCP, with 8.8.8.8 as your DNS server.
vyatta dhcp add 192.168.0.0/24 192.168.0.1 192.168.0.2 192.168.0.250 8.8.8.8

# Set up IPv6 too
vyatta interfaces6 2001:5:5:5::5/64 2001:192:168:0::1/64

# Enable router advertisement for IPv6
vyatta rad 2001:5:5:5::0/64 2001:192:168:0::0/64

# Enable OSPF3 for IPv6 interfaces
vyatta ospf3 eth0 eth1

# Write out the configuration as a floppy disk
vyatta write router.img
