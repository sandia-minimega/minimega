# send foo.bash (from minimega's -filepath directory) to all clients
cc send foo.bash

# execute it from the client's files directory
cc exec bash /tmp/miniccc/files/foo.bash

# check on the commands, then read the output of the exec
cc commands
cc responses 2
