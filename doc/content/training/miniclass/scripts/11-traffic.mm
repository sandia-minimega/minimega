# 11-traffic.mm: background traffic with protonuke on the router sandwich.
# Needs miniccc.kernel/.initrd and minirouter.kernel/.initrd from chapter 2
# in /tmp/minimega/files, and protonuke installed on the host.
clear namespace sandwich
namespace sandwich

# the sandwich from chapter 3, with fixed MACs and static leases so that
# vm_left is always 10.0.0.10 and vm_right is always 10.0.1.10
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config networks net_left,00:00:00:00:01:01
vm launch kvm vm_left
vm config networks net_right,00:00:00:00:02:01
vm launch kvm vm_right
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
shell sleep 30
.annotate false .columns name,state,ip vm info

# put protonuke where cc send can find it, then send it to every client;
# it lands in /tmp/miniccc/files on the guests and runs by name
shell cp /usr/bin/protonuke /tmp/minimega/files/protonuke
cc send protonuke

# serve HTTP, HTTPS and SMTP on vm_left (sshd already owns port 22)
cc filter name=vm_left
cc background protonuke -serve -http -https -smtp

# fetch one page from vm_right to prove the path through the router works
cc filter name=vm_right
cc exec curl -s http://10.0.0.10/
shell sleep 10
cc responses 6

# then generate steady load from vm_right, one action every 500 ms per protocol
cc background protonuke -http -https -smtp -u 500ms 10.0.0.10
clear cc filter
shell sleep 15

# watch it from the host
.annotate false .columns name,pid,command cc process list all
.annotate false .columns id,command,responses,background cc commands
.annotate false .columns name,cpu,rx,tx vm top 5

# stop both ends
cc process killall protonuke
