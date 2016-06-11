package main

var dnsTmpl = `
# minirouter dnsmasq template

# don't read /etc/resolv.conf
no-resolv

# dns entries
# address=/foo.com/1.2.3.4

# dhcp
# dhcp-range=192.168.0.1,192.168.0.100,255.255.255.0
# dhcp-host=00:11:22:33:44:55,192.168.0.1,foo

# todo: ipv6 route advertisements for SLAAC

# todo: logging, stats, etc. that minirouter can consume
`
