# intead of blocking on this command, run in the background:
cc background protonuke -serve -dns 100.0.2.2
# show all running processes
cc process list all
# you can kill a process by PID
cc process kill <pid>
# or all at once, by name
cc process killall protonuke 
