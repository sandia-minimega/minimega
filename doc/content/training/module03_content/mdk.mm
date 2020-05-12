vm stop foo
vm stop bar[1-3,5]
vm stop all
# 'vm stop' puts the specified VM(s) in the PAUSED state
vm info

vm kill foo
vm kill bar[1-5]
vm kill all
# 'vm kill' puts the specified VM(s) in the QUIT state
vm info

# 'vm flush' will clear all VMs in the QUIT or ERROR state
vm flush
