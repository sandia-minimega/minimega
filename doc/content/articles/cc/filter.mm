# only clients with an IP in 10.0.0.0/24 execute later commands
cc filter ip=10.0.0.0/24

# show the current filter
cc filter

# filters can also use vm info columns and tags
cc filter name=server os=linux

# remove the filter
clear cc filter
