shell echo "Now let's check our traffic levels:"
vm top
# Notice that we have values in rx/tx now
# Notice, also the difference in values for the roles each are playing

# Now let's turn on qos to manipulate the network traffic
qos add vm_left 0 loss 0.5
# this will force the interface for vm_right to drop hald the packets it receives to simulate packet loss

# let's see how this changes what we see over the network:
shell sleep 20
vm top

# clean up 
#clear namespace sandwich
