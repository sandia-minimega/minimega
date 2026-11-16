# Give VMs on the "lan" network a route to the outside world through the host.
# Replace eth0 with the host interface that has Internet access.
vm config net lan
tap create lan ip 10.0.0.1/24 nat0

# Enable forwarding and NAT on the host. "shell" runs these commands on the
# minimega host as the same user as minimega, typically root.
shell sysctl -w net.ipv4.ip_forward=1
shell iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
shell iptables -A FORWARD -i eth0 -o nat0 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
shell iptables -A FORWARD -i nat0 -o eth0 -j ACCEPT

# Hand out addresses and set the tap as the default gateway. Set a resolver
# that forwards to the resolver of your network.
dnsmasq start 10.0.0.1 10.0.0.2 10.0.0.254
dnsmasq configure 0 options option:router,10.0.0.1
dnsmasq configure 0 dns upstream server 192.168.1.1
