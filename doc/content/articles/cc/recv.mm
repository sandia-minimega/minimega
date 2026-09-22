# send a script that creates /var/log/inventory.out, run it, then fetch it back
cc send collect.bash
cc exec bash /tmp/miniccc/files/collect.bash
cc recv /var/log/inventory.out

# check on the received file (command 3 here), with and without headers
cc responses 3
cc responses 3 raw

# show every response received so far
cc responses all
