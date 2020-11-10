# upload the protonuke file to all VMs matching the current cc filter.
# protonuke is located in /tmp/minimega/files/
cc send protonuke

# Send all files (globs) with the * operator
cc send /somedirectory/*

# use commands to check recent file operations, just as for commands
cc commands
