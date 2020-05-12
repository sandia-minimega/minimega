# send and receive files - including wildcards and directories
cc send foo
cc send my_whole_directory/*

# 'exec' waits for a process to complete before moving on to the next command
cc exec touch /root/tumbleweed

# 'background' forks the process and immediately moves on to the next command
cc background sleep 300
cc background touch /root/impatience
shell sleep 5

# A backgrounded process can be tracked and killed
cc process list foo
cc process killall sleep

# `prefix` allows for grouping like commands
cc prefix foo
cc exec hostname
shell sleep 5
cc response foo

# filter groups allows for sending commands to one or more hosts based on any field
cc prefix foo
cc filter name=foo
cc exec hostname
shell sleep 5
cc response foo
