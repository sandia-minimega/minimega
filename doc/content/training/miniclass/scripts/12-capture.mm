# 12-capture.mm: PCAP, netflow and a mirrored monitor VM on the sandwich.
# Needs miniccc.kernel/.initrd and minirouter.kernel/.initrd from chapter 2
# in /tmp/minimega/files. nfcat is expected at /opt/minimega/bin/nfcat.
clear namespace sandwich
namespace sandwich
clear cc responses

# the sandwich from chapter 3 with fixed client addresses (chapter 11)
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 3072
vm config networks net_left,00:00:00:00:01:01
vm launch kvm vm-left
vm config networks net_right,00:00:00:00:02:01
vm launch kvm vm-right
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config networks net_left net_right
vm launch kvm router
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.0 range 10.0.0.2 10.0.0.254
router router dhcp 10.0.0.0 router 10.0.0.1
router router dhcp 10.0.0.0 static 00:00:00:00:01:01 10.0.0.10
router router interface 1 10.0.1.1/24
router router dhcp 10.0.1.0 range 10.0.1.2 10.0.1.254
router router dhcp 10.0.1.0 router 10.0.1.1
router router dhcp 10.0.1.0 static 00:00:00:00:02:01 10.0.1.10
router router commit
vm start router
shell sleep 10
vm start all
# wait until every VM has a connected agent and an address (about 90 s)
shell timeout 300 sh -c "until minimega -e .annotate false .columns cc_active,ip namespace sandwich vm info 2>/dev/null | grep -c 'true.*10[.]' | grep -qx 3; do sleep 3; done"

# PCAP of vm-left's only interface, with a ping through the router as traffic
capture pcap vm vm-left 0 vm-left.pcap
.annotate false capture
cc filter name=vm-left
cc exec ping -c 5 10.0.1.10
shell sleep 10
capture pcap delete vm vm-left

# PCAP of the whole bridge: both sides of the router this time
capture pcap bridge mega_bridge sandwich.pcap
cc exec ping -c 5 10.0.1.10
shell sleep 10
capture pcap delete bridge mega_bridge

# netflow as text; wait past the 10 s active-flow timeout before stopping
capture netflow mode ascii
capture netflow bridge mega_bridge sandwich.nf
cc exec ping -c 5 10.0.1.10
shell sleep 15
capture netflow delete bridge mega_bridge

# netflow as gzipped raw records, converted to text with nfcat
capture netflow mode raw
capture netflow gzip true
capture netflow bridge mega_bridge sandwich.nf.gz
cc exec ping -c 5 10.0.1.10
shell sleep 15
capture netflow delete bridge mega_bridge
shell /opt/minimega/bin/nfcat -gunzip /tmp/minimega/files/sandwich.nf.gz

# a client-image monitor on net_left; let it boot before its port is a mirror
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config networks net_left
vm launch kvm monitor
vm start monitor
# wait until the monitor has an agent and a lease: four VMs now
shell timeout 300 sh -c "until minimega -e .annotate false .columns cc_active,ip namespace sandwich vm info 2>/dev/null | grep -c 'true.*10[.]' | grep -qx 4; do sleep 3; done"
tap mirror vm-left 0 monitor 0
.annotate false .columns name,tap vm info

# tcpdump on the monitor sees vm-left's traffic; drive it over cc
cc filter name=monitor
cc prefix monitor
cc background tcpdump -U -n -i eth0 -w /tmp/mirror.pcap
cc filter name=vm-right
cc exec ping -c 3 10.0.0.10
shell sleep 10
cc filter name=monitor
cc process killall tcpdump
cc exec tcpdump -n -r /tmp/mirror.pcap
shell sleep 5
cc responses monitor
clear cc prefix
clear cc filter

# remove the mirror (keyed by its destination) and stop any capture left
clear tap mirror monitor 0
clear capture
