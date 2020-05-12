# create a prefix
cc prefix foo

# and issue some commands
cc exec ls /
cc exec echo "foo"

# we'll clear the prefix moving forward
clear cc prefix

cc commands
