# start a background process
cc background sshd

# list processes started in the background on every client
cc process list all

# kill by PID, or every process whose command line contains "sshd"
# cc process kill <pid>
cc process killall sshd
