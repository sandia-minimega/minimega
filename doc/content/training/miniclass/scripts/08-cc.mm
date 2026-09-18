# Chapter 8: command and control
# Rebuilds the sandwich, then drives the miniccc agents in its VMs.
# The send step writes hello.sh into the default files directory,
# /tmp/minimega/files; adjust the path if minimega runs with another -filepath.
clear namespace sandwich
namespace sandwich

vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config networks net_left
vm launch kvm vm_left
vm config networks net_right
vm launch kvm vm_right
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config networks net_left net_right
vm launch kvm router
router router interface 0 10.0.0.1/24
router router dhcp 10.0.0.0 range 10.0.0.2 10.0.0.254
router router dhcp 10.0.0.0 router 10.0.0.1
router router interface 1 10.0.1.1/24
router router dhcp 10.0.1.0 range 10.0.1.2 10.0.1.254
router router dhcp 10.0.1.0 router 10.0.1.1
router router commit
vm start router
shell sleep 10
vm start all
shell sleep 20

# every VM should have a connected agent
.annotate false .columns name,cc_active vm info
cc
.annotate false .columns arch,os cc clients

# the commit above queued three cc commands (ids 1 to 3); this one is id 4
cc exec cat /etc/motd
shell sleep 5
cc responses 4

# ids 5 and 6: only vm_left from here on
cc filter name=vm_left
cc exec ping -c 3 10.0.1.1
cc test-conn tcp 10.0.1.1 22 wait 10s
shell sleep 10
cc responses 5
cc exitcode 5 vm_left
cc responses 6

# id 7 sends a script; ids 8 and 9 run it and fetch a file back, under a prefix
shell bash -c "echo 'echo hello from cc' > /tmp/minimega/files/hello.sh"
shell bash -c "echo 'uname -n' >> /tmp/minimega/files/hello.sh"
cc send hello.sh
cc prefix hello
cc exec sh /tmp/miniccc/files/hello.sh
cc recv /etc/motd
clear cc prefix
shell sleep 5
cc responses hello

# id 10: a long-running program belongs in the background
cc background sleep 600
shell sleep 5
.annotate false .columns name,command cc process list vm_left
cc process killall sleep

# what is queued, then clean up
.annotate false .columns id,command,responses,filter cc commands
clear cc filter
cc delete command all
clear cc responses
