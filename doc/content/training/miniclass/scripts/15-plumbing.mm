# 15-plumbing.mm: pipes, a pipeline and a via between the two clients.
# Needs miniccc.kernel/.initrd and minirouter.kernel/.initrd from chapter 2
# in /tmp/minimega/files. The via expects /opt/minimega/bin/normal.
clear namespace sandwich
namespace sandwich

# the sandwich from chapter 3 with fixed client addresses (chapter 11)
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

# a write with no readers is discarded, but the pipe now exists
pipe hello "the sandwich says hello"
.annotate false pipe

# vm_left pings vm_right and writes the output to the pipe "pings";
# vm_right reads the pipe into a file from a guest shell with miniccc -pipe
# (cc background stdin=pings tee /tmp/pings.log would do the same without one)
cc filter name=vm_left
cc background stdout=pings ping -i 1 10.0.1.10
cc filter name=vm_right
cc background sh -c "miniccc -pipe pings > /tmp/pings.log"
shell sleep 10
cc exec tail -n 3 /tmp/pings.log
shell sleep 5
cc responses 6
clear cc filter
.annotate false .columns name,mode,readers,writers,count pipe

# a pipeline on the host: prefix every line and write it to "tagged"
plumb pings "sed -u s/^/left:/" tagged
shell sleep 5
.annotate false plumb
.annotate false .columns name,mode,readers,writers,count pipe

# delivery modes: one reader per message instead of every reader
pipe tagged mode round-robin
.annotate false .columns name,mode pipe
clear pipe tagged mode

# a via: every reader of "rtt" gets its own sample around the written mean
pipe rtt via /opt/minimega/bin/normal -stddev 5.0
pipe rtt 100
.annotate false .columns name,via,count pipe
clear pipe rtt via

# clearing the first pipe sends EOF down the pipeline; stop the ping too
clear pipe pings
cc process killall ping
shell sleep 2
.annotate false plumb
clear plumb
clear pipe
