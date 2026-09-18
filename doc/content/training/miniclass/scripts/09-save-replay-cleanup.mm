# Chapter 9: save, replay, and clean up
# Rebuilds the sandwich from saved templates, records the session as a
# script, checkpoints one VM, then tears everything down.
# write and file list use the default files directory, /tmp/minimega/files.
clear history
clear namespace sandwich
namespace sandwich

# two templates: a client and a router
clear vm config
vm config kernel miniccc.kernel
vm config initrd miniccc.initrd
vm config memory 2048
vm config save client
clear vm config
vm config kernel minirouter.kernel
vm config initrd minirouter.initrd
vm config save minirouter
vm config restore

# launch from the templates
vm config restore client
vm config networks net_left
vm launch kvm vm_left
vm config networks net_right
vm launch kvm vm_right
vm config restore minirouter
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
.annotate false .columns name,state vm info

# everything since clear history, as a script you can read back later
write /tmp/minimega/files/sandwich.mm
shell head -8 /tmp/minimega/files/sandwich.mm

# checkpoint vm_left: only a state file, since it boots from a kernel and initrd
vm save vm_left
shell sleep 30
.annotate false .columns name,status vm save
.annotate false .columns name,state vm info
file list saved

# tear down: kill, flush, and destroy the namespace
vm kill all
vm flush
.annotate false .columns name,state vm info
tap
clear namespace sandwich
