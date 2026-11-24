# group the next commands under the prefix "inventory"
cc prefix inventory
cc exec ls /
cc exec uname -a

# clear the prefix so later commands are not grouped
clear cc prefix
cc commands

# read the responses for every command with the prefix
cc responses inventory
