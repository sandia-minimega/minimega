# send a script that creates /foo/bar.out, run it, then fetch the file back
cc send bar.bash
cc exec bash /tmp/miniccc/files/bar.bash
cc recv /foo/bar.out

# check on the received file (command 3 here), with and without headers
cc responses 3
cc responses 3 raw

# show every response received so far
cc responses all
