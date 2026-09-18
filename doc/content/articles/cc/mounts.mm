# mount the filesystem of VM client0 on the host, from anywhere in the cluster
shell mkdir /tmp/mount
cc mount client0 /tmp/mount

# list mounts, then unmount
cc mount
clear cc mount client0
