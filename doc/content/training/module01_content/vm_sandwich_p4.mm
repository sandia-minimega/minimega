# collect some control data
shell echo "vm top:"
vm top
# notice there is no data traveling across the network (rx/tx == 0.00)
# Let's fix that!

# Let's use miniccc to push out a tool to our VMs
cc send file:protonuke

# We will have vm_left server traffic
cc filter name=vm_left
cc background /tmp/miniccc/files/protonuke -serve -http
cc background /tmp/miniccc/files/protonuke -serve -https

# We will have vm_right request http/https
clear cc filter
cc filter name=vm_right
cc background /tmp/miniccc/files/protonuke -level info -logfile proto.log -http -https 10.0.0.2

