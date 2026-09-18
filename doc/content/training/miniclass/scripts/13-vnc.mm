# 13-vnc.mm: record, play back and script the console of vm_right.
# Needs miniccc.kernel/.initrd and minirouter.kernel/.initrd from chapter 2
# in /tmp/minimega/files.
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

# record keyboard and mouse events sent to vm_right, then send some:
# vnc type handles Shift, vnc inject sends one raw event without a delay
vnc record kb vm_right session.kb
vnc type vm_right "ping -c 3 10.0.0.10"
shell sleep 1
vnc inject vm_right KeyEvent,true,Return
vnc inject vm_right KeyEvent,false,Return
.annotate false vnc
shell sleep 5
vnc stop kb vm_right
shell cat /tmp/minimega/files/session.kb

# record the screen while the session is played back into the same VM
vnc record fb vm_right session.fb
vnc play vm_right session.kb
.annotate false vnc
shell sleep 10
vnc stop fb vm_right

# a screenshot of the console as it looks now
vm screenshot vm_right file /tmp/minimega/files/vm_right.png

# nothing should be left recording or playing
.annotate false vnc
clear vnc
