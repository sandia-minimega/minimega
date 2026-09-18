# group the next commands under the prefix "foo"
cc prefix foo
cc exec ls /
cc exec echo "foo"

# clear the prefix so later commands are not grouped
clear cc prefix
cc commands

# read the responses for every command with the prefix
cc responses foo
