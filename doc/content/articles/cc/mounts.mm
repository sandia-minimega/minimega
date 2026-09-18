# mount the filesystem of VM bar on the host, from anywhere in the cluster
shell mkdir /tmp/mount
cc mount bar /tmp/mount

# list mounts, then unmount
cc mount
clear cc mount bar
