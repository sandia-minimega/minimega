shell echo "start the router first, and give it a chance to configure"
vm start router
shell sleep 10

# "all" is a special keyword
vm start all
shell echo "give VMs 20 seconds to obtain IP..."
shell sleep 20

# make sure everything booted and connected
shell echo "vm info:"
.column id,name,state,vlan,ip,qos vm info
