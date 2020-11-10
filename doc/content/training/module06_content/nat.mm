vm config net LAN

# Set up a NAT on the host machine by creating a tap:
tap create LAN ip 10.0.0.1/24 nat0

# Enabling IP forwarding on the host machine via the shell command:
shell sysctl -w net.ipv4.ip_forward=1

# And configuring iptables to enable the NAT on the host machine:
shell iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
shell iptables -A INPUT -i nat0 -j ACCEPT
shell iptables -A INPUT -i eth0 -m state --state ESTABLISHED,RELATED -j ACCEPT
shell iptables -A OUTPUT -j ACCEPT
# NOTE: You may need to change eth0 to match the interface on the host machine with Internet access.

# On the VM, we would then configure a static IP of 10.0.0.2/24, using 10.0.0.1 as the default gateway. On Linux this can be achieved with the following:
ip addr add 10.0.0.2/24 dev eth0
ip route add default via 10.0.0.1
